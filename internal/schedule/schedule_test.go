package schedule

import (
	"strings"
	"testing"
	"time"

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

func TestInWindow_Inside(t *testing.T) {
	now := time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local)
	if !InWindow(gameWithSchedule("Thu", "19:00", "21:00"), now) {
		t.Error("expected true inside window")
	}
}

func TestInWindow_Outside(t *testing.T) {
	now := time.Date(2024, 1, 18, 22, 0, 0, 0, time.Local)
	if InWindow(gameWithSchedule("Thu", "19:00", "21:00"), now) {
		t.Error("expected false after window end")
	}
}

func TestInWindow_BeforeStart(t *testing.T) {
	now := time.Date(2024, 1, 18, 18, 59, 0, 0, time.Local) // Thursday 18:59
	if InWindow(gameWithSchedule("Thu", "19:00", "21:00"), now) {
		t.Error("expected outside window (before start)")
	}
}

func TestInWindow_WrongDay(t *testing.T) {
	now := time.Date(2024, 1, 17, 19, 30, 0, 0, time.Local) // Wednesday 19:30
	if InWindow(gameWithSchedule("Thu", "19:00", "21:00"), now) {
		t.Error("expected outside window (wrong day)")
	}
}

func TestInWindow_NoSchedules(t *testing.T) {
	now := time.Date(2024, 1, 18, 19, 30, 0, 0, time.Local)
	game := config.Game{GameName: "X", Enabled: true}
	if InWindow(game, now) {
		t.Error("game with no schedules should never be in window")
	}
}

func TestInWindow_CaseInsensitiveDay(t *testing.T) {
	now := time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local) // Thursday
	game := config.Game{
		Enabled:   true,
		Schedules: []config.Schedule{{Days: []string{"thu"}, StartTime: "19:00", EndTime: "21:00"}},
	}
	if !InWindow(game, now) {
		t.Error("day matching should be case-insensitive")
	}
}

func TestInWindow_MultipleWindows(t *testing.T) {
	game := config.Game{
		Enabled: true,
		Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "08:00", EndTime: "10:00"},
			{Days: []string{"Thu"}, StartTime: "19:00", EndTime: "21:00"},
		},
	}
	if !InWindow(game, time.Date(2024, 1, 15, 9, 0, 0, 0, time.Local)) {
		t.Error("expected true for Mon window")
	}
	if !InWindow(game, time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local)) {
		t.Error("expected true for Thu window")
	}
	if InWindow(game, time.Date(2024, 1, 16, 9, 0, 0, 0, time.Local)) {
		t.Error("expected false on Tuesday (no window)")
	}
}

func TestLaunchedInWindow_LaunchedInsideWindow(t *testing.T) {
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
	last := launchTime

	if !LaunchedInWindow(game, last, now) {
		t.Error("launch was in window, should return true")
	}
}

func TestLaunchedInWindow_LaunchedEarlierSameWindow(t *testing.T) {
	game := config.Game{
		GameName: "G", Enabled: true,
		Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}},
	}
	now := time.Date(2024, 1, 15, 20, 55, 0, 0, time.Local) // still inside 19:00-21:00
	last := time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	if !LaunchedInWindow(game, last, now) {
		t.Error("launch at 20:00 falls within the still-open Mon 19:00-21:00 window; should return true")
	}
}

// TestLaunchedInWindow_DifferentWindowSameDayNotSuppressed guards
// against a real bug: a game with two windows on the same day (e.g. a
// morning and an evening slot) launched during the first window must not
// have that suppress the second, distinct window later that day.
func TestLaunchedInWindow_DifferentWindowSameDayNotSuppressed(t *testing.T) {
	game := config.Game{
		GameName: "G", Enabled: true,
		Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "07:00", EndTime: "09:00"},
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		},
	}
	last := time.Date(2024, 1, 15, 7, 30, 0, 0, time.Local) // launched in the morning window
	now := time.Date(2024, 1, 15, 19, 30, 0, 0, time.Local) // now inside the evening window
	if LaunchedInWindow(game, last, now) {
		t.Error("the morning launch must not suppress the distinct evening window")
	}
}

func TestLaunchedInWindow_WrongDay(t *testing.T) {
	game := config.Game{
		GameName: "G", Enabled: true,
		Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}},
	}
	now := time.Date(2024, 1, 16, 20, 0, 0, 0, time.Local) // Tuesday
	last := time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	if LaunchedInWindow(game, last, now) {
		t.Error("schedule is Mon only; today is Tue — should return false")
	}
}

func TestLaunchedInWindow_BeforeWindowStart(t *testing.T) {
	game := config.Game{
		GameName: "G", Enabled: true,
		Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}},
	}
	now := time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	// Launch was at 18:00 — before window start
	last := time.Date(2024, 1, 15, 18, 0, 0, 0, time.Local)
	if LaunchedInWindow(game, last, now) {
		t.Error("launch before window start should return false")
	}
}

func TestNextStart_TodayLater(t *testing.T) {
	// Wednesday at 08:00 → schedule is Wed 19:00–21:00 → next is same day 19:00
	now := time.Date(2024, 1, 17, 8, 0, 0, 0, time.Local) // Wednesday
	game := gameWithSchedule("Wed", "19:00", "21:00")

	next, ok := NextStart(game, now)
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

func TestNextStart_TomorrowWhenTodayPassed(t *testing.T) {
	// Wednesday at 20:00 — today's 19:00 start has already passed.
	// The loop now searches up to daysAhead<=7, so next Wed (7 days out) is found.
	now := time.Date(2024, 1, 17, 20, 0, 0, 0, time.Local) // Wednesday 20:00
	game := gameWithSchedule("Wed", "19:00", "21:00")

	next, ok := NextStart(game, now)
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

func TestNextStart_NoSchedule(t *testing.T) {
	game := config.Game{GameName: "NoSched", Enabled: true}
	_, ok := NextStart(game, time.Now())
	if ok {
		t.Error("game with no schedules should return ok=false")
	}
}

func TestNextStart_MultipleSchedules(t *testing.T) {
	// Monday 08:00 — game has Mon 19:00 and Fri 18:00; Mon should come first
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local) // Monday
	game := config.Game{
		GameName: "Multi", Enabled: true,
		Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
			{Days: []string{"Fri"}, StartTime: "18:00", EndTime: "20:00"},
		},
	}
	next, ok := NextStart(game, now)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if next.Weekday() != time.Monday {
		t.Errorf("expected Monday (earlier), got %s", next.Weekday())
	}
}

func TestUpcoming_Ordering(t *testing.T) {
	// Today is Monday 08:00; game A is Mon 19:00, game B is Mon 20:00
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local)
	gameA := config.Game{GameName: "A", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}}}
	gameB := config.Game{GameName: "B", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "20:00", EndTime: "22:00"}}}
	gameC := config.Game{GameName: "C", Enabled: false, Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "10:00", EndTime: "12:00"}}}

	games := []config.Game{gameB, gameA, gameC} // B first in config, A second

	// Use the real nextScheduleTime but override "now" via a test shim:
	// nextScheduledGames calls time.Now() internally; we verify ordering by looking
	// at real values since the test machines will be running at some time of day.
	// To be deterministic, set game schedules relative to a time we control by
	// exercising nextScheduleTime directly.
	tA, _ := NextStart(gameA, now)
	tB, _ := NextStart(gameB, now)
	if !tA.Before(tB) {
		t.Errorf("expected A (%v) before B (%v)", tA, tB)
	}

	// Verify disabled game is excluded
	for _, g := range Upcoming(games, 3, now) {
		if g.GameName == "C" {
			t.Error("disabled game C should not appear in nextScheduledGames")
		}
	}
}

func TestUpcoming_Limit(t *testing.T) {
	games := []config.Game{
		{GameName: "A", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "10:00", EndTime: "12:00"}}},
		{GameName: "B", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Tue"}, StartTime: "10:00", EndTime: "12:00"}}},
		{GameName: "C", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Wed"}, StartTime: "10:00", EndTime: "12:00"}}},
		{GameName: "D", Enabled: true, Schedules: []config.Schedule{{Days: []string{"Thu"}, StartTime: "10:00", EndTime: "12:00"}}},
	}
	got := Upcoming(games, 2, time.Now())
	if len(got) > 2 {
		t.Errorf("expected at most 2, got %d", len(got))
	}
}

func TestUpcoming_Empty(t *testing.T) {
	got := Upcoming(nil, 5, time.Now())
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

func TestNextLabel_Unscheduled(t *testing.T) {
	got := NextLabel(config.Game{Enabled: true}, time.Date(2024, 1, 15, 10, 0, 0, 0, time.Local))
	if got != "unscheduled" {
		t.Errorf("expected 'unscheduled', got %q", got)
	}
}

func TestNextLabel_Today(t *testing.T) {
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local) // Monday 08:00
	got := NextLabel(gameWithSchedule("Mon", "19:00", "21:00"), now)
	if !strings.HasPrefix(got, "Today ") {
		t.Errorf("expected 'Today ...' label, got %q", got)
	}
	if !strings.Contains(got, "19:00") {
		t.Errorf("expected time in label, got %q", got)
	}
}

func TestNextLabel_FutureDay(t *testing.T) {
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local) // Monday
	got := NextLabel(gameWithSchedule("Thu", "19:00", "21:00"), now)
	if !strings.Contains(got, "Thu") {
		t.Errorf("expected 'Thu' in label, got %q", got)
	}
	if !strings.Contains(got, "19:00") {
		t.Errorf("expected '19:00' in label, got %q", got)
	}
}

func TestOverlaps_Overlapping(t *testing.T) {
	cases := []struct {
		s1, e1, s2, e2 string
		want           bool
		desc           string
	}{
		{"19:00", "21:00", "20:00", "22:00", true, "partial overlap"},
		{"19:00", "21:00", "19:00", "21:00", true, "identical"},
		{"18:00", "22:00", "19:00", "20:00", true, "B inside A"},
		{"19:00", "21:00", "17:00", "23:00", true, "A inside B"},
	}
	for _, c := range cases {
		got := Overlaps(c.s1, c.e1, c.s2, c.e2)
		if got != c.want {
			t.Errorf("[%s] Overlaps(%q,%q,%q,%q) = %v, want %v",
				c.desc, c.s1, c.e1, c.s2, c.e2, got, c.want)
		}
	}
}

func TestOverlaps_NonOverlapping(t *testing.T) {
	cases := []struct {
		s1, e1, s2, e2 string
		desc           string
	}{
		{"19:00", "21:00", "21:00", "23:00", "adjacent (no gap, no overlap)"},
		{"19:00", "21:00", "22:00", "23:00", "gap between"},
		{"22:00", "23:00", "19:00", "21:00", "reversed order"},
	}
	for _, c := range cases {
		got := Overlaps(c.s1, c.e1, c.s2, c.e2)
		if got {
			t.Errorf("[%s] Overlaps(%q,%q,%q,%q) = true, want false",
				c.desc, c.s1, c.e1, c.s2, c.e2)
		}
	}
}

func TestValidClock(t *testing.T) {
	valid := []string{"00:00", "23:59", "19:00", "09:05", "0:0"}
	for _, v := range valid {
		if !ValidClock(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}

	invalid := []string{"", "25:00", "12:60", "12", "12:00:00", "ab:cd", ":30", "12:"}
	for _, v := range invalid {
		if ValidClock(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

// ---- Conflict / WarnOverlaps -------------------------------------------------

func TestConflict(t *testing.T) {
	games := []config.Game{
		{GameName: "A", Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}}},
	}
	if got := Conflict(games, "", []string{"mon"}, "20:00", "22:00"); got != "A" {
		t.Errorf("overlapping window should conflict with A, got %q", got)
	}
	if got := Conflict(games, "A", []string{"Mon"}, "20:00", "22:00"); got != "" {
		t.Errorf("the game being edited must not conflict with itself, got %q", got)
	}
	if got := Conflict(games, "", []string{"Mon"}, "21:00", "23:00"); got != "" {
		t.Errorf("back-to-back windows should not conflict, got %q", got)
	}
}

func TestWarnOverlaps_NoPanic(t *testing.T) {
	WarnOverlaps(nil)
	WarnOverlaps([]config.Game{
		{GameName: "A", Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}}},
		{GameName: "B", Schedules: []config.Schedule{{Days: []string{"Mon"}, StartTime: "20:00", EndTime: "22:00"}}},
	})
}
