package platform

import (
	"os"
	"strings"
	"testing"
)

func TestDataDir_ContainsAppName(t *testing.T) {
	if dir := DataDir(); !strings.Contains(dir, "FrictionlessLauncher") {
		t.Errorf("DataDir() = %q, want it to contain FrictionlessLauncher", dir)
	}
}

func TestOpenWithDefaultApp_MissingFile(t *testing.T) {
	OpenWithDefaultApp("/definitely/does/not/exist/xyz.log") // must not panic
}

func TestOpenWithDefaultApp_ExistingFile(t *testing.T) {
	// The OS command may fail in CI (no display); this only checks that the
	// function doesn't panic.
	f, err := os.CreateTemp("", "open_test_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())
	OpenWithDefaultApp(f.Name())
}
