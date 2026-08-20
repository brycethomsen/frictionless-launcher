package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"frictionless-launcher/internal/config"
)

// newTestApp creates an App backed by a fresh temp-dir config path.
func newTestApp(t *testing.T) (*App, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "frictionless_test_*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	a := &App{
		configPath:     filepath.Join(dir, "config.yaml"),
		lastLaunchTime: make(map[string]time.Time),
	}
	return a, dir
}

func gameWithSchedule(day, start, end string) config.Game {
	return config.Game{
		GameName:     "TestGame",
		GamePath:     "steam://rungameid/1",
		LaunchMethod: "steam",
		Enabled:      true,
		Schedules: []config.Schedule{
			{Days: []string{day}, StartTime: start, EndTime: end},
		},
	}
}

// appWithGames builds an App around the given games, skipping config file I/O
// entirely — for tests that only need in-memory state.
func appWithGames(games []config.Game) *App {
	return &App{
		config:         &config.Config{Games: games, BootDelay: 10},
		lastLaunchTime: make(map[string]time.Time),
	}
}
