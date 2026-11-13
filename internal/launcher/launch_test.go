package launcher

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"frictionless-launcher/internal/config"
)

// ---- shouldLaunchGameAt ------------------------------------------------------

func TestShouldLaunchGameAt_InsideWindow(t *testing.T) {
	now := time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local) // Thu 20:00
	game := gameWithSchedule("Thu", "19:00", "21:00")
	app := appWithGames(t, []config.Game{game})
	if !app.shouldLaunchGameAt(game, now) {
		t.Error("expected true inside window with no prior launch")
	}
}

func TestShouldLaunchGameAt_OutsideWindow(t *testing.T) {
	now := time.Date(2024, 1, 18, 17, 0, 0, 0, time.Local) // Thu 17:00
	game := gameWithSchedule("Thu", "19:00", "21:00")
	app := appWithGames(t, []config.Game{game})
	if app.shouldLaunchGameAt(game, now) {
		t.Error("expected false outside window")
	}
}

func TestShouldLaunchGameAt_AlreadyLaunched(t *testing.T) {
	now := time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local)
	game := gameWithSchedule("Thu", "19:00", "21:00")
	app := appWithGames(t, []config.Game{game})
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
	app := appWithGames(t, []config.Game{game})
	_ = app.shouldLaunchGame(game)
}

// ---- isInScheduleWindowAt (production method) -------------------------------

// ---- launchedInCurrentWindow --------------------------------------------------
// (The window math itself is tested in internal/schedule; these check the
// wiring to the recorded launch times.)

func TestLaunchedInCurrentWindow_NoRecord(t *testing.T) {
	app := appWithGames(t, nil)
	game := gameWithSchedule("Mon", "19:00", "21:00")
	now := time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	if app.launchedInCurrentWindow(game, now) {
		t.Error("no launch record should return false")
	}
}

func TestLaunchedInCurrentWindow_Recorded(t *testing.T) {
	app := appWithGames(t, nil)
	game := gameWithSchedule("Mon", "19:00", "21:00")
	now := time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	app.lastLaunchTime[game.GameName] = now.Add(-10 * time.Minute)
	if !app.launchedInCurrentWindow(game, now) {
		t.Error("a launch recorded earlier in this window should count")
	}
}

// ---- recordLaunch -------------------------------------------------------------

func TestRecordLaunch(t *testing.T) {
	app := appWithGames(t, nil)
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
	app := appWithGames(t, nil)
	game := config.Game{GameName: "NullGame", GamePath: "", LaunchMethod: "direct"}
	app.Launch(game) // must not panic
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
	app := appWithGames(t, []config.Game{game})
	app.Launch(game) // must not panic
}

func TestLaunchGameByStruct_EpicWarningPath(t *testing.T) {
	game := config.Game{
		GameName:     "EpicWarn",
		GamePath:     "/absolutely/does/not/exist/game",
		LaunchMethod: "epic",
		Enabled:      true,
	}
	app := appWithGames(t, []config.Game{game})
	app.Launch(game) // must not panic
}

// ---- isPlatformRunning (smoke test) --------------------------------------------

func TestIsPlatformRunning_UnknownPlatform(t *testing.T) {
	app := appWithGames(t, nil)
	if app.isPlatformRunning("unknown_platform_xyz") {
		t.Error("unknown platform should return false")
	}
}

func TestIsPlatformRunning_KnownPlatforms_NoPanic(t *testing.T) {
	app := appWithGames(t, nil)
	// Just verify no panic; result depends on whether steam/epic are actually running
	_ = app.isPlatformRunning("steam")
	_ = app.isPlatformRunning("epic")
}

// ---- isGameRunning ---------------------------------------------------------------

func TestIsGameRunning_DirectGame_NotRunning(t *testing.T) {
	app := appWithGames(t, []config.Game{
		{GameName: "X", GamePath: "/nonexistent/xyz_game_definitely_not_running_12345", LaunchMethod: "direct", Enabled: true},
	})
	if app.isGameRunning() {
		t.Error("nonexistent game should not be detected as running")
	}
}

func TestIsGameRunning_NoDirectGames(t *testing.T) {
	app := appWithGames(t, []config.Game{
		{GameName: "A", LaunchMethod: "steam", GamePath: "steam://rungameid/1"},
	})
	// Steam games don't use process detection
	if app.isGameRunning() {
		t.Error("non-direct games should not trigger process check")
	}
}

func TestIsGameRunning_NoGames(t *testing.T) {
	app := appWithGames(t, []config.Game{})
	if app.isGameRunning() {
		t.Error("no games should return false")
	}
}

// ---- getForegroundAppName --------------------------------------------------------

func TestGetForegroundAppName_NoPanic(t *testing.T) {
	app := appWithGames(t, nil)
	name, err := app.getForegroundAppName()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if name == "Frictionless" || name == "frictionless-launcher" {
		t.Error("getForegroundAppName should filter out own app name")
	}
}

// ---- autoLaunchGame -----------------------------------------------------------

// stubCountdownPopup replaces the --countdown process for the duration of the
// test, and reports no foreground app so the popup path is always taken. It
// counts calls; if cancelAfter is >= 0 it cancels the
// launch that long after being started, as if the user clicked Cancel.
func stubCountdownPopup(t *testing.T, cancelAfter time.Duration) *int {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	origFrontmost := frontmostApp
	frontmostApp = func() string { return "" } // nothing in front: take the popup path
	orig := countdownPopup
	countdownPopup = func(gameName string, seconds int, onCancel func()) *exec.Cmd {
		mu.Lock()
		calls++
		mu.Unlock()
		if cancelAfter >= 0 {
			time.AfterFunc(cancelAfter, onCancel)
		}
		return nil
	}
	t.Cleanup(func() { countdownPopup, frontmostApp = orig, origFrontmost })
	return &calls
}

// launchableGame is a direct-launch game whose executable exits immediately.
func launchableGame(t *testing.T) config.Game {
	t.Helper()
	exe, err := os.Executable() // this test binary; any real file works
	if err != nil {
		t.Fatal(err)
	}
	return config.Game{
		GameName:     "Launchable",
		GamePath:     exe,
		LaunchArgs:   "-test.run=^$",
		LaunchMethod: "direct",
		Enabled:      true,
	}
}

func TestAutoLaunchGame_LaunchesAfterDelay(t *testing.T) {
	stubCountdownPopup(t, -1)
	game := launchableGame(t)
	app := appWithGames(t, []config.Game{game})
	app.Config().BootDelay = 1

	start := time.Now()
	app.autoLaunchGame(game)

	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("launched after %v, want about the 1s boot delay", elapsed)
	}
	if app.hasPendingLaunch() {
		t.Error("pending launch should be cleared after launching")
	}
	if _, ok := app.lastLaunchTime[game.GameName]; !ok {
		t.Error("a successful launch should be recorded")
	}
}

func TestAutoLaunchGame_CancelFromPopup(t *testing.T) {
	stubCountdownPopup(t, 50*time.Millisecond)
	game := launchableGame(t)
	game.GamePath = "/nonexistent/should-never-run"
	app := appWithGames(t, []config.Game{game})
	app.Config().BootDelay = 30

	start := time.Now()
	app.autoLaunchGame(game)

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("cancel took %v; it should end the countdown immediately", elapsed)
	}
	if app.hasPendingLaunch() {
		t.Error("pending launch should be cleared after cancel")
	}
	// A cancel is recorded as a launch so the scheduler doesn't immediately
	// start another countdown for the same window.
	if _, ok := app.lastLaunchTime[game.GameName]; !ok {
		t.Error("cancelled launch should be recorded to suppress the rest of the window")
	}
}

func TestAutoLaunchGame_CancelFromTray(t *testing.T) {
	stubCountdownPopup(t, -1)
	game := launchableGame(t)
	app := appWithGames(t, []config.Game{game})
	app.Config().BootDelay = 30

	done := make(chan struct{})
	go func() {
		app.autoLaunchGame(game)
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for !app.hasPendingLaunch() {
		if time.Now().After(deadline) {
			t.Fatal("launch never became pending")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, ok := app.PendingLabel(); !ok {
		t.Error("cancelLabel should report the pending launch")
	}
	app.CancelPending()
	app.CancelPending() // a second click must not panic on a closed channel

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("autoLaunchGame did not return after tray cancel")
	}
	if app.hasPendingLaunch() {
		t.Error("pending launch should be cleared after cancel")
	}
}

func TestAutoLaunchGame_OnlyOnePending(t *testing.T) {
	popups := stubCountdownPopup(t, -1)
	game := launchableGame(t)
	app := appWithGames(t, []config.Game{game})
	app.Config().BootDelay = 30

	done := make(chan struct{})
	go func() {
		app.autoLaunchGame(game)
		close(done)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !app.hasPendingLaunch() {
		if time.Now().After(deadline) {
			t.Fatal("launch never became pending")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// A second launch while one is pending must return immediately.
	second := make(chan struct{})
	go func() {
		app.autoLaunchGame(game)
		close(second)
	}()
	select {
	case <-second:
	case <-time.After(2 * time.Second):
		t.Error("second autoLaunchGame should return immediately while one is pending")
	}

	app.CancelPending()
	<-done
	if *popups > 1 {
		t.Errorf("countdown popup started %d times, want at most 1", *popups)
	}
}

// ---- shouldLaunchGameAt ordering ----------------------------------------------

func TestShouldLaunchGameAt_OutsideWindowSkipsProcessScan(t *testing.T) {
	// A direct game makes isGameRunning enumerate processes. Outside the
	// window, shouldLaunchGameAt must return false without needing to.
	game := config.Game{
		GameName: "Direct", GamePath: "/nonexistent/game", LaunchMethod: "direct", Enabled: true,
		Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}},
	}
	app := appWithGames(t, []config.Game{game})
	monday8am := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local)
	if app.shouldLaunchGameAt(game, monday8am) {
		t.Error("should not launch outside the schedule window")
	}
	monday8pm := time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	if !app.shouldLaunchGameAt(game, monday8pm) {
		t.Error("should launch inside the window when not running and not yet launched")
	}
}
