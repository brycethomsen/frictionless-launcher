package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"frictionless-launcher/internal/config"
)

// ---- Config: load / save / migrate ------------------------------------------

func TestConfig_LoadAndSave(t *testing.T) {
	app, _ := newTestApp(t)

	app.loadConfig()
	if app.config == nil {
		t.Fatal("config must not be nil after loadConfig")
	}
	// Fresh config must start with no games — never a hardcoded sample.
	if len(app.config.Games) != 0 {
		t.Errorf("default config should have no games, got %d", len(app.config.Games))
	}
	if app.config.BootDelay != 10 {
		t.Errorf("expected BootDelay 10, got %d", app.config.BootDelay)
	}

	// Add a game and round-trip
	app.config.Games = []config.Game{{GameName: "RoundTripGame"}}
	app.config.BootDelay = 30
	// saveConfig calls refreshTrayMenu (no-op when desk==nil)
	app.saveConfig()

	if _, err := os.Stat(app.configPath); os.IsNotExist(err) {
		t.Fatal("config file should exist after saveConfig")
	}

	app2, _ := newTestApp(t)
	app2.configPath = app.configPath
	app2.loadConfig()
	if app2.config.Games[0].GameName != "RoundTripGame" {
		t.Errorf("expected RoundTripGame, got %s", app2.config.Games[0].GameName)
	}
	if app2.config.BootDelay != 30 {
		t.Errorf("expected BootDelay 30, got %d", app2.config.BootDelay)
	}
}

func TestConfig_InvalidYAML(t *testing.T) {
	app, dir := newTestApp(t)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("invalid: yaml: [unclosed"), 0644); err != nil {
		t.Fatal(err)
	}
	app.loadConfig()
	// Must fall back to defaults — not nil, not panicking
	if app.config == nil {
		t.Fatal("config must not be nil after invalid YAML")
	}
	if len(app.config.Games) != 0 {
		t.Errorf("expected empty game list after invalid YAML, got %d", len(app.config.Games))
	}
}

func TestConfig_LegacyMigration(t *testing.T) {
	app, _ := newTestApp(t)

	// The legacy format: top-level game_path with no games list. The migration
	// logic itself (internal/config.MigrateLegacy) has its own focused tests;
	// this one just confirms loadConfig actually wires it in end-to-end.
	legacy := `
game_path: /usr/games/mygame
game_name: My Legacy Game
enabled: true
schedule: always
`
	if err := os.WriteFile(app.configPath, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	app.loadConfig()

	if len(app.config.Games) != 1 {
		t.Fatalf("expected 1 migrated game, got %d", len(app.config.Games))
	}
	g := app.config.Games[0]
	if g.GameName != "My Legacy Game" {
		t.Errorf("expected 'My Legacy Game', got %q", g.GameName)
	}
	if g.LaunchMethod != "direct" {
		t.Errorf("legacy migration should set launch_method=direct, got %q", g.LaunchMethod)
	}
	// Legacy fields should be cleared after migration
	if app.config.GamePath != "" {
		t.Error("legacy GamePath should be cleared after migration")
	}
}

func TestSaveConfig_ErrorHandling(t *testing.T) {
	app := &App{
		configPath:     "/nonexistent/path/config.yaml",
		config:         &config.Config{Games: []config.Game{{GameName: "X"}}},
		lastLaunchTime: make(map[string]time.Time),
	}
	// Must not panic
	app.saveConfig()
}

func TestLoadConfig_ReadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod permission test unreliable on Windows")
	}
	dir, _ := os.MkdirTemp("", "cfg_perm_*")
	defer os.RemoveAll(dir)

	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("boot_delay: 5\n"), 0644)
	os.Chmod(dir, 0000)
	defer os.Chmod(dir, 0755)

	app := &App{configPath: cfgPath, lastLaunchTime: make(map[string]time.Time)}
	app.loadConfig()
	if app.config == nil {
		t.Error("config must not be nil after read error")
	}
}

// ---- warnScheduleOverlaps ---------------------------------------------------

func TestWarnScheduleOverlaps_NoOverlap(t *testing.T) {
	app := appWithGames([]config.Game{
		{GameName: "A", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}}},
		{GameName: "B", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Tue"}, StartTime: "19:00", EndTime: "21:00"}}},
	})
	// Should not panic
	app.warnScheduleOverlaps()
}

func TestWarnScheduleOverlaps_WithOverlap(t *testing.T) {
	app := appWithGames([]config.Game{
		{GameName: "A", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}}},
		{GameName: "B", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "20:00", EndTime: "22:00"}}},
	})
	// Should not panic; overlap warning is logged only
	app.warnScheduleOverlaps()
}

// ---- getConfigPath ----------------------------------------------------------

func TestGetConfigPath_LocalConfig(t *testing.T) {
	dir, _ := os.MkdirTemp("", "cfg_local_*")
	defer os.RemoveAll(dir)

	localCfg := filepath.Join(dir, "config.yaml")
	os.WriteFile(localCfg, []byte("boot_delay: 5"), 0644)

	got := getConfigPath(filepath.Join(dir, "frictionless"))
	if got != localCfg {
		t.Errorf("expected %s, got %s", localCfg, got)
	}
}

func TestGetConfigPath_OSSpecific(t *testing.T) {
	dir, _ := os.MkdirTemp("", "cfg_os_*")
	defer os.RemoveAll(dir)

	// No local config.yaml → should fall back to OS path
	got := getConfigPath(filepath.Join(dir, "frictionless"))
	if !strings.Contains(got, "FrictionlessLauncher") {
		t.Errorf("expected FrictionlessLauncher in path, got %s", got)
	}
	if !strings.HasSuffix(got, "config.yaml") {
		t.Errorf("expected config.yaml suffix, got %s", got)
	}
}
