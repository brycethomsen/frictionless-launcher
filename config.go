package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"

	"frictionless-launcher/internal/config"
)

func (app *App) loadConfig() {
	if app.config == nil {
		// First load of the process: there's no previous in-memory config to
		// fall back to, so an empty default is the best we can do.
		app.config = &config.Config{BootDelay: 10}
	}

	if _, err := os.Stat(app.configPath); os.IsNotExist(err) {
		log.Println("No config found, creating empty config.yaml")
		app.saveConfig()
		return
	}

	data, err := os.ReadFile(app.configPath)
	if err != nil {
		log.Printf("Error reading config: %v", err)
		log.Println("Keeping previously loaded config due to read error")
		return
	}

	// Unmarshal into a fresh struct rather than app.config directly, and only
	// publish it once fully parsed. Reloads are triggered by our own saves
	// (fsnotify fires on every write, including ones we make), so a reload can
	// race a concurrent write; if the read or parse fails we must leave the
	// previous good config in place instead of swapping in a blank one.
	loaded := config.Config{BootDelay: 10}
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		log.Printf("Error parsing config: %v", err)
		log.Println("Config file has invalid YAML, keeping previously loaded config - please check your config.yaml file for syntax errors")
		return
	}
	app.config = &loaded

	// Migrate legacy single-game config to new format if needed
	if config.MigrateLegacy(app.config) {
		log.Println("Migrating legacy config format to new multi-game format")
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

// getConfigPath returns the config file path to use: a config.yaml next to
// executablePath if one exists there (portable/dev installs), otherwise an
// OS-appropriate app-support location.
func getConfigPath(executablePath string) string {
	// Try local directory first (for development/portable installs)
	localDir := filepath.Dir(executablePath)
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
