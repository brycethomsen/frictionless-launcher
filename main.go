package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/systray"
	"gopkg.in/yaml.v3"
)

//go:embed icon.ico
var iconData []byte

type Config struct {
	GamePath   string `yaml:"game_path"`
	GameName   string `yaml:"game_name"`
	LaunchArgs string `yaml:"launch_args"`
	Enabled    bool   `yaml:"enabled"`
	BootDelay  int    `yaml:"boot_delay"`
	Schedule   string `yaml:"schedule"` // Simple schedule name from examples
}

type App struct {
	config        *Config
	configPath    string
	launchPending bool
	shouldCancel  int32 // Atomic flag: 1 = cancel, 0 = continue
}

func main() {
	app := &App{
		configPath:    getConfigPath(),
		launchPending: false,
	}

	app.loadConfig()

	// If enabled and schedule matches, launch the game
	if app.config.Enabled && app.shouldLaunchNow() {
		app.launchPending = true
		go app.autoLaunchGame()
	}

	// Start system tray
	systray.Run(app.onReady, app.onExit)
}

func (app *App) onReady() {
	app.updateTrayIcon()

	// Use embedded .ico file
	systray.SetIcon(iconData)

	// Current game display
	currentGame := systray.AddMenuItem(app.config.GameName, "Current game")
	currentGame.Disable()

	scheduleStatus := systray.AddMenuItem("Schedule: "+app.config.Schedule, "Current schedule")
	scheduleStatus.Disable()

	systray.AddSeparator()

	// Menu items
	launchNow := systray.AddMenuItem("Launch Now", "Launch the game immediately")
	toggleEnabled := systray.AddMenuItem("", "") // Text set dynamically

	systray.AddSeparator()

	about := systray.AddMenuItem("Edit Config", "Open config.yaml in default editor")
	quit := systray.AddMenuItem("Exit", "Exit Frictionless Launcher")

	// Update toggle text based on current state
	app.updateToggleMenuItem(toggleEnabled)

	// Handle menu clicks
	go func() {
		for {
			select {
			case <-launchNow.ClickedCh:
				go app.launchGame()

			case <-toggleEnabled.ClickedCh:
				app.config.Enabled = !app.config.Enabled
				fmt.Printf("DEBUG: Toggled enabled to %v, launchPending: %v\n", app.config.Enabled, app.launchPending)

				// Set atomic cancel flag if disabling during launch
				if !app.config.Enabled && app.launchPending {
					atomic.StoreInt32(&app.shouldCancel, 1)
					fmt.Println("🚨 DEBUG: Set shouldCancel flag to 1 - goroutine should see this!")
				}

				app.saveConfig()
				app.updateToggleMenuItem(toggleEnabled)
				app.updateTrayIcon()

			case <-about.ClickedCh:
				app.openConfigFile()

			case <-quit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func (app *App) onExit() {
	// Cleanup if needed
}

func (app *App) loadConfig() {
	// Set defaults based on platform
	app.config = &Config{
		GameName:   "Test Command",
		GamePath:   "/usr/bin/say", // macOS text-to-speech for testing
		LaunchArgs: "Game launched successfully",
		Enabled:    true,
		BootDelay:  5,
		Schedule:   "always",
	}

	if _, err := os.Stat(app.configPath); os.IsNotExist(err) {
		fmt.Println("No config found, creating default config.yaml")
		app.saveConfig()
		return
	}

	data, err := os.ReadFile(app.configPath)
	if err != nil {
		log.Printf("Error reading config: %v", err)
		fmt.Println("Using default config due to read error")
		return
	}

	if err := yaml.Unmarshal(data, app.config); err != nil {
		log.Printf("Error parsing config: %v", err)
		fmt.Println("Config file has invalid YAML, using defaults")
		fmt.Println("Please check your config.yaml file for syntax errors")
		return
	}

	fmt.Printf("Loaded config: %s\n", app.config.GameName)
}

func (app *App) saveConfig() {
	data, err := yaml.Marshal(app.config)
	if err != nil {
		log.Printf("Error marshaling config: %v", err)
		return
	}

	if err := os.WriteFile(app.configPath, data, 0644); err != nil {
		log.Printf("Error saving config: %v", err)
	}
}

func (app *App) shouldLaunchNow() bool {
	now := time.Now()

	// Simple schedule checking based on predefined schedules
	switch app.config.Schedule {
	case "always":
		return true

	case "after_5pm_daily":
		return now.Hour() >= 17

	case "weekends_anytime":
		weekday := now.Weekday()
		return weekday == time.Saturday || weekday == time.Sunday

	case "tue_thu_after_8pm":
		weekday := now.Weekday()
		return (weekday == time.Tuesday || weekday == time.Thursday) && now.Hour() >= 20

	case "weekdays_evening":
		weekday := now.Weekday()
		return weekday >= time.Monday && weekday <= time.Friday && now.Hour() >= 18 && now.Hour() < 22

	default:
		return false
	}
}

func (app *App) autoLaunchGame() {
	defer func() {
		app.launchPending = false
		atomic.StoreInt32(&app.shouldCancel, 0) // Reset flag
	}()

	fmt.Printf("Auto-launching %s in %d seconds...\n", app.config.GameName, app.config.BootDelay)

	// Countdown checking atomic flag every 100ms for responsiveness
	for i := 0; i < app.config.BootDelay*10; i++ {
		if i%10 == 0 { // Print countdown every second
			cancelFlag := atomic.LoadInt32(&app.shouldCancel)
			fmt.Printf("Countdown: %d seconds remaining, shouldCancel=%d\n", app.config.BootDelay-(i/10), cancelFlag)
		}

		// Check atomic cancel flag every 100ms
		cancelFlag := atomic.LoadInt32(&app.shouldCancel)
		if cancelFlag == 1 {
			fmt.Println("✅ CANCELLED! shouldCancel flag was set to 1")
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Final check before launching
	finalFlag := atomic.LoadInt32(&app.shouldCancel)
	if finalFlag == 1 {
		fmt.Println("✅ CANCELLED! shouldCancel flag was 1 at final check")
		return
	}

	fmt.Println("Proceeding with launch...")
	app.launchGame()
}

func (app *App) launchGame() {
	if app.config.GamePath == "" {
		fmt.Println("No game configured")
		return
	}

	fmt.Printf("Launching %s...\n", app.config.GameName)

	var cmd *exec.Cmd
	if app.config.LaunchArgs != "" {
		// Split launch args properly
		args := strings.Fields(app.config.LaunchArgs)
		cmd = exec.Command(app.config.GamePath, args...)
	} else {
		cmd = exec.Command(app.config.GamePath)
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Error launching game: %v", err)
		return
	}

	fmt.Printf("%s launched successfully\n", app.config.GameName)
}

func (app *App) updateToggleMenuItem(item *systray.MenuItem) {
	if app.config.Enabled {
		item.SetTitle("Disable Auto-Launch")
	} else {
		item.SetTitle("Enable Auto-Launch")
	}
}

func (app *App) updateTrayIcon() {
	tooltip := "Frictionless Launcher - "
	if app.config.Enabled {
		if app.shouldLaunchNow() {
			tooltip += "Active (in schedule)"
		} else {
			tooltip += "Active (outside schedule)"
		}
	} else {
		tooltip += "Disabled"
	}
	systray.SetTooltip(tooltip)
}

func (app *App) openConfigFile() {
	configPath := app.configPath

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("Config file not found: %s\n", configPath)
		return
	}

	// Open with default program based on OS
	var cmd *exec.Cmd

	switch {
	case strings.Contains(strings.ToLower(os.Getenv("OS")), "windows"):
		// Windows: use start command
		cmd = exec.Command("cmd", "/c", "start", configPath)
	case fileExists("/usr/bin/open"):
		// macOS: use open command
		cmd = exec.Command("open", configPath)
	default:
		// Linux: use xdg-open
		cmd = exec.Command("xdg-open", configPath)
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("Error opening config file: %v\n", err)
		fmt.Printf("Config file location: %s\n", configPath)
	} else {
		fmt.Printf("Opened config file: %s\n", configPath)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func (app *App) getStatusText() string {
	if app.config.Enabled {
		if app.shouldLaunchNow() {
			return "Active (in schedule)"
		} else {
			return "Active (outside schedule)"
		}
	} else {
		return "Disabled"
	}
}

func getConfigPath() string {
	// Try local directory first (for development/portable installs)
	exe, _ := os.Executable()
	localDir := filepath.Dir(exe)
	localConfig := filepath.Join(localDir, "config.yaml")

	if _, err := os.Stat(localConfig); err == nil {
		return localConfig
	}

	// Fall back to OS-appropriate location
	var configDir string

	switch {
	case strings.Contains(strings.ToLower(os.Getenv("OS")), "windows"):
		// Windows: %LOCALAPPDATA%\FrictionlessLauncher\config.json
		configDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "FrictionlessLauncher")
	case fileExists("/Users"):
		// macOS: ~/Library/Application Support/FrictionlessLauncher/config.json
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, "Library", "Application Support", "FrictionlessLauncher")
	default:
		// Linux: ~/.config/FrictionlessLauncher/config.json
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config", "FrictionlessLauncher")
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Printf("Warning: Could not create config directory %s: %v\n", configDir, err)
		// Fall back to local directory
		return localConfig
	}

	return filepath.Join(configDir, "config.yaml")
}
