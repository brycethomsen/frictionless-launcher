package config

import "testing"

func TestMigrateLegacy_AlwaysSchedule(t *testing.T) {
	cfg := &Config{
		GamePath: "/usr/games/mygame",
		GameName: "My Legacy Game",
		Enabled:  true,
		Schedule: "always",
	}

	if migrated := MigrateLegacy(cfg); !migrated {
		t.Fatal("expected MigrateLegacy to report a migration")
	}

	if len(cfg.Games) != 1 {
		t.Fatalf("expected 1 migrated game, got %d", len(cfg.Games))
	}
	g := cfg.Games[0]
	if g.GameName != "My Legacy Game" {
		t.Errorf("expected 'My Legacy Game', got %q", g.GameName)
	}
	if g.GamePath != "/usr/games/mygame" {
		t.Errorf("expected '/usr/games/mygame', got %q", g.GamePath)
	}
	if g.LaunchMethod != "direct" {
		t.Errorf("legacy migration should set launch_method=direct, got %q", g.LaunchMethod)
	}
	// "always" schedule maps to Mon-Sun 00:00-23:59
	if len(g.Schedules) == 0 {
		t.Fatal("expected at least one schedule after migration")
	}
	if len(g.Schedules[0].Days) != 7 {
		t.Errorf("expected 7 days, got %d", len(g.Schedules[0].Days))
	}
	// Legacy fields should be cleared after migration
	if cfg.GamePath != "" {
		t.Error("legacy GamePath should be cleared after migration")
	}
}

func TestMigrateLegacy_NoSchedule(t *testing.T) {
	cfg := &Config{
		GamePath: "/usr/games/other",
		GameName: "Other Game",
		Enabled:  false,
	}

	if migrated := MigrateLegacy(cfg); !migrated {
		t.Fatal("expected MigrateLegacy to report a migration")
	}
	if len(cfg.Games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(cfg.Games))
	}
	// No schedule string -> empty Schedules slice (not nil panic)
	_ = cfg.Games[0].Schedules
}

func TestMigrateLegacy_NoOpWhenNoLegacyPath(t *testing.T) {
	cfg := &Config{Games: []Game{{GameName: "Already Migrated"}}}
	if migrated := MigrateLegacy(cfg); migrated {
		t.Error("expected no migration when GamePath is empty")
	}
}

func TestMigrateLegacy_NoOpWhenGamesAlreadyPresent(t *testing.T) {
	cfg := &Config{
		GamePath: "/usr/games/mygame",
		Games:    []Game{{GameName: "Existing"}},
	}
	if migrated := MigrateLegacy(cfg); migrated {
		t.Error("expected no migration when Games is already populated")
	}
	if len(cfg.Games) != 1 || cfg.Games[0].GameName != "Existing" {
		t.Error("existing Games should be left untouched")
	}
}
