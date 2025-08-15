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
game_path: 'C:\Program Files (x86)\Steam\steamapps\common\Cyberpunk 2077\bin\x64\Cyberpunk2077.exe'
game_name: "Cyberpunk 2077"
launch_args: "-skipStartScreen --launcher-skip"
enabled: true
boot_delay: 10
schedule: "after_5pm_daily"
```
