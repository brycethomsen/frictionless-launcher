package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/systray"
	"github.com/fsnotify/fsnotify"
	"github.com/shirou/gopsutil/v4/process"
	"gopkg.in/yaml.v3"
)

//go:embed icon.ico
var iconData []byte

type Schedule struct {
	Days      []string `yaml:"days"`       // e.g., ["Mon", "Tue", "Wed"]
	StartTime string   `yaml:"start_time"` // e.g., "19:00"
	EndTime   string   `yaml:"end_time"`   // e.g., "21:00"
}

type Game struct {
	GameName     string     `yaml:"game_name"`
	GamePath     string     `yaml:"game_path"`
	LaunchMethod string     `yaml:"launch_method"` // "steam", "gog", "epic", "direct"
	LaunchArgs   string     `yaml:"launch_args"`
	Schedules    []Schedule `yaml:"schedules"`
	Enabled      bool       `yaml:"enabled"`
}

type Config struct {
	Games     []Game `yaml:"games"`
	BootDelay int    `yaml:"boot_delay"`

	// Legacy fields for backwards compatibility
	GamePath   string `yaml:"game_path,omitempty"`
	GameName   string `yaml:"game_name,omitempty"`
	LaunchArgs string `yaml:"launch_args,omitempty"`
	Enabled    bool   `yaml:"enabled,omitempty"`
	Schedule   string `yaml:"schedule,omitempty"`
}

type App struct {
	config         *Config
	configPath     string
	launchPending  bool
	shouldCancel   int32              // Atomic flag: 1 = cancel, 0 = continue
	logFile        *os.File           // Log file handle for proper cleanup
	lastLaunchTime map[string]time.Time // Track last launch time per game name
}

func main() {
	app := &App{
		configPath:     getConfigPath(),
		launchPending:  false,
		lastLaunchTime: make(map[string]time.Time),
	}

	// Set up logging to file and store handle in app
	app.setupLogging()
	defer app.closeLogFile() // Ensure log file is closed on exit

	app.loadConfig()

	// Check each game's schedule at boot
	for _, game := range app.config.Games {
		if game.Enabled && app.shouldLaunchGame(game) {
			app.launchPending = true
			go app.autoLaunchGameByName(game)
			break // Only launch one game at boot
		}
	}

	// Start continuous schedule monitoring
	go app.scheduleMonitor()

	// Start config file watcher for hot-reload
	go app.watchConfigFile()

	// Start system tray
	systray.Run(app.onReady, app.onExit)
}

func (app *App) onReady() {
	app.updateTrayIcon()

	// Use embedded .ico file
	systray.SetIcon(iconData)

	// Display games as clickable menu items
	var gameMenuItems []*systray.MenuItem
	if len(app.config.Games) > 0 {
		for _, game := range app.config.Games {
			scheduleInfo := app.getGameScheduleInfo(game)
			gameItem := systray.AddMenuItem(game.GameName, scheduleInfo)
			gameMenuItems = append(gameMenuItems, gameItem)
		}
	} else {
		noGames := systray.AddMenuItem("No games configured", "Add games in config.yaml")
		noGames.Disable()
	}

	systray.AddSeparator()

	editConfig := systray.AddMenuItem("Edit Config", "Open config.yaml in default editor")
	openLogs := systray.AddMenuItem("View Logs", "Open log file in default editor")
	quit := systray.AddMenuItem("Exit", "Exit Frictionless Launcher")

	// Handle menu clicks
	go func() {
		for {
			select {
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

	// Handle game launches
	for i, gameItem := range gameMenuItems {
		go func(idx int, item *systray.MenuItem) {
			for range item.ClickedCh {
				if idx < len(app.config.Games) {
					game := app.config.Games[idx]
					log.Printf("User clicked to launch: %s", game.GameName)
					go app.launchGameByStruct(game)
				}
			}
		}(i, gameItem)
	}
}

func (app *App) onExit() {
	// Cleanup if needed
}

func (app *App) loadConfig() {
	// Set defaults based on platform
	app.config = &Config{
		BootDelay: 10,
		Games: []Game{
			{
				GameName:     "Stardew Valley",
				GamePath:     "steam://rungameid/413150",
				LaunchMethod: "steam",
				LaunchArgs:   "",
				Schedules: []Schedule{
					{
						Days:      []string{"Thu"},
						StartTime: "19:00",
						EndTime:   "21:00",
					},
				},
				Enabled: true,
			},
		},
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

	// Migrate legacy single-game config to new format if needed
	if app.config.GamePath != "" && len(app.config.Games) == 0 {
		log.Println("Migrating legacy config format to new multi-game format")
		legacyGame := Game{
			GameName:     app.config.GameName,
			GamePath:     app.config.GamePath,
			LaunchMethod: "direct",
			LaunchArgs:   app.config.LaunchArgs,
			Enabled:      app.config.Enabled,
			Schedules:    []Schedule{},
		}

		// Convert old schedule string to new format
		if app.config.Schedule == "always" {
			legacyGame.Schedules = []Schedule{
				{
					Days:      []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"},
					StartTime: "00:00",
					EndTime:   "23:59",
				},
			}
		}

		app.config.Games = []Game{legacyGame}
		// Clear legacy fields
		app.config.GamePath = ""
		app.config.GameName = ""
		app.config.LaunchArgs = ""
		app.config.Schedule = ""
		app.saveConfig()
	}

	log.Printf("Loaded config with %d game(s)", len(app.config.Games))
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

func (app *App) getGameScheduleInfo(game Game) string {
	if !game.Enabled {
		return "Disabled"
	}

	if len(game.Schedules) == 0 {
		return "No schedule configured"
	}

	// Show first schedule as summary
	schedule := game.Schedules[0]
	daysStr := strings.Join(schedule.Days, ", ")
	return "Schedule: " + daysStr + " " + schedule.StartTime + "-" + schedule.EndTime
}

func (app *App) shouldLaunchGame(game Game) bool {
	// Don't launch if a game is already running
	if app.isGameRunning() {
		return false
	}

	// Don't launch if we already launched this game in the current window
	if app.hasLaunchedInCurrentWindow(game) {
		return false
	}

	now := time.Now()
	currentTime := now.Format("15:04")
	currentDay := now.Weekday().String()[:3] // "Mon", "Tue", "Wed", etc.

	for _, schedule := range game.Schedules {
		// Check if current day matches any scheduled day
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

		// Check if current time is within the scheduled window
		if currentTime >= schedule.StartTime && currentTime <= schedule.EndTime {
			return true
		}
	}

	return false
}

func (app *App) isInScheduleWindow(game Game) bool {
	now := time.Now()
	currentTime := now.Format("15:04")
	currentDay := now.Weekday().String()[:3]

	for _, schedule := range game.Schedules {
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

		if currentTime >= schedule.StartTime && currentTime <= schedule.EndTime {
			return true
		}
	}

	return false
}

func (app *App) getForegroundAppName() (string, error) {
	processes, err := process.Processes()
	if err != nil {
		return "", err
	}

	for _, proc := range processes {
		isFg, err := proc.Foreground()
		if err != nil {
			continue
		}

		if isFg {
			name, err := proc.Name()
			if err != nil {
				continue
			}
			return name, nil
		}
	}

	return "", nil
}

func (app *App) watchConfigFile() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Error creating file watcher: %v", err)
		return
	}
	defer watcher.Close()

	// Watch the config file
	if err := watcher.Add(app.configPath); err != nil {
		log.Printf("Error watching config file: %v", err)
		return
	}

	log.Printf("Watching config file for changes: %s", app.configPath)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// Only care about write events
			if event.Op&fsnotify.Write == fsnotify.Write {
				log.Printf("Config file changed, reloading...")
				app.loadConfig()
				log.Println("Config reloaded successfully")
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

func (app *App) scheduleMonitor() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	var lastChecked time.Time

	for {
		select {
		case <-ticker.C:
			now := time.Now()

			// Skip if we already checked this minute
			if now.Minute() == lastChecked.Minute() && now.Hour() == lastChecked.Hour() {
				continue
			}
			lastChecked = now

			// Check each enabled game's schedule
			for _, game := range app.config.Games {
				if !game.Enabled {
					continue
				}

				// Check if we're in the schedule window
				if app.isInScheduleWindow(game) {
					// If a game is already running, skip
					if app.isGameRunning() {
						log.Printf("Game already running, skipping launch")
						continue
					}

					// If we already launched in this window, skip
					if app.hasLaunchedInCurrentWindow(game) {
						continue
					}

					// Check if something is in the foreground
					fgApp, _ := app.getForegroundAppName()
					if fgApp != "" {
						log.Printf("App in foreground (%s), showing notification for %s", fgApp, game.GameName)
						systray.SetTooltip(fmt.Sprintf("Time to play %s! (%s is in foreground)", game.GameName, fgApp))
						// Don't launch, but don't skip either - user can click launch manually
						continue
					}

					// All clear - launch the game
					log.Printf("Schedule triggered for %s", game.GameName)
					go app.autoLaunchGameByName(game)
					break // Only launch one game per check cycle
				}
			}
		}
	}
}

func (app *App) autoLaunchGameByName(game Game) {
	defer func() {
		app.launchPending = false
		atomic.StoreInt32(&app.shouldCancel, 0) // Reset flag
	}()

	log.Printf("Auto-launching %s in %d seconds", game.GameName, app.config.BootDelay)

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
	app.launchGameByStruct(game)
}

func (app *App) launchGame() {
	// Launch the first enabled game (for backwards compatibility with UI)
	for _, game := range app.config.Games {
		if game.Enabled {
			app.launchGameByStruct(game)
			return
		}
	}
	log.Println("No enabled games configured")
}

func (app *App) launchGameByStruct(game Game) {
	if game.GamePath == "" {
		log.Println("No game path configured")
		return
	}

	log.Printf("Launching %s via %s", game.GameName, game.LaunchMethod)

	var cmd *exec.Cmd

	switch game.LaunchMethod {
	case "steam":
		// Launch via Steam protocol handler (keeps cloud saves)
		// Format: steam://rungameid/APPID
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("open", "-g", game.GamePath)
		} else if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/c", "start", game.GamePath)
		} else {
			cmd = exec.Command("xdg-open", game.GamePath)
		}

	case "epic":
		// Launch via Epic Games protocol handler (keeps cloud saves)
		// Format: com.epicgames.launcher://apps/APPID/launch
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("open", "-g", game.GamePath)
		} else if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/c", "start", game.GamePath)
		} else {
			cmd = exec.Command("xdg-open", game.GamePath)
		}

	case "direct":
		// Direct executable launch (no cloud saves)
		var args []string
		if game.LaunchArgs != "" {
			args = strings.Fields(game.LaunchArgs)
		}
		cmd = exec.Command(game.GamePath, args...)

	default:
		log.Printf("Unknown launch method: %s, defaulting to direct", game.LaunchMethod)
		var args []string
		if game.LaunchArgs != "" {
			args = strings.Fields(game.LaunchArgs)
		}
		cmd = exec.Command(game.GamePath, args...)
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Error launching game: %v", err)
		return
	}

	app.recordLaunch(game)
	log.Printf("%s launched successfully", game.GameName)
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
	activeGames := 0
	for _, game := range app.config.Games {
		if game.Enabled && app.shouldLaunchGame(game) {
			activeGames++
		}
	}

	if activeGames > 0 {
		tooltip += "Active (in schedule)"
	} else if len(app.config.Games) > 0 {
		tooltip += "Active (outside schedule)"
	} else {
		tooltip += "No games configured"
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
	activeGames := 0
	for _, game := range app.config.Games {
		if game.Enabled && app.shouldLaunchGame(game) {
			activeGames++
		}
	}

	if activeGames > 0 {
		return "Active (in schedule)"
	} else if len(app.config.Games) > 0 {
		return "Active (outside schedule)"
	} else {
		return "No games configured"
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

func (app *App) isGameRunning() bool {
	processes, err := process.Processes()
	if err != nil {
		log.Printf("Error checking processes: %v", err)
		return false
	}

	// Check if any configured game's executable is running
	for _, game := range app.config.Games {
		for _, proc := range processes {
			// Try to match by executable name
			name, err := proc.Name()
			if err != nil {
				continue
			}

			exeName := filepath.Base(game.GamePath)
			if strings.EqualFold(name, exeName) {
				log.Printf("Game process found: %s", name)
				return true
			}

			// Also try full path match for direct executables
			exe, err := proc.Exe()
			if err != nil {
				continue
			}

			if strings.EqualFold(exe, game.GamePath) {
				log.Printf("Game process found: %s", exe)
				return true
			}
		}
	}

	return false
}

func (app *App) hasLaunchedInCurrentWindow(game Game) bool {
	lastLaunch, exists := app.lastLaunchTime[game.GameName]
	if !exists {
		return false
	}

	// Check if last launch is within any current schedule window
	now := time.Now()
	for _, schedule := range game.Schedules {
		dayMatches := false
		for _, day := range schedule.Days {
			if strings.EqualFold(day, now.Weekday().String()[:3]) {
				dayMatches = true
				break
			}
		}

		if !dayMatches {
			continue
		}

		// Parse times
		startTime, _ := time.Parse("15:04", schedule.StartTime)
		endTime, _ := time.Parse("15:04", schedule.EndTime)

		// Adjust times to today's date for comparison
		startTime = time.Date(now.Year(), now.Month(), now.Day(), startTime.Hour(), startTime.Minute(), 0, 0, now.Location())
		endTime = time.Date(now.Year(), now.Month(), now.Day(), endTime.Hour(), endTime.Minute(), 0, 0, now.Location())

		// If last launch is after this window's start time, we already launched in this window
		if lastLaunch.After(startTime) && lastLaunch.Before(endTime.Add(time.Minute)) {
			return true
		}
	}

	return false
}

func (app *App) recordLaunch(game Game) {
	app.lastLaunchTime[game.GameName] = time.Now()
}

func (app *App) closeLogFile() {
	if app.logFile != nil {
		log.Printf("=== Frictionless Launcher shutting down ===")
		app.logFile.Close()
	}
}
