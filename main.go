package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/fsnotify/fsnotify"
	"github.com/shirou/gopsutil/v4/process"
	"gopkg.in/yaml.v3"
)

//go:embed icon.png
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
	config             *Config
	configPath         string
	logFile            *os.File
	lastLaunchTime     map[string]time.Time
	ui                 *GameManagerUI
	desk               desktop.App
	cancelLaunch       func()
	pendingGameName    string
	pendingSecondsLeft int
}

func main() {
	a := &App{
		configPath:     getConfigPath(),
		lastLaunchTime: make(map[string]time.Time),
	}

	a.setupLogging()
	defer a.closeLogFile()

	a.loadConfig()
	log.Printf("Config path: %s", a.configPath)
	a.ui = newGameManagerUI(a)

	go a.scheduleMonitor()
	go a.watchConfigFile()

	setupDockBehavior(a.ui.fyneApp, func() {
		bootLaunched := false
		for _, game := range a.config.Games {
			if game.Enabled && a.shouldLaunchGame(game) {
				log.Printf("Boot within schedule window for %s — queuing auto-launch", game.GameName)
				go a.autoLaunchGameByName(game)
				bootLaunched = true
				break
			}
		}
		if !bootLaunched {
			log.Println("No games in schedule window at boot")
		}
	})

	iconRes := fyne.NewStaticResource("icon.png", iconData)
	a.ui.fyneApp.SetIcon(iconRes)

	if desk, ok := a.ui.fyneApp.(desktop.App); ok {
		a.desk = desk
		desk.SetSystemTrayIcon(iconRes)
		desk.SetSystemTrayMenu(a.buildTrayMenu(desk))
	}

	a.ui.fyneApp.Run()
	a.closeLogFile()
}

func (app *App) buildTrayMenu(desk desktop.App) *fyne.Menu {
	quitItem := fyne.NewMenuItem("Quit", func() { app.ui.fyneApp.Quit() })
	quitItem.IsQuit = true

	items := []*fyne.MenuItem{}

	if app.cancelLaunch != nil {
		label := fmt.Sprintf("⏳ %s launching in %ds... — Cancel", app.pendingGameName, app.pendingSecondsLeft)
		cancelItem := fyne.NewMenuItem(label, func() {
			if app.cancelLaunch != nil {
				app.cancelLaunch()
			}
		})
		items = append(items, cancelItem)
	} else {
		upcoming := app.nextScheduledGames(3)
		if len(upcoming) == 0 {
			noGames := fyne.NewMenuItem("No games scheduled", nil)
			noGames.Disabled = true
			items = append(items, noGames)
		} else {
			for _, g := range upcoming {
				game := g
				label := fmt.Sprintf("%s — %s", game.GameName, app.nextScheduleLabel(game))
				items = append(items, fyne.NewMenuItem(label, func() {
					go app.launchGameByStruct(game)
				}))
			}
		}
	}

	items = append(items,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Manage Games...", func() { app.ui.show() }),
		fyne.NewMenuItem("View Logs", func() { app.openLogFile() }),
		fyne.NewMenuItemSeparator(),
		quitItem,
	)

	return fyne.NewMenu("Frictionless", items...)
}

// nextScheduledGames returns up to n enabled games sorted by their next upcoming schedule time.
func (app *App) nextScheduledGames(n int) []Game {
	now := time.Now()
	type candidate struct {
		game Game
		next time.Time
	}
	var candidates []candidate
	for _, game := range app.config.Games {
		if !game.Enabled {
			continue
		}
		if t, ok := app.nextScheduleTime(game, now); ok {
			candidates = append(candidates, candidate{game, t})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].next.Before(candidates[j].next)
	})
	if len(candidates) > n {
		candidates = candidates[:n]
	}
	result := make([]Game, len(candidates))
	for i, c := range candidates {
		result[i] = c.game
	}
	return result
}

// nextScheduleTime returns the next time a game's schedule will start, within the next 7 days.
func (app *App) nextScheduleTime(game Game, from time.Time) (time.Time, bool) {
	for daysAhead := 0; daysAhead <= 7; daysAhead++ {
		day := from.AddDate(0, 0, daysAhead)
		dayName := day.Weekday().String()[:3]
		var earliest time.Time
		found := false
		for _, s := range game.Schedules {
			for _, d := range s.Days {
				if !strings.EqualFold(d, dayName) {
					continue
				}
				startH, startM := 0, 0
				fmt.Sscanf(s.StartTime, "%d:%d", &startH, &startM)
				candidate := time.Date(day.Year(), day.Month(), day.Day(), startH, startM, 0, 0, day.Location())
				if !candidate.After(from) {
					continue
				}
				if !found || candidate.Before(earliest) {
					earliest = candidate
					found = true
				}
			}
		}
		if found {
			return earliest, true
		}
	}
	return time.Time{}, false
}

// nextScheduleLabel returns a human-readable label for the next schedule, e.g. "Thu 19:00".
func (app *App) nextScheduleLabel(game Game) string {
	return app.nextScheduleLabelAt(game, time.Now())
}

func (app *App) nextScheduleLabelAt(game Game, now time.Time) string {
	t, ok := app.nextScheduleTime(game, now)
	if !ok {
		return "unscheduled"
	}
	if t.Before(now.Add(24 * time.Hour)) {
		return "Today " + t.Format("15:04")
	}
	return t.Format("Mon 15:04")
}

func (app *App) loadConfig() {
	app.config = &Config{
		BootDelay: 10,
	}

	if _, err := os.Stat(app.configPath); os.IsNotExist(err) {
		log.Println("No config found, creating empty config.yaml")
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
	app.warnScheduleOverlaps()
}

func (app *App) warnScheduleOverlaps() {
	for i := 0; i < len(app.config.Games); i++ {
		for _, si := range app.config.Games[i].Schedules {
			for _, di := range si.Days {
				for j := i + 1; j < len(app.config.Games); j++ {
					for _, sj := range app.config.Games[j].Schedules {
						for _, dj := range sj.Days {
							if strings.EqualFold(di, dj) && si.StartTime < sj.EndTime && sj.StartTime < si.EndTime {
								log.Printf("WARNING: schedule overlap between %q and %q on %s (%s-%s vs %s-%s)",
									app.config.Games[i].GameName, app.config.Games[j].GameName,
									di, si.StartTime, si.EndTime, sj.StartTime, sj.EndTime)
							}
						}
					}
				}
			}
		}
	}
}

func (app *App) saveConfig() {
	data, err := yaml.Marshal(app.config)
	if err != nil {
		log.Printf("Error marshaling config: %v", err)
		return
	}

	if err := os.WriteFile(app.configPath, data, 0644); err != nil {
		log.Printf("Error saving config: %v", err)
		return
	}

	app.refreshTrayMenu()
}

func fadedIcon(src []byte, alpha uint8) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	out := image.NewNRGBA(bounds)
	draw.Draw(out, bounds, img, bounds.Min, draw.Src)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := out.NRGBAAt(x, y)
			out.SetNRGBA(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: uint8(uint16(c.A) * uint16(alpha) / 255)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (app *App) startIconPulse(stop <-chan struct{}) {
	if app.desk == nil {
		return
	}
	dimData, err := fadedIcon(iconData, 80)
	if err != nil {
		return
	}
	normalRes := fyne.NewStaticResource("icon.png", iconData)
	dimRes := fyne.NewStaticResource("icon-dim.png", dimData)

	go func() {
		ticker := time.NewTicker(600 * time.Millisecond)
		defer ticker.Stop()
		dim := false
		for {
			select {
			case <-stop:
				fyne.Do(func() { app.desk.SetSystemTrayIcon(normalRes) })
				return
			case <-ticker.C:
				dim = !dim
				res := normalRes
				if dim {
					res = dimRes
				}
				r := res
				fyne.Do(func() { app.desk.SetSystemTrayIcon(r) })
			}
		}
	}()
}

func (app *App) refreshTrayMenu() {
	if app.desk == nil {
		return
	}
	fyne.Do(func() {
		app.desk.SetSystemTrayMenu(app.buildTrayMenu(app.desk))
	})
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
	return app.shouldLaunchGameAt(game, time.Now())
}

func (app *App) shouldLaunchGameAt(game Game, now time.Time) bool {
	if app.isGameRunning() {
		return false
	}
	if app.hasLaunchedInCurrentWindowAt(game, now) {
		return false
	}
	return app.isInScheduleWindowAt(game, now)
}

func (app *App) isInScheduleWindow(game Game) bool {
	return app.isInScheduleWindowAt(game, time.Now())
}

func (app *App) isInScheduleWindowAt(game Game, now time.Time) bool {
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
	name := getFrontmostApp()
	// Ignore our own app and empty results
	if name == "Frictionless" || name == "frictionless-launcher" {
		return "", nil
	}
	return name, nil
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
				app.refreshTrayMenu()
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
						log.Printf("Already launched %s in current window, skipping", game.GameName)
						continue
					}

					// All clear or foreground app — both go through countdown popup
					log.Printf("Schedule triggered for %s", game.GameName)
					go app.autoLaunchGameByName(game)
					break // Only launch one game per check cycle
				}
			}
		}
	}
}

func (app *App) autoLaunchGameByName(game Game) {
	fgApp, _ := app.getForegroundAppName()

	cancelled := make(chan struct{})
	app.pendingGameName = game.GameName
	app.pendingSecondsLeft = app.config.BootDelay
	app.cancelLaunch = func() {
		select {
		case <-cancelled:
		default:
			close(cancelled)
		}
	}
	app.refreshTrayMenu()
	app.startIconPulse(cancelled)

	// Tick down the tray label every second
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-cancelled:
				return
			case <-ticker.C:
				app.pendingSecondsLeft--
				if app.pendingSecondsLeft <= 0 {
					return
				}
				app.refreshTrayMenu()
			}
		}
	}()

	cleanup := func() {
		app.cancelLaunch = nil
		app.pendingGameName = ""
		app.pendingSecondsLeft = 0
		app.refreshTrayMenu()
	}

	if fgApp != "" {
		log.Printf("Foreground app detected (%s) — notifying, launching %s in %ds unless cancelled via tray", fgApp, game.GameName, app.config.BootDelay)
		sendNativeNotification("Frictionless", fmt.Sprintf("%s is launching in %d seconds — cancel from the menu bar if needed", game.GameName, app.config.BootDelay))

		select {
		case <-cancelled:
			log.Printf("Launch cancelled by user for %s — suppressing for remainder of schedule window", game.GameName)
			cleanup()
			app.recordLaunch(game)
			return
		case <-time.After(time.Duration(app.config.BootDelay) * time.Second):
		}

		cleanup()
		app.launchGameByStruct(game)
		return
	}

	log.Printf("Showing launch countdown for %s", game.GameName)
	sendNativeNotification("Frictionless", fmt.Sprintf("Launching %s in %d seconds", game.GameName, app.config.BootDelay))

	done := make(chan bool, 1)
	app.ui.showLaunchCountdown(game.GameName, app.config.BootDelay, func(launch bool) {
		done <- launch
	})

	// Also respect tray cancel for the countdown case
	select {
	case <-cancelled:
		log.Printf("Launch cancelled via tray for %s — suppressing for remainder of schedule window", game.GameName)
		cleanup()
		app.recordLaunch(game)
		return
	case launch := <-done:
		cleanup()
		if launch {
			app.launchGameByStruct(game)
		} else {
			log.Printf("Launch cancelled by user for %s — suppressing for remainder of schedule window", game.GameName)
			app.recordLaunch(game)
		}
	}
}

// buildLaunchCmd returns the exec.Cmd that would launch the given game on the
// current OS. It does not start the command.
func buildLaunchCmd(game Game, goos string) *exec.Cmd {
	switch game.LaunchMethod {
	case "steam", "epic":
		switch goos {
		case "darwin":
			return exec.Command("open", "-g", game.GamePath)
		case "windows":
			return exec.Command("cmd", "/c", "start", game.GamePath)
		default:
			return exec.Command("xdg-open", game.GamePath)
		}
	default: // "direct" and unknown methods
		var args []string
		if game.LaunchArgs != "" {
			args = strings.Fields(game.LaunchArgs)
		}
		return exec.Command(game.GamePath, args...)
	}
}

func (app *App) launchGameByStruct(game Game) {
	if game.GamePath == "" {
		log.Println("No game path configured")
		return
	}

	log.Printf("Launching %s via %s", game.GameName, game.LaunchMethod)

	switch game.LaunchMethod {
	case "steam":
		if !app.isPlatformRunning("steam") {
			log.Printf("Warning: Steam does not appear to be running — it will launch first, adding delay")
		}
	case "epic":
		if !app.isPlatformRunning("epic") {
			log.Printf("Warning: Epic Games Launcher does not appear to be running — it will launch first, adding delay")
		}
	}

	cmd := buildLaunchCmd(game, runtime.GOOS)
	if err := cmd.Start(); err != nil {
		log.Printf("Error launching game: %v", err)
		return
	}

	app.recordLaunch(game)
	log.Printf("%s launched successfully", game.GameName)
}

// appLogDir returns the platform-appropriate directory for log files.
func appLogDir() string {
	switch {
	case runtime.GOOS == "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "FrictionlessLauncher")
	case fileExists("/Users"):
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "FrictionlessLauncher")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "FrictionlessLauncher")
	}
}

func (app *App) openLogFile() {
	openFileWithOS(filepath.Join(appLogDir(), "frictionless-launcher.log"))
}

// openFileWithOS opens path with the OS default program.
// Returns early (with a log) if the file does not exist.
func openFileWithOS(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Printf("File not found: %s", path)
		return
	}

	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	case fileExists("/usr/bin/open"):
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Error opening file: %v (location: %s)", err, path)
	} else {
		log.Printf("Opened file: %s", path)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
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
	logDir := appLogDir()

	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		// If we can't create log directory, just use default logger (stderr)
		log.Printf("Warning: Could not create log directory %s: %v", logDir, err)
		return
	}

	// Create log file
	logFilePath := filepath.Join(logDir, "frictionless-launcher.log")
	// Rotate if log exceeds 5MB
	const maxLogSize = 5 * 1024 * 1024
	if info, err := os.Stat(logFilePath); err == nil && info.Size() > maxLogSize {
		rotated := logFilePath + ".1"
		os.Remove(rotated)
		os.Rename(logFilePath, rotated)
	}

	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Warning: Could not open log file %s: %v", logFilePath, err)
		return
	}

	app.logFile = file
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

func (app *App) isPlatformRunning(platform string) bool {
	var names []string
	switch platform {
	case "steam":
		names = []string{"steam", "steam.exe", "Steam"}
	case "epic":
		names = []string{"EpicGamesLauncher", "EpicGamesLauncher.exe"}
	default:
		return false
	}
	procs, err := process.Processes()
	if err != nil {
		return false
	}
	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			continue
		}
		for _, n := range names {
			if strings.EqualFold(name, n) {
				return true
			}
		}
	}
	return false
}

func (app *App) isGameRunning() bool {
	// For steam/epic, hasLaunchedInCurrentWindow prevents double-launches.
	// Only check processes for direct-launch games where we have a real executable path.
	var directGames []Game
	for _, game := range app.config.Games {
		if game.LaunchMethod == "direct" && game.GamePath != "" {
			directGames = append(directGames, game)
		}
	}
	if len(directGames) == 0 {
		return false
	}

	processes, err := process.Processes()
	if err != nil {
		log.Printf("Error checking processes: %v", err)
		return false
	}

	for _, game := range directGames {
		exeName := filepath.Base(game.GamePath)
		for _, proc := range processes {
			name, err := proc.Name()
			if err != nil {
				continue
			}
			if strings.EqualFold(name, exeName) {
				log.Printf("Game process found: %s", name)
				return true
			}
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
	return app.hasLaunchedInCurrentWindowAt(game, time.Now())
}

func (app *App) hasLaunchedInCurrentWindowAt(game Game, now time.Time) bool {
	lastLaunch, exists := app.lastLaunchTime[game.GameName]
	if !exists {
		return false
	}

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

		startTime, _ := time.Parse("15:04", schedule.StartTime)
		endTime, _ := time.Parse("15:04", schedule.EndTime)

		startTime = time.Date(now.Year(), now.Month(), now.Day(), startTime.Hour(), startTime.Minute(), 0, 0, now.Location())
		endTime = time.Date(now.Year(), now.Month(), now.Day(), endTime.Hour(), endTime.Minute(), 0, 0, now.Location())

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
