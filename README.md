# tridennote (terminal)

Tridennote in your terminal: notes, next level. Browse your vault in a Neovim-style tree, read notes as rendered markdown (Mermaid diagrams included), run kanban boards, and edit them in the built-in editor or your own. Built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Install

**Homebrew** (macOS / Linux)

```bash
brew install tridennote/tap/tridennote
```

**Install script** (macOS / Linux): downloads the latest release, verifies its checksum and
puts `tridennote` in `/usr/local/bin` (or `~/.local/bin` if that isn't writable)

```bash
curl -fsSL https://raw.githubusercontent.com/tridennote/cli/main/install.sh | bash
```

Set `TRIDENNOTE_INSTALL_DIR` to choose the folder, or `TRIDENNOTE_VERSION=v1.1.0` to pin a
version.

**Go**

```bash
go install github.com/tridennote/cli/cmd/tridennote@latest
```

**PowerShell** (Windows): downloads the latest release, verifies its checksum, installs to
`%LOCALAPPDATA%\Programs\tridennote` and adds it to your user PATH

```powershell
irm https://raw.githubusercontent.com/tridennote/cli/main/install.ps1 | iex
```

**Scoop** (Windows)

```powershell
scoop bucket add tridennote https://github.com/tridennote/scoop-bucket
scoop install tridennote
```

Both installers accept `TRIDENNOTE_INSTALL_DIR` (PowerShell only) and `TRIDENNOTE_VERSION`
environment variables. The zips are also on the
[releases page](https://github.com/tridennote/cli/releases). On Windows, set a
[Nerd Font](https://www.nerdfonts.com/) in Windows Terminal or switch icons to Minimal in
settings.

Then run `tridennote`.

## Features

- **Browser sign-in**: on first launch you're prompted to sign in. Press `enter` and the
  app opens `https://app.tridennote.pixcel.org` (always production, even for local builds) to
  confirm a short code. The resulting Neon Auth session is saved to
  `<user config dir>/tridennote/session.json` (mode `0600`) and reused until it expires or
  you log out.
- **Vault tree**: a Neovim-style file tree on the right lists your folders and markdown
  notes only (sketches, PDFs and other attachments are hidden).
- **Note preview**: the selected note is rendered as markdown on the left.
- **Editing**: edit notes in the built-in editor, or hand them to your own editor
  (VS Code, Neovim, …). New notes can be created from the tree.
- **Kanban boards**: notes named `*.kanban` show up in the tree as boards and preview as
  columns of cards. Open one to move, add, edit and check off cards; every change is saved
  straight back to the note (see [Kanban boards](#kanban-boards)).

- **Settings** (`,`): pick your editor, put the sidebar on the left or right, and choose
  Nerd Font or minimal icons. Saved to `<user config dir>/tridennote/settings.json`.

### Editors

Choose in settings: Built-in, VS Code, VS Code Insiders, Cursor, Zed, Neovim, Vim, Nano, or
`$EDITOR`. Editors that aren't installed are greyed out. On macOS the app bundles in
`/Applications` are detected even if their shell command isn't on your `PATH`. GUI editors
are launched with their wait flag (`--wait`), so the note is saved when you close its tab.
Terminal editors take over the screen until you quit them.

`e` uses your chosen editor, `i` always uses the built-in one. If the built-in editor is
selected, `ctrl+o` inside it falls back to `$TRIDENNOTE_EDITOR`, `$VISUAL`, `$EDITOR`, then
the first installed editor.

### Mermaid diagrams

Flowcharts, sequence diagrams and ER diagrams are drawn right in the note as Unicode box
art (via [mermaid-ascii](https://github.com/AlexanderGrooff/mermaid-ascii)). Other types
(pie, gantt, class, state, …) show their source with a hint.

Press `m` to render every diagram in the note as an image with
[mermaid-cli](https://github.com/mermaid-js/mermaid-cli) (`npm i -g @mermaid-js/mermaid-cli`).
It reuses an installed Chrome, Chromium, Brave, Edge or Arc, so Puppeteer doesn't need to
download its own. Images are shown in the terminal when [chafa](https://hpjansson.org/chafa/)
is installed (`brew install chafa`) or with `kitten icat` in kitty/Ghostty, and otherwise
open in your system image viewer.

### Kanban boards

A board is an ordinary note whose name ends in `.kanban` (the suffix is hidden in the tree),
so the web app and any other client still see it as plain markdown:

```markdown
## Backlog

- [ ] Set up CI pipeline !high #devops #ci @harith due:2026-10-10
  - Card body lines (anything indented under a card) stay with the card
- [ ] Write API docs

## In progress

## Done

- [x] Fix login bug !critical
```

Each `##` heading is a column and each top-level `- [ ]` / `-` item under it is a card.
Cards can carry inline metadata anywhere in their text: `!low` `!medium` `!high`
`!critical` for priority, `#tags`, `@assignee` and `due:YYYY-MM-DD` (overdue dates turn
red). Anything before the first `##` is kept as-is and shown in the markdown view.

Press `N` to create a new board (it starts with Backlog, Todo, In progress and Done), or
type a name ending in `.kanban` in the normal `n` prompt. Selecting a board shows it as
columns; `v` flips between the board and the raw markdown. Press `enter` on a board to work
on it:

| Key                       | Action                                         |
| ------------------------- | ---------------------------------------------- |
| `h` `l` / `←` `→`         | Previous / next column                         |
| `j` `k` / `↓` `↑`         | Next / previous card (`g` `G` first / last)    |
| `H` `L` / `shift+←` `→`   | Move the card to the previous / next column    |
| `J` `K` / `shift+↓` `↑`   | Move the card down / up within its column      |
| `x` / `space`             | Toggle done                                    |
| `a` / `n`                 | Add a card (metadata syntax works here)        |
| `e` / `enter`             | Edit the card's text                           |
| `d`                       | Delete the card (press `d` again to confirm)   |
| `c`                       | Add a column after the focused one             |
| `v`                       | Leave and show the markdown                    |
| `?`                       | Board shortcuts                                |
| `esc` / `q`               | Leave the board                                |

Moving a card into the last column checks it off, and moving it out of the last column
unchecks it. Toggling done never moves a card.

The Nerd Font icons need a [Nerd Font](https://www.nerdfonts.com/) in your terminal.

## Prerequisites

- [Go](https://golang.org/) 1.27.1 or higher

## Building and Running

```bash
make build   # builds bin/tridennote (version from git describe)
make run     # builds and runs
go test ./...
```

## Keys

Press `?` in the app for the full list.

| Key                               | Action                                    |
| --------------------------------- | ----------------------------------------- |
| `↑` `↓` / `k` `j`                 | Move through the tree (wraps around)      |
| `←` `→`                           | Switch pane (wraps around)                |
| `l` / `h`                         | Expand / collapse, or jump to parent      |
| `enter` / `space`                 | Toggle folder / open note / work on board |
| `e`                               | Edit in your chosen editor                |
| `i`                               | Edit in the built-in editor               |
| `n`                               | New note in the selected folder           |
| `N`                               | New kanban board in the selected folder   |
| `v`                               | Switch boards between board and markdown  |
| `y`                               | Copy the note as markdown                 |
| `m`                               | Open the note's diagrams as images        |
| `b`                               | Show / hide the sidebar                   |
| `tab`                             | Switch focus between tree and note        |
| `pgup` `pgdn` / `ctrl+u` `ctrl+d` | Scroll the note                           |
| `w`                               | Next workspace                            |
| `r`                               | Refresh                                   |
| `,`                               | Settings                                  |
| `?`                               | Shortcut help                             |
| `L`                               | Log out (forget the saved session)        |
| `q` / `ctrl+c`                    | Quit                                      |

Opening a note keeps focus in the tree, so you can keep arrowing through notes. `←` and `→`
move focus between the panes wherever the sidebar is, wrapping around at either edge; with
the note focused, `↑` `↓` scroll it.

Inside the built-in editor: `ctrl+s` saves, `ctrl+o` continues in an external editor, and
`esc` closes (pressing it twice discards unsaved changes).

## Deployment / Distribution

Since this is a TUI client, deployment typically means distributing the compiled binary to users.

You can build the binary for different platforms using the Makefile:

```bash
make build-linux
make build-mac
make build-windows
```

Then distribute the resulting binaries found in the `bin/` directory.

## Releasing

Releases are cut with git flow (`git flow release start 1.2.0` / `finish`, tags are
`v`-prefixed). Pushing a `v*` tag runs GoReleaser in GitHub Actions, which builds
`tridennote_<os>_<arch>` archives plus `checksums.txt` and publishes the GitHub release. The
[homebrew-tap](https://github.com/tridennote/homebrew-tap) formula picks up the new release
within 6 hours, or immediately with
`gh workflow run update-formula.yml -R tridennote/homebrew-tap`; the
[scoop-bucket](https://github.com/tridennote/scoop-bucket) manifest works the same way
(`gh workflow run update-manifest.yml -R tridennote/scoop-bucket`).
