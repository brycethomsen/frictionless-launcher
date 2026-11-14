package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

// DataDir returns the per-user directory holding config.yaml and the log
// file: %LOCALAPPDATA%\FrictionlessLauncher on Windows,
// ~/Library/Application Support/FrictionlessLauncher on macOS, and
// ~/.config/FrictionlessLauncher elsewhere. It does not create it.
func DataDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "FrictionlessLauncher")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "FrictionlessLauncher")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "FrictionlessLauncher")
	}
}
