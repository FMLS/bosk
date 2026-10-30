# bosk

bosk /bɒsk/ — a small wood. One named workspace is a cluster of git worktrees, one per repository, on the same branch.

```bash
bosk init ~/dev
bosk create -b feat/login -r svc-a,svc-b
bosk add feat-login -r svc-c
bosk remove feat-login -r svc-c
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
| `bosk create [name] -b <branch> -r a --shared x,y` | Also link repositories every workspace shares (symlinks, no worktree or branch) |
| `bosk add [name] -r a,b` | Add repositories to an existing workspace on its branch (`--all` adds the rest) |
| `bosk add [name] --shared x` | Link a shared repository into an existing workspace |
| `bosk remove [name] -r a,b` | Remove repositories, worktree or shared, from a workspace (not the last worktree one; use `delete`) |
| `bosk list` | List workspaces |
| `bosk status [name]` | Branch, dirty, ahead/behind per repo; link health for shared repos |
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
  feat-login.code-workspace       # Cursor / VS Code workspace file
  svc-a/                          # linked worktree
  svc-b/
  shared-lib -> ~/dev/shared-lib  # shared repository: a symlink, not a worktree
```

Set `BOSK_HOME` to override `~/.bosk` (used in tests).

## Safety

- Dirty or drifted worktrees refuse `delete` and `remove` unless `--force`.
- Non-interactive `delete` and `remove` require `--yes`.
- Local branches created by bosk are kept unless `--delete-branch`.
- Remote branches are never deleted.
- Create fetches remotes, then adds worktrees one repo at a time. A later failure removes worktrees (and branches) created by that attempt.
- Add does the same for an existing workspace; a failure removes only what that attempt added.
- Delete quarantines the workspace directory, prunes git worktree registrations, then drops the registry entry.
- Remove does the same per repository: it quarantines only the named worktrees, prunes them, and drops them from the registry. A failure puts them back.

- A shared repository is only a symlink to its source clone. `delete` and `remove` drop the link and never touch the clone; `doctor` reports a missing source and `doctor --fix` rebuilds a missing or mis-pointed link. A path that is no longer a symlink is left alone. Creating symlinks on Windows needs developer mode or elevated rights.

## Out of scope

Plugins, lifecycle hooks, TUI, editor/PR integrations, process runners, and sync/rebase across repos. Those can stay out of core.

## Name

`copse` and `holt` are already used by other worktree tools. bosk was still free.
