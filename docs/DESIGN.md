# Frictionless Launcher — Design Document

## Overview

Frictionless Launcher automatically launches games on a schedule so your game is ready when you sit down. No decision fatigue, no startup friction.

---

## Architecture

Single Go binary. System tray app with no dock icon on macOS. The binary
runs in one of three modes, chosen by its first argument:

| Mode            | Process lifetime        | What it runs |
|-----------------|-------------------------|--------------|
| *(no flag)*     | resident, from login    | tray icon + menu, schedule monitor, config watcher, launching |
| `--manage`      | while the window is open | the schedule board (Manage Games) |
| `--countdown N GAME` | ~N seconds          | the launch countdown window; exit code 3 = user cancelled |

The resident tray process never initialises Fyne. It drives the OS tray
directly through `fyne.io/systray` and blocks on timers between minute
ticks, so it has no OpenGL context and no render loop. Fyne only runs in the
short-lived child processes, which the tray starts with `os.Executable()`.
The processes share state only through `config.yaml` (the `--manage` window
saves it; the tray's fsnotify watcher reloads it) and the countdown's exit
code.

```
main.go               mode dispatch only
internal/
  config/      Config types; Store (load/save, keeps last good config); file watcher; config path
  schedule/    pure window math: InWindow, NextStart, NextLabel, LaunchedInWindow, Upcoming, Conflict
  launcher/    tray-process scheduler: App state (mutex-guarded), minute monitor, countdown, launch commands
  tray/        systray icon + menu (implements launcher.Tray)         — never imports Fyne
  ui/          all Fyne code: --manage board, editor/picker, --countdown window, theme
  logging/     log file setup/rotation (only the tray process rotates)
  platform/    per-OS code: Dock/frontmost app (CGo, macOS), data dir, open file, .ico tray icon
  discovery/   Steam + Epic install discovery
  assets/      embedded icon.png
packaging/macos/  Info.plist + build-app.sh for the .app bundle
```

Dependency direction: `config` ← `schedule` ← `launcher` ← `tray`, and `ui`
uses only `config`, `schedule`, `discovery`, `platform` and `assets`. Nothing
the tray depends on imports Fyne; `internal/tray/deps_test.go` enforces this.

### Dependencies

```
fyne.io/fyne/v2          — UI framework (--manage / --countdown processes only)
fyne.io/systray          — native system tray for the resident process
github.com/fyne-io/image — .ico encoding for the Windows tray icon
gopkg.in/yaml.v3         — config parsing
github.com/fsnotify/fsnotify      — config file hot-reload
github.com/shirou/gopsutil/v4     — process detection (direct-launch games only)
```

---

## Launch Methods

All launches go through platform clients to preserve cloud saves, achievements, and playtime.

| Method   | Path format                                          | Notes                        |
|----------|------------------------------------------------------|------------------------------|
| `steam`  | `steam://rungameid/APPID`                            | Discovered from `.acf` files |
| `epic`   | `com.epicgames.launcher://apps/APPNAME?action=launch&silent=true` | Uses `AppName` field, not `CatalogItemId` |
| `direct` | `/path/to/game.exe`                                  | No cloud saves; process-detectable |

### Why `direct` is different

Steam and Epic games are launched via URL handlers — the running process is the platform client, not the game. Process detection by executable name doesn't work. Instead, `hasLaunchedInCurrentWindow` prevents re-launching within the same schedule window. `isGameRunning()` only checks processes for `direct` method games.

---

## Schedule Monitoring

```
boot → shouldLaunchGame() → autoLaunchGameByName() [if in window]
     → scheduleMonitor() goroutine (every 1 minute)
          → isInScheduleWindow() for each enabled game
          → isGameRunning() — skip if direct-launch game running
          → hasLaunchedInCurrentWindow() — skip if already launched this window
          → getForegroundAppName() — skip if any app is in foreground
          → showLaunchCountdown() — countdown window, user can cancel
```

### Launch guards (in order)

1. **Game running** — `isGameRunning()`, direct-method only
2. **Already launched** — `hasLaunchedInCurrentWindow()`, all methods
3. **Foreground app** — `getForegroundAppName()` — if anything is focused, skip entirely
4. **Countdown** — user sees countdown window with cancel button

The foreground check means: if you're at your desk working, nothing launches. The schedule monitor retries every minute, so it picks up naturally once you step away or close the foreground app.

---

## UI

Built with Fyne. No dock icon on macOS (CGo sets `NSApplicationActivationPolicyAccessory`).

### Tray menu

```
Launch Now
──────────
Manage Games...
View Logs
──────────
Quit
```

### Game manager window

List of configured games with name + schedule summary. Edit and delete buttons per row. Add button at bottom opens the game picker.

### Game picker

Shows discovered Steam + Epic games in a list. "Enter manually..." option at bottom falls through to the editor with blank fields.

### Game editor

Single-dialog form: name, path/URL, launch method, launch args, day checkboxes, time window, enabled toggle. Path row is adaptive — steam/epic show a plain text entry with protocol placeholder; direct shows entry + Browse button.

Validation on save:
- Name and path required
- At least one day selected
- Times in `HH:MM` format
- No schedule overlap with any other configured game (checked by day + time window intersection)

### Countdown window

Standalone `fyne.Window` (not a dialog hosted in the game manager). Shows game name, seconds remaining, cancel button. Closing the window cancels the launch.

---

## Game Discovery

`discoverGames()` in `discovery.go` scans platform manifests and returns `[]DiscoveredGame`.

### Steam

Reads `libraryfolders.vdf` to find all library paths, then scans `.acf` manifest files for `appid` + `name`. Generates `steam://rungameid/APPID`.

Platform paths:
- macOS: `~/Library/Application Support/Steam/steamapps/`
- Windows: `Program Files (x86)/Steam/steamapps/`
- Linux: multiple paths including flatpak/snap variants

### Epic

Reads `.item` JSON manifest files. Uses `AppName` field (not `CatalogItemId`) for the launch URL. Generates `com.epicgames.launcher://apps/{AppName}?action=launch&silent=true`.

Platform paths:
- macOS: `~/Library/Application Support/Epic/EpicGamesLauncher/Data/Manifests/`
- Windows: `%ProgramData%/Epic/EpicGamesLauncher/Data/Manifests/`
- Linux: `~/.config/legendary/metadata/` (Heroic/Legendary)

---

## Config

```yaml
games:
  - game_name: "Stardew Valley"
    game_path: "steam://rungameid/413150"
    launch_method: "steam"
    launch_args: ""
    schedules:
      - days: [Thu]
        start_time: "19:00"
        end_time: "21:00"
    enabled: true

boot_delay: 10  # seconds for countdown before auto-launch
```

Config location: next to the binary if a `config.yaml` exists there (dev/portable), otherwise platform-appropriate app support directory. Hot-reloaded via fsnotify on write.

Legacy single-game format is migrated to the multi-game array on first load.

---

## Decisions Made

**Overlap handling via UI validation, not priority field** — rather than a `priority` field that silently picks a winner, the editor rejects overlapping schedules at save time. YAML edits that create overlaps are logged as warnings on load (TODO: implement).

**Foreground = skip, not defer** — if an app is in the foreground, the launch is skipped for that minute. The monitor retries on the next tick. This is intentionally silent — no popups, no interruption.

**Countdown is a standalone window** — not a dialog parented to the game manager, and in fact its own `--countdown` process. The tray process owns the countdown timer; the window can only cancel (exit code 3), so if it fails to start or crashes the launch still happens on schedule and the tray's Cancel item still works.

**No priority system** — superseded by overlap validation. Two games cannot be scheduled at the same time.

**Tray process doesn't use Fyne; `fyne.io/systray` is used directly** — previously Fyne's `desktop.App` (`desk.SetSystemTrayMenu`) owned the tray, which made the resident process a full Fyne app: setting a tray menu makes Fyne create a hidden "SystrayMonitor" window with an OpenGL context at startup (loading the GPU driver — the main reason Windows rated startup impact "High"), and Fyne's event loop ticks at 60 Hz forever even with no window visible (~2% of a core, measured on macOS). Splitting the UI into on-demand child processes took the idle tray process to ~0 CPU and cut its memory footprint from ~76 MB to ~19 MB (macOS, `top` MEM). The cost is a short Fyne startup (~0.5 s) each time Manage Games or the countdown opens.

**No zenity** — replaced entirely by Fyne dialogs.
