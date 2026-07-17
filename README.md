# Frictionless Game Launcher

I built this app for myself, sometimes after a day of work I found it hard to actually start up a game. This is a lightweight system tray application that automatically launches games on a schedule after boot, reducing the friction of starting a game.

## Current Features

- **Multi-Game Support** - Configure multiple games with individual schedules
- **Flexible Scheduling** - Custom time windows by day of week (e.g., "Thursdays 7-9 PM")
- **Steam Integration** - Launch games via Steam protocol for cloud save sync
- **Launch Arguments** - Skip intros, splash screens, and optimize startup
- **Clickable Tray Menu** - Click any game in the system tray to launch instantly
- **Auto-Launch on Boot** - Games launch automatically when schedule matches
- **File-based Configuration** - Simple YAML config that's easy to edit and backup
- **Cross-platform** - Works on Windows, macOS, and Linux/SteamOS

## Quick Start

1. **Download and run** - On first launch, an empty config file is created (no games pre-configured)
2. **Add your games** - Click the tray icon → "Manage Games..." to add, edit, and schedule games through the GUI
3. **Optional: Launch on boot**
   - **Windows**: Press `Win + R`, type `shell:startup`, and drop a shortcut to the binary there
   - **macOS**: System Settings → General → Login Items
   - **Linux**: Add to your desktop environment's autostart

## Configuration

The app uses a YAML config file. See [config.example.yaml](config.example.yaml) for a full example. You can edit games through the tray icon's "Manage Games..." window, or edit the YAML file directly.

### Config file location

On startup, the app first checks for a `config.yaml` sitting next to the binary itself (portable mode — handy for a USB stick or a self-contained install). If none is found there, it falls back to an OS-specific location:

- **Windows**: `%LOCALAPPDATA%\FrictionlessLauncher\config.yaml`
- **macOS**: `~/Library/Application Support/FrictionlessLauncher/config.yaml`
- **Linux**: `~/.config/FrictionlessLauncher/config.yaml`

### Basic Example

```yaml
boot_delay: 10  # Seconds to wait before auto-launching

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
        start_time: "10:00"
        end_time: "23:59"
    enabled: true
```

### Launch Methods

- **`steam`** - Uses Steam protocol handler (e.g., `steam://rungameid/413150`)
  - ✅ Cloud saves sync automatically
  - ✅ Achievements tracked
  - ✅ Play time recorded
  - 📝 Find Steam App IDs at [steamdb.info](https://steamdb.info)

- **`direct`** - Launches game executable directly
  - ⚡ Faster startup (no Steam overhead)
  - ❌ No cloud save sync
  - ❌ No achievements

### Schedule Format

- **days**: Array of day abbreviations: `Mon`, `Tue`, `Wed`, `Thu`, `Fri`, `Sat`, `Sun`
- **start_time**: 24-hour format `HH:MM` (e.g., `19:00` for 7 PM)
- **end_time**: 24-hour format `HH:MM`
- Multiple schedules per game are supported

## Development

### Prerequisites
- Go 1.24 or later

### Build

```bash
git clone git@github.com:brycethomsen/frictionless-launcher.git
cd frictionless-launcher
make build
```

### Project Structure

- `main.go` - Core app logic, system tray, scheduling
- `config.example.yaml` - Example configuration
- `DESIGN.md` - Detailed design document and future plans
- `TODO.md` - Known issues and planned features

## Roadmap

See [TODO.md](TODO.md) for the full list. Key features planned:
- Continuous schedule monitoring (currently only checks at boot)
- Auto-discovery of installed Steam games
- Launch args auto-detection from community databases
- GUI for game management

## Resources

- **[PCGamingWiki](https://www.pcgamingwiki.com)** - Find launch arguments to skip intros and optimize startup
- **[SteamDB](https://steamdb.info)** - Look up Steam App IDs for games
