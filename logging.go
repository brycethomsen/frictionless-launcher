package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

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

func (app *App) closeLogFile() {
	if app.logFile != nil {
		log.Printf("=== Frictionless Launcher shutting down ===")
		app.logFile.Close()
	}
}
