package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
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
const (
	boardStartHour            = 8
	boardEndHour              = 24
	boardHourHeight   float32 = 28
	boardGutterWidth  float32 = 40
	boardDayWidth     float32 = 128
	boardHeaderHeight float32 = 28
)

func boardGridSize() (width, height float32) {
	return boardGutterWidth + boardDayWidth*float32(len(weekDays)), boardHourHeight * float32(boardEndHour-boardStartHour)
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

// timeToBoardY converts an "HH:MM" clock string to a vertical pixel offset
// within the grid, clamped to the visible [boardStartHour, boardEndHour) span.
func timeToBoardY(clock string) float32 {
	var h, m int
	fmt.Sscanf(clock, "%d:%d", &h, &m)
	frac := float64(h) + float64(m)/60
	if frac < boardStartHour {
		frac = boardStartHour
	}
	if frac > boardEndHour {
		frac = boardEndHour
	}
	return float32(frac-boardStartHour) * boardHourHeight
}

func withAlpha(c color.Color, a uint8) color.Color {
	r, g, b, _ := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: a}
}

// buildScheduleBoard is the redesigned window content: a Mon..Sun timetable
// of every armed game's launch windows, plus a plain list of games that
// currently have nowhere on the board to go.
func (ui *GameManagerUI) buildScheduleBoard() fyne.CanvasObject {
	games := ui.appRef.config.Games
	gridWidth, gridHeight := boardGridSize()

	header := container.NewWithoutLayout()
	headerSizer := canvas.NewRectangle(color.Transparent)
	headerSizer.SetMinSize(fyne.NewSize(gridWidth, boardHeaderHeight))
	header.Add(headerSizer)

	todayName := time.Now().Weekday().String()[:3]
	todayIdx := dayIndex(todayName)

	for i, d := range weekDays {
		lbl := canvas.NewText(d, theme.Color(theme.ColorNameForeground))
		lbl.Alignment = fyne.TextAlignCenter
		lbl.TextStyle = fyne.TextStyle{Bold: true}
		if i == todayIdx {
			lbl.Color = colorReady
		}
		header.Add(lbl)
		lbl.Resize(fyne.NewSize(boardDayWidth, boardHeaderHeight))
		lbl.Move(fyne.NewPos(boardGutterWidth+float32(i)*boardDayWidth, 0))
	}
	header.Resize(fyne.NewSize(gridWidth, boardHeaderHeight))

	grid := container.NewWithoutLayout()
	gridSizer := canvas.NewRectangle(color.Transparent)
	gridSizer.SetMinSize(fyne.NewSize(gridWidth, gridHeight))
	grid.Add(gridSizer)

	place := func(o fyne.CanvasObject, x, y, w, h float32) {
		o.Resize(fyne.NewSize(w, h))
		o.Move(fyne.NewPos(x, y))
		grid.Add(o)
	}

	// Hour gridlines + a two-digit hour label in the gutter.
	for hour := boardStartHour; hour <= boardEndHour; hour++ {
		y := float32(hour-boardStartHour) * boardHourHeight
		line := canvas.NewRectangle(withAlpha(colorMuted, 0x22))
		place(line, boardGutterWidth, y, gridWidth-boardGutterWidth, 1)
		if hour < boardEndHour {
			label := canvas.NewText(fmt.Sprintf("%02d", hour), colorMuted)
			label.TextSize = theme.CaptionTextSize()
			label.TextStyle = fyne.TextStyle{Monospace: true}
			place(label, 2, y+2, boardGutterWidth-4, 14)
		}
	}

	// Day column separators.
	for i := 0; i <= len(weekDays); i++ {
		x := boardGutterWidth + float32(i)*boardDayWidth
		line := canvas.NewRectangle(withAlpha(colorMuted, 0x22))
		place(line, x, 0, 1, gridHeight)
	}

	// Today gets a quiet wash so the week orients around "where am I".
	if todayIdx >= 0 {
		hl := canvas.NewRectangle(withAlpha(colorReady, 0x12))
		place(hl, boardGutterWidth+float32(todayIdx)*boardDayWidth, 0, boardDayWidth, gridHeight)
	}

	// The "now" line — amber, the same color the countdown thaws to, since
	// both are about the same idea: where the present moment sits in time.
	now := time.Now()
	nowFrac := float64(now.Hour()) + float64(now.Minute())/60
	if nowFrac >= boardStartHour && nowFrac <= boardEndHour {
		y := float32(nowFrac-boardStartHour) * boardHourHeight
		nowLine := canvas.NewRectangle(colorMeltAmber)
		place(nowLine, boardGutterWidth, y, gridWidth-boardGutterWidth, 2)
	}

	var offBoard []int
	for gi, game := range games {
		if !game.Enabled || len(game.Schedules) == 0 {
			offBoard = append(offBoard, gi)
			continue
		}
		placed := false
		for _, s := range game.Schedules {
			y0 := timeToBoardY(s.StartTime)
			y1 := timeToBoardY(s.EndTime)
			const minBlockHeight = 20
			if y1-y0 < minBlockHeight {
				y1 = y0 + minBlockHeight
			}
			for _, d := range s.Days {
				col := dayIndex(d)
				if col < 0 {
					continue
				}
				placed = true
				x := boardGutterWidth + float32(col)*boardDayWidth + 2
				place(ui.newScheduleBlock(games[gi]), x, y0+1, boardDayWidth-4, y1-y0-2)
			}
		}
		if !placed {
			offBoard = append(offBoard, gi)
		}
	}

	board := container.NewVBox(header, grid)

	content := container.NewVBox(board)
	if len(offBoard) > 0 {
		eyebrow := canvas.NewText("NOT ON THE BOARD", colorMuted)
		eyebrow.TextSize = theme.CaptionTextSize()
		eyebrow.TextStyle = fyne.TextStyle{Bold: true}
		content.Add(widget.NewSeparator())
		content.Add(container.NewPadded(eyebrow))
		content.Add(ui.buildOffBoardList(offBoard))
	}

	footer := container.NewVBox(
		widget.NewSeparator(),
		container.NewHBox(ui.addGameButton(), ui.exportButton(), ui.importButton()),
	)

	// Both directions: the board's a fixed-width grid (7 real day columns,
	// not a reflowing list), so a window narrower than that needs a
	// horizontal scrollbar rather than silently clipping days off the edge.
	scroll := container.NewScroll(content)
	return container.NewBorder(nil, footer, nil, nil, scroll)
}

// newScheduleBlock renders one armed launch window as a small card. Tapping
// it opens the same editor as everywhere else — the board doesn't need its
// own inline controls, since editing is never more than one tap away.
func (ui *GameManagerUI) newScheduleBlock(game Game) fyne.CanvasObject {
	card := canvas.NewRectangle(withAlpha(colorReady, 0x2a))
	card.CornerRadius = 3

	nameText := canvas.NewText(game.GameName, colorReady)
	nameText.TextStyle = fyne.TextStyle{Bold: true}
	nameText.TextSize = theme.CaptionTextSize()

	timeText := canvas.NewText(scheduleSummary(game), colorReady)
	timeText.TextStyle = fyne.TextStyle{Monospace: true}
	timeText.TextSize = theme.CaptionTextSize() - 1

	labels := container.NewPadded(container.NewVBox(nameText, timeText))

	tap := widget.NewButton("", func() {
		ui.showGameEditor(&game, false, func(updated Game) {
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
	tap.Importance = widget.LowImportance

	return container.NewStack(card, labels, tap)
}

// scheduleSummary returns the first schedule's time range, e.g. "19:00-21:00",
// for display on a board block. A game only ever gets one block per distinct
// (schedule, day) pair, so within a single block there's exactly one window.
func scheduleSummary(game Game) string {
	for _, s := range game.Schedules {
		if s.StartTime != "" {
			return s.StartTime + "-" + s.EndTime
		}
	}
	return ""
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
				ui.showGameEditor(&game, false, func(updated Game) {
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
		ui.showGamePicker(func(created Game) {
			ui.appRef.config.Games = append(ui.appRef.config.Games, created)
			ui.appRef.saveConfig()
			fyne.Do(ui.refresh)
		})
	})
}
