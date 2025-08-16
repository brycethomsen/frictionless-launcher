package main

import (
	_ "embed"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	shouldCancel  int32    // Atomic flag: 1 = cancel, 0 = continue
	logFile       *os.File // Log file handle for proper cleanup
}

func main() {
	app := &App{
		configPath:    getConfigPath(),
		launchPending: false,
	}

	// Set up logging to file and store handle in app
	app.setupLogging()
	defer app.closeLogFile() // Ensure log file is closed on exit

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

	editConfig := systray.AddMenuItem("Edit", "Open config.yaml in default editor")
	openLogs := systray.AddMenuItem("Log", "Open log file in default editor")
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
				log.Printf("Toggled enabled to %v, launchPending: %v", app.config.Enabled, app.launchPending)

				// Set atomic cancel flag if disabling during launch
				if !app.config.Enabled && app.launchPending {
					atomic.StoreInt32(&app.shouldCancel, 1)
					log.Println("Set shouldCancel flag to 1 - goroutine should see this")
				}

				app.saveConfig()
				app.updateToggleMenuItem(toggleEnabled)
				app.updateTrayIcon()

			case <-editConfig.ClickedCh:
				app.openConfigFile()

			case <-openLogs.ClickedCh:
				app.openLogFile()

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
		log.Println("No config found, creating default config.yaml")
		app.saveConfig()
		return
	}

	data, err := os.ReadFile(app.configPath)
	if err != nil {
		log.Printf("Error reading config: %v", err)
		log.Println("Using default config due to read error")
		return
	}

	if err := yaml.Unmarshal(data, app.config); err != nil {
		log.Printf("Error parsing config: %v", err)
		log.Println("Config file has invalid YAML, using defaults - please check your config.yaml file for syntax errors")
		return
	}

	log.Printf("Loaded config: %s", app.config.GameName)
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

	log.Printf("Auto-launching %s in %d seconds", app.config.GameName, app.config.BootDelay)

	// Countdown checking atomic flag every 100ms for responsiveness
	for i := 0; i < app.config.BootDelay*10; i++ {
		if i%10 == 0 { // Print countdown every second
			cancelFlag := atomic.LoadInt32(&app.shouldCancel)
			log.Printf("Countdown: %d seconds remaining, shouldCancel=%d", app.config.BootDelay-(i/10), cancelFlag)
		}

		// Check atomic cancel flag every 100ms
		cancelFlag := atomic.LoadInt32(&app.shouldCancel)
		if cancelFlag == 1 {
			log.Println("CANCELLED - shouldCancel flag was set to 1")
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Final check before launching
	finalFlag := atomic.LoadInt32(&app.shouldCancel)
	if finalFlag == 1 {
		log.Println("CANCELLED - shouldCancel flag was 1 at final check")
		return
	}

	log.Println("Proceeding with launch")
	app.launchGame()
}

func (app *App) launchGame() {
	if app.config.GamePath == "" {
		log.Println("No game configured")
		return
	}

	log.Printf("Launching %s", app.config.GameName)

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

	log.Printf("%s launched successfully", app.config.GameName)
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
		log.Printf("Config file not found: %s", configPath)
		return
	}

	// Open with default program based on OS
	var cmd *exec.Cmd

	switch {
	case runtime.GOOS == "windows":
		// Windows: use rundll32 to open the file with the default handler (preferred over "start" to avoid issues with empty strings and console flashes in cross-platform builds)
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", configPath)
	case fileExists("/usr/bin/open"):
		// macOS: use open command
		cmd = exec.Command("open", configPath)
	default:
		// Linux: use xdg-open
		cmd = exec.Command("xdg-open", configPath)
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Error opening config file: %v (location: %s)", err, configPath)
	} else {
		log.Printf("Opened config file: %s", configPath)
	}
}

func (app *App) openLogFile() {
	// Get log file path (same logic as setupLogging)
	var logDir string

	switch {
	case runtime.GOOS == "windows":
		logDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "FrictionlessLauncher")
	case fileExists("/Users"):
		// macOS
		home, _ := os.UserHomeDir()
		logDir = filepath.Join(home, "Library", "Application Support", "FrictionlessLauncher")
	default:
		// Linux
		home, _ := os.UserHomeDir()
		logDir = filepath.Join(home, ".config", "FrictionlessLauncher")
	}

	logPath := filepath.Join(logDir, "frictionless-launcher.log")

	// Check if log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		log.Printf("Log file not found: %s", logPath)
		return
	}

	// Open with default program based on OS
	var cmd *exec.Cmd

	switch {
	case runtime.GOOS == "windows":
		// Windows: use start command (brief console flash is unavoidable in cross-platform builds)
		// Windows: use rundll32 to open the log file with the default handler (brief console flash is unavoidable in cross-platform builds)
		// Windows: use rundll32 to open the log file with the default handler (brief console flash is unavoidable in cross-platform builds)
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", logPath)
	case fileExists("/usr/bin/open"):
		// macOS: use open command
		cmd = exec.Command("open", logPath)
	default:
		// Linux: use xdg-open
		cmd = exec.Command("xdg-open", logPath)
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Error opening log file: %v (location: %s)", err, logPath)
	} else {
		log.Printf("Opened log file: %s", logPath)
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
		log.Printf("Warning: Could not create config directory %s: %v", configDir, err)
		// Fall back to local directory
		return localConfig
	}

	return filepath.Join(configDir, "config.yaml")
}

func (app *App) setupLogging() {
	// Get log directory (same logic as config directory)
	var logDir string

	switch {
	case runtime.GOOS == "windows":
		logDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "FrictionlessLauncher")
	case fileExists("/Users"):
		// macOS
		home, _ := os.UserHomeDir()
		logDir = filepath.Join(home, "Library", "Application Support", "FrictionlessLauncher")
	default:
		// Linux
		home, _ := os.UserHomeDir()
		logDir = filepath.Join(home, ".config", "FrictionlessLauncher")
	}

	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		// If we can't create log directory, just use default logger (stderr)
		log.Printf("Warning: Could not create log directory %s: %v", logDir, err)
		return
	}

	// Create log file
	logFilePath := filepath.Join(logDir, "frictionless-launcher.log")
	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Warning: Could not open log file %s: %v", logFilePath, err)
		return
	}

	// Store file handle in app for proper cleanup
	app.logFile = file

	// Clean up old log files before setting up new logging
	cleanupOldLogs(logDir)

	// Set log output to file with timestamp
	log.SetOutput(file)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("=== Frictionless Launcher started ===")
}

func cleanupOldLogs(logDir string) {
	// Find all log files in the directory
	entries, err := os.ReadDir(logDir)
	if err != nil {
		// Directory doesn't exist or can't be read, nothing to clean up
		return
	}

	// Calculate cutoff time (1 week ago)
	oneWeekAgo := time.Now().AddDate(0, 0, -7)

	for _, entry := range entries {
		// Only process .log files
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		filePath := filepath.Join(logDir, entry.Name())

		// Get file info to check modification time
		info, err := entry.Info()
		if err != nil {
			continue // Skip files we can't get info for
		}

		// Delete files older than 1 week
		if info.ModTime().Before(oneWeekAgo) {
			if err := os.Remove(filePath); err != nil {
				// Don't log this error since logging isn't set up yet
				continue
			}
		}
	}
}

func (app *App) closeLogFile() {
	if app.logFile != nil {
		log.Printf("=== Frictionless Launcher shutting down ===")
		app.logFile.Close()
	}
}
