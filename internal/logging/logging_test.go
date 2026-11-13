package logging

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- Dir ---------------------------------------------------------------------

func TestDir_ContainsAppName(t *testing.T) {
	if dir := Dir(); !strings.Contains(dir, "FrictionlessLauncher") {
		t.Errorf("log dir should contain FrictionlessLauncher, got %q", dir)
	}
}

func TestFilePath_InDir(t *testing.T) {
	if got := FilePath(); filepath.Dir(got) != Dir() || filepath.Base(got) != fileName {
		t.Errorf("FilePath() = %q, want %s inside %s", got, fileName, Dir())
	}
}

// ---- cleanupOld -------------------------------------------------------------

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

	cleanupOld(dir)

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
	cleanupOld(dir) // must not panic
}

func TestCleanupOldLogs_NonexistentDir(t *testing.T) {
	cleanupOld("/does/not/exist/at/all") // must not panic
}

func TestCleanupOldLogs_OlderThanSevenDays(t *testing.T) {
	dir, _ := os.MkdirTemp("", "logs_7d_*")
	defer os.RemoveAll(dir)

	oldLog := filepath.Join(dir, "old.log")
	os.WriteFile(oldLog, []byte("data"), 0644)
	past := time.Now().AddDate(0, 0, -7).Add(-time.Minute) // just past 7-day cutoff
	os.Chtimes(oldLog, past, past)

	cleanupOld(dir)
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

	cleanupOld(dir)
	if _, err := os.Stat(recentLog); os.IsNotExist(err) {
		t.Error("file inside 7-day window should be kept")
	}
}

// ---- Setup / rotation -----------------------------------------------------------

// restoreLogger puts the standard logger back after a test redirects it.
func restoreLogger(t *testing.T) {
	t.Helper()
	orig, origFlags := log.Writer(), log.Flags()
	t.Cleanup(func() {
		log.SetOutput(orig)
		log.SetFlags(origFlags)
	})
}

func TestSetup_WritesToFileAndCloses(t *testing.T) {
	restoreLogger(t)
	dir := t.TempDir()

	closeLog := setupIn(dir, true)
	log.Print("hello from test")
	closeLog()
	closeLog() // closing twice must be harmless

	data, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatalf("log file not created: %v", err)
	}
	for _, want := range []string{"started", "hello from test", "shutting down"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("log file missing %q:\n%s", want, data)
		}
	}
}

func TestSetup_UnwritableDirFallsBack(t *testing.T) {
	restoreLogger(t)
	file := filepath.Join(t.TempDir(), "not-a-dir")
	os.WriteFile(file, nil, 0644)
	closeLog := setupIn(filepath.Join(file, "sub"), true) // MkdirAll fails
	closeLog()                                            // no-op, must not panic
}

func TestSetup_RotatesOversizedLog(t *testing.T) {
	restoreLogger(t)
	dir := t.TempDir()
	path := filepath.Join(dir, fileName)
	os.WriteFile(path, make([]byte, maxLogSize+1), 0644)

	setupIn(dir, true)()

	if info, err := os.Stat(path + ".1"); err != nil || info.Size() != maxLogSize+1 {
		t.Errorf("oversized log should be rotated to %s.1", fileName)
	}
	if info, err := os.Stat(path); err != nil || info.Size() > maxLogSize {
		t.Error("a fresh log file should replace the rotated one")
	}
}

func TestSetup_ChildDoesNoHousekeeping(t *testing.T) {
	restoreLogger(t)
	dir := t.TempDir()
	path := filepath.Join(dir, fileName)
	os.WriteFile(path, make([]byte, maxLogSize+1), 0644)
	old := filepath.Join(dir, "old.log")
	os.WriteFile(old, nil, 0644)
	past := time.Now().AddDate(0, 0, -30)
	os.Chtimes(old, past, past)

	setupIn(dir, false)()

	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Error("a child process must not rotate the shared log")
	}
	if _, err := os.Stat(old); err != nil {
		t.Error("a child process must not delete old logs")
	}
}
