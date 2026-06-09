# Frictionless Launcher - Design Document

## Overview

Frictionless Launcher automatically launches games on a schedule to reduce the friction of starting a game after work or on weekends. Instead of deciding what to play and waiting through startup screens, your game is ready when you are.

## Core Features

### 1. Schedule-Based Auto-Launch

**Current Behavior (MVP):**
- Checks schedule on boot/startup
- If within scheduled time window, launches the game after a delay
- User can cancel during countdown

**Planned Behavior:**
- Continuously monitors schedule while running
- Launches games when scheduled time begins (even if already running)
- Smart detection to avoid launching if:
  - Computer is sleeping/locked
  - User is already in a game
  - System is under heavy load

### 2. Multi-Game Scheduling

Each game can have multiple time windows:

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

  - game_name: "Cyberpunk 2077"
    game_path: "steam://rungameid/1091500"
    launch_method: "steam"
    launch_args: "-skipStartScreen --launcher-skip"
    schedules:
      - days: [Sat, Sun]
        start_time: "00:00"
        end_time: "23:59"
    enabled: true
```

### 3. Launch Methods & Cloud Saves

**Problem:** Directly launching game `.exe` files bypasses platform clients, breaking:
- Cloud save sync
- Achievement tracking
- Playtime recording
- Platform overlays

**Solution:** Support multiple launch methods

#### Steam Launch
```yaml
launch_method: "steam"
game_path: "steam://rungameid/413150"
```

**How it works:**
- Uses Steam protocol handler
- Steam client launches game
- Cloud saves sync before launch and after exit
- All Steam features work

#### GOG Galaxy Launch
```yaml
launch_method: "gog"
game_path: "C:/Program Files (x86)/GOG Galaxy/GalaxyClient.exe"
launch_args: "/command=runGame /gameId=1207665503"
```

**How it works:**
- Launches through GOG Galaxy client
- Cloud saves sync on launch/exit
- GOG achievements tracked

#### Epic Games Launch
```yaml
launch_method: "epic"
game_path: "com.epicgames.launcher://apps/9773aa1aa54f4f7b80e44bef04986cea%3A530145df28a24424923f5828cc9031a1%3ACypherGame?action=launch&silent=true"
```

#### Direct Launch (Fast, No Cloud)
```yaml
launch_method: "direct"
game_path: "C:/GOG Games/Cyberpunk 2077/bin/x64/Cyberpunk2077.exe"
launch_args: "-skipintro"
```

**Tradeoff:**
- ⚡ Faster startup (no client overhead)
- ❌ No cloud save sync
- ❌ No achievements
- ❌ No playtime tracking

### 4. Smart Game Discovery

**Problem:** Manual configuration is tedious. Users don't know:
- Where games are installed
- What Steam App IDs to use
- What launch arguments optimize startup

**Solution:** Automated game scanner

#### Steam Scanner
Uses `github.com/doctype/steam` library to:
- Find Steam installation directory
- Parse `.acf` manifest files from `steamapps/` folders
- Extract: App ID, game name, install path, last played time
- Generate proper `steam://rungameid/XXXXX` URLs

#### Epic Games Scanner
Uses `github.com/meszmate/manifest` library to:
- Read manifests from `C:\ProgramData\Epic\EpicGamesLauncher\Data\Manifests\`
- Parse JSON for game name, install path, catalog ID
- Generate Epic protocol URLs

#### GOG Galaxy Scanner
**Status:** No Go library available
**Workaround:**
- Parse GOG Galaxy's SQLite database directly
- Or scan common install directories

### 5. Launch Arguments Auto-Detection

**Problem:** Many games have slow startup due to:
- Splash screens
- Intro videos
- Launcher windows
- Legal notices

**Solution:** Auto-populate optimal launch arguments

#### Data Sources (Priority Order)

1. **SKIF Database** - Community-maintained JSON database
   - URL: `https://github.com/SpecialKO/SKIF_launch_configs`
   - Format: `{"appid": {"name": "Game", "launch_args": "-skipintro"}}`
   - Covers ~100+ popular games

2. **Built-in Database** - Hardcoded common patterns
   ```go
   var knownGameArgs = map[string]string{
       "413150":  "",                                  // Stardew Valley
       "1091500": "-skipStartScreen --launcher-skip", // Cyberpunk 2077
       "72850":   "-nosplash",                        // Skyrim
   }
   ```

3. **PCGamingWiki API** - Fallback for rare games
   - Uses MediaWiki Cargo API
   - Query game-specific launch args
   - More comprehensive but slower

4. **Common Patterns** - Try these for unknown games
   - `-nosplash` - Skip splash screens (works in many UE/Unity games)
   - `-skipintro` - Skip intro videos
   - `-nostartupmovies` - Skip startup movies

#### User Experience

Scanner shows detected args with explanations:
```
☑ Cyberpunk 2077 (Steam)
  🕐 Played 1 week ago
  🚀 Launch args: -skipStartScreen --launcher-skip
  ℹ️  Skips startup screens for faster launch
  [Edit...] [Learn More]
```

Users can:
- Accept auto-detected args
- Manually edit
- Test launch before saving
- See what each arg does

## Schedule Monitoring Logic

### Current Implementation (Boot-Only)

```go
func main() {
    app := &App{}
    app.loadConfig()

    // Check schedule once at startup
    if app.config.Enabled && app.shouldLaunchNow() {
        app.launchPending = true
        go app.autoLaunchGame()
    }

    systray.Run(app.onReady, app.onExit)
}
```

**Limitation:** Only checks schedule at boot. If you boot at 6 PM and Thursday schedule starts at 7 PM, nothing happens.

### Planned Implementation (Continuous Monitoring)

```go
func main() {
    app := &App{}
    app.loadConfig()

    // Check schedule at startup
    if app.config.Enabled && app.shouldLaunchNow() {
        app.launchPending = true
        go app.autoLaunchGame()
    }

    // Start schedule monitor
    go app.scheduleMonitor()

    systray.Run(app.onReady, app.onExit)
}

func (app *App) scheduleMonitor() {
    ticker := time.NewTicker(1 * time.Minute) // Check every minute
    defer ticker.Stop()

    var lastChecked time.Time

    for {
        select {
        case <-ticker.C:
            now := time.Now()

            // Skip if we already checked this minute
            if now.Minute() == lastChecked.Minute() {
                continue
            }
            lastChecked = now

            // Skip if system just woke from sleep (avoid spam launches)
            if app.isSystemWakingFromSleep() {
                log.Println("System recently woke from sleep, skipping launch")
                continue
            }

            // Skip if user is already in a game
            if app.isGameRunning() {
                log.Println("Game already running, skipping launch")
                continue
            }

            // Check each game's schedule
            for _, game := range app.config.Games {
                if !game.Enabled {
                    continue
                }

                if app.shouldLaunchGame(game, now) {
                    log.Printf("Schedule triggered for %s", game.GameName)
                    go app.launchGameWithDelay(game)
                }
            }
        }
    }
}
```

### Smart Launch Guards

Current MVP includes guards 1 & 2. Guard 3 (app in foreground) is basic - can be enhanced with popup UI later.

#### 1. Detect System Sleep/Wake (IMPLICIT)

**Handled implicitly:** The schedule window check covers this. If system sleeps before scheduled time and wakes within the window, it will launch. If it wakes after the window, it won't launch (by design - schedule is a time window, not a catch-up mechanism).

**Example:**
- Schedule: 7:00-9:00 PM
- System wakes at 8:30 PM → Launches (within window)
- System wakes at 10:00 PM → Doesn't launch (window closed)

No additional code needed.

#### 2. Detect Running Games (IMPLEMENTED)

**Problem:** Don't launch Stardew Valley if user is already playing Cyberpunk

**Solution:** Check for running game processes using `github.com/shirou/gopsutil/v4`

```go
func (app *App) isGameRunning() bool {
    processes, err := process.Processes()
    if err != nil {
        return false
    }

    // Check if any configured game's executable is running
    for _, game := range app.config.Games {
        for _, proc := range processes {
            // Match by executable name or full path
            name, _ := proc.Name()
            exe, _ := proc.Exe()
            
            if strings.EqualFold(name, filepath.Base(game.GamePath)) ||
               strings.EqualFold(exe, game.GamePath) {
                return true
            }
        }
    }
    return false
}
```

**Status:** ✅ Implemented. Prevents launching while any game is running.

#### 3. Detect App in Foreground (IMPLEMENTED)

**Problem:** Don't silently auto-launch if user is actively working in another app

**Solution:** Check which app has focus using `gopsutil`

```go
func (app *App) getForegroundAppName() (string, error) {
    processes, _ := process.Processes()
    for _, proc := range processes {
        isFg, _ := proc.Foreground()
        if isFg {
            return proc.Name()
        }
    }
    return "", nil
}
```

**Behavior:**
- If app is in foreground during schedule window: Show warning in system tray tooltip instead of auto-launching
- User can manually click "Launch Now" to override
- Future: Could show popup with countdown timer + cancel button (TBD)

**Status:** ✅ Implemented (basic version)

#### 4. Check System Load

**Problem:** Don't launch games during Windows updates or heavy workloads

**Solution:** Monitor CPU/memory usage

```go
func (app *App) isSystemBusy() bool {
    cpuUsage := getCurrentCPUUsage()
    memUsage := getCurrentMemoryUsage()

    // Don't launch if system is under heavy load
    return cpuUsage > 80.0 || memUsage > 90.0
}
```

### Schedule Transition Handling

**Scenario:** Thursday 7 PM schedule ends at 9 PM. What happens at 9:01 PM?

**Answer:** Nothing. We only launch at schedule START times.

```go
func (app *App) shouldLaunchGame(game GameSchedule, now time.Time) bool {
    currentTime := now.Format("15:04")
    currentDay := now.Weekday().String()[:3] // "Mon", "Tue", etc.

    for _, schedule := range game.Schedules {
        // Check if today matches
        dayMatches := false
        for _, day := range schedule.Days {
            if strings.EqualFold(day, currentDay) {
                dayMatches = true
                break
            }
        }

        if !dayMatches {
            continue
        }

        // Only launch at START time (not during entire window)
        if currentTime == schedule.StartTime {
            return true
        }
    }

    return false
}
```

### Edge Cases

1. **Overlapping Schedules**
   - Two games scheduled for same time
   - Solution: Use priority field or launch first enabled game

2. **Multi-Day Schedules**
   - "Friday night 11 PM to Saturday 2 AM"
   - Solution: Support `end_day` field or split into two schedules

3. **Daylight Saving Time**
   - Schedule at 2 AM on DST transition day
   - Solution: Use `time.LoadLocation()` with local timezone

4. **Rapid Enable/Disable Toggling**
   - User toggles game on/off during countdown
   - Solution: Use atomic flags (already implemented)

## UI/UX Design

### Philosophy: Minimal Resources

To minimize boot speed impact and background resource usage, the app avoids heavyweight UI frameworks. Configuration is done via YAML files, not graphical UI.

### System Tray Menu (Ultra-Minimal)

```
┌────────────────────────────┐
│ Frictionless Launcher      │
├────────────────────────────┤
│ Next: Stardew Valley       │  <- Shows next scheduled game
│ Thu 19:00-21:00            │
├────────────────────────────┤
│ Launch Now                 │
│ Cancel Countdown           │
├────────────────────────────┤
│ Open Config File           │  <- Opens in text editor
│ View Logs                  │
│ Exit                       │
└────────────────────────────┘
```

### Configuration via YAML (config.yaml)

Users edit schedules directly in their text editor:

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

  - game_name: "Cyberpunk 2077"
    game_path: "steam://rungameid/1091500"
    launch_method: "steam"
    launch_args: "-skipStartScreen --launcher-skip"
    schedules:
      - days: [Sat, Sun]
        start_time: "00:00"
        end_time: "23:59"
    enabled: true

boot_delay: 10
```

**Advantages:**
- No GUI framework overhead (no Fyne, no web server)
- Zero impact on boot speed
- Minimal memory footprint
- Version-controllable config
- Works in any text editor

**Future Enhancement:** If game discovery becomes important, a separate CLI tool (`frictionless-scan`) could scan Steam/Epic libraries and suggest games to add to config.yaml.

## Technical Architecture

### File Structure

```
frictionless/
├── main.go                 # Entry point, systray, schedule logic
├── config.go               # Config loading/saving
├── scheduler.go            # Continuous schedule monitoring
├── game_launcher.go        # Game launch logic
├── system_monitors.go      # Sleep/idle/load detection (future)
├── game_scanner.go         # Steam/Epic/GOG detection (future CLI tool)
└── config.yaml             # User configuration (YAML)
```

### Dependencies

```go
require (
    fyne.io/systray v1.12.0             // System tray
    gopkg.in/yaml.v3 v3.0.1             // Config parsing
    github.com/shirou/gopsutil/v4 v4.26.5 // Process monitoring
    github.com/fsnotify/fsnotify v1.10.1  // Config file watching
)
```

Future optional enhancements:
```go
// For game auto-discovery:
github.com/meszmate/manifest            // Epic Games manifest parsing
```

### Configuration Format (Extended)

```yaml
games:
  - game_name: "Stardew Valley"
    game_path: "steam://rungameid/413150"
    launch_method: "steam"  # steam, gog, epic, direct
    launch_args: ""
    launch_args_source: "auto-detected"  # or "user-customized"
    steam_app_id: "413150"  # For reference
    last_played: "2026-03-16T19:30:00Z"
    schedules:
      - days: [Thu]
        start_time: "19:00"
        end_time: "21:00"
    enabled: true
    priority: 10  # Higher priority wins if overlap

  - game_name: "Cyberpunk 2077"
    game_path: "steam://rungameid/1091500"
    launch_method: "steam"
    launch_args: "-skipStartScreen --launcher-skip"
    launch_args_source: "auto-detected"
    steam_app_id: "1091500"
    schedules:
      - days: [Sat, Sun]
        start_time: "00:00"
        end_time: "23:59"
    enabled: true
    priority: 5

boot_delay: 10  # Seconds to wait before auto-launch on boot
check_interval: 60  # Seconds between schedule checks
launch_guards:
  skip_if_sleeping: true
  skip_if_game_running: true
  skip_if_user_active: true  # Don't interrupt active user
  min_idle_minutes: 5
  max_cpu_percent: 80
  max_memory_percent: 90
```

## Future Enhancements

### 1. Game Usage Analytics
- Track actual play time
- Suggest schedule adjustments based on patterns
- "You usually play Cyberpunk on Saturdays, not Sundays"

### 2. Profile Switching
- "Work Week" profile vs "Vacation" profile
- Different games for different contexts

### 3. Integration with Other Launchers
- Playnite integration
- Lutris support (Linux)
- Heroic Launcher (Epic alternative)

### 4. Launch Conditions
- Only launch if specific friend is online (Steam API)
- Only launch if game was updated recently
- Only launch if achievement progress < 100%

### 5. Pre-Launch Tasks
- Update game before launch
- Launch Discord/voice chat
- Close other apps (browsers, etc.)
- Set system to "Gaming Mode" (disable notifications)

### 6. Post-Game Actions
- Automatically backup saves
- Upload screenshots to cloud
- Log play session notes

## References

- **Playnite:** Open source game launcher (doesn't do scheduling)
- **PCGamingWiki:** Community database of game launch args
- **SKIF Launch Configs:** Curated launch args database
- **Steam Protocol Handlers:** `steam://rungameid/XXXXX`
- **GOG Galaxy Integration API:** Python-based plugin system

## Contributing

See `NOTE.md` for development notes and design decisions made during implementation.
