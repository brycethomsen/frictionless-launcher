package main

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frictionless-launcher/internal/config"
)

// weekDays is the canonical Mon..Sun order used throughout the app —
// schedule checkboxes, overlap validation, and the board's day columns.
var weekDays = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// The schedule board replaces a flat game list with a literal view of the
// week: each armed game appears as a block positioned by day and time,
// because the thing this app actually manages is a schedule, not a list of
// records. Games that aren't currently on the week (disabled, or enabled but
// unscheduled) can't be placed spatially, so they drop to a plain list below
// — the board shows what WILL happen; the list below shows what's waiting.
//
// The visible hour range isn't fixed — computeBoardHourRange fits it to
// whatever's actually scheduled (with an hour of padding on each end), so a
// tight cluster of evening games isn't dwarfed by a mostly-empty 24-hour
// grid, while a midnight or 3am game still pulls the range out to cover it
// rather than ever being clipped off the board. boardAbsoluteStart/EndHour
// are just the hard day boundaries that range can never cross.
// Column widths and hour rows are computed live by boardGridLayout, so
// resizing the window actually reflows the grid instead of just scrolling a
// fixed-size one.
const (
	boardAbsoluteStartHour float64 = 0
	boardAbsoluteEndHour   float64 = 24
	boardGutterWidth       float32 = 40
	boardHeaderHeight      float32 = 28
	boardMinDayWidth       float32 = 90
	boardMinHourHeight     float32 = 14
	// boardMax{DayWidth,HourHeight} are the "comfortable" cell size
	// preferredWindowSize targets when it sizes the window itself — not a
	// live cap on the grid. The grid always fills whatever container it's
	// actually given (see board_layout.go); keeping the window from
	// opening bigger than this is the only thing standing between a tight
	// schedule and a day column stretched into a near-empty band, so it has
	// to happen at the window level, not by the grid refusing to fill it.
	boardMaxDayWidth   float32 = 140
	boardMaxHourHeight float32 = 34
)

// computeBoardHourRange fits the visible grid to the games that are
// actually on it: the earliest start and latest end across every enabled,
// scheduled game, padded by an hour on each side and rounded to whole
// hours, clamped to the real bounds of a day. With nothing on the board yet,
// it falls back to a generic evening window rather than an empty 24 rows.
func computeBoardHourRange(games []config.Game) (start, end float64) {
	haveAny := false
	var minH, maxH float64

	for _, g := range games {
		if !g.Enabled || len(g.Schedules) == 0 {
			continue
		}
		for _, s := range g.Schedules {
			h0, h1 := clockHour(s.StartTime), clockHour(s.EndTime)
			if !haveAny {
				minH, maxH, haveAny = h0, h1, true
			}
			minH = math.Min(minH, h0)
			maxH = math.Max(maxH, h1)
		}
	}

	if !haveAny {
		return 16, 24
	}

	start = math.Max(boardAbsoluteStartHour, math.Floor(minH)-1)
	end = math.Min(boardAbsoluteEndHour, math.Ceil(maxH)+1)
	if end-start < 1 {
		end = start + 1
	}
	return start, end
}

// expandRangeForHeight grows an hour range beyond its tight ±1h padding when
// there's more vertical space available than that range needs at a
// comfortable per-hour height — a 2-hour schedule opened in a window sized
// for the game editor dialog would otherwise stretch just those few hours
// across a lot of space, either sparse or (now that the grid always fills
// its container) uncomfortably tall rows. Showing more hours at a normal
// size fills the same space and reads as a calendar, not a stretched one.
// Growth is split evenly across both ends and never crosses the absolute
// day bounds; padding that doesn't fit on one side is given to the other.
func expandRangeForHeight(start, end, availableHeight, targetHourHeight float64) (float64, float64) {
	if targetHourHeight <= 0 {
		return start, end
	}
	wantSpan := availableHeight / targetHourHeight
	span := end - start
	if wantSpan <= span {
		return start, end
	}
	grow := (wantSpan - span) / 2
	start -= grow
	end += grow

	if start < boardAbsoluteStartHour {
		end += boardAbsoluteStartHour - start
		start = boardAbsoluteStartHour
	}
	if end > boardAbsoluteEndHour {
		start -= end - boardAbsoluteEndHour
		end = boardAbsoluteEndHour
	}
	if start < boardAbsoluteStartHour {
		start = boardAbsoluteStartHour
	}
	return start, end
}

// offBoardIndices returns the indices of games that can't be placed on the
// board: disabled, or enabled with no schedule that lands on a real day.
// Shared by buildScheduleBoard (to build the actual board) and
// preferredWindowSize (to size the window for it) so the two can't drift
// apart on what counts as "on the board."
func offBoardIndices(games []config.Game) []int {
	var indices []int
	for gi, game := range games {
		if !game.Enabled || len(game.Schedules) == 0 {
			indices = append(indices, gi)
			continue
		}
		placed := false
		for _, s := range game.Schedules {
			for _, d := range s.Days {
				if dayIndex(d) >= 0 {
					placed = true
				}
			}
		}
		if !placed {
			indices = append(indices, gi)
		}
	}
	return indices
}

// dayIndex returns weekDays' 0-based index for a "Mon".."Sun" name, or -1.
func dayIndex(day string) int {
	for i, d := range weekDays {
		if strings.EqualFold(d, day) {
			return i
		}
	}
	return -1
}

// clockHour converts an "HH:MM" string to a fractional hour (e.g. "19:30" ->
// 19.5), clamped defensively to the absolute bounds of a day.
func clockHour(clock string) float64 {
	var h, m int
	fmt.Sscanf(clock, "%d:%d", &h, &m)
	frac := float64(h) + float64(m)/60
	if frac < boardAbsoluteStartHour {
		frac = boardAbsoluteStartHour
	}
	if frac > boardAbsoluteEndHour {
		frac = boardAbsoluteEndHour
	}
	return frac
}

func withAlpha(c color.Color, a uint8) color.Color {
	r, g, b, _ := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: a}
}

// buildScheduleBoard is the redesigned window content: a Mon..Sun timetable
// of every armed game's launch windows, plus a plain list of games that
// currently have nowhere on the board to go. The header and grid both use
// custom fyne.Layouts so they reflow live as the window is resized, rather
// than being laid out once at a fixed pixel size.
func (ui *GameManagerUI) buildScheduleBoard() fyne.CanvasObject {
	games := ui.appRef.config.Games
	offBoard := offBoardIndices(games)

	// Built early and reused below (rather than rebuilt) so the same
	// objects are both measured here, to know how much height the grid
	// actually has to fill, and placed in the final layout — no risk of the
	// measurement drifting from what's actually rendered.
	footer := ui.buildFooter()
	var offBoardSection fyne.CanvasObject
	offBoardHeight := float32(0)
	if len(offBoard) > 0 {
		offBoardSection = ui.buildOffBoardSection(offBoard)
		offBoardHeight = offBoardSection.MinSize().Height
	}

	rangeStart, rangeEnd := computeBoardHourRange(games)
	if canvasHeight := ui.window.Canvas().Size().Height; canvasHeight > 0 {
		available := canvasHeight - boardHeaderHeight - footer.MinSize().Height - offBoardHeight
		rangeStart, rangeEnd = expandRangeForHeight(rangeStart, rangeEnd, float64(available), float64(boardMaxHourHeight))
	}

	todayName := time.Now().Weekday().String()[:3]
	todayIdx := dayIndex(todayName)

	headerLayout := &dayHeaderLayout{gutter: boardGutterWidth, days: map[fyne.CanvasObject]int{}}
	header := container.New(headerLayout)
	for i, d := range weekDays {
		lbl := canvas.NewText(d, theme.Color(theme.ColorNameForeground))
		lbl.Alignment = fyne.TextAlignCenter
		lbl.TextStyle = fyne.TextStyle{Bold: true}
		if i == todayIdx {
			lbl.Color = colorReady
		}
		headerLayout.days[lbl] = i
		header.Add(lbl)
	}

	gridLayout := &boardGridLayout{
		gutter: boardGutterWidth, startHour: rangeStart, endHour: rangeEnd,
		items: map[fyne.CanvasObject]gridItem{},
	}
	grid := container.New(gridLayout)
	place := func(o fyne.CanvasObject, it gridItem) {
		gridLayout.items[o] = it
		grid.Add(o)
	}

	// Hour gridlines + a two-digit hour label in the gutter, across
	// whichever range is actually in view (computeBoardHourRange), so a
	// tight evening cluster isn't lost in a mostly-empty 24-row grid.
	for hour := int(rangeStart); hour <= int(math.Ceil(rangeEnd)); hour++ {
		place(canvas.NewRectangle(withAlpha(colorMuted, 0x22)), gridItem{kind: kindHourLine, hourFrom: float64(hour)})
		if hour < int(math.Ceil(rangeEnd)) {
			label := canvas.NewText(fmt.Sprintf("%02d", hour), colorMuted)
			label.TextSize = theme.CaptionTextSize()
			label.TextStyle = fyne.TextStyle{Monospace: true}
			place(label, gridItem{kind: kindHourLabel, hourFrom: float64(hour)})
		}
	}

	// Day column separators.
	for i := 0; i <= len(weekDays); i++ {
		place(canvas.NewRectangle(withAlpha(colorMuted, 0x22)), gridItem{kind: kindDayLine, day: float64(i)})
	}

	// Today gets a quiet wash so the week orients around "where am I".
	if todayIdx >= 0 {
		hl := canvas.NewRectangle(withAlpha(colorReady, 0x12))
		place(hl, gridItem{kind: kindDayHighlight, day: float64(todayIdx)})
	}

	// The "now" line — amber, the same color the countdown thaws to, since
	// both are about the same idea: where the present moment sits in time.
	// It only spans today's column: "now" is a fact about today, not a
	// horizontal claim about every day in the week at once. Omitted rather
	// than clipped if the auto-fit range doesn't happen to cover right now
	// (e.g. it's 3pm and everything's scheduled for the evening).
	if todayIdx >= 0 {
		now := time.Now()
		nowHour := float64(now.Hour()) + float64(now.Minute())/60
		if nowHour >= rangeStart && nowHour <= rangeEnd {
			nowLine := canvas.NewRectangle(colorMeltAmber)
			place(nowLine, gridItem{kind: kindNowLine, day: float64(todayIdx), hourFrom: nowHour})
		}
	}

	// Placements are collected per day first and drawn in a second pass so a
	// short window's minimum visual height (below) can be capped against
	// its neighbor's real start time. Games are validated to never actually
	// overlap, but a stretched-for-legibility short block could still be
	// drawn into a neighbor's space if that cap weren't there — the data
	// would be fine and the board would still look like it wasn't.
	type placement struct {
		game             config.Game
		timeRange        string
		hourFrom, hourTo float64
	}
	placementsByDay := map[int][]*placement{}

	isOffBoard := make(map[int]bool, len(offBoard))
	for _, gi := range offBoard {
		isOffBoard[gi] = true
	}

	hasAnyBlock := false
	for gi, game := range games {
		if isOffBoard[gi] {
			continue
		}
		for _, s := range game.Schedules {
			h0 := clockHour(s.StartTime)
			h1 := clockHour(s.EndTime)
			// timeRange is captured per schedule s, not looked up from the
			// game afterward — a game with more than one window otherwise
			// has every one of its blocks show the same (first) window's
			// time, no matter which window that particular block is.
			timeRange := s.StartTime + "-" + s.EndTime
			for _, d := range s.Days {
				col := dayIndex(d)
				if col < 0 {
					continue
				}
				hasAnyBlock = true
				placementsByDay[col] = append(placementsByDay[col], &placement{game: games[gi], timeRange: timeRange, hourFrom: h0, hourTo: h1})
			}
		}
	}

	const minBlockHours = 0.5
	for col, items := range placementsByDay {
		sort.Slice(items, func(i, j int) bool { return items[i].hourFrom < items[j].hourFrom })
		for i, p := range items {
			renderEnd := p.hourTo
			if renderEnd-p.hourFrom < minBlockHours {
				renderEnd = p.hourFrom + minBlockHours
			}
			ceiling := rangeEnd
			if i+1 < len(items) {
				ceiling = items[i+1].hourFrom
			}
			if renderEnd > ceiling {
				renderEnd = ceiling
			}
			place(container.NewPadded(ui.newScheduleBlock(p.game, p.timeRange)), gridItem{
				kind: kindBlock, day: float64(col), hourFrom: p.hourFrom, hourTo: renderEnd,
			})
		}
	}

	// header+grid share a Border so the grid gets all remaining height
	// stretched to it (Border's center fills leftover space; a plain VBox
	// would only ever give the grid its MinSize, defeating the resize).
	var gridArea fyne.CanvasObject = grid
	if !hasAnyBlock {
		// Nothing armed yet — an empty grid with no explanation is the
		// worst possible first thing to see in an app whose whole pitch is
		// removing friction. Say what to do instead of staying silent.
		headline := canvas.NewText("Nothing scheduled yet", theme.Color(theme.ColorNameForeground))
		headline.Alignment = fyne.TextAlignCenter
		headline.TextStyle = fyne.TextStyle{Bold: true}
		headline.TextSize = theme.TextSubHeadingSize()

		sub := canvas.NewText("Add a game below and give it a launch window", colorMuted)
		sub.Alignment = fyne.TextAlignCenter
		sub.TextSize = theme.CaptionTextSize()

		gridArea = container.NewStack(grid, container.NewCenter(container.NewVBox(headline, sub)))
	}
	board := container.NewBorder(header, nil, nil, nil, gridArea)

	var content fyne.CanvasObject = board
	if offBoardSection != nil {
		content = container.NewBorder(nil, offBoardSection, nil, nil, board)
	}

	return container.NewBorder(nil, footer, nil, nil, content)
}

func (ui *GameManagerUI) buildFooter() fyne.CanvasObject {
	return container.NewVBox(
		widget.NewSeparator(),
		container.NewHBox(ui.addGameButton(), ui.exportButton(), ui.importButton()),
	)
}

// buildOffBoardSection wraps buildOffBoardList with the same separator +
// eyebrow header buildScheduleBoard renders above it, so
// preferredWindowSize can measure the real thing instead of guessing at an
// approximation that could quietly drift out of sync with it.
func (ui *GameManagerUI) buildOffBoardSection(offBoard []int) fyne.CanvasObject {
	eyebrow := canvas.NewText("Not on the board", colorMuted)
	eyebrow.TextSize = theme.CaptionTextSize()
	eyebrow.TextStyle = fyne.TextStyle{Bold: true}
	return container.NewVBox(
		widget.NewSeparator(),
		container.NewPadded(eyebrow),
		ui.buildOffBoardList(offBoard),
	)
}

// preferredWindowSize computes a snug window size for the current config,
// instead of reopening at a fixed default that leaves blank space around a
// tight schedule or feels cramped around a busy one. The grid's
// contribution uses boardMax{DayWidth,HourHeight} — the same "comfortable
// maximum" live resizing caps out at — rather than its bare MinSize floor,
// which is deliberately tiny to allow shrinking while the window is open.
func (ui *GameManagerUI) preferredWindowSize() fyne.Size {
	games := ui.appRef.config.Games
	rangeStart, rangeEnd := computeBoardHourRange(games)
	hourSpan := float32(rangeEnd - rangeStart)

	width := boardGutterWidth + float32(len(weekDays))*boardMaxDayWidth
	height := boardHeaderHeight + hourSpan*boardMaxHourHeight + ui.buildFooter().MinSize().Height

	if offBoard := offBoardIndices(games); len(offBoard) > 0 {
		height += ui.buildOffBoardSection(offBoard).MinSize().Height
	}

	// maxW/maxH are a hard ceiling on the window this app ever opens at —
	// a small tray utility shouldn't default to dominating the screen. Even
	// the widest reasonable schedule (a near-24h span) stays well under a
	// quarter of a 4K display (1920x1080); most real schedules land far
	// smaller than even this ceiling.
	//
	// minW/minH are not an arbitrary "comfortable" number — they're exactly
	// what the game editor dialog needs (editorDialog{Width,Height} plus
	// clampDialogSize's own margin). Shrinking the window below that would
	// force clampDialogSize to squash the editor to fit a window that's
	// hugging a tiny board, which trades one bit of tightness for a much
	// worse one: the dialog you actually use to add a game.
	const minW, maxW float32 = editorDialogWidth + dialogMargin, 1100
	const minH, maxH float32 = editorDialogHeight + dialogMargin, 750
	switch {
	case width < minW:
		width = minW
	case width > maxW:
		width = maxW
	}
	switch {
	case height < minH:
		height = minH
	case height > maxH:
		height = maxH
	}

	return fyne.NewSize(width, height)
}

// newScheduleBlock renders one armed launch window as a small card. Tapping
// it opens the same editor as everywhere else — the board doesn't need its
// own inline controls, since editing is never more than one tap away. The
// time range only shows on hover (see scheduleBlock) so a short window's
// text doesn't overlap the block below it.
func (ui *GameManagerUI) newScheduleBlock(game config.Game, timeRange string) fyne.CanvasObject {
	return newScheduleBlockWidget(game.GameName, timeRange, func() {
		ui.showGameEditor(&game, false, func(updated config.Game) {
			for i := range ui.appRef.config.Games {
				if ui.appRef.config.Games[i].GameName == game.GameName {
					ui.appRef.config.Games[i] = updated
					break
				}
			}
			ui.appRef.saveConfig()
			fyne.Do(ui.refresh)
		}, func() {
			ui.deleteGameByName(game.GameName)
		})
	})
}

// buildOffBoardList renders games at the given indices (disabled, or enabled
// with no schedule) as a plain, compact list — reusing the card-row language
// from the board itself so it reads as one system, not a bolted-on leftover.
func (ui *GameManagerUI) buildOffBoardList(indices []int) fyne.CanvasObject {
	rows := map[fyne.CanvasObject]*gameRow{}

	list := widget.NewList(
		func() int { return len(indices) },
		func() fyne.CanvasObject {
			card := canvas.NewRectangle(color.Transparent)

			nameText := canvas.NewText("", colorMuted)
			nameText.TextStyle = fyne.TextStyle{Bold: true}

			dot := canvas.NewCircle(colorMuted)
			dotWrap := container.NewGridWrap(fyne.NewSize(8, 8), dot)
			statusText := canvas.NewText("", colorMuted)
			statusRow := container.NewHBox(vCenter(dotWrap), statusText)

			textBlock := container.NewVBox(nameText, statusRow)

			check := widget.NewCheck("", nil)
			editBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), nil)
			editBtn.Importance = widget.LowImportance
			deleteBtn := widget.NewButtonWithIcon("", theme.NewColoredResource(theme.DeleteIcon(), theme.ColorNameError), nil)
			deleteBtn.Importance = widget.LowImportance

			content := container.NewBorder(
				nil, nil,
				container.NewCenter(check),
				container.NewCenter(container.NewHBox(editBtn, deleteBtn)),
				vCenter(textBlock),
			)

			root := container.NewStack(card, content)
			rows[root] = &gameRow{card: card, nameText: nameText, dot: dot, wordText: statusText, check: check, editBtn: editBtn, deleteBtn: deleteBtn}
			return root
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= len(indices) {
				return
			}
			gi := indices[id]
			game := ui.appRef.config.Games[gi]
			row := rows[obj]

			row.card.FillColor = colorSurface()
			row.card.Refresh()

			row.nameText.Text = game.GameName
			row.nameText.Color = theme.Color(theme.ColorNameForeground)
			row.nameText.Refresh()

			row.dot.FillColor = colorMuted
			row.dot.Refresh()

			row.wordText.Text = gameStatusLabel(ui.appRef, game)
			row.wordText.Color = colorMuted
			row.wordText.TextSize = theme.CaptionTextSize()
			row.wordText.Refresh()

			row.check.Checked = game.Enabled
			row.check.Refresh()
			row.check.OnChanged = func(checked bool) {
				ui.appRef.config.Games[gi].Enabled = checked
				ui.appRef.saveConfig()
				fyne.Do(ui.refresh)
			}

			row.editBtn.OnTapped = func() {
				ui.showGameEditor(&game, false, func(updated config.Game) {
					ui.appRef.config.Games[gi] = updated
					ui.appRef.saveConfig()
					fyne.Do(ui.refresh)
				}, func() {
					ui.deleteGameByName(game.GameName)
				})
			}
			row.deleteBtn.OnTapped = func() {
				ui.showConfirmDialog("Delete Game",
					fmt.Sprintf("Remove %s from auto-launch?", game.GameName),
					"Delete", widget.DangerImportance,
					func() { ui.deleteGameByName(game.GameName) },
				)
			}
		},
	)

	return list
}

// deleteGameByName removes a game by name, saves, and refreshes. Deletion
// happens from several places now (board block editor, off-board row, its
// editor) so this is the one place that does it.
func (ui *GameManagerUI) deleteGameByName(name string) {
	for i, g := range ui.appRef.config.Games {
		if g.GameName == name {
			ui.appRef.config.Games = append(ui.appRef.config.Games[:i], ui.appRef.config.Games[i+1:]...)
			break
		}
	}
	ui.appRef.saveConfig()
	fyne.Do(ui.refresh)
}

func (ui *GameManagerUI) addGameButton() fyne.CanvasObject {
	return widget.NewButtonWithIcon("Add Game", theme.ContentAddIcon(), func() {
		ui.showGamePicker(func(created config.Game) {
			ui.appRef.config.Games = append(ui.appRef.config.Games, created)
			ui.appRef.saveConfig()
			fyne.Do(ui.refresh)
		})
	})
}
