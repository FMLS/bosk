# bosk

bosk /bɒsk/ — a small wood. One named workspace is a cluster of git worktrees, one per repository, on the same branch.

```bash
bosk init ~/dev
bosk create -b feat/login -r svc-a,svc-b
bosk status feat-login
bosk delete feat-login
```

## Why

A feature that spans multiple repositories still means: create a branch in each repo, jump between directories, and remember to clean every worktree later. bosk does that as one operation.

Git stays authoritative. A local registry records what bosk created, and a failed create rolls back worktrees from that attempt. Plugins, hooks, TUI, MCP, presets, and PR integrations stay out of core.

## Install

Requires `git` on `PATH`.

```bash
go install github.com/FMLS/bosk/cmd/bosk@latest
```

From a checkout:

```bash
go build -o bosk ./cmd/bosk
```

Optional shell integration, so `bosk go` changes the current directory:

```bash
eval "$(bosk shell-init)"   # add to ~/.zshrc or ~/.bashrc
```

## Commands

| Command | Purpose |
|---|---|
| `bosk init <dir...>` | Register directories to scan for git repositories |
| `bosk repos` | List discovered source repositories |
| `bosk create [name] -b <branch> -r a,b` | Create worktrees and record the workspace |
| `bosk create -b <branch> --all` | Include every discovered repository |
| `bosk list` | List workspaces |
| `bosk status [name]` | Branch, dirty, ahead/behind per repo |
| `bosk go [name]` | Print the workspace path (`cd` with shell-init) |
| `bosk delete <name>` | Remove worktrees; keeps local branches by default |
| `bosk doctor` | Compare registry, filesystem, and git |
| `bosk shell-init` | Print the `cd` wrapper |

`list`, `status`, `repos`, and `doctor` accept `--json`.

## Layout

```
~/dev/svc-a/                      # source clone (unchanged)
~/dev/svc-b/

~/.bosk/config.yaml               # scan roots + workspace dir
~/.bosk/state.json                # what bosk created
~/.bosk/workspaces/feat-login/
  svc-a/                          # linked worktree
  svc-b/
```

Set `BOSK_HOME` to override `~/.bosk` (used in tests).

## Safety

- Dirty or drifted worktrees refuse `delete` unless `--force`.
- Non-interactive `delete` requires `--yes`.
- Local branches created by bosk are kept unless `--delete-branch`.
- Remote branches are never deleted.
- Create fetches remotes, then adds worktrees one repo at a time. A later failure removes worktrees (and branches) created by that attempt.
- Delete quarantines the workspace directory, prunes git worktree registrations, then drops the registry entry.

## Out of scope

Plugins, lifecycle hooks, TUI, editor/PR integrations, process runners, and sync/rebase across repos. Those can stay out of core.

## Name

`copse` and `holt` are already used by other worktree tools. bosk was still free.
