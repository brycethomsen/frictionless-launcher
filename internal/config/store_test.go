package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestStore returns a Store for config.yaml in a fresh temp dir.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(filepath.Join(t.TempDir(), "config.yaml"), nil)
}

// ---- Store: load / save / migrate -------------------------------------------

func TestStore_LoadAndSave(t *testing.T) {
	s := newTestStore(t)

	s.Load()
	cfg := s.Current()
	if cfg == nil {
		t.Fatal("config must not be nil after Load")
	}
	// Fresh config must start with no games — never a hardcoded sample.
	if len(cfg.Games) != 0 {
		t.Errorf("default config should have no games, got %d", len(cfg.Games))
	}
	if cfg.BootDelay != DefaultBootDelay {
		t.Errorf("expected BootDelay %d, got %d", DefaultBootDelay, cfg.BootDelay)
	}
	if _, err := os.Stat(s.Path()); err != nil {
		t.Fatalf("Load should create a missing config file: %v", err)
	}

	// Add a game and round-trip
	cfg.Games = []Game{{GameName: "RoundTripGame"}}
	cfg.BootDelay = 30
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2 := NewStore(s.Path(), nil)
	s2.Load()
	if got := s2.Current(); len(got.Games) != 1 || got.Games[0].GameName != "RoundTripGame" {
		t.Errorf("expected RoundTripGame after reload, got %+v", got.Games)
	}
	if got := s2.Current().BootDelay; got != 30 {
		t.Errorf("expected BootDelay 30, got %d", got)
	}
}

func TestStore_InvalidYAMLKeepsPrevious(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "config.yaml"), &Config{Games: []Game{{GameName: "Kept"}}})
	if err := os.WriteFile(s.Path(), []byte("invalid: yaml: [unclosed"), 0644); err != nil {
		t.Fatal(err)
	}
	s.Load()
	if got := s.Current(); len(got.Games) != 1 || got.Games[0].GameName != "Kept" {
		t.Errorf("invalid YAML must keep the previous config, got %+v", got.Games)
	}
}

func TestStore_LegacyMigration(t *testing.T) {
	s := newTestStore(t)

	// The legacy format: top-level game_path with no games list. The migration
	// logic itself (MigrateLegacy) has its own focused tests; this one just
	// confirms Load actually wires it in end-to-end and saves the result.
	legacy := `
game_path: /usr/games/mygame
game_name: My Legacy Game
enabled: true
schedule: always
`
	if err := os.WriteFile(s.Path(), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	s.Load()

	cfg := s.Current()
	if len(cfg.Games) != 1 {
		t.Fatalf("expected 1 migrated game, got %d", len(cfg.Games))
	}
	g := cfg.Games[0]
	if g.GameName != "My Legacy Game" {
		t.Errorf("expected 'My Legacy Game', got %q", g.GameName)
	}
	if g.LaunchMethod != "direct" {
		t.Errorf("legacy migration should set launch_method=direct, got %q", g.LaunchMethod)
	}
	if cfg.GamePath != "" {
		t.Error("legacy GamePath should be cleared after migration")
	}
	data, _ := os.ReadFile(s.Path())
	if !strings.Contains(string(data), "games:") {
		t.Error("migrated config should be saved back in the new format")
	}
}

func TestStore_SaveError(t *testing.T) {
	s := NewStore("/nonexistent/path/config.yaml", &Config{Games: []Game{{GameName: "X"}}})
	if err := s.Save(); err == nil {
		t.Error("Save to an unwritable path should return an error")
	}
}

func TestStore_ReadErrorKeepsPrevious(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod permission test unreliable on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("boot_delay: 5\n"), 0644)
	os.Chmod(dir, 0000)
	defer os.Chmod(dir, 0755)

	s := NewStore(path, nil)
	s.Load()
	if s.Current() == nil {
		t.Error("config must not be nil after read error")
	}
}

func TestStore_LoadPublishesNewSnapshot(t *testing.T) {
	// Goroutines in the tray process hold on to the pointer returned by
	// Current; a reload must replace it rather than mutate it.
	s := newTestStore(t)
	s.Load()
	before := s.Current()

	if err := Write(s.Path(), &Config{BootDelay: 3, Games: []Game{{GameName: "New"}}}); err != nil {
		t.Fatal(err)
	}
	s.Load()

	if len(before.Games) != 0 {
		t.Error("previous snapshot was mutated by reload")
	}
	if after := s.Current(); after == before || len(after.Games) != 1 {
		t.Error("reload should publish a new config snapshot")
	}
}

// ---- Read / Write -----------------------------------------------------------

func TestRead_MissingFile(t *testing.T) {
	_, _, err := Read(filepath.Join(t.TempDir(), "nope.yaml"))
	if !os.IsNotExist(err) {
		t.Errorf("Read of a missing file should report not-exist, got %v", err)
	}
}

func TestRead_DefaultsBootDelay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("games: []\n"), 0644)
	cfg, migrated, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if migrated {
		t.Error("a current-format config should not report migration")
	}
	if cfg.BootDelay != DefaultBootDelay {
		t.Errorf("BootDelay = %d, want default %d when unset", cfg.BootDelay, DefaultBootDelay)
	}
}

func TestWrite_Nil(t *testing.T) {
	if err := Write(filepath.Join(t.TempDir(), "c.yaml"), nil); err == nil {
		t.Error("Write(nil) should return an error")
	}
}

// ---- DefaultPath ------------------------------------------------------------

func TestDefaultPath_LocalConfig(t *testing.T) {
	dir := t.TempDir()
	localCfg := filepath.Join(dir, "config.yaml")
	os.WriteFile(localCfg, []byte("boot_delay: 5"), 0644)

	if got := DefaultPath(filepath.Join(dir, "frictionless")); got != localCfg {
		t.Errorf("expected %s, got %s", localCfg, got)
	}
}

func TestDefaultPath_OSSpecific(t *testing.T) {
	// No local config.yaml → should fall back to the per-user data dir
	got := DefaultPath(filepath.Join(t.TempDir(), "frictionless"))
	if !strings.Contains(got, "FrictionlessLauncher") {
		t.Errorf("expected FrictionlessLauncher in path, got %s", got)
	}
	if !strings.HasSuffix(got, "config.yaml") {
		t.Errorf("expected config.yaml suffix, got %s", got)
	}
}

// ---- Watch ------------------------------------------------------------------

func TestWatch_CallsOnChangeOnceForABurst(t *testing.T) {
	s := newTestStore(t)
	s.Load() // creates the file

	var calls atomic.Int32
	stop := make(chan struct{})
	defer close(stop)
	go Watch(s.Path(), stop, func() { calls.Add(1) })
	time.Sleep(200 * time.Millisecond) // let the watcher start

	// Simulate the --manage process saving: several writes in quick succession.
	for i := range 3 {
		if err := Write(s.Path(), &Config{BootDelay: i}); err != nil {
			t.Fatal(err)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("onChange not called after external write")
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(2 * reloadDebounce)
	if n := calls.Load(); n != 1 {
		t.Errorf("onChange called %d times for one burst of writes, want 1", n)
	}
}
