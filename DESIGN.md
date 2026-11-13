# Frictionless Launcher — Design Document

## Overview

Frictionless Launcher automatically launches games on a schedule so your game is ready when you sit down. No decision fatigue, no startup friction.

---

## Architecture

Single Go binary. System tray app with no dock icon on macOS.

```
main.go          — app lifecycle, tray menu, schedule monitor, launch logic
ui.go            — Fyne UI: game manager window, countdown window, game editor/picker
discovery.go     — Steam + Epic manifest parsing for auto-discovery
dialogs.go       — shared helpers (isValidTimeFormat)
dock_darwin.go   — CGo: NSApplicationActivationPolicyAccessory to hide dock icon
dock_other.go    — no-op stub for non-macOS
```

### Dependencies

```
fyne.io/fyne/v2          — UI framework + system tray (desktop.App interface)
fyne.io/systray          — indirect dep used internally by Fyne
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

**Countdown is a standalone window** — not a dialog parented to the game manager. The game manager window stays hidden unless the user opens it from the tray.

**No priority system** — superseded by overlap validation. Two games cannot be scheduled at the same time.

**`fyne.io/systray` not used directly** — Fyne's `desktop.App` interface (`desk.SetSystemTrayMenu`) owns the tray. `fyne.io/systray` remains as an indirect dependency used internally by Fyne.

**No zenity** — replaced entirely by Fyne dialogs.
