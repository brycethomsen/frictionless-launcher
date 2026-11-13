package platform

import (
	"log"
	"os"
	"os/exec"
	"runtime"
)

// OpenWithDefaultApp opens path with the OS's default program for it (e.g.
// a text editor for the log file). It logs and returns if path is missing.
func OpenWithDefaultApp(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Printf("File not found: %s", path)
		return
	}

	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	case runtime.GOOS == "darwin":
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
