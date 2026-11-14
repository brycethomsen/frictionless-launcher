package tray

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/systray"

	"frictionless-launcher/internal/assets"
	"frictionless-launcher/internal/config"
	"frictionless-launcher/internal/launcher"
	"frictionless-launcher/internal/schedule"
)

// ---- fadedIcon ---------------------------------------------------------------

func TestFadedIcon_Valid(t *testing.T) {
	result, err := fadedIcon(assets.Icon, 128)
	if err != nil {
		t.Fatalf("fadedIcon failed: %v", err)
	}
	if len(result) == 0 {
		t.Error("fadedIcon returned empty bytes")
	}
	// Result should still be a valid PNG
	if len(result) < 8 || result[0] != 0x89 || result[1] != 'P' {
		t.Error("fadedIcon result is not a valid PNG header")
	}
}

func TestFadedIcon_ZeroAlpha(t *testing.T) {
	result, err := fadedIcon(assets.Icon, 0)
	if err != nil {
		t.Fatalf("fadedIcon with alpha=0 failed: %v", err)
	}
	if len(result) == 0 {
		t.Error("fadedIcon returned empty bytes for alpha=0")
	}
}

func TestFadedIcon_FullAlpha(t *testing.T) {
	result, err := fadedIcon(assets.Icon, 255)
	if err != nil {
		t.Fatalf("fadedIcon with alpha=255 failed: %v", err)
	}
	if len(result) == 0 {
		t.Error("fadedIcon returned empty bytes for alpha=255")
	}
}

func TestFadedIcon_InvalidInput(t *testing.T) {
	_, err := fadedIcon([]byte("not a png"), 128)
	if err == nil {
		t.Error("fadedIcon should return error for non-PNG input")
	}
}

func TestFadedIcon_AlphaDiffers(t *testing.T) {
	full, err := fadedIcon(assets.Icon, 255)
	if err != nil {
		t.Fatal(err)
	}
	half, err := fadedIcon(assets.Icon, 128)
	if err != nil {
		t.Fatal(err)
	}
	if string(full) == string(half) {
		t.Error("alpha=255 and alpha=128 outputs should produce different PNGs")
	}
}

// ---- trayEntries / sameMenu --------------------------------------------------

func entryKinds(entries []trayEntry) []trayEntryKind {
	kinds := make([]trayEntryKind, len(entries))
	for i, e := range entries {
		kinds[i] = e.kind
	}
	return kinds
}

func TestTrayEntries_NoGames(t *testing.T) {
	entries := trayEntries("", nil, time.Now())
	if entries[0].kind != entryInfo || entries[0].label != "No games scheduled" {
		t.Fatalf("first entry = %+v, want disabled 'No games scheduled'", entries[0])
	}
	want := []trayEntryKind{entryInfo, entrySeparator, entryManage, entryLogs, entrySeparator, entryQuit}
	if got := entryKinds(entries); !equalKinds(got, want) {
		t.Errorf("kinds = %v, want %v", got, want)
	}
}

func TestTrayEntries_UpcomingGames(t *testing.T) {
	everyDay := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	games := []config.Game{
		{GameName: "A", Enabled: true, Schedules: []config.Schedule{{Days: everyDay, StartTime: "23:59", EndTime: "23:59"}}},
		{GameName: "Off", Enabled: false, Schedules: []config.Schedule{{Days: everyDay, StartTime: "23:59", EndTime: "23:59"}}},
	}
	now := time.Now()
	entries := trayEntries("", schedule.Upcoming(games, 3, now), now)
	if entries[0].kind != entryGame || entries[0].game.GameName != "A" {
		t.Fatalf("first entry = %+v, want game A", entries[0])
	}
	if !strings.HasPrefix(entries[0].label, "A — ") {
		t.Errorf("label = %q, want it to start with %q", entries[0].label, "A — ")
	}
	for _, e := range entries {
		if e.game.GameName == "Off" {
			t.Error("disabled game should not be in the tray menu")
		}
	}
}

func TestTrayEntries_PendingLaunchShowsOnlyCancel(t *testing.T) {
	upcoming := []config.Game{{GameName: "Stardew", Enabled: true}}
	label := "Stardew launching in 7s... — Cancel"

	entries := trayEntries(label, upcoming, time.Now())
	if entries[0].kind != entryCancel {
		t.Fatalf("first entry kind = %v, want entryCancel", entries[0].kind)
	}
	if want := label; entries[0].label != want {
		t.Errorf("label = %q, want %q", entries[0].label, want)
	}
	for _, e := range entries[1:] {
		if e.kind == entryGame || e.kind == entryCancel {
			t.Errorf("unexpected entry while a launch is pending: %+v", e)
		}
	}
}

func TestSameMenu(t *testing.T) {
	base := []trayEntry{{kind: entryGame, label: "A — Today 19:00", game: config.Game{GameName: "A"}}, {kind: entryQuit, label: "Quit"}}
	same := []trayEntry{{kind: entryGame, label: "A — Today 19:00", game: config.Game{GameName: "A"}}, {kind: entryQuit, label: "Quit"}}
	relabeled := []trayEntry{{kind: entryGame, label: "A — Thu 19:00", game: config.Game{GameName: "A"}}, {kind: entryQuit, label: "Quit"}}

	if !sameMenu(base, same) {
		t.Error("identical menus should compare equal")
	}
	if sameMenu(base, relabeled) {
		t.Error("menus with different labels should differ")
	}
	if sameMenu(base, base[:1]) {
		t.Error("menus of different length should differ")
	}

	// The countdown label ticks every second and is updated in place, so it
	// must not count as a menu change on its own.
	c5 := []trayEntry{{kind: entryCancel, label: "X launching in 5s... — Cancel"}}
	c4 := []trayEntry{{kind: entryCancel, label: "X launching in 4s... — Cancel"}}
	if !sameMenu(c5, c4) {
		t.Error("cancel entries differing only in countdown should compare equal")
	}
}

func equalKinds(a, b []trayEntryKind) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---- onClick -----------------------------------------------------------------

func TestOnClick_RunsUntilMenuRebuilt(t *testing.T) {
	item := &systray.MenuItem{ClickedCh: make(chan struct{})}
	gen := make(chan struct{})
	clicks := make(chan struct{}, 4)
	onClick(item, gen, func() { clicks <- struct{}{} })

	for i := 0; i < 2; i++ {
		item.ClickedCh <- struct{}{}
		select {
		case <-clicks:
		case <-time.After(2 * time.Second):
			t.Fatalf("click %d did not run the handler", i+1)
		}
	}

	// Rebuilding the menu closes gen; the old listener must stop, or a
	// click on a stale item would still fire its old action.
	close(gen)
	select {
	case item.ClickedCh <- struct{}{}:
		t.Error("listener still receiving clicks after its menu was rebuilt")
	case <-time.After(200 * time.Millisecond):
	}
}

// ---- openManager -------------------------------------------------------------

// TestHelperProcess is not a real test: it stands in for the --manage
// process, staying open until its stdin is closed (the window closing).
func TestHelperProcess(t *testing.T) {
	if os.Getenv("FL_TRAY_HELPER") == "" {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

func TestOpenManager_OneWindowAtATime(t *testing.T) {
	var mu sync.Mutex
	var started []io.WriteCloser
	orig := startSelf
	startSelf = func(args ...string) (*exec.Cmd, error) {
		if len(args) != 1 || args[0] != launcher.FlagManage {
			t.Errorf("started with %q, want [%s]", args, launcher.FlagManage)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
		cmd.Env = append(os.Environ(), "FL_TRAY_HELPER=1")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		mu.Lock()
		started = append(started, stdin)
		mu.Unlock()
		return cmd, nil
	}
	t.Cleanup(func() {
		startSelf = orig
		mu.Lock()
		defer mu.Unlock()
		for _, w := range started {
			w.Close()
		}
	})
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(started)
	}
	managerOpen := func(tc *trayController) bool {
		tc.mu.Lock()
		defer tc.mu.Unlock()
		return tc.manager != nil
	}

	tc := &trayController{}
	tc.openManager()
	tc.openManager() // already open: must not start a second window
	if n := count(); n != 1 {
		t.Fatalf("started %d manager processes, want 1", n)
	}

	// Closing the window lets the next click open a new one.
	mu.Lock()
	started[0].Close()
	mu.Unlock()
	deadline := time.Now().Add(10 * time.Second)
	for managerOpen(tc) {
		if time.Now().After(deadline) {
			t.Fatal("manager process exited but the tray still thinks it is open")
		}
		time.Sleep(10 * time.Millisecond)
	}
	tc.openManager()
	if n := count(); n != 2 {
		t.Errorf("started %d manager processes after reopening, want 2", n)
	}
}

func TestOpenManager_StartFailureLeavesNoWindow(t *testing.T) {
	orig := startSelf
	startSelf = func(args ...string) (*exec.Cmd, error) { return nil, os.ErrNotExist }
	t.Cleanup(func() { startSelf = orig })

	tc := &trayController{}
	tc.openManager()
	if tc.manager != nil {
		t.Error("a failed start must not be remembered as an open window")
	}
}
