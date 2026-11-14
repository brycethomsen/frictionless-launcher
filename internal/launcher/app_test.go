package launcher

import (
	"testing"

	"frictionless-launcher/internal/config"
)

func TestLaunchIfInWindowAtBoot_InsideWindow(t *testing.T) {
	launched := stubAutoLaunch(t)
	app := appWithGames(t, []config.Game{gameWithSchedule("Mon", "19:00", "21:00")})

	app.launchIfInWindowAtBoot(monday(20, 0, 0))

	if got := launchedNames(launched, 1); len(got) != 1 || got[0] != "TestGame" {
		t.Errorf("launched %v at boot, want [TestGame]", got)
	}
}

func TestLaunchIfInWindowAtBoot_OutsideWindow(t *testing.T) {
	launched := stubAutoLaunch(t)
	app := appWithGames(t, []config.Game{gameWithSchedule("Mon", "19:00", "21:00")})

	app.launchIfInWindowAtBoot(monday(8, 0, 0))

	if got := launchedNames(launched, 0); len(got) != 0 {
		t.Errorf("launched %v at boot outside the window, want nothing", got)
	}
}

func TestUpcoming_UsesCurrentConfig(t *testing.T) {
	app := appWithGames(t, []config.Game{gameWithSchedule("Mon", "19:00", "21:00")})
	if got := app.Upcoming(3); len(got) != 1 || got[0].GameName != "TestGame" {
		t.Errorf("Upcoming = %v, want [TestGame]", got)
	}
}
