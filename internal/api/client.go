package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const DefaultBaseURL = "https://app.tridennote.pixcel.org"

var ErrUnauthorized = errors.New("not signed in")

type Client struct {
	BaseURL    string
	Token      string
	CookieName string
	OnRotate   func(token string)
	http       *http.Client
}

func New(baseURL string) *Client {
	return &Client{BaseURL: baseURL, http: &http.Client{Timeout: 20 * time.Second}}
}

type apiErrorBody struct {
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) do(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if origin, err := url.Parse(c.BaseURL); err == nil {
		req.Header.Set("Origin", origin.Scheme+"://"+origin.Host)
	}
	if c.Token != "" && c.CookieName != "" {
		req.AddCookie(&http.Cookie{Name: c.CookieName, Value: c.Token})
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	c.captureRotation(resp)

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var e apiErrorBody
		if json.Unmarshal(raw, &e) == nil && e.Error != nil {
			return fmt.Errorf("%s", e.Error.Message)
		}
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) captureRotation(resp *http.Response) {
	if c.CookieName == "" {
		return
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name != c.CookieName {
			continue
		}
		if cookie.Value == "" || cookie.MaxAge < 0 {
			return
		}
		if cookie.Value != c.Token {
			c.Token = cookie.Value
			if c.OnRotate != nil {
				c.OnRotate(cookie.Value)
			}
		}
	}
}

type envelope[T any] struct {
	Data T `json:"data"`
}

func getData[T any](c *Client, method, path string, body any) (T, error) {
	var out envelope[T]
	err := c.do(method, path, body, &out)
	return out.Data, err
}

// ErrTooLarge means a download went past the caller's size limit.
var ErrTooLarge = errors.New("file is too large to show here")

// Attachment downloads an uploaded file's raw bytes, refusing anything over limit bytes.
func (c *Client) Attachment(id string, limit int64) ([]byte, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/api/attachments/"+id+"/download", nil)
	if err != nil {
		return nil, err
	}
	if c.Token != "" && c.CookieName != "" {
		req.AddCookie(&http.Cookie{Name: c.CookieName, Value: c.Token})
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	c.captureRotation(resp)

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}
	if resp.ContentLength > limit {
		return nil, ErrTooLarge
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, ErrTooLarge
	}
	return raw, nil
}
