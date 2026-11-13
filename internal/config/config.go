// Package config holds the app's data model — the YAML-serialized shape of
// a user's configured games and schedules — independent of how it's loaded,
// saved, or acted on.
package config

// Schedule is one recurring launch window, e.g. Mon/Tue/Wed 19:00-21:00.
type Schedule struct {
	Days      []string `yaml:"days"`       // e.g., ["Mon", "Tue", "Wed"]
	StartTime string   `yaml:"start_time"` // e.g., "19:00"
	EndTime   string   `yaml:"end_time"`   // e.g., "21:00"
}

// Game is one managed game: how to launch it and when.
type Game struct {
	GameName     string     `yaml:"game_name"`
	GamePath     string     `yaml:"game_path"`
	LaunchMethod string     `yaml:"launch_method"` // "steam", "gog", "epic", "direct"
	LaunchArgs   string     `yaml:"launch_args"`
	Schedules    []Schedule `yaml:"schedules"`
	Enabled      bool       `yaml:"enabled"`
}

// Config is the top-level shape of config.yaml.
type Config struct {
	Games     []Game `yaml:"games"`
	BootDelay int    `yaml:"boot_delay"`

	// Legacy fields for backwards compatibility
	GamePath   string `yaml:"game_path,omitempty"`
	GameName   string `yaml:"game_name,omitempty"`
	LaunchArgs string `yaml:"launch_args,omitempty"`
	Enabled    bool   `yaml:"enabled,omitempty"`
	Schedule   string `yaml:"schedule,omitempty"`
}

// MigrateLegacy converts a pre-multi-game config (a single game_path/game_name
// at the top level) into the Games array format, clearing the legacy fields
// once migrated. Reports whether it changed anything, so the caller knows
// whether the result needs to be persisted.
func MigrateLegacy(cfg *Config) bool {
	if cfg.GamePath == "" || len(cfg.Games) != 0 {
		return false
	}

	legacyGame := Game{
		GameName:     cfg.GameName,
		GamePath:     cfg.GamePath,
		LaunchMethod: "direct",
		LaunchArgs:   cfg.LaunchArgs,
		Enabled:      cfg.Enabled,
		Schedules:    []Schedule{},
	}

	// Convert old schedule string to new format
	if cfg.Schedule == "always" {
		legacyGame.Schedules = []Schedule{
			{
				Days:      []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"},
				StartTime: "00:00",
				EndTime:   "23:59",
			},
		}
	}

	cfg.Games = []Game{legacyGame}
	// Clear legacy fields
	cfg.GamePath = ""
	cfg.GameName = ""
	cfg.LaunchArgs = ""
	cfg.Schedule = ""

	return true
}
