package main

import (
	"strings"
	"testing"
	"time"

	"frictionless-launcher/internal/config"
)

// ---- nextScheduleTime ---------------------------------------------------------

func TestNextScheduleTime_TodayLater(t *testing.T) {
	// Wednesday at 08:00 → schedule is Wed 19:00–21:00 → next is same day 19:00
	now := time.Date(2024, 1, 17, 8, 0, 0, 0, time.Local) // Wednesday
	app := appWithGames(nil)
	game := gameWithSchedule("Wed", "19:00", "21:00")

	next, ok := app.nextScheduleTime(game, now)
	if !ok {
		t.Fatal("expected a next schedule time")
	}
	if next.Weekday() != time.Wednesday {
		t.Errorf("expected Wednesday, got %s", next.Weekday())
	}
	if next.Hour() != 19 || next.Minute() != 0 {
		t.Errorf("expected 19:00, got %02d:%02d", next.Hour(), next.Minute())
	}
}

func TestNextScheduleTime_TomorrowWhenTodayPassed(t *testing.T) {
	// Wednesday at 20:00 — today's 19:00 start has already passed.
	// The loop now searches up to daysAhead<=7, so next Wed (7 days out) is found.
	now := time.Date(2024, 1, 17, 20, 0, 0, 0, time.Local) // Wednesday 20:00
	app := appWithGames(nil)
	game := gameWithSchedule("Wed", "19:00", "21:00")

	next, ok := app.nextScheduleTime(game, now)
	if !ok {
		t.Fatal("expected a schedule within the 7-day window")
	}
	if next.Weekday() != time.Wednesday {
		t.Errorf("expected Wednesday, got %s", next.Weekday())
	}
	// next Wed 19:00 is 167 hours after Wed 20:00 — confirm it's ~7 days ahead
	diff := next.Sub(now)
	if diff < 6*24*time.Hour || diff > 8*24*time.Hour {
		t.Errorf("expected ~7 days ahead, got %v", diff)
	}
}

func TestNextScheduleTime_NoSchedule(t *testing.T) {
	app := appWithGames(nil)
	game := config.Game{GameName: "NoSched", Enabled: true}
	_, ok := app.nextScheduleTime(game, time.Now())
	if ok {
		t.Error("game with no schedules should return ok=false")
	}
}

func TestNextScheduleTime_MultipleSchedules(t *testing.T) {
	// Monday 08:00 — game has Mon 19:00 and Fri 18:00; Mon should come first
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local) // Monday
	app := appWithGames(nil)
	game := config.Game{
		GameName: "Multi", Enabled: true,
		Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
			{Days: []string{"Fri"}, StartTime: "18:00", EndTime: "20:00"},
		},
	}
	next, ok := app.nextScheduleTime(game, now)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if next.Weekday() != time.Monday {
		t.Errorf("expected Monday (earlier), got %s", next.Weekday())
	}
}

// ---- nextScheduledGames ---------------------------------------------------------

func TestNextScheduledGames_Ordering(t *testing.T) {
	// Today is Monday 08:00; game A is Mon 19:00, game B is Mon 20:00
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local)
	gameA := config.Game{GameName: "A", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}}}
	gameB := config.Game{GameName: "B", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "20:00", EndTime: "22:00"}}}
	gameC := config.Game{GameName: "C", Enabled: false, Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "10:00", EndTime: "12:00"}}}

	app := appWithGames([]config.Game{gameB, gameA, gameC}) // B first in config, A second

	// Use the real nextScheduleTime but override "now" via a test shim:
	// nextScheduledGames calls time.Now() internally; we verify ordering by looking
	// at real values since the test machines will be running at some time of day.
	// To be deterministic, set game schedules relative to a time we control by
	// exercising nextScheduleTime directly.
	tA, _ := app.nextScheduleTime(gameA, now)
	tB, _ := app.nextScheduleTime(gameB, now)
	if !tA.Before(tB) {
		t.Errorf("expected A (%v) before B (%v)", tA, tB)
	}

	// Verify disabled game is excluded
	games := app.nextScheduledGames(3)
	for _, g := range games {
		if g.GameName == "C" {
			t.Error("disabled game C should not appear in nextScheduledGames")
		}
	}
}

func TestNextScheduledGames_Limit(t *testing.T) {
	app := appWithGames([]config.Game{
		{GameName: "A", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "10:00", EndTime: "12:00"}}},
		{GameName: "B", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Tue"}, StartTime: "10:00", EndTime: "12:00"}}},
		{GameName: "C", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Wed"}, StartTime: "10:00", EndTime: "12:00"}}},
		{GameName: "D", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Thu"}, StartTime: "10:00", EndTime: "12:00"}}},
	})
	got := app.nextScheduledGames(2)
	if len(got) > 2 {
		t.Errorf("expected at most 2, got %d", len(got))
	}
}

func TestNextScheduledGames_Empty(t *testing.T) {
	app := appWithGames([]config.Game{})
	got := app.nextScheduledGames(5)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

// ---- nextScheduleLabelAt ---------------------------------------------------------

func TestNextScheduleLabelAt_Unscheduled(t *testing.T) {
	app := appWithGames(nil)
	got := app.nextScheduleLabelAt(config.Game{Enabled: true}, time.Date(2024, 1, 15, 10, 0, 0, 0, time.Local))
	if got != "unscheduled" {
		t.Errorf("expected 'unscheduled', got %q", got)
	}
}

func TestNextScheduleLabelAt_Today(t *testing.T) {
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local) // Monday 08:00
	app := appWithGames(nil)
	got := app.nextScheduleLabelAt(gameWithSchedule("Mon", "19:00", "21:00"), now)
	if !strings.HasPrefix(got, "Today ") {
		t.Errorf("expected 'Today ...' label, got %q", got)
	}
	if !strings.Contains(got, "19:00") {
		t.Errorf("expected time in label, got %q", got)
	}
}

func TestNextScheduleLabelAt_FutureDay(t *testing.T) {
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local) // Monday
	app := appWithGames(nil)
	got := app.nextScheduleLabelAt(gameWithSchedule("Thu", "19:00", "21:00"), now)
	if !strings.Contains(got, "Thu") {
		t.Errorf("expected 'Thu' in label, got %q", got)
	}
	if !strings.Contains(got, "19:00") {
		t.Errorf("expected '19:00' in label, got %q", got)
	}
}

func TestNextScheduleLabel_DelegatesCorrectly(t *testing.T) {
	game := gameWithSchedule("Mon", "19:00", "21:00")
	app := appWithGames([]config.Game{game})
	// Both wrappers should return the same string for the same game.
	now := time.Now()
	want := app.nextScheduleLabelAt(game, now)
	got := app.nextScheduleLabel(game)
	if got != want {
		t.Errorf("nextScheduleLabel()=%q but nextScheduleLabelAt(now)=%q", got, want)
	}
}

// ---- fadedIcon ---------------------------------------------------------------

func TestFadedIcon_Valid(t *testing.T) {
	// iconData is embedded; test with it directly
	result, err := fadedIcon(iconData, 128)
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
	result, err := fadedIcon(iconData, 0)
	if err != nil {
		t.Fatalf("fadedIcon with alpha=0 failed: %v", err)
	}
	if len(result) == 0 {
		t.Error("fadedIcon returned empty bytes for alpha=0")
	}
}

func TestFadedIcon_FullAlpha(t *testing.T) {
	result, err := fadedIcon(iconData, 255)
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
	full, err := fadedIcon(iconData, 255)
	if err != nil {
		t.Fatal(err)
	}
	half, err := fadedIcon(iconData, 128)
	if err != nil {
		t.Fatal(err)
	}
	if string(full) == string(half) {
		t.Error("alpha=255 and alpha=128 outputs should produce different PNGs")
	}
}
