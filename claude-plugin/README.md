# Tridennote for Claude Code

Paste a note's terminal command into Claude Code and Claude reads that note for you.

```
tridennote -c my-workspace specs/login-flow implement this
```

## Install

1. Install the [tridennote CLI](https://tridennote.pixcel.org/#install) and run `tridennote` once to sign in.
2. In Claude Code, run:

   ```
   /plugin install tridennote --marketplace tridennote/cli
   ```

   On Claude Code older than 2.1.275, add the marketplace first:

   ```
   /plugin marketplace add tridennote/cli
   /plugin install tridennote@tridennote
   ```

## What it does

- **Hook (`UserPromptSubmit`)**: when your message contains `tridennote -c <workspace> <note>`, it runs `tridennote -c <workspace> <note> --print` before Claude sees the message and attaches the note. It reads up to three notes per message, at most 60 KB each. It only ever reads: arguments that start with `-` or contain shell characters are skipped, so a pasted `--write` never runs.
- **Skill (`tridennote`)**: teaches Claude to find notes with `--list`, read them with `--print`, and update one with `--write` only when you ask and after you confirm the change.

Get a note's command from the web app: open the note's menu and choose **Copy terminal command**.

The hook needs a POSIX shell (`sh`), which Claude Code already uses on macOS, Linux and Windows (Git Bash).
