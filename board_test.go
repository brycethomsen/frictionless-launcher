package main

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	fynetest "fyne.io/fyne/v2/test"

	"frictionless-launcher/internal/config"
)

// containsText recursively walks a CanvasObject tree, including into
// compound widgets via their renderer, looking for a *canvas.Text whose
// content contains substr.
func containsText(obj fyne.CanvasObject, substr string) bool {
	if obj == nil {
		return false
	}
	if txt, ok := obj.(*canvas.Text); ok && strings.Contains(txt.Text, substr) {
		return true
	}
	if c, ok := obj.(*fyne.Container); ok {
		for _, child := range c.Objects {
			if containsText(child, substr) {
				return true
			}
		}
		return false
	}
	if w, ok := obj.(fyne.Widget); ok {
		for _, child := range fynetest.WidgetRenderer(w).Objects() {
			if containsText(child, substr) {
				return true
			}
		}
	}
	return false
}

func TestClockHour_ParsesAndClamps(t *testing.T) {
	cases := []struct {
		clock string
		want  float64
	}{
		{"00:00", 0},
		{"19:30", 19.5},
		{"23:59", 23 + 59.0/60},
	}
	for _, c := range cases {
		if got := clockHour(c.clock); got != c.want {
			t.Errorf("clockHour(%q) = %v, want %v", c.clock, got, c.want)
		}
	}
}

func TestDayIndex(t *testing.T) {
	if got := dayIndex("wed"); got != 2 {
		t.Errorf("dayIndex(%q) = %d, want 2", "wed", got)
	}
	if got := dayIndex("WED"); got != 2 {
		t.Errorf("dayIndex should be case-insensitive, got %d, want 2", got)
	}
	if got := dayIndex("nope"); got != -1 {
		t.Errorf("dayIndex(unknown) = %d, want -1", got)
	}
}

// TestBoardGridLayout_ScalesWithSize verifies a block positioned at day=2,
// hours [12,13) lands at proportionally different pixels for two different
// container sizes — the whole point of replacing the old fixed-pixel board
// with a real fyne.Layout is that resizing the window actually moves things.
func TestBoardGridLayout_ScalesWithSize(t *testing.T) {
	block := canvas.NewRectangle(colorReady)
	layout := &boardGridLayout{
		gutter: 40, startHour: 0, endHour: 24,
		items: map[fyne.CanvasObject]gridItem{
			block: {kind: kindBlock, day: 2, hourFrom: 12, hourTo: 13},
		},
	}

	small := fyne.NewSize(180, 240) // gutter(40) + 7 cols of 20px; 24h over 240px = 10px/h
	layout.Layout([]fyne.CanvasObject{block}, small)
	smallPos, smallSize := block.Position(), block.Size()

	large := fyne.NewSize(320, 480) // 7 cols of 40px; 24h over 480px = 20px/h
	layout.Layout([]fyne.CanvasObject{block}, large)
	largePos, largeSize := block.Position(), block.Size()

	if largePos.X <= smallPos.X {
		t.Errorf("expected block to move right in a wider container: small.X=%v large.X=%v", smallPos.X, largePos.X)
	}
	if largeSize.Width <= smallSize.Width {
		t.Errorf("expected block to widen in a wider container: small.W=%v large.W=%v", smallSize.Width, largeSize.Width)
	}
	if largeSize.Height <= smallSize.Height {
		t.Errorf("expected block to grow taller in a taller container: small.H=%v large.H=%v", smallSize.Height, largeSize.Height)
	}

	// Day 2 (Wed) of 7 columns should sit at gutter + 2*dayWidth regardless
	// of overall size.
	wantDayWidth := (large.Width - layout.gutter) / 7
	wantX := layout.gutter + 2*wantDayWidth
	if largePos.X != wantX {
		t.Errorf("block.X = %v, want %v (day column 2 at the larger size)", largePos.X, wantX)
	}
}

// TestBoardGridLayout_NowLineSpansOnlyOneColumn guards the specific fix
// requested: the "now" line must span exactly one day column, not the full
// width of the grid.
func TestBoardGridLayout_NowLineSpansOnlyOneColumn(t *testing.T) {
	nowLine := canvas.NewRectangle(colorMeltAmber)
	layout := &boardGridLayout{
		gutter: 40, startHour: 0, endHour: 24,
		items: map[fyne.CanvasObject]gridItem{
			nowLine: {kind: kindNowLine, day: 3, hourFrom: 12},
		},
	}

	size := fyne.NewSize(320, 480)
	layout.Layout([]fyne.CanvasObject{nowLine}, size)

	dayWidth := (size.Width - layout.gutter) / 7
	if nowLine.Size().Width != dayWidth {
		t.Errorf("now line width = %v, want exactly one day column (%v)", nowLine.Size().Width, dayWidth)
	}
	if fullWidth := size.Width - layout.gutter; nowLine.Size().Width >= fullWidth {
		t.Errorf("now line width = %v should be narrower than the full grid width %v", nowLine.Size().Width, fullWidth)
	}
}

// TestBoardGridLayout_FullDayRangeNeverClips verifies a late-night schedule
// (23:00-23:59) and a just-after-midnight one (00:00-01:00) both land within
// the grid's visible bounds when given the absolute full-day range — the
// layout math itself must never clip, regardless of what range
// computeBoardHourRange decides to actually show.
func TestBoardGridLayout_FullDayRangeNeverClips(t *testing.T) {
	lateNight := canvas.NewRectangle(colorReady)
	afterMidnight := canvas.NewRectangle(colorReady)
	layout := &boardGridLayout{
		gutter: 40, startHour: boardAbsoluteStartHour, endHour: boardAbsoluteEndHour,
		items: map[fyne.CanvasObject]gridItem{
			lateNight:     {kind: kindBlock, day: 5, hourFrom: clockHour("23:00"), hourTo: clockHour("23:59")},
			afterMidnight: {kind: kindBlock, day: 0, hourFrom: clockHour("00:00"), hourTo: clockHour("01:00")},
		},
	}

	size := fyne.NewSize(980, 700)
	layout.Layout([]fyne.CanvasObject{lateNight, afterMidnight}, size)

	for name, o := range map[string]*canvas.Rectangle{"23:00-23:59": lateNight, "00:00-01:00": afterMidnight} {
		pos, sz := o.Position(), o.Size()
		if pos.Y < 0 || pos.Y+sz.Height > size.Height {
			t.Errorf("%s block at y=%v height=%v falls outside grid height %v", name, pos.Y, sz.Height, size.Height)
		}
		if sz.Height <= 0 {
			t.Errorf("%s block has non-positive height %v", name, sz.Height)
		}
	}
}

// ============================================================================
// computeBoardHourRange — auto-fit to whatever's actually scheduled
// ============================================================================

func TestComputeBoardHourRange_FitsWithOneHourPadding(t *testing.T) {
	// The exact example from the ask: games from 5pm-10pm should show a
	// 4pm-11pm board, not a fixed 24-hour grid.
	games := []config.Game{
		{GameName: "A", Enabled: true, Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "17:00", EndTime: "19:00"},
		}},
		{GameName: "B", Enabled: true, Schedules: []config.Schedule{
			{Days: []string{"Wed"}, StartTime: "20:00", EndTime: "22:00"},
		}},
	}
	start, end := computeBoardHourRange(games)
	if start != 16 || end != 23 {
		t.Errorf("computeBoardHourRange() = (%v, %v), want (16, 23)", start, end)
	}
}

func TestComputeBoardHourRange_MidnightGameExpandsRange(t *testing.T) {
	// A game scheduled right at midnight must still pull the range out to
	// cover it — the auto-fit relaxes the grid when schedules are tight, but
	// must never clip a real schedule to do it.
	games := []config.Game{
		{GameName: "NightOwl", Enabled: true, Schedules: []config.Schedule{
			{Days: []string{"Fri"}, StartTime: "00:00", EndTime: "01:30"},
		}},
	}
	start, end := computeBoardHourRange(games)
	if start != 0 {
		t.Errorf("start = %v, want 0 (clamped, not negative, for a midnight game)", start)
	}
	if end < 1.5 {
		t.Errorf("end = %v, want at least 1.5 to cover the 00:00-01:30 game", end)
	}
}

func TestComputeBoardHourRange_IgnoresDisabledAndUnscheduledGames(t *testing.T) {
	games := []config.Game{
		{GameName: "Disabled", Enabled: false, Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "02:00", EndTime: "03:00"},
		}},
		{GameName: "Unscheduled", Enabled: true},
		{GameName: "Active", Enabled: true, Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "18:00", EndTime: "19:00"},
		}},
	}
	start, end := computeBoardHourRange(games)
	if start != 17 || end != 20 {
		t.Errorf("computeBoardHourRange() = (%v, %v), want (17, 20) — disabled/unscheduled games shouldn't widen the range", start, end)
	}
}

func TestComputeBoardHourRange_EmptyBoardFallsBackToEvening(t *testing.T) {
	start, end := computeBoardHourRange(nil)
	if start >= end {
		t.Fatalf("expected a valid non-empty default range, got (%v, %v)", start, end)
	}
	if start < boardAbsoluteStartHour || end > boardAbsoluteEndHour {
		t.Errorf("default range (%v, %v) exceeds absolute day bounds [%v, %v]", start, end, boardAbsoluteStartHour, boardAbsoluteEndHour)
	}
}

// ============================================================================
// The grid always fills its container — no live cap. Keeping the window a
// reasonable size on open is preferredWindowSize's job (it targets
// boardMax{DayWidth,HourHeight}); if the window ends up bigger than that for
// any reason, the grid fills it rather than leaving a dead margin.
// ============================================================================

func TestDayColumnWidth_FillsWideWindow(t *testing.T) {
	got := dayColumnWidth(3000, 40) // a much wider window than the board needs
	want := (3000 - float32(40)) / 7
	if got != want {
		t.Errorf("dayColumnWidth(3000, 40) = %v, want it to fill the width uncapped (%v)", got, want)
	}
}

func TestDayColumnWidth_FillsNarrowWindow(t *testing.T) {
	got := dayColumnWidth(700, 40)
	want := (700 - float32(40)) / 7
	if got != want {
		t.Errorf("dayColumnWidth(700, 40) = %v, want %v", got, want)
	}
}

func TestBoardGridLayout_BlockFillsLargeContainer(t *testing.T) {
	block := canvas.NewRectangle(colorReady)
	layout := &boardGridLayout{
		gutter: 40, startHour: 17, endHour: 22, // a tight 5-hour schedule
		items: map[fyne.CanvasObject]gridItem{
			block: {kind: kindBlock, day: 0, hourFrom: 19, hourTo: 20},
		},
	}

	// A much bigger container than a 5-hour, single-day board strictly
	// needs — the block's cell should still scale to fill it, not leave a
	// dead margin below/beside a capped-size block.
	layout.Layout([]fyne.CanvasObject{block}, fyne.NewSize(2400, 1600))

	wantHeight := float32(1600) / 5 // one of five hour rows filling the full height
	wantWidth := (float32(2400) - 40) / 7
	const epsilon = 0.01
	if h := block.Size().Height; h < wantHeight-epsilon {
		t.Errorf("block height = %v, want it to fill its share of the container (%v), not stay capped", h, wantHeight)
	}
	if w := block.Size().Width; w < wantWidth-epsilon {
		t.Errorf("block width = %v, want it to fill its share of the container (%v), not stay capped", w, wantWidth)
	}
}

// ============================================================================
// Empty-state message — the board shouldn't be silent on first run
// ============================================================================

func TestBuildScheduleBoard_EmptyStateMessageWhenNothingScheduled(t *testing.T) {
	ui := newTestUI(t, nil)
	content := ui.buildScheduleBoard()

	if !containsText(content, "Nothing scheduled yet") {
		t.Error("expected an empty-state message when no games are on the board")
	}
}

func TestBuildScheduleBoard_NoEmptyStateWhenGameIsScheduled(t *testing.T) {
	ui := newTestUI(t, []config.Game{
		{GameName: "Stardew Valley", Enabled: true, Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		}},
	})
	content := ui.buildScheduleBoard()

	if containsText(content, "Nothing scheduled yet") {
		t.Error("did not expect the empty-state message when a game is scheduled")
	}
}

// ============================================================================
// preferredWindowSize — resize on reopen, no leftover blank space
// ============================================================================

func TestPreferredWindowSize_WithinBounds(t *testing.T) {
	ui := newTestUI(t, nil)
	size := ui.preferredWindowSize()

	minW := editorDialogWidth + dialogMargin
	minH := editorDialogHeight + dialogMargin
	if size.Width < minW || size.Width > 1100 {
		t.Errorf("width = %v, want within [%v, 1100]", size.Width, minW)
	}
	if size.Height < minH || size.Height > 750 {
		t.Errorf("height = %v, want within [%v, 750]", size.Height, minH)
	}
}

func TestPreferredWindowSize_GrowsWithHourSpan(t *testing.T) {
	tight := newTestUI(t, []config.Game{
		{GameName: "A", Enabled: true, Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "20:00"},
		}},
	})
	wide := newTestUI(t, []config.Game{
		{GameName: "A", Enabled: true, Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "02:00", EndTime: "03:00"},
		}},
		{GameName: "B", Enabled: true, Schedules: []config.Schedule{
			{Days: []string{"Tue"}, StartTime: "23:00", EndTime: "23:30"},
		}},
	})

	tightHeight := tight.preferredWindowSize().Height
	wideHeight := wide.preferredWindowSize().Height
	if wideHeight <= tightHeight {
		t.Errorf("expected a wider hour span to want a taller window: tight=%v wide=%v", tightHeight, wideHeight)
	}
}

func TestPreferredWindowSize_TallerWithOffBoardGames(t *testing.T) {
	// Base case is a 15-hour schedule specifically so its own height lands
	// comfortably inside [minH, maxH] with margin on both sides — if both
	// cases clamped to the same floor (now driven by the editor dialog's
	// own minimum size, not an arbitrary number), adding an off-board game
	// wouldn't show up as a difference.
	base := []config.Game{
		{GameName: "A", Enabled: true, Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "06:00", EndTime: "21:00"},
		}},
	}
	noOffBoard := newTestUI(t, base)
	withOffBoard := newTestUI(t, append(append([]config.Game{}, base...), config.Game{GameName: "B", Enabled: false}))

	noHeight := noOffBoard.preferredWindowSize().Height
	withHeight := withOffBoard.preferredWindowSize().Height
	if withHeight <= noHeight {
		t.Errorf("expected an off-board game to make the preferred window taller: without=%v with=%v", noHeight, withHeight)
	}
}

// ============================================================================
// Regression: two short, valid (non-overlapping), back-to-back schedules
// must never render as overlapping blocks. The minimum-block-height stretch
// in buildScheduleBoard (for legibility of very short windows) must never
// intrude into a neighboring block's real start time.
// ============================================================================

func TestBuildScheduleBoard_ShortAdjacentBlocksDoNotVisuallyOverlap(t *testing.T) {
	// Both games are valid, non-overlapping 15-minute windows back to back
	// on the same day: 19:00-19:15 and 19:15-19:30. Neither is long enough
	// to clear the 30-minute minimum render height on its own.
	games := []config.Game{
		{GameName: "First", Enabled: true, Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "19:15"},
		}},
		{GameName: "Second", Enabled: true, Schedules: []config.Schedule{
			{Days: []string{"Mon"}, StartTime: "19:15", EndTime: "19:30"},
		}},
	}
	ui := newTestUI(t, games)
	content := ui.buildScheduleBoard()
	content.Resize(fyne.NewSize(980, 700))

	// Each block is wrapped (container.NewPadded) before being placed on the
	// grid, and canvas object Position() is relative to its immediate
	// parent — so the thing to compare is the padded wrapper the grid
	// layout actually Move()s/Resize()s, not the scheduleBlock's own
	// position (which is relative to that wrapper, not the shared grid).
	first := findScheduleBlockWrapper(t, content, "First")
	second := findScheduleBlockWrapper(t, content, "Second")

	firstBottom := first.Position().Y + first.Size().Height
	if firstBottom > second.Position().Y {
		t.Errorf("First's rendered block (bottom=%v) overlaps Second's start (top=%v) — a short block's legibility stretch must be capped at its neighbor's real start time",
			firstBottom, second.Position().Y)
	}
}

// findScheduleBlockWrapper locates the direct parent (the container.NewPadded
// wrapper) of the *scheduleBlock for the given game name — that wrapper, not
// the block itself, is what the grid's custom layout positions, so it's the
// only thing whose Position() is comparable across two different blocks.
func findScheduleBlockWrapper(t *testing.T, obj fyne.CanvasObject, name string) fyne.CanvasObject {
	t.Helper()
	var found fyne.CanvasObject
	var walk func(fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		if o == nil || found != nil {
			return
		}
		if c, ok := o.(*fyne.Container); ok {
			for _, child := range c.Objects {
				if b, ok := child.(*scheduleBlock); ok && b.nameText.Text == name {
					found = o
					return
				}
			}
			for _, child := range c.Objects {
				walk(child)
			}
			return
		}
		if w, ok := o.(fyne.Widget); ok {
			for _, child := range fynetest.WidgetRenderer(w).Objects() {
				walk(child)
			}
		}
	}
	walk(obj)
	if found == nil {
		t.Fatalf("could not find schedule block wrapper for %q", name)
	}
	return found
}

// ============================================================================
// expandRangeForHeight — grow the visible range beyond ±1h padding to fill
// available vertical space at a comfortable per-hour size
// ============================================================================

func TestExpandRangeForHeight_NoOpWhenRangeAlreadyFills(t *testing.T) {
	// 4-hour range at 34px/h needs 136px; only 100px available, so there's
	// no room to grow into — the tight range should come back unchanged.
	start, end := expandRangeForHeight(18, 22, 100, 34)
	if start != 18 || end != 22 {
		t.Errorf("expandRangeForHeight() = (%v, %v), want unchanged (18, 22)", start, end)
	}
}

func TestExpandRangeForHeight_GrowsSymmetrically(t *testing.T) {
	// 2-hour range at 34px/h wants 340px of space (10h): grow by 4h each side.
	start, end := expandRangeForHeight(18, 20, 340, 34)
	if start != 14 || end != 24 {
		t.Errorf("expandRangeForHeight() = (%v, %v), want (14, 24)", start, end)
	}
}

func TestExpandRangeForHeight_OverflowAtStartGoesToEnd(t *testing.T) {
	// Range starting near midnight: symmetric growth would push start below
	// 0, so that unused growth should extend the end further instead of
	// just being lost, as long as it still fits within the day.
	start, end := expandRangeForHeight(1, 3, 340, 34) // wants a 10h span
	if start != 0 {
		t.Errorf("start = %v, want clamped to 0", start)
	}
	if end != 10 {
		t.Errorf("end = %v, want 10 (the full 10h span, with start's unused growth given to end)", end)
	}
}

func TestExpandRangeForHeight_OverflowAtEndGoesToStart(t *testing.T) {
	// Symmetric case: range ending near midnight gives its overflow to start.
	start, end := expandRangeForHeight(21, 23, 340, 34) // wants a 10h span
	if end != 24 {
		t.Errorf("end = %v, want clamped to 24", end)
	}
	if start != 14 {
		t.Errorf("start = %v, want 14 (the full 10h span, with end's unused growth given to start)", start)
	}
}

func TestExpandRangeForHeight_NeverExceedsAbsoluteDayBounds(t *testing.T) {
	// Demanding way more space than a 24h day has must clamp to exactly
	// the full day, not overshoot in either direction.
	start, end := expandRangeForHeight(10, 14, 100000, 34)
	if start != boardAbsoluteStartHour || end != boardAbsoluteEndHour {
		t.Errorf("expandRangeForHeight() = (%v, %v), want the full day bounds (%v, %v)",
			start, end, boardAbsoluteStartHour, boardAbsoluteEndHour)
	}
}
