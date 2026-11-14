// Package logging sends the standard logger to the app's log file, which the
// tray and UI processes share.
package logging

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"frictionless-launcher/internal/platform"
)

const (
	fileName      = "frictionless-launcher.log"
	maxLogSize    = 5 * 1024 * 1024 // rotate past 5 MB
	maxLogAgeDays = 7               // delete .log files older than this
)

// Dir returns the directory holding the log file (shared with config.yaml).
func Dir() string { return platform.DataDir() }

// FilePath returns the path of the log file.
func FilePath() string { return filepath.Join(Dir(), fileName) }

// Setup points the standard logger at the log file and returns a function
// that closes it. If the file can't be opened, logging stays on stderr.
//
// Only the long-lived tray process should pass housekeeping=true (rotate an
// oversized log, delete week-old ones): a --manage/--countdown child shares
// the file with it, and renaming a file another process holds open fails on
// Windows anyway.
func Setup(housekeeping bool) (closeLog func()) {
	return setupIn(Dir(), housekeeping)
}

func setupIn(dir string, housekeeping bool) func() {
	noop := func() {}
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Warning: Could not create log directory %s: %v", dir, err)
		return noop
	}

	path := filepath.Join(dir, fileName)
	if housekeeping {
		rotate(path)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Warning: Could not open log file %s: %v", path, err)
		return noop
	}
	if housekeeping {
		cleanupOld(dir)
	}

	log.SetOutput(file)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("=== Frictionless Launcher started ===")

	closed := false
	return func() {
		if closed {
			return
		}
		closed = true
		log.Printf("=== Frictionless Launcher shutting down ===")
		log.SetOutput(os.Stderr)
		file.Close()
	}
}

// rotate renames path to path.1 if it has grown past maxLogSize.
func rotate(path string) {
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogSize {
		rotated := path + ".1"
		os.Remove(rotated)
		os.Rename(path, rotated)
	}
}

// cleanupOld deletes .log files in dir last modified more than maxLogAgeDays
// calendar days ago.
func cleanupOld(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -maxLogAgeDays)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, entry.Name())) // errors ignored: best effort
		}
	}
}
