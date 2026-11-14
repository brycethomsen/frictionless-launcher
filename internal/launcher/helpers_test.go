package launcher

import (
	"path/filepath"
	"testing"

	"frictionless-launcher/internal/config"
)

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

// appWithGames builds an App around the given games and a 10s boot delay,
// backed by a config file path in a temp dir that is never written unless
// the test saves.
func appWithGames(t *testing.T, games []config.Game) *App {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	return New(config.NewStore(path, &config.Config{Games: games, BootDelay: 10}))
}
