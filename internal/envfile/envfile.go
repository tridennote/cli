// Package envfile parses .env text and renders variables as .env, YAML, plain text or JSON,
// matching the web app's shared/env-file.ts so both clients produce identical output.
package envfile

import (
	"bytes"
	"encoding/json"
	"path"
	"regexp"
	"strings"
)

type Var struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Format string

const (
	FormatEnv  Format = "env"
	FormatYAML Format = "yaml"
	FormatTXT  Format = "txt"
	FormatJSON Format = "json"
)

var Formats = []Format{FormatEnv, FormatYAML, FormatTXT, FormatJSON}

var formatLabels = map[Format]string{FormatEnv: ".env", FormatYAML: "YAML", FormatTXT: "plain text", FormatJSON: "JSON"}
var formatExtensions = map[Format]string{FormatEnv: "env", FormatYAML: "yml", FormatTXT: "txt", FormatJSON: "json"}

func (f Format) Label() string { return formatLabels[f] }

// ParseFormat accepts the names people type: env, dotenv, yaml, yml, txt, text, json.
func ParseFormat(s string) (Format, bool) {
	switch strings.ToLower(strings.TrimPrefix(s, ".")) {
	case "env", "dotenv":
		return FormatEnv, true
	case "yaml", "yml":
		return FormatYAML, true
	case "txt", "text", "plain":
		return FormatTXT, true
	case "json":
		return FormatJSON, true
	}
	return "", false
}

var linePattern = regexp.MustCompile(`^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_.-]*)\s*[=:]\s?(.*)$`)
var inlineComment = regexp.MustCompile(`\s#`)

func closingQuote(text string, quote byte) int {
	for i := 0; i < len(text); i++ {
		if text[i] == '\\' && quote == '"' {
			i++
			continue
		}
		if text[i] == quote {
			return i
		}
	}
	return -1
}

func unescapeDouble(raw string) string {
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			switch raw[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 'r':
				b.WriteByte('\r')
				i++
				continue
			case 't':
				b.WriteByte('\t')
				i++
				continue
			case '"', '\\', '$':
				b.WriteByte(raw[i+1])
				i++
				continue
			}
		}
		b.WriteByte(raw[i])
	}
	return b.String()
}

// Parse reads .env text. A key that appears twice keeps its last value, in its last position.
func Parse(text string) []Var {
	text = strings.TrimPrefix(text, "\uFEFF")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var order []string
	values := map[string]string{}
	set := func(key, value string) {
		if _, seen := values[key]; seen {
			for i, k := range order {
				if k == key {
					order = append(order[:i], order[i+1:]...)
					break
				}
			}
		}
		order = append(order, key)
		values[key] = value
	}
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		match := linePattern.FindStringSubmatch(lines[i])
		if match == nil {
			continue
		}
		key, rest := match[1], strings.TrimLeft(match[2], " \t")
		if rest != "" && (rest[0] == '"' || rest[0] == '\'' || rest[0] == '`') {
			quote := rest[0]
			body := rest[1:]
			end := closingQuote(body, quote)
			for end == -1 && i+1 < len(lines) {
				i++
				body += "\n" + lines[i]
				end = closingQuote(body, quote)
			}
			if end != -1 {
				body = body[:end]
			}
			if quote == '"' {
				body = unescapeDouble(body)
			}
			set(key, body)
			continue
		}
		if loc := inlineComment.FindStringIndex(rest); loc != nil {
			rest = rest[:loc[0]]
		}
		set(key, strings.TrimSpace(rest))
	}
	out := make([]Var, 0, len(order))
	for _, k := range order {
		out = append(out, Var{Key: k, Value: values[k]})
	}
	return out
}

var needsQuotes = regexp.MustCompile("[\\s#\"'`\\\\$=]")

func dotenvValue(v string) string {
	if v == "" || (!needsQuotes.MatchString(v) && v == strings.TrimSpace(v)) {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(v) + `"`
}

func jsonString(v string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSuffix(buf.String(), "\n")
}

// Render writes vars in the given format, always ending with a newline when non-empty.
func Render(vars []Var, f Format) string {
	if f == FormatJSON {
		if len(vars) == 0 {
			return "{}\n"
		}
		var b strings.Builder
		b.WriteString("{\n")
		for i, v := range vars {
			b.WriteString("  " + jsonString(v.Key) + ": " + jsonString(v.Value))
			if i < len(vars)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString("}\n")
		return b.String()
	}
	var b strings.Builder
	for _, v := range vars {
		switch f {
		case FormatYAML:
			b.WriteString(v.Key + ": " + jsonString(v.Value))
		case FormatTXT:
			b.WriteString(v.Key + "=" + v.Value)
		default:
			b.WriteString(v.Key + "=" + dotenvValue(v.Value))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

var stemSuffix = regexp.MustCompile(`(?i)\.(env|ya?ml|txt|json)$`)

// DownloadName picks a file name for an env file exported in format f.
func DownloadName(name string, f Format) string {
	leaf := path.Base(strings.ReplaceAll(name, `\`, "/"))
	stem := stemSuffix.ReplaceAllString(strings.TrimLeft(leaf, "."), "")
	if stem == "" {
		stem = "env"
	}
	if f == FormatEnv {
		if strings.HasPrefix(leaf, ".env") {
			return leaf
		}
		return stem + ".env"
	}
	return stem + "." + formatExtensions[f]
}

var envLeaf = regexp.MustCompile(`(?i)^\.env(\..+)?$|\.env$`)

// LooksLikeEnvFile reports whether a path's file name looks like a dotenv file.
func LooksLikeEnvFile(name string) bool {
	return envLeaf.MatchString(path.Base(strings.ReplaceAll(name, `\`, "/")))
}
