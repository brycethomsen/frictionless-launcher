# Frictionless Launcher

This is a lightweight system tray application that automatically launches games on a schedule, reducing the friction of starting a gaming session.

## ✅ Current Features (MVP)

- **🕒 Scheduled Auto-Launch** - Launch games based on predefined schedules
- **🎮 System Tray Integration** - Unobtrusive background operation
- **⚡ Launch Arguments Support** - Skip intros, splash screens, and optimize startup
- **🔄 Toggle Control** - Quick enable/disable from system tray
- **🚀 Manual Launch** - "Launch Now" button for immediate gaming
- **📁 File-based Configuration** - Simple YAML config that's easy to edit and backup
- **🖥️ Cross-platform** - Works on Windows, macOS, and Linux/SteamOS

### Available Schedules

- `always` - Launch anytime on boot
- `after_5pm_daily` - Every day after 5 PM
- `weekends_anytime` - Saturday and Sunday anytime  
- `tue_thu_after_8pm` - Tuesday and Thursday after 8 PM
- `weekdays_evening` - Monday-Friday 6-10 PM

## 🚀 Quick Start

### Prerequisites
- Go 1.24 or later

### Installation

1. **Clone the repository**
   ```bash
   git clone <repo-url>
   cd frictionless
   ```

2. **Build for your platform**
   ```bash
   # Current platform
   make build
   
   # Specific platforms
   make mac-intel    # macOS Intel
   make mac-arm      # macOS Apple Silicon  
   make windows      # Windows
   make linux        # Linux/SteamOS
   ```

3. **Run the application**
   ```bash
   # The binary will be named based on your platform
   ./frictionless-launcher-darwin-amd64   # macOS Intel example
   ```

4. **Configure your game**
   
   Edit the generated `config.yaml` file:
   ```yaml
   game_path: "/path/to/your/game.exe"
   game_name: "Your Game Name"
   launch_args: "-skipintro -nosplash"
   enabled: true
   boot_delay: 10
   schedule: "after_5pm_daily"
   ```

### Example Configurations

**Cyberpunk 2077 (Windows)**
```yaml
game_path: "C:\\Program Files (x86)\\Steam\\steamapps\\common\\Cyberpunk 2077\\bin\\x64\\Cyberpunk2077.exe"
game_name: "Cyberpunk 2077"
launch_args: "-skipStartScreen --launcher-skip"
enabled: true
boot_delay: 10
schedule: "after_5pm_daily"
```

**Skyrim (macOS)**
```yaml
game_path: "/Users/username/Library/Application Support/Steam/steamapps/common/Skyrim Special Edition/SkyrimSE.exe"
game_name: "The Elder Scrolls V: Skyrim Special Edition"
launch_args: "-skipintro -nosplash"
enabled: true
boot_delay: 15
schedule: "weekends_anytime"
```

## 📋 Configuration Reference

### Launch Arguments by Game

**Cyberpunk 2077:**
- `-skipStartScreen` - Skip intro videos and splash screens
- `--launcher-skip` - Skip GOG/Steam launcher overlay

**Bethesda Games (Skyrim, Fallout):**
- `-skipintro` - Skip intro videos
- `-nosplash` - Skip splash screens
- `-windowed` - Start in windowed mode

**Baldur's Gate 3:**
- `--skip-launcher` - Skip Larian launcher

**Source Engine Games:**
- `-novid` - Skip intro videos
- `-windowed` - Start in windowed mode

## 🛠️ Development

### Build Commands

```bash
make help           # Show all available commands
make build          # Build for current platform
make all-platforms  # Build for all platforms
make run            # Run without building
make clean          # Remove build artifacts
make deps           # Update dependencies
make info           # Show build environment info
```

### Platform Support

- **Windows** - Full support with system tray
- **macOS** - System tray integration (Intel and Apple Silicon)
- **Linux/SteamOS** - System tray in Desktop Mode

## 📅 Roadmap & Future Features

The following features were designed but not implemented in the MVP, planned for future releases:

### 🔮 Advanced Scheduler UI
- **Visual Time/Day Picker Interface** - The Option 3 UI we designed with checkboxes and time selectors
- **Live Preview** - "Your game will launch on boot during these times: Tue, Thu, Sun 6:00-11:00 PM"
- **Test Indicator** - Real-time "would launch now" status with green/red dots
- **Apply/Cancel System** - Clean confirmation workflow

### 🕵️ Game Scanner & Detection
- **Multi-Launcher Support** - Auto-detect Steam, Epic Games, GOG, Ubisoft, EA, Battle.net games
- **Steam Library Parsing** - Read ACF files, library folders, get Steam app IDs
- **Epic Games Manifests** - Parse Epic's JSON manifests for installed games
- **Registry Integration** - Read GOG games from Windows registry
- **Recent Activity Detection** - File modification times, Steam user data for last played
- **Smart Executable Finding** - Locate actual game .exe files in install directories

### 🚀 Launch Arguments Database & Detection
- **PCGamingWiki Integration** - Automatic lookup of optimal launch arguments
- **Game-Specific Database** - Pre-configured args for popular games (Cyberpunk 2077, Skyrim, etc.)
- **Automatic Detection** - Analyze game files and configs to suggest launch parameters
- **Launch Args Description System** - Human-readable explanations of what each argument does

### 🎨 Launch Arguments UI
- **Visual Editor Interface** - The UI we designed with current args display and suggestions
- **Suggestion Buttons** - Click to add common args like `-skipintro`, `-nosplash`
- **Auto-Detect Button** - Automatically find optimal args for detected games
- **Game-Specific Recommendations** - Show known working combinations per game
- **Test Launch Feature** - Try arguments before saving them
- **Clear/Reset Functions** - Easy way to start over or return to defaults

### 🔧 System Integration
- **Windows Startup Integration** - Proper Windows service or startup folder integration
- **Better System Tray Icons** - Proper gamepad icons (green/gray states)
- **Cross-Platform Support** - Full SteamOS/Linux system tray integration

### 🎯 Smart Game Selection
- **Recent Game Auto-Selection** - Automatically choose most recently played game on first run
- **Cyberpunk 2077 Priority** - Smart detection and prioritization of specific games
- **Game Selector Window** - The UI we designed showing found games with "Select (Recent)" buttons

## 🤝 Contributing

We welcome contributions! The codebase is intentionally simple to make it easy for new contributors to get involved.

### Architecture Notes
- **`main.go`** - Single-file application for simplicity
- **`config.json`** - Human-readable configuration
- **Cross-platform** - Uses Go's excellent cross-compilation support
- **Minimal dependencies** - Only system tray library required

---

**TL;DR**: This app automatically launches your games when you want to play them, skipping the decision fatigue and startup friction. Perfect for after-work gaming sessions or weekend gaming marathons.