package tray

import (
	"strings"
	"testing"
	"time"

	"frictionless-launcher/internal/assets"
	"frictionless-launcher/internal/config"
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
	label := "⏳ Stardew launching in 7s... — Cancel"

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
	c5 := []trayEntry{{kind: entryCancel, label: "⏳ X launching in 5s... — Cancel"}}
	c4 := []trayEntry{{kind: entryCancel, label: "⏳ X launching in 4s... — Cancel"}}
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
