package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/envfile"
)

const envUsage = `Usage:
  tridennote env list                           list env files in the workspace
  tridennote env get <name> [flags]             print an env file
      -f, --format env|yaml|txt|json            output format (default env)
      -k, --keys KEY1,KEY2                      only these keys
      -o, --out PATH                            write to a file (mode 0600) instead of stdout
  tridennote env push <file|folder>... [--merge]
                                                upload .env files; a folder uploads every .env* inside
                                                --merge keeps keys that aren't in the upload

Every command takes -w, --workspace NAME (default: your default workspace).`

func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func runEnv(client *api.Client, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(stdout, envUsage)
		return nil
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("env "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var workspace, format, keys, out string
	var merge bool
	fs.StringVar(&workspace, "workspace", "", "")
	fs.StringVar(&workspace, "w", "", "")
	if sub == "get" {
		fs.StringVar(&format, "format", "env", "")
		fs.StringVar(&format, "f", "env", "")
		fs.StringVar(&keys, "keys", "", "")
		fs.StringVar(&keys, "k", "", "")
		fs.StringVar(&out, "out", "", "")
		fs.StringVar(&out, "o", "", "")
	}
	if sub == "push" {
		fs.BoolVar(&merge, "merge", false, "")
	}
	fs.Usage = func() { fmt.Fprintln(stderr, envUsage) }
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if client.Token == "" {
		return errors.New("not signed in — run `tridennote` once to sign in")
	}
	ws, err := pickWorkspace(client, workspace)
	if err != nil {
		return err
	}

	switch sub {
	case "list", "ls":
		files, err := client.EnvFiles(ws.ID)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			fmt.Fprintln(stderr, "No env files in "+ws.Name+" yet. Try: tridennote env push .env")
			return nil
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tVARS\tUPDATED")
		for _, f := range files {
			fmt.Fprintf(tw, "%s\t%d\t%s\n", f.Name, f.VarCount, strings.SplitN(f.UpdatedAt, "T", 2)[0])
		}
		return tw.Flush()

	case "get", "show":
		if len(positional) != 1 {
			return errors.New("usage: tridennote env get <name> [-f yaml] [-k KEY1,KEY2] [-o path]")
		}
		f, ok := envfile.ParseFormat(format)
		if !ok {
			return fmt.Errorf("unknown format %q (use env, yaml, txt or json)", format)
		}
		summary, err := findEnvFile(client, ws.ID, positional[0])
		if err != nil {
			return err
		}
		file, err := client.EnvFile(ws.ID, summary.ID)
		if err != nil {
			return err
		}
		vars, err := filterKeys(file.Vars, keys)
		if err != nil {
			return err
		}
		body := envfile.Render(vars, f)
		if out == "" {
			_, err = io.WriteString(stdout, body)
			return err
		}
		if err := os.WriteFile(envfile.ExpandHome(out), []byte(body), 0o600); err != nil {
			return err
		}
		fmt.Fprintf(stderr, "Wrote %d variables to %s\n", len(vars), out)
		return nil

	case "push", "upload":
		if len(positional) == 0 {
			return errors.New("usage: tridennote env push <file|folder>... [--merge]")
		}
		var uploads []api.EnvUpload
		skippedTotal := 0
		for _, target := range positional {
			sources, skipped, err := envfile.Collect(target)
			if err != nil {
				return err
			}
			skippedTotal += skipped
			for _, s := range sources {
				uploads = append(uploads, api.EnvUpload{Name: s.Name, Content: s.Content})
			}
		}
		mode := "replace"
		if merge {
			mode = "merge"
		}
		saved := 0
		for start := 0; start < len(uploads); start += 50 {
			result, err := client.UploadEnvFiles(ws.ID, uploads[start:min(len(uploads), start+50)], mode)
			if err != nil {
				return err
			}
			saved += len(result.SavedIDs)
		}
		for _, u := range uploads {
			fmt.Fprintf(stderr, "  %s (%d vars)\n", u.Name, len(envfile.Parse(u.Content)))
		}
		fmt.Fprintf(stderr, "Saved %d env file(s) to %s", saved, ws.Name)
		if skippedTotal > 0 {
			fmt.Fprintf(stderr, ", skipped %d", skippedTotal)
		}
		fmt.Fprintln(stderr)
		return nil
	}
	return fmt.Errorf("unknown env command %q\n\n%s", sub, envUsage)
}

func pickWorkspace(client *api.Client, name string) (api.Workspace, error) {
	all, err := client.Workspaces()
	if err != nil {
		if errors.Is(err, api.ErrUnauthorized) {
			return api.Workspace{}, errors.New("your session expired — run `tridennote` to sign in again")
		}
		return api.Workspace{}, err
	}
	if len(all) == 0 {
		return api.Workspace{}, errors.New("no workspaces found")
	}
	if name == "" {
		for _, w := range all {
			if w.IsDefault {
				return w, nil
			}
		}
		return all[0], nil
	}
	var names []string
	for _, w := range all {
		if strings.EqualFold(w.Name, name) || w.ID == name {
			return w, nil
		}
		names = append(names, w.Name)
	}
	return api.Workspace{}, fmt.Errorf("no workspace named %q (have: %s)", name, strings.Join(names, ", "))
}

func findEnvFile(client *api.Client, workspaceID, name string) (api.EnvFileSummary, error) {
	files, err := client.EnvFiles(workspaceID)
	if err != nil {
		return api.EnvFileSummary{}, err
	}
	var suffix []api.EnvFileSummary
	for _, f := range files {
		if f.Name == name || f.ID == name {
			return f, nil
		}
		if strings.HasSuffix(f.Name, "/"+name) {
			suffix = append(suffix, f)
		}
	}
	if len(suffix) == 1 {
		return suffix[0], nil
	}
	if len(suffix) > 1 {
		var names []string
		for _, f := range suffix {
			names = append(names, f.Name)
		}
		return api.EnvFileSummary{}, fmt.Errorf("%q matches several files: %s", name, strings.Join(names, ", "))
	}
	return api.EnvFileSummary{}, fmt.Errorf("no env file named %q — see `tridennote env list`", name)
}

func filterKeys(vars []envfile.Var, keys string) ([]envfile.Var, error) {
	if strings.TrimSpace(keys) == "" {
		return vars, nil
	}
	have := map[string]envfile.Var{}
	for _, v := range vars {
		have[v.Key] = v
	}
	var out []envfile.Var
	var missing []string
	for _, k := range strings.Split(keys, ",") {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		v, ok := have[k]
		if !ok {
			missing = append(missing, k)
			continue
		}
		out = append(out, v)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("not in this file: %s", strings.Join(missing, ", "))
	}
	return out, nil
}
