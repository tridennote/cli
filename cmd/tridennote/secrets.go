package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/envfile"
)

const runUsage = `Usage:
  tridennote run [flags] -- <command> [args...]
  tridennote env run [flags] -- <command> [args...]

Runs <command> with an env file's variables added to its environment.
  -w, --workspace NAME   workspace (default $TRIDENNOTE_WORKSPACE, else your default); an ID with token auth
  -e, --env NAME         env file (default $TRIDENNOTE_ENV, else .env)
      --no-override      keep variables that are already set in the environment

Auth, first match wins:
  TRIDENNOTE_TOKEN       a service token (tdn_st_...); the workspace is optional
  GitHub Actions OIDC    with permissions: id-token: write; set -w or TRIDENNOTE_WORKSPACE to a workspace ID
  saved session          from signing in with ` + "`tridennote`"

const oidcAudience = "tridennote"

type authMode int

const (
	authSession authMode = iota
	authServiceToken
	authGitHubOIDC
)

func selectAuth(getenv func(string) string) authMode {
	if getenv("TRIDENNOTE_TOKEN") != "" {
		return authServiceToken
	}
	if getenv("ACTIONS_ID_TOKEN_REQUEST_URL") != "" && getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN") != "" {
		return authGitHubOIDC
	}
	return authSession
}

// authenticate picks an auth mode, sets it on client and returns the workspace ID to send
// (empty when a service token implies it) plus a label for messages.
func authenticate(client *api.Client, workspace string, getenv func(string) string) (authMode, string, string, error) {
	if workspace == "" {
		workspace = getenv("TRIDENNOTE_WORKSPACE")
	}
	mode := selectAuth(getenv)
	switch mode {
	case authServiceToken:
		client.BearerToken = getenv("TRIDENNOTE_TOKEN")
		if workspace == "" {
			return mode, "", "the token's workspace", nil
		}
		return mode, workspace, workspace, nil
	case authGitHubOIDC:
		if workspace == "" {
			return mode, "", "", errors.New("GitHub OIDC needs a workspace ID: pass -w <id> or set TRIDENNOTE_WORKSPACE (names can't be looked up with token auth)")
		}
		token, err := api.GitHubOIDCToken(getenv("ACTIONS_ID_TOKEN_REQUEST_URL"), getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN"), oidcAudience)
		if err != nil {
			return mode, "", "", err
		}
		client.BearerToken = token
		return mode, workspace, workspace, nil
	}
	if client.Token == "" {
		return mode, "", "", errors.New("not signed in — run `tridennote` once to sign in, or set TRIDENNOTE_TOKEN")
	}
	ws, err := pickWorkspace(client, workspace)
	if err != nil {
		return mode, "", "", err
	}
	return mode, ws.ID, ws.Name, nil
}

func secretsError(mode authMode, err error) error {
	if !errors.Is(err, api.ErrUnauthorized) {
		return err
	}
	switch mode {
	case authServiceToken:
		return errors.New("TRIDENNOTE_TOKEN was rejected (unauthorized)")
	case authGitHubOIDC:
		return errors.New("the GitHub OIDC token was rejected (unauthorized)")
	}
	return errors.New("your session expired — run `tridennote` to sign in again")
}

type runOptions struct {
	Workspace  string
	Env        string
	NoOverride bool
	Command    []string
}

func parseRunArgs(args []string, getenv func(string) string, stderr io.Writer) (runOptions, error) {
	var opts runOptions
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprintln(stderr, runUsage) }
	fs.StringVar(&opts.Workspace, "workspace", "", "")
	fs.StringVar(&opts.Workspace, "w", "", "")
	fs.StringVar(&opts.Env, "env", "", "")
	fs.StringVar(&opts.Env, "e", "", "")
	fs.BoolVar(&opts.NoOverride, "no-override", false, "")

	flagArgs, command, dashed := args, []string(nil), false
	for i, a := range args {
		if a == "--" {
			flagArgs, command, dashed = args[:i], args[i+1:], true
			break
		}
	}
	if err := fs.Parse(flagArgs); err != nil {
		return opts, err
	}
	if dashed {
		if fs.NArg() > 0 {
			return opts, fmt.Errorf("unexpected %q before --\n\n%s", fs.Arg(0), runUsage)
		}
		opts.Command = command
	} else {
		opts.Command = fs.Args()
	}
	if len(opts.Command) == 0 {
		return opts, errors.New("no command given\n\n" + runUsage)
	}
	if opts.Env == "" {
		opts.Env = getenv("TRIDENNOTE_ENV")
	}
	if opts.Env == "" {
		opts.Env = ".env"
	}
	return opts, nil
}

func mergeEnv(base []string, vars []envfile.Var, override bool) []string {
	index := map[string]int{}
	out := make([]string, 0, len(base)+len(vars))
	for _, kv := range base {
		key, _, _ := strings.Cut(kv, "=")
		if i, ok := index[key]; ok {
			out[i] = kv
			continue
		}
		index[key] = len(out)
		out = append(out, kv)
	}
	for _, v := range vars {
		kv := v.Key + "=" + v.Value
		if i, ok := index[v.Key]; ok {
			if override {
				out[i] = kv
			}
			continue
		}
		index[v.Key] = len(out)
		out = append(out, kv)
	}
	return out
}

type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

func runRun(client *api.Client, args []string, stderr io.Writer) error {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		fmt.Fprintln(os.Stdout, runUsage)
		return nil
	}
	opts, err := parseRunArgs(args, os.Getenv, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	path, err := exec.LookPath(opts.Command[0])
	if err != nil {
		return err
	}
	mode, wsID, _, err := authenticate(client, opts.Workspace, os.Getenv)
	if err != nil {
		return err
	}
	secrets, err := client.Secrets(wsID, opts.Env)
	if err != nil {
		return secretsError(mode, err)
	}

	cmd := exec.Command(path, opts.Command[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = mergeEnv(os.Environ(), secrets.Vars, !opts.NoOverride)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		for sig := range signals {
			_ = cmd.Process.Signal(sig)
		}
	}()
	err = cmd.Wait()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return exitCode(128 + int(status.Signal()))
		}
		return exitCode(exitErr.ExitCode())
	}
	return err
}

// parseAssignments reads KEY=VALUE pairs; a value of "-" is read from stdin (once).
func parseAssignments(args []string, stdin io.Reader) ([]envfile.Var, error) {
	var vars []envfile.Var
	usedStdin := false
	for _, a := range args {
		key, value, ok := strings.Cut(a, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("expected KEY=VALUE, got %q", key)
		}
		if value == "-" {
			if usedStdin {
				return nil, errors.New("only one KEY=- can read from stdin")
			}
			usedStdin = true
			raw, err := io.ReadAll(stdin)
			if err != nil {
				return nil, err
			}
			value = strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
		}
		vars = append(vars, envfile.Var{Key: key, Value: value})
	}
	return vars, nil
}
