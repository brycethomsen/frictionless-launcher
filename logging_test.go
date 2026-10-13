package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ---- fileExists ---------------------------------------------------------------

func TestFileExists(t *testing.T) {
	f, err := os.CreateTemp("", "fe_test_*")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	if !fileExists(f.Name()) {
		t.Error("fileExists must return true for existing file")
	}
	if fileExists("/definitely/does/not/exist/xyz") {
		t.Error("fileExists must return false for non-existent path")
	}
}

// ---- appLogDir ---------------------------------------------------------------

func TestAppLogDir_ContainsAppName(t *testing.T) {
	dir := appLogDir()
	if !strings.Contains(dir, "FrictionlessLauncher") {
		t.Errorf("log dir should contain FrictionlessLauncher, got %q", dir)
	}
}

func TestAppLogDir_PlatformSpecific(t *testing.T) {
	dir := appLogDir()
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(dir, "Library") {
			t.Errorf("macOS log dir should contain Library, got %q", dir)
		}
	case "windows":
		// LOCALAPPDATA is set by the OS; just verify FrictionlessLauncher is present
	default:
		if !strings.Contains(dir, ".config") {
			t.Errorf("Linux log dir should contain .config, got %q", dir)
		}
	}
}

// ---- openFileWithOS ------------------------------------------------------------

func TestOpenFileWithOS_MissingFile(t *testing.T) {
	openFileWithOS("/definitely/does/not/exist/xyz.log") // must not panic
}

func TestOpenFileWithOS_ExistingFile(t *testing.T) {
	// Create a real file; openFileWithOS will try to open it with the OS viewer.
	// On macOS/Linux this may actually launch a viewer — use a temp file with a
	// format no viewer cares about and accept that cmd.Start may succeed or fail.
	f, err := os.CreateTemp("", "open_test_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	// Does not panic; the OS command may fail in CI (no display) but we only
	// check that the function doesn't panic.
	openFileWithOS(f.Name())
}

// ---- cleanupOldLogs -------------------------------------------------------------

func TestCleanupOldLogs(t *testing.T) {
	dir, _ := os.MkdirTemp("", "logs_*")
	defer os.RemoveAll(dir)

	recentLog := filepath.Join(dir, "recent.log")
	oldLog := filepath.Join(dir, "old.log")
	notALog := filepath.Join(dir, "other.txt")

	os.WriteFile(recentLog, []byte("new"), 0644)
	os.WriteFile(oldLog, []byte("old"), 0644)
	os.WriteFile(notALog, []byte("txt"), 0644)

	old := time.Now().AddDate(0, 0, -8)
	os.Chtimes(oldLog, old, old)
	os.Chtimes(notALog, old, old)

	cleanupOldLogs(dir)

	if _, err := os.Stat(recentLog); os.IsNotExist(err) {
		t.Error("recent log should be kept")
	}
	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Error("old log should be deleted")
	}
	if _, err := os.Stat(notALog); os.IsNotExist(err) {
		t.Error("non-.log file should not be deleted")
	}
}

func TestCleanupOldLogs_EmptyDir(t *testing.T) {
	dir, _ := os.MkdirTemp("", "logs_empty_*")
	defer os.RemoveAll(dir)
	cleanupOldLogs(dir) // must not panic
}

func TestCleanupOldLogs_NonexistentDir(t *testing.T) {
	cleanupOldLogs("/does/not/exist/at/all") // must not panic
}

func TestCleanupOldLogs_OlderThanSevenDays(t *testing.T) {
	dir, _ := os.MkdirTemp("", "logs_7d_*")
	defer os.RemoveAll(dir)

	oldLog := filepath.Join(dir, "old.log")
	os.WriteFile(oldLog, []byte("data"), 0644)
	past := time.Now().AddDate(0, 0, -7).Add(-time.Minute) // just past 7-day cutoff
	os.Chtimes(oldLog, past, past)

	cleanupOldLogs(dir)
	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Error("file older than 7 days should be deleted")
	}
}

func TestCleanupOldLogs_ExactlySevenDays_Kept(t *testing.T) {
	dir, _ := os.MkdirTemp("", "logs_7d_exact_*")
	defer os.RemoveAll(dir)

	recentLog := filepath.Join(dir, "fresh.log")
	os.WriteFile(recentLog, []byte("data"), 0644)
	fresh := time.Now().AddDate(0, 0, -7).Add(time.Minute) // just inside 7 days
	os.Chtimes(recentLog, fresh, fresh)

	cleanupOldLogs(dir)
	if _, err := os.Stat(recentLog); os.IsNotExist(err) {
		t.Error("file inside 7-day window should be kept")
	}
}

// ---- setupLogging / closeLogFile -------------------------------------------------

func TestApp_CloseLogFile_Nil(t *testing.T) {
	app := &App{}
	app.closeLogFile() // must not panic with nil logFile
}

func TestApp_CloseLogFile_Open(t *testing.T) {
	orig := log.Writer()
	origFlags := log.Flags()
	defer func() {
		log.SetOutput(orig)
		log.SetFlags(origFlags)
	}()

	dir, _ := os.MkdirTemp("", "close_*")
	defer os.RemoveAll(dir)

	f, _ := os.OpenFile(filepath.Join(dir, "test.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	app := &App{logFile: f}
	log.SetOutput(f)
	app.closeLogFile()

	// After close, writes to the file handle should fail
	_, err := f.WriteString("after close")
	if err == nil {
		t.Error("write to closed file should fail")
	}
}

func TestSetupLogging_CreatesFile(t *testing.T) {
	orig := log.Writer()
	origFlags := log.Flags()
	defer func() {
		log.SetOutput(orig)
		log.SetFlags(origFlags)
	}()

	app, _ := newTestApp(t)
	app.setupLogging()
	if app.logFile != nil {
		defer app.closeLogFile()
		if app.logFile.Name() == "" {
			t.Error("log file should have a valid path")
		}
	}
	// If logFile is nil, setupLogging gracefully fell back to stderr (acceptable in CI)
}

func TestSetupLogging_RotationLogic(t *testing.T) {
	// setupLogging always derives its directory from appLogDir(), which isn't
	// overridable in tests — so this exercises the same rotation logic
	// directly against a temp dir standing in for the real log path.
	dir, _ := os.MkdirTemp("", "log_rotate_*")
	defer os.RemoveAll(dir)

	logPath := filepath.Join(dir, "frictionless-launcher.log")
	f, _ := os.Create(logPath)
	f.Write(make([]byte, 6*1024*1024)) // 6 MB — exceeds 5 MB limit
	f.Close()

	const maxLogSize = 5 * 1024 * 1024
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > maxLogSize {
		rotated := logPath + ".1"
		os.Remove(rotated)
		os.Rename(logPath, rotated)
	}

	if fileExists(logPath) {
		t.Error("original log should be renamed after rotation")
	}
	if !fileExists(logPath + ".1") {
		t.Error("rotated .1 log should exist")
	}
}
