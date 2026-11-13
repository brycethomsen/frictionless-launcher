# AGENTS.md

Notes for coding agents (and people) working in this repo. Read this before
adding, moving, or naming files. [docs/DESIGN.md](docs/DESIGN.md) explains
*why* things are built the way they are; this file explains *where things go*.

## Before you start

- **Check which branch the work belongs on.** `main` can lag behind
  unmerged feature branches. Run `git branch -a` and
  `git log --oneline main..<branch>` before basing new work on `main`.
- Branch before committing; don't commit to `main`.

## Layout

One Go module (`frictionless-launcher`), one binary. This follows the
standard Go layout for a single command with supporting packages
(go.dev/doc/modules/layout): `main.go` at the root, everything else under
`internal/`, one package per responsibility.

```
main.go                 mode dispatch only (tray / --manage / --countdown); no logic
internal/
  config/               Config types, legacy migration, Store (load/save), Watch, DefaultPath
  schedule/             pure time-window math; no state, no I/O
  launcher/             tray-process scheduler: App state, minute monitor, countdown, launching
  tray/                 systray icon + menu; implements launcher.Tray
  ui/                   ALL Fyne code: schedule board, dialogs, countdown window, theme
  logging/              log file setup and rotation
  platform/             per-OS code (CGo on macOS, build-tagged stubs elsewhere)
  discovery/            Steam / Epic install discovery
  assets/               embedded files (icon.png)
packaging/macos/        Info.plist + build-app.sh for the .app bundle
docs/                   DESIGN.md (architecture, decisions), TODO.md (roadmap, bugs)
.github/workflows/      ci.yml (test/vet/fmt on 3 OSes), release.yml (native builds + packaging)
.goreleaser.yaml        build matrix used by release.yml
README.md  AGENTS.md     (no CLAUDE.md: Claude Code loads AGENTS.md when
                        a project has none, so don't add one)
```

The root holds only `main.go`, module/build config, and these top-level
docs. **Don't add files to the root**; put code in the package it belongs
to, or create a new `internal/<name>/` package for a genuinely new
responsibility (and add it to the lists here and in docs/DESIGN.md).

### Dependency rules

```
config  ←  schedule  ←  launcher  ←  tray
                 ↖ ui (config, schedule, discovery, platform, assets)
```

- **The tray process must never depend on Fyne.** It is resident all day.
  Fyne's driver creates an OpenGL context and a 60 Hz event loop at startup,
  which is the background cost the tray/UI process split removed.
  `config`, `schedule`, `launcher`, `tray`, `logging`, `platform` and
  `assets` must not import `fyne.io/fyne/v2/...`;
  `internal/tray/deps_test.go` fails if they do. `fyne.io/systray` is fine
  (it's a separate module).
- `ui` must not import `launcher` or `tray`. The UI process talks to the
  tray process only through `config.yaml` (via `config.Store`) and the
  countdown's exit code.
- `launcher` reaches the tray only through the `launcher.Tray` interface.
- In the tray process, read config through `Store.Current()` /
  `App.Config()` and treat it as read-only; `Store.Load` publishes a new
  struct rather than mutating. `App.mu` guards the launch history and the
  pending countdown.

## Conventions

- **Naming:** name files after what they contain, never after a task or
  refactor (`split.go`, `fixes.go`, `new_tray.go`).
- **Per-OS code** goes in `internal/platform/` as `<topic>_<goos>.go` /
  `<topic>_other.go` with build tags, behind an exported function.
- **Tests** sit next to their source, named `<file>_test.go`, in the same
  package. Test pure logic (`schedule`, `config`) directly. Fyne UI tests
  use `fynetest.NewTempApp` (headless). For code that calls the OS or spawns
  processes, use a package-level `var x = realFunc` seam and swap it in the
  test (see `countdownPopup` and `frontmostApp` in internal/launcher).
- Keep comments explaining *why*, matching the existing style.

## Build artifacts and scratch files

- Never leave build output, coverage files, or scratch files in the repo
  tree. Build to a temp directory, or use the Makefile, whose outputs
  (`frictionless-launcher*`, `dist/`) are gitignored and removed by
  `make clean`.
- Don't commit `.DS_Store`, binaries, or `coverage.*` (all gitignored).

## Checks (match CI)

```sh
gofmt -s -l .            # must print nothing
go vet ./...
go test -race ./...
# Windows cross-compile from macOS (needs: brew install mingw-w64)
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build -o /tmp/fl.exe .
```

Fyne needs cgo on every OS. On Linux, CI installs
`gcc libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev`.
