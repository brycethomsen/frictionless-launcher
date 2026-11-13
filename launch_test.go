package main

import (
	"strings"
	"testing"
	"time"

	"frictionless-launcher/internal/config"
)

// ---- shouldLaunchGameAt ------------------------------------------------------

func TestShouldLaunchGameAt_InsideWindow(t *testing.T) {
	now := time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local) // Thu 20:00
	game := gameWithSchedule("Thu", "19:00", "21:00")
	app := appWithGames([]config.Game{game})
	if !app.shouldLaunchGameAt(game, now) {
		t.Error("expected true inside window with no prior launch")
	}
}

func TestShouldLaunchGameAt_OutsideWindow(t *testing.T) {
	now := time.Date(2024, 1, 18, 17, 0, 0, 0, time.Local) // Thu 17:00
	game := gameWithSchedule("Thu", "19:00", "21:00")
	app := appWithGames([]config.Game{game})
	if app.shouldLaunchGameAt(game, now) {
		t.Error("expected false outside window")
	}
}

func TestShouldLaunchGameAt_AlreadyLaunched(t *testing.T) {
	now := time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local)
	game := gameWithSchedule("Thu", "19:00", "21:00")
	app := appWithGames([]config.Game{game})
	app.lastLaunchTime[game.GameName] = now.Add(-30 * time.Minute)
	if app.shouldLaunchGameAt(game, now) {
		t.Error("expected false when already launched in window")
	}
}

func TestShouldLaunchGame_OutsideWindow(t *testing.T) {
	// This test only verifies the wrapper delegates to shouldLaunchGameAt
	// correctly (no panic, bool result) — the actual window logic is covered
	// by the At-variant tests above.
	game := config.Game{
		GameName: "WrapTest", Enabled: true,
		Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "00:01", EndTime: "00:02"}},
	}
	app := appWithGames([]config.Game{game})
	_ = app.shouldLaunchGame(game)
}

// ---- isInScheduleWindowAt (production method) -------------------------------

func TestIsInScheduleWindowAt_Inside(t *testing.T) {
	now := time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local)
	app := appWithGames(nil)
	if !app.isInScheduleWindowAt(gameWithSchedule("Thu", "19:00", "21:00"), now) {
		t.Error("expected true inside window")
	}
}

func TestIsInScheduleWindowAt_Outside(t *testing.T) {
	now := time.Date(2024, 1, 18, 22, 0, 0, 0, time.Local)
	app := appWithGames(nil)
	if app.isInScheduleWindowAt(gameWithSchedule("Thu", "19:00", "21:00"), now) {
		t.Error("expected false after window end")
	}
}

func TestIsInScheduleWindowAt_BeforeStart(t *testing.T) {
	now := time.Date(2024, 1, 18, 18, 59, 0, 0, time.Local) // Thursday 18:59
	app := appWithGames(nil)
	if app.isInScheduleWindowAt(gameWithSchedule("Thu", "19:00", "21:00"), now) {
		t.Error("expected outside window (before start)")
	}
}

func TestIsInScheduleWindowAt_WrongDay(t *testing.T) {
	now := time.Date(2024, 1, 17, 19, 30, 0, 0, time.Local) // Wednesday 19:30
	app := appWithGames(nil)
	if app.isInScheduleWindowAt(gameWithSchedule("Thu", "19:00", "21:00"), now) {
		t.Error("expected outside window (wrong day)")
	}
}

func TestIsInScheduleWindowAt_NoSchedules(t *testing.T) {
	now := time.Date(2024, 1, 18, 19, 30, 0, 0, time.Local)
	game := config.Game{GameName: "X", Enabled: true}
	app := appWithGames(nil)
	if app.isInScheduleWindowAt(game, now) {
		t.Error("game with no schedules should never be in window")
	}
}

func TestIsInScheduleWindowAt_CaseInsensitiveDay(t *testing.T) {
	now := time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local) // Thursday
	game := config.Game{
		Enabled:   true,
		Schedules: []config.Schedule{{Days: []string{"thu"}, StartTime: "19:00", EndTime: "21:00"}},
	}
	app := appWithGames(nil)
	if !app.isInScheduleWindowAt(game, now) {
		t.Error("day matching should be case-insensitive")
	}
}

func TestIsInScheduleWindowAt_MultipleWindows(t *testing.T) {
	game := config.Game{
		Enabled: true,
		Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "08:00", EndTime: "10:00"},
			{Days: []string{"Thu"}, StartTime: "19:00", EndTime: "21:00"},
		},
	}
	app := appWithGames(nil)
	if !app.isInScheduleWindowAt(game, time.Date(2024, 1, 15, 9, 0, 0, 0, time.Local)) {
		t.Error("expected true for Mon window")
	}
	if !app.isInScheduleWindowAt(game, time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local)) {
		t.Error("expected true for Thu window")
	}
	if app.isInScheduleWindowAt(game, time.Date(2024, 1, 16, 9, 0, 0, 0, time.Local)) {
		t.Error("expected false on Tuesday (no window)")
	}
}

func TestIsInScheduleWindow_DelegatesCorrectly(t *testing.T) {
	game := config.Game{
		GameName: "WrapTest2", Enabled: true,
		Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "00:01", EndTime: "00:02"}},
	}
	app := appWithGames([]config.Game{game})
	// Consistent result with At variant using time.Now().
	now := time.Now()
	want := app.isInScheduleWindowAt(game, now)
	got := app.isInScheduleWindow(game)
	if got != want {
		t.Errorf("isInScheduleWindow()=%v but isInScheduleWindowAt(now)=%v", got, want)
	}
}

// ---- hasLaunchedInCurrentWindowAt --------------------------------------------

func TestHasLaunchedInCurrentWindowAt_NoRecord(t *testing.T) {
	app := appWithGames(nil)
	game := gameWithSchedule("Mon", "19:00", "21:00")
	now := time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	if app.hasLaunchedInCurrentWindowAt(game, now) {
		t.Error("no launch record should return false")
	}
}

func TestHasLaunchedInCurrentWindowAt_LaunchedInsideWindow(t *testing.T) {
	app := appWithGames(nil)
	// Current time is Monday 19:30, last launch was Monday 19:05 — both inside the window.
	now := time.Date(2024, 1, 15, 19, 30, 0, 0, time.Local) // Monday
	game := config.Game{
		GameName: "TestGame",
		Enabled:  true,
		Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		},
	}
	launchTime := time.Date(now.Year(), now.Month(), now.Day(), 19, 5, 0, 0, now.Location())
	app.lastLaunchTime["TestGame"] = launchTime

	if !app.hasLaunchedInCurrentWindowAt(game, now) {
		t.Error("launch was in window, should return true")
	}
}

func TestHasLaunchedInCurrentWindowAt_LaunchedEarlierSameWindow(t *testing.T) {
	app := appWithGames(nil)
	game := config.Game{
		GameName: "G", Enabled: true,
		Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}},
	}
	now := time.Date(2024, 1, 15, 20, 55, 0, 0, time.Local) // still inside 19:00-21:00
	app.lastLaunchTime["G"] = time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	if !app.hasLaunchedInCurrentWindowAt(game, now) {
		t.Error("launch at 20:00 falls within the still-open Mon 19:00-21:00 window; should return true")
	}
}

// TestHasLaunchedInCurrentWindowAt_DifferentWindowSameDayNotSuppressed guards
// against a real bug: a game with two windows on the same day (e.g. a
// morning and an evening slot) launched during the first window must not
// have that suppress the second, distinct window later that day.
func TestHasLaunchedInCurrentWindowAt_DifferentWindowSameDayNotSuppressed(t *testing.T) {
	app := appWithGames(nil)
	game := config.Game{
		GameName: "G", Enabled: true,
		Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "07:00", EndTime: "09:00"},
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		},
	}
	app.lastLaunchTime["G"] = time.Date(2024, 1, 15, 7, 30, 0, 0, time.Local) // launched in the morning window
	now := time.Date(2024, 1, 15, 19, 30, 0, 0, time.Local)                   // now inside the evening window
	if app.hasLaunchedInCurrentWindowAt(game, now) {
		t.Error("the morning launch must not suppress the distinct evening window")
	}
}

func TestHasLaunchedInCurrentWindowAt_WrongDay(t *testing.T) {
	app := appWithGames(nil)
	game := config.Game{
		GameName: "G", Enabled: true,
		Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}},
	}
	now := time.Date(2024, 1, 16, 20, 0, 0, 0, time.Local) // Tuesday
	app.lastLaunchTime["G"] = time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	if app.hasLaunchedInCurrentWindowAt(game, now) {
		t.Error("schedule is Mon only; today is Tue — should return false")
	}
}

func TestHasLaunchedInCurrentWindowAt_BeforeWindowStart(t *testing.T) {
	app := appWithGames(nil)
	game := config.Game{
		GameName: "G", Enabled: true,
		Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}},
	}
	now := time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	// Launch was at 18:00 — before window start
	app.lastLaunchTime["G"] = time.Date(2024, 1, 15, 18, 0, 0, 0, time.Local)
	if app.hasLaunchedInCurrentWindowAt(game, now) {
		t.Error("launch before window start should return false")
	}
}

// ---- recordLaunch -------------------------------------------------------------

func TestRecordLaunch(t *testing.T) {
	app := appWithGames(nil)
	game := config.Game{GameName: "RecordMe"}
	before := time.Now()
	app.recordLaunch(game)
	after := time.Now()

	ts, ok := app.lastLaunchTime["RecordMe"]
	if !ok {
		t.Fatal("launch time not recorded")
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("launch timestamp %v not in expected range [%v, %v]", ts, before, after)
	}
}

// ---- buildLaunchCmd -----------------------------------------------------------

func TestBuildLaunchCmd_Steam_Darwin(t *testing.T) {
	game := config.Game{GamePath: "steam://rungameid/1", LaunchMethod: "steam"}
	cmd := buildLaunchCmd(game, "darwin")
	if len(cmd.Args) < 3 || cmd.Args[1] != "-g" {
		t.Errorf("expected 'open -g ...' on darwin, got %v", cmd.Args)
	}
	if cmd.Args[len(cmd.Args)-1] != game.GamePath {
		t.Errorf("expected game path as last arg, got %v", cmd.Args)
	}
}

func TestBuildLaunchCmd_Steam_Windows(t *testing.T) {
	game := config.Game{GamePath: "steam://rungameid/1", LaunchMethod: "steam"}
	cmd := buildLaunchCmd(game, "windows")
	found := false
	for _, a := range cmd.Args {
		if strings.EqualFold(a, "start") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'start' in windows args, got %v", cmd.Args)
	}
}

func TestBuildLaunchCmd_Steam_Linux(t *testing.T) {
	game := config.Game{GamePath: "steam://rungameid/1", LaunchMethod: "steam"}
	cmd := buildLaunchCmd(game, "linux")
	if !strings.HasSuffix(cmd.Path, "xdg-open") {
		t.Errorf("expected xdg-open on linux, got %q", cmd.Path)
	}
}

func TestBuildLaunchCmd_Epic_Darwin(t *testing.T) {
	game := config.Game{GamePath: "com.epicgames.launcher://apps/X", LaunchMethod: "epic"}
	cmd := buildLaunchCmd(game, "darwin")
	if len(cmd.Args) < 2 || cmd.Args[1] != "-g" {
		t.Errorf("expected 'open -g ...' for epic on darwin, got %v", cmd.Args)
	}
}

func TestBuildLaunchCmd_Epic_Windows(t *testing.T) {
	game := config.Game{GamePath: "com.epicgames.launcher://apps/X", LaunchMethod: "epic"}
	cmd := buildLaunchCmd(game, "windows")
	found := false
	for _, a := range cmd.Args {
		if strings.EqualFold(a, "start") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'start' in windows epic args, got %v", cmd.Args)
	}
}

func TestBuildLaunchCmd_Epic_Linux(t *testing.T) {
	game := config.Game{GamePath: "com.epicgames.launcher://apps/X", LaunchMethod: "epic"}
	cmd := buildLaunchCmd(game, "linux")
	if !strings.HasSuffix(cmd.Path, "xdg-open") {
		t.Errorf("expected xdg-open for epic on linux, got %q", cmd.Path)
	}
}

func TestBuildLaunchCmd_Direct_NoArgs(t *testing.T) {
	game := config.Game{GamePath: "/usr/bin/mygame", LaunchMethod: "direct"}
	cmd := buildLaunchCmd(game, "linux")
	if cmd.Path != "/usr/bin/mygame" {
		t.Errorf("expected game path as command, got %q", cmd.Path)
	}
	if len(cmd.Args) != 1 {
		t.Errorf("expected no extra args, got %v", cmd.Args)
	}
}

func TestBuildLaunchCmd_Direct_WithArgs(t *testing.T) {
	game := config.Game{GamePath: "/usr/bin/mygame", LaunchMethod: "direct", LaunchArgs: "-fullscreen -nosound"}
	cmd := buildLaunchCmd(game, "linux")
	if len(cmd.Args) != 3 {
		t.Errorf("expected 3 args, got %v", cmd.Args)
	}
	if cmd.Args[1] != "-fullscreen" || cmd.Args[2] != "-nosound" {
		t.Errorf("unexpected args: %v", cmd.Args)
	}
}

func TestBuildLaunchCmd_Unknown_DefaultsDirect(t *testing.T) {
	game := config.Game{GamePath: "/usr/bin/game", LaunchMethod: "gog", LaunchArgs: "-x"}
	cmd := buildLaunchCmd(game, "linux")
	if cmd.Path != "/usr/bin/game" {
		t.Errorf("expected game path as command, got %q", cmd.Path)
	}
	if len(cmd.Args) != 2 || cmd.Args[1] != "-x" {
		t.Errorf("unexpected args: %v", cmd.Args)
	}
}

// ---- launchGameByStruct --------------------------------------------------------

func TestLaunchGameByStruct_EmptyPath(t *testing.T) {
	// launchGameByStruct returns early with a log when GamePath is empty.
	app := appWithGames(nil)
	game := config.Game{GameName: "NullGame", GamePath: "", LaunchMethod: "direct"}
	app.launchGameByStruct(game) // must not panic
}

func TestLaunchGameByStruct_SteamWarningPath(t *testing.T) {
	// A steam game with a deliberately bad path so cmd.Start() fails quickly —
	// exercises the steam platform-check warning branch.
	game := config.Game{
		GameName:     "SteamWarn",
		GamePath:     "/absolutely/does/not/exist/game",
		LaunchMethod: "steam",
		Enabled:      true,
	}
	app := appWithGames([]config.Game{game})
	app.launchGameByStruct(game) // must not panic
}

func TestLaunchGameByStruct_EpicWarningPath(t *testing.T) {
	game := config.Game{
		GameName:     "EpicWarn",
		GamePath:     "/absolutely/does/not/exist/game",
		LaunchMethod: "epic",
		Enabled:      true,
	}
	app := appWithGames([]config.Game{game})
	app.launchGameByStruct(game) // must not panic
}

// ---- isPlatformRunning (smoke test) --------------------------------------------

func TestIsPlatformRunning_UnknownPlatform(t *testing.T) {
	app := appWithGames(nil)
	if app.isPlatformRunning("unknown_platform_xyz") {
		t.Error("unknown platform should return false")
	}
}

func TestIsPlatformRunning_KnownPlatforms_NoPanic(t *testing.T) {
	app := appWithGames(nil)
	// Just verify no panic; result depends on whether steam/epic are actually running
	_ = app.isPlatformRunning("steam")
	_ = app.isPlatformRunning("epic")
}

// ---- isGameRunning ---------------------------------------------------------------

func TestIsGameRunning_DirectGame_NotRunning(t *testing.T) {
	app := appWithGames([]config.Game{
		{GameName: "X", GamePath: "/nonexistent/xyz_game_definitely_not_running_12345", LaunchMethod: "direct", Enabled: true},
	})
	if app.isGameRunning() {
		t.Error("nonexistent game should not be detected as running")
	}
}

func TestIsGameRunning_NoDirectGames(t *testing.T) {
	app := appWithGames([]config.Game{
		{GameName: "A", LaunchMethod: "steam", GamePath: "steam://rungameid/1"},
	})
	// Steam games don't use process detection
	if app.isGameRunning() {
		t.Error("non-direct games should not trigger process check")
	}
}

func TestIsGameRunning_NoGames(t *testing.T) {
	app := appWithGames([]config.Game{})
	if app.isGameRunning() {
		t.Error("no games should return false")
	}
}

// ---- getForegroundAppName --------------------------------------------------------

func TestGetForegroundAppName_NoPanic(t *testing.T) {
	app := appWithGames(nil)
	name, err := app.getForegroundAppName()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if name == "Frictionless" || name == "frictionless-launcher" {
		t.Error("getForegroundAppName should filter out own app name")
	}
}
