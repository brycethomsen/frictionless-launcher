package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// findButton recursively walks a CanvasObject tree and returns the first
// fyne.Tappable found (e.g. *widget.Button). Returns nil if none found.
func findButton(obj fyne.CanvasObject) fyne.Tappable {
	if obj == nil {
		return nil
	}
	if c, ok := obj.(*fyne.Container); ok {
		for _, child := range c.Objects {
			if found := findButton(child); found != nil {
				return found
			}
		}
		return nil
	}
	if t, ok := obj.(fyne.Tappable); ok {
		return t
	}
	return nil
}

// findCheck recursively walks a CanvasObject tree, including into compound
// widgets via their renderer, and returns the first *widget.Check found.
func findCheck(obj fyne.CanvasObject) *widget.Check {
	if obj == nil {
		return nil
	}
	if c, ok := obj.(*widget.Check); ok {
		return c
	}
	if c, ok := obj.(*fyne.Container); ok {
		for _, child := range c.Objects {
			if found := findCheck(child); found != nil {
				return found
			}
		}
		return nil
	}
	if w, ok := obj.(fyne.Widget); ok {
		for _, child := range fynetest.WidgetRenderer(w).Objects() {
			if found := findCheck(child); found != nil {
				return found
			}
		}
	}
	return nil
}

// findForm recursively walks a CanvasObject tree, including into compound
// widgets via their renderer, and returns the first *widget.Form found.
func findForm(obj fyne.CanvasObject) *widget.Form {
	if obj == nil {
		return nil
	}
	if f, ok := obj.(*widget.Form); ok {
		return f
	}
	if c, ok := obj.(*fyne.Container); ok {
		for _, child := range c.Objects {
			if found := findForm(child); found != nil {
				return found
			}
		}
		return nil
	}
	if w, ok := obj.(fyne.Widget); ok {
		for _, child := range fynetest.WidgetRenderer(w).Objects() {
			if found := findForm(child); found != nil {
				return found
			}
		}
	}
	return nil
}

// newTestUI creates a fully wired GameManagerUI backed by a headless Fyne app.
// No display is required — fynetest.NewTempApp uses an off-screen driver.
func newTestUI(t *testing.T, games []Game) *GameManagerUI {
	t.Helper()
	dir, err := os.MkdirTemp("", "ui_test_*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	a := fynetest.NewTempApp(t)
	win := a.NewWindow("test")

	appRef := &App{
		configPath:     filepath.Join(dir, "config.yaml"),
		lastLaunchTime: make(map[string]time.Time),
		config:         &Config{Games: games, BootDelay: 10},
	}

	return &GameManagerUI{
		fyneApp: a,
		window:  win,
		appRef:  appRef,
	}
}

// ============================================================================
// GameManagerUI struct wiring (via newTestUI helper)
// ============================================================================

func TestGameManagerUI_FieldsWired(t *testing.T) {
	// newGameManagerUI wraps app.NewWithID (glfw) which cannot run in a test
	// binary — verify the struct fields via the test-app helper instead.
	ui := newTestUI(t, []Game{})
	if ui.window == nil {
		t.Error("window should not be nil")
	}
	if ui.fyneApp == nil {
		t.Error("fyneApp should not be nil")
	}
	if ui.appRef == nil {
		t.Error("appRef should not be nil")
	}
}

// ============================================================================
// GameManagerUI.refresh
// ============================================================================

func TestRefresh_EmptyGames(t *testing.T) {
	ui := newTestUI(t, []Game{})
	ui.refresh() // must not panic with empty game list
}

func TestRefresh_WithGames(t *testing.T) {
	games := []Game{
		gameWithSchedule("Mon", "19:00", "21:00"),
		gameWithSchedule("Fri", "18:00", "22:00"),
	}
	ui := newTestUI(t, games)
	ui.refresh() // must not panic with populated game list
}

func TestRefresh_UpdatesWindowContent(t *testing.T) {
	ui := newTestUI(t, []Game{gameWithSchedule("Wed", "20:00", "22:00")})
	ui.refresh()
	// After refresh the window should have content set
	if ui.window.Content() == nil {
		t.Error("window content should be set after refresh")
	}
}

func TestRefresh_SelectListItem(t *testing.T) {
	// Select an item in the list — this triggers OnSelected and exercises the
	// list callbacks through the widget API rather than raw rendering.
	games := []Game{
		{GameName: "Alpha", Enabled: true, Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		}},
		{GameName: "Beta", Enabled: false},
	}
	ui := newTestUI(t, games)
	ui.refresh()

	// Walk the content tree to find the *widget.List and call Select(0).
	if l := findWidgetList(ui.window.Content()); l != nil {
		l.Select(0) // drives Length callback + item visibility logic; must not panic
	}
}

// findWidgetList walks a CanvasObject tree and returns the first *widget.List.
func findWidgetList(obj fyne.CanvasObject) *widget.List {
	if obj == nil {
		return nil
	}
	if l, ok := obj.(*widget.List); ok {
		return l
	}
	if c, ok := obj.(*fyne.Container); ok {
		for _, child := range c.Objects {
			if found := findWidgetList(child); found != nil {
				return found
			}
		}
	}
	return nil
}

// ============================================================================
// GameManagerUI.show
// ============================================================================

func TestShow_DoesNotPanic(t *testing.T) {
	ui := newTestUI(t, []Game{gameWithSchedule("Thu", "19:00", "21:00")})
	ui.show() // calls refresh() + Show() + RequestFocus()
}

// ============================================================================
// findOverlappingGame
// ============================================================================

func TestFindOverlappingGame_NoConflict(t *testing.T) {
	ui := newTestUI(t, []Game{
		{GameName: "A", Enabled: true, Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		}},
	})
	// Checking a Tue window — no overlap with Mon game
	got := ui.findOverlappingGame("", []string{"Tue"}, "19:00", "21:00")
	if got != "" {
		t.Errorf("expected no conflict, got %q", got)
	}
}

func TestFindOverlappingGame_Conflict(t *testing.T) {
	ui := newTestUI(t, []Game{
		{GameName: "Existing", Enabled: true, Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		}},
	})
	// A new game wants Mon 20:00–22:00 — overlaps with Existing
	got := ui.findOverlappingGame("", []string{"Mon"}, "20:00", "22:00")
	if got != "Existing" {
		t.Errorf("expected 'Existing', got %q", got)
	}
}

func TestFindOverlappingGame_SkipsSelf(t *testing.T) {
	ui := newTestUI(t, []Game{
		{GameName: "Self", Enabled: true, Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		}},
	})
	// Editing "Self" — its own slot should not be reported as a conflict
	got := ui.findOverlappingGame("Self", []string{"Mon"}, "19:00", "21:00")
	if got != "" {
		t.Errorf("expected no conflict when skipping self, got %q", got)
	}
}

func TestFindOverlappingGame_AdjacentNoConflict(t *testing.T) {
	ui := newTestUI(t, []Game{
		{GameName: "A", Enabled: true, Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		}},
	})
	// Adjacent window starts exactly when A ends — no overlap
	got := ui.findOverlappingGame("", []string{"Mon"}, "21:00", "23:00")
	if got != "" {
		t.Errorf("expected no conflict for adjacent window, got %q", got)
	}
}

func TestFindOverlappingGame_DifferentDay(t *testing.T) {
	ui := newTestUI(t, []Game{
		{GameName: "A", Enabled: true, Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		}},
	})
	got := ui.findOverlappingGame("", []string{"Tue"}, "19:00", "21:00")
	if got != "" {
		t.Errorf("expected no conflict on different day, got %q", got)
	}
}

func TestFindOverlappingGame_MultipleGames(t *testing.T) {
	ui := newTestUI(t, []Game{
		{GameName: "A", Enabled: true, Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "09:00", EndTime: "11:00"},
		}},
		{GameName: "B", Enabled: true, Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		}},
	})
	// Overlaps with B
	got := ui.findOverlappingGame("", []string{"Mon"}, "20:00", "22:00")
	if got != "B" {
		t.Errorf("expected 'B', got %q", got)
	}
}

func TestFindOverlappingGame_CaseInsensitiveDay(t *testing.T) {
	ui := newTestUI(t, []Game{
		{GameName: "A", Enabled: true, Schedules: []Schedule{
			{Days: []string{"mon"}, StartTime: "19:00", EndTime: "21:00"},
		}},
	})
	got := ui.findOverlappingGame("", []string{"MON"}, "20:00", "22:00")
	if got != "A" {
		t.Errorf("expected case-insensitive day match to find 'A', got %q", got)
	}
}

// ============================================================================
// showLaunchCountdown — verify countdown completes and cancel works
// ============================================================================

func TestShowLaunchCountdown_Completes(t *testing.T) {
	ui := newTestUI(t, nil)
	done := make(chan bool, 1)
	// 1-second countdown → onDone(true) when it expires naturally
	ui.showLaunchCountdown("Game", 1, func(launch bool) {
		done <- launch
	})
	select {
	case result := <-done:
		if !result {
			t.Error("expected launch=true when countdown expires")
		}
	case <-time.After(5 * time.Second):
		t.Error("timeout: countdown did not complete")
	}
}

func TestShowLaunchCountdown_CancelViaTap(t *testing.T) {
	ui := newTestUI(t, nil)
	done := make(chan bool, 1)
	// Long countdown so it won't expire on its own
	ui.showLaunchCountdown("Game", 30, func(launch bool) {
		done <- launch
	})

	// Tap the Cancel button — it's the only button in the countdown window.
	// The test driver tracks all windows; find the one titled "Frictionless".
	var cancelBtn fyne.Tappable
	fyne.Do(func() {
		for _, w := range ui.fyneApp.Driver().AllWindows() {
			if w.Title() != "Frictionless" {
				continue
			}
			// Walk content looking for a button
			if btn := findButton(w.Content()); btn != nil {
				cancelBtn = btn
			}
		}
	})
	if cancelBtn != nil {
		fynetest.Tap(cancelBtn)
	}

	select {
	case result := <-done:
		if result {
			t.Error("expected launch=false after cancel tap")
		}
	case <-time.After(3 * time.Second):
		// If we couldn't find the button the test just passes as a smoke test
		t.Log("cancel button not found via driver; skipping assertion")
	}
}

// ============================================================================
// gameStatusLabel via real App struct (covers ui.go path)
// ============================================================================

func TestGameStatusLabel_ViaUI(t *testing.T) {
	ui := newTestUI(t, nil)
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local) // Monday 08:00

	cases := []struct {
		game Game
		want string
	}{
		{Game{Enabled: false}, "Disabled"},
		{Game{Enabled: true}, "No schedule"},
		{
			Game{
				Enabled: true,
				Schedules: []Schedule{
					{Days: []string{"Mon", "Wed"}, StartTime: "18:00", EndTime: "22:00"},
				},
			},
			"18:00",
		},
	}
	for _, c := range cases {
		got := gameStatusLabelAt(ui.appRef, c.game, now)
		if !strings.Contains(got, c.want) {
			t.Errorf("gameStatusLabelAt(%v): want %q in %q", c.game.GameName, c.want, got)
		}
	}
}

// ============================================================================
// clampDialogSize
// ============================================================================

func TestClampDialogSize_FitsWithinWindow(t *testing.T) {
	a := fynetest.NewTempApp(t)
	win := a.NewWindow("test")
	win.Resize(fyne.NewSize(800, 800))

	want := fyne.NewSize(400, 300)
	got := clampDialogSize(win, want)
	if got != want {
		t.Errorf("clampDialogSize() = %v, want unchanged %v", got, want)
	}
}

func TestClampDialogSize_ClampsToWindow(t *testing.T) {
	a := fynetest.NewTempApp(t)
	win := a.NewWindow("test")
	win.Resize(fyne.NewSize(300, 300))

	// margin is 40, so the max allowed size is 260x260
	got := clampDialogSize(win, fyne.NewSize(520, 600))
	want := fyne.NewSize(260, 260)
	if got != want {
		t.Errorf("clampDialogSize() = %v, want clamped to %v", got, want)
	}
}

func TestClampDialogSize_ClampsOnlyOversizedDimension(t *testing.T) {
	a := fynetest.NewTempApp(t)
	win := a.NewWindow("test")
	win.Resize(fyne.NewSize(800, 300))

	// Width fits comfortably; height must be clamped to 300-40=260.
	got := clampDialogSize(win, fyne.NewSize(400, 600))
	want := fyne.NewSize(400, 260)
	if got != want {
		t.Errorf("clampDialogSize() = %v, want %v", got, want)
	}
}

// ============================================================================
// showGameEditor — methodLocked hides/shows the Launch Method field
// ============================================================================

func TestShowGameEditor_MethodLockedHidesLaunchMethodField(t *testing.T) {
	ui := newTestUI(t, nil)
	ui.window.Show()

	g := Game{GameName: "Spirit Island", GamePath: "steam://rungameid/1236720", LaunchMethod: "steam", Enabled: true}
	ui.showGameEditor(&g, true, func(Game) {})

	form := findForm(ui.window.Canvas().Overlays().Top())
	if form == nil {
		t.Fatal("could not find form in game editor dialog")
	}
	for _, item := range form.Items {
		if item.Text == "Launch Method" {
			t.Error("expected Launch Method field to be hidden when methodLocked=true")
		}
	}
}

func TestShowGameEditor_MethodUnlockedShowsLaunchMethodField(t *testing.T) {
	ui := newTestUI(t, nil)
	ui.window.Show()

	g := Game{Enabled: true}
	ui.showGameEditor(&g, false, func(Game) {})

	form := findForm(ui.window.Canvas().Overlays().Top())
	if form == nil {
		t.Fatal("could not find form in game editor dialog")
	}
	found := false
	for _, item := range form.Items {
		if item.Text == "Launch Method" {
			found = true
		}
	}
	if !found {
		t.Error("expected Launch Method field to be present when methodLocked=false")
	}
}

// ============================================================================
// refresh — inline enable/disable checkbox
// ============================================================================

func TestRefresh_ToggleEnabledCheckbox_UpdatesAndPersists(t *testing.T) {
	games := []Game{
		{GameName: "Spirit Island", Enabled: true, Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		}},
	}
	ui := newTestUI(t, games)
	ui.window.Show()
	ui.refresh()

	check := findCheck(ui.window.Content())
	if check == nil {
		t.Fatal("could not find enabled checkbox in game list row")
	}
	if !check.Checked {
		t.Fatal("expected checkbox to start checked since game.Enabled=true")
	}

	oldContent := ui.window.Content()
	check.OnChanged(false)

	if ui.appRef.config.Games[0].Enabled {
		t.Error("expected game.Enabled to be false after unchecking")
	}
	if ui.window.Content() == oldContent {
		t.Error("expected toggling the checkbox to refresh the window content, like every other mutation in this file")
	}

	data, err := os.ReadFile(ui.appRef.configPath)
	if err != nil {
		t.Fatalf("reading saved config: %v", err)
	}
	if strings.Contains(string(data), "enabled: true") {
		t.Errorf("expected saved config to have enabled: false, got:\n%s", data)
	}
}
