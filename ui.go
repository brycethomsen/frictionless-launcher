package main

import (
	"fmt"
	"image/color"
	"io"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"gopkg.in/yaml.v3"

	"frictionless-launcher/internal/config"
	"frictionless-launcher/internal/discovery"
)

// editorDialog{Width,Height} are the game editor's comfortable size — not
// just a Resize() argument, but also what preferredWindowSize uses to set
// the window's floor, so the main window can never shrink so far that
// clampDialogSize is forced to squash the editor to fit it.
const (
	editorDialogWidth  float32 = 520
	editorDialogHeight float32 = 600
	dialogMargin       float32 = 40
)

type GameManagerUI struct {
	fyneApp fyne.App
	window  fyne.Window
	appRef  *App
}

func newGameManagerUI(appRef *App) *GameManagerUI {
	fyneApp := app.NewWithID("com.frictionless.launcher")
	fyneApp.Settings().SetTheme(newFrictionlessTheme())

	win := fyneApp.NewWindow("Frictionless Launcher")
	win.SetCloseIntercept(func() { win.Hide() })

	ui := &GameManagerUI{
		fyneApp: fyneApp,
		window:  win,
		appRef:  appRef,
	}

	win.Resize(ui.preferredWindowSize())
	win.CenterOnScreen()

	return ui
}

// showLaunchCountdown shows a countdown window. Calls onDone(true) if countdown
// completes, onDone(false) if the user cancels. Safe to call from any goroutine.
//
// The numeral and progress rule run from Glacier to Melt Amber as the seconds
// tick down, hitting full warmth exactly at zero — the one moment this app
// visualizes friction (ice) melting away as the game launches. The color
// steps once per tick rather than easing between them: no deceleration
// curve, same as the app's name.
func (ui *GameManagerUI) showLaunchCountdown(gameName string, seconds int, onDone func(launch bool)) {
	cancelled := make(chan struct{})

	const barWidth float32 = 260
	const barHeight float32 = 3

	eyebrow := canvas.NewText("Time to play", colorMuted)
	eyebrow.Alignment = fyne.TextAlignCenter
	eyebrow.TextSize = theme.CaptionTextSize()
	eyebrow.TextStyle = fyne.TextStyle{Bold: true}

	nameText := canvas.NewText(gameName, colorIce)
	nameText.Alignment = fyne.TextAlignCenter
	nameText.TextSize = theme.TextSubHeadingSize()
	nameText.TextStyle = fyne.TextStyle{Bold: true}

	numeral := canvas.NewText("", colorIce)
	numeral.Alignment = fyne.TextAlignCenter
	numeral.TextSize = 64
	numeral.TextStyle = fyne.TextStyle{Monospace: true}

	caption := canvas.NewText("", colorMuted)
	caption.Alignment = fyne.TextAlignCenter
	caption.TextSize = theme.CaptionTextSize()

	track := canvas.NewRectangle(color.NRGBA{R: colorIce.R, G: colorIce.G, B: colorIce.B, A: 0x33})
	track.Resize(fyne.NewSize(barWidth, barHeight))
	track.Move(fyne.NewPos(0, 0))
	fill := canvas.NewRectangle(colorIce)
	fill.Resize(fyne.NewSize(barWidth, barHeight))
	fill.Move(fyne.NewPos(0, 0))
	bar := container.NewWithoutLayout(track, fill)
	bar.Resize(fyne.NewSize(barWidth, barHeight))

	cancelBtn := widget.NewButton("Cancel", func() { close(cancelled) })
	cancelBtn.Importance = widget.LowImportance

	update := func(remaining int) {
		fraction := float64(remaining) / float64(seconds)
		thawed := lerpColor(colorIce, colorMeltAmber, 1-fraction)

		numeral.Text = fmt.Sprintf("%d", remaining)
		numeral.Color = thawed
		numeral.Refresh()

		caption.Text = fmt.Sprintf("second%s until launch", map[bool]string{true: "", false: "s"}[remaining == 1])
		caption.Refresh()

		fill.FillColor = thawed
		fill.Resize(fyne.NewSize(barWidth*float32(fraction), barHeight))
		fill.Refresh()
	}
	update(seconds)

	content := container.NewBorder(nil, cancelBtn, nil, nil, container.NewVBox(
		eyebrow,
		nameText,
		container.NewPadded(numeral),
		caption,
		container.NewCenter(bar),
	))

	var win fyne.Window
	fyne.Do(func() {
		win = ui.fyneApp.NewWindow("Frictionless")
		win.SetContent(content)
		win.Resize(fyne.NewSize(340, 260))
		win.SetFixedSize(true)
		win.CenterOnScreen()
		win.SetCloseIntercept(func() { close(cancelled) })
		win.Show()
	})

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		remaining := seconds
		for {
			select {
			case <-cancelled:
				fyne.Do(func() { win.Close() })
				onDone(false)
				return
			case <-ticker.C:
				remaining--
				if remaining <= 0 {
					fyne.Do(func() { win.Close() })
					onDone(true)
					return
				}
				r := remaining
				fyne.Do(func() { update(r) })
			}
		}
	}()
}

func (ui *GameManagerUI) show() {
	// Resize on every reopen, not just first launch — a schedule that's
	// gotten tighter or looser since the window was last shown (or the
	// window being maximized-then-hidden-then-reopened) shouldn't reopen at
	// a stale size with a band of empty space around the board. This has to
	// happen before refresh(): buildScheduleBoard reads the window's
	// current canvas size to decide how many hours to show, so the size
	// needs to already be the one it's about to be shown at.
	ui.window.Resize(ui.preferredWindowSize())
	ui.refresh()
	ui.window.Show()
	ui.window.RequestFocus()
}

// vCenter vertically centers obj within whatever height its parent stretches
// it to, without pulling it off the left edge horizontally. Fyne's Border and
// HBox layouts stretch side content to the full row height but leave a plain
// VBox pinned to the top of that space — list rows need every column on the
// same vertical centerline, so this is applied consistently to all of them.
func vCenter(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.NewVBox(layout.NewSpacer(), obj, layout.NewSpacer())
}

// gameRow holds direct references to a list row's widgets. The row's nesting
// (card surface + rim, centered columns, status line) got too deep for
// reliable Objects[i].(*T) chasing, so the create/update closures below share
// this instead of re-deriving structure from the generic fyne.CanvasObject
// widget.List hands back.
type gameRow struct {
	card      *canvas.Rectangle
	nameText  *canvas.Text
	dot       *canvas.Circle
	wordText  *canvas.Text
	check     *widget.Check
	editBtn   *widget.Button
	deleteBtn *widget.Button
}

// refresh rebuilds the window content from the current config. The primary
// view is the schedule board (board.go) — a literal week rather than a list
// of records, since a schedule is what this app actually manages.
func (ui *GameManagerUI) refresh() {
	ui.window.SetContent(ui.buildScheduleBoard())
}

func (ui *GameManagerUI) exportButton() fyne.CanvasObject {
	return widget.NewButtonWithIcon("Export", theme.DocumentSaveIcon(), func() {
		d := dialog.NewFileSave(func(f fyne.URIWriteCloser, err error) {
			if err != nil || f == nil {
				return
			}
			defer f.Close()
			data, err := yaml.Marshal(struct {
				Games []config.Game `yaml:"games"`
			}{Games: ui.appRef.config.Games})
			if err != nil {
				dialog.ShowError(err, ui.window)
				return
			}
			if _, err := f.Write(data); err != nil {
				dialog.ShowError(err, ui.window)
			}
		}, ui.window)
		d.SetFileName("frictionless-games.yaml")
		d.Show()
	})
}

func (ui *GameManagerUI) importButton() fyne.CanvasObject {
	return widget.NewButtonWithIcon("Import", theme.FolderOpenIcon(), func() {
		dialog.ShowFileOpen(func(f fyne.URIReadCloser, err error) {
			if err != nil || f == nil {
				return
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil {
				dialog.ShowError(err, ui.window)
				return
			}
			var imported struct {
				Games []config.Game `yaml:"games"`
			}
			if err := yaml.Unmarshal(data, &imported); err != nil {
				dialog.ShowError(fmt.Errorf("invalid YAML: %w", err), ui.window)
				return
			}
			if len(imported.Games) == 0 {
				dialog.ShowError(fmt.Errorf("no games found in file"), ui.window)
				return
			}
			ui.showConfirmDialog("Import Games",
				fmt.Sprintf("Replace all %d current game(s) with %d imported game(s)?", len(ui.appRef.config.Games), len(imported.Games)),
				"Replace", widget.DangerImportance,
				func() {
					ui.appRef.config.Games = imported.Games
					ui.appRef.saveConfig()
					fyne.Do(ui.refresh)
				},
			)
		}, ui.window)
	})
}

func (ui *GameManagerUI) showGamePicker(onSave func(config.Game)) {
	discovered := discovery.DiscoverGames()

	if len(discovered) == 0 {
		// No games found — fall straight through to manual entry
		blank := config.Game{Enabled: true}
		ui.showGameEditor(&blank, false, onSave, nil)
		return
	}

	// Build display names for the list
	names := make([]string, len(discovered)+1)
	for i, g := range discovered {
		names[i] = fmt.Sprintf("%s (%s)", g.Name, g.LaunchMethod)
	}
	names[len(discovered)] = "Enter manually..."

	var pickerDialog *dialog.CustomDialog

	list := widget.NewList(
		func() int { return len(names) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(names[id])
		},
	)

	list.OnSelected = func(id widget.ListItemID) {
		pickerDialog.Hide()
		if id >= len(discovered) {
			blank := config.Game{Enabled: true}
			ui.showGameEditor(&blank, false, onSave, nil)
			return
		}
		d := discovered[id]
		g := config.Game{
			GameName:     d.Name,
			GamePath:     d.GamePath,
			LaunchMethod: d.LaunchMethod,
			Enabled:      true,
		}
		ui.showGameEditor(&g, true, onSave, nil)
	}

	cancelBtn := widget.NewButton("Cancel", func() {
		pickerDialog.Hide()
	})

	content := container.NewBorder(
		widget.NewLabel("Select an installed game or enter manually:"),
		cancelBtn, nil, nil,
		list,
	)

	pickerDialog = dialog.NewCustomWithoutButtons("Add Game", content, ui.window)
	pickerDialog.Resize(clampDialogSize(ui.window, fyne.NewSize(420, 400)))
	pickerDialog.Show()
}

// clampDialogSize caps want to the window's current canvas size (minus a
// margin) so a dialog can never force the window to grow past the screen.
func clampDialogSize(win fyne.Window, want fyne.Size) fyne.Size {
	avail := win.Canvas().Size()

	size := want
	if maxW := avail.Width - dialogMargin; maxW < size.Width {
		size.Width = maxW
	}
	if maxH := avail.Height - dialogMargin; maxH < size.Height {
		size.Height = maxH
	}
	return size
}

// showConfirmDialog shows a Yes/No-style confirmation with the affirmative
// action on the left and Cancel on the right. Fyne's built-in dialog.ShowConfirm
// always puts Cancel on the left, so destructive/primary actions need this
// custom layout instead.
func (ui *GameManagerUI) showConfirmDialog(title, message, confirmLabel string, importance widget.Importance, onConfirm func()) {
	var d *dialog.CustomDialog

	content := widget.NewLabel(message)
	content.Wrapping = fyne.TextWrapWord

	confirmBtn := widget.NewButton(confirmLabel, func() {
		d.Hide()
		onConfirm()
	})
	confirmBtn.Importance = importance
	cancelBtn := widget.NewButton("Cancel", func() { d.Hide() })

	d = dialog.NewCustomWithoutButtons(title, content, ui.window)
	d.SetButtons([]fyne.CanvasObject{confirmBtn, cancelBtn})
	d.Show()
}

// scheduleOverlaps returns true if [start1,end1] and [start2,end2] share any time.
// Times are "HH:MM" strings.
func scheduleOverlaps(start1, end1, start2, end2 string) bool {
	return start1 < end2 && start2 < end1
}

// findOverlappingGame returns the name of any existing game whose schedule overlaps
// with the given days+times, excluding the game being edited (skipName).
func (ui *GameManagerUI) findOverlappingGame(skipName string, days []string, startTime, endTime string) string {
	for _, existing := range ui.appRef.config.Games {
		if existing.GameName == skipName {
			continue
		}
		for _, es := range existing.Schedules {
			for _, eDay := range es.Days {
				for _, newDay := range days {
					if strings.EqualFold(eDay, newDay) && scheduleOverlaps(startTime, endTime, es.StartTime, es.EndTime) {
						return existing.GameName
					}
				}
			}
		}
	}
	return ""
}

var hourOptions = func() []string {
	opts := make([]string, 24)
	for i := range opts {
		opts[i] = fmt.Sprintf("%02d", i)
	}
	return opts
}()

// minuteOptions steps by 5 minutes — fine enough for any real schedule,
// coarse enough to stay a short dropdown instead of 60 options to scroll.
var minuteOptions = []string{"00", "05", "10", "15", "20", "25", "30", "35", "40", "45", "50", "55"}

// newTimeSelect builds an hour+minute dropdown pair for one clock value,
// replacing free-text "type HH:MM and hope it's valid" entry — picking from
// a list can't produce a malformed time, so there's nothing left to
// validate. Minutes snap down to the nearest 5 if the stored value (e.g.
// hand-edited YAML) doesn't already land on one.
func newTimeSelect(clock, defaultHour, defaultMinute string) (hourSel, minSel *widget.Select) {
	h, m := defaultHour, defaultMinute
	if isValidTimeFormat(clock) {
		parts := strings.Split(clock, ":")
		h = parts[0]
		var mi int
		fmt.Sscanf(parts[1], "%d", &mi)
		m = fmt.Sprintf("%02d", mi-mi%5)
	}
	hourSel = widget.NewSelect(hourOptions, nil)
	hourSel.SetSelected(h)
	minSel = widget.NewSelect(minuteOptions, nil)
	minSel.SetSelected(m)
	return hourSel, minSel
}

// showGameEditor opens the game edit form. When methodLocked is true, the
// Launch Method field is omitted entirely — the user already picked a
// discovered game (and thus its launch method) in the picker dialog, so
// re-asking here would just be redundant. onDelete is nil when creating a
// new game (nothing to delete yet); when non-nil, a Delete button appears —
// the board has no separate delete icon of its own, so this is the one path
// for removing a game that's currently on it.
func (ui *GameManagerUI) showGameEditor(game *config.Game, methodLocked bool, onSave func(config.Game), onDelete func()) {
	nameEntry := widget.NewEntry()
	nameEntry.SetText(game.GameName)
	nameEntry.SetPlaceHolder("e.g. Stardew Valley")

	pathEntry := widget.NewEntry()
	pathEntry.SetText(game.GamePath)

	browseBtn := widget.NewButton("Browse...", func() {
		dialog.ShowFileOpen(func(f fyne.URIReadCloser, err error) {
			if err != nil || f == nil {
				return
			}
			pathEntry.SetText(f.URI().Path())
		}, ui.window)
	})

	// pathRow is a single-slot container; we swap its contents based on method
	pathRow := container.NewStack(pathEntry)

	initialMethod := game.LaunchMethod
	if initialMethod == "" {
		initialMethod = "steam"
	}

	updatePathRow := func(method string) {
		switch method {
		case "steam":
			pathEntry.SetPlaceHolder("steam://rungameid/413150")
			pathRow.Objects = []fyne.CanvasObject{pathEntry}
		case "epic":
			pathEntry.SetPlaceHolder("com.epicgames.launcher://apps/APPID/launch")
			pathRow.Objects = []fyne.CanvasObject{pathEntry}
		case "direct":
			pathEntry.SetPlaceHolder("/path/to/game.exe")
			pathRow.Objects = []fyne.CanvasObject{container.NewBorder(nil, nil, nil, browseBtn, pathEntry)}
		}
		pathRow.Refresh()
	}

	methodSelect := widget.NewSelect([]string{"steam", "epic", "direct"}, updatePathRow)
	methodSelect.SetSelected(initialMethod)
	updatePathRow(initialMethod)

	argsEntry := widget.NewEntry()
	argsEntry.SetText(game.LaunchArgs)
	argsEntry.SetPlaceHolder("optional launch arguments")

	allDays := weekDays

	type scheduleRow struct {
		selected            []bool
		startHour, startMin *widget.Select
		endHour, endMin     *widget.Select
	}

	schedulesBox := container.NewVBox()

	var rows []*scheduleRow

	buildRow := func(s config.Schedule) {
		selected := make([]bool, len(allDays))
		for i, day := range allDays {
			for _, d := range s.Days {
				if strings.EqualFold(d, day) {
					selected[i] = true
					break
				}
			}
		}

		// One compact row of day toggles instead of a checkbox grid — every
		// day is visible and scannable at once, and a filled chip reads at a
		// glance the way a lit-up day does on a real weekly planner.
		dayButtons := make([]*widget.Button, len(allDays))
		var refreshDayButtons func()
		for i, day := range allDays {
			i := i
			dayButtons[i] = widget.NewButton(day, func() {
				selected[i] = !selected[i]
				refreshDayButtons()
			})
		}
		refreshDayButtons = func() {
			for i, btn := range dayButtons {
				if selected[i] {
					btn.Importance = widget.HighImportance
				} else {
					btn.Importance = widget.MediumImportance
				}
				btn.Refresh()
			}
		}
		refreshDayButtons()

		startHour, startMin := newTimeSelect(s.StartTime, "19", "00")
		endHour, endMin := newTimeSelect(s.EndTime, "21", "00")

		row := &scheduleRow{selected: selected, startHour: startHour, startMin: startMin, endHour: endHour, endMin: endMin}
		rows = append(rows, row)

		dayRow := container.NewHBox()
		for _, btn := range dayButtons {
			dayRow.Add(btn)
		}

		// Icon-only, same red-tinted-on-tap convention as every other delete
		// action in the app, instead of a full-width "Remove" button — this
		// is a small per-row action, not something that needs its own row.
		removeBtn := widget.NewButtonWithIcon("", theme.NewColoredResource(theme.DeleteIcon(), theme.ColorNameError), nil)
		removeBtn.Importance = widget.LowImportance

		var rowBox *fyne.Container
		removeBtn.OnTapped = func() {
			if len(rows) <= 1 {
				return
			}
			for i, r := range rows {
				if r == row {
					rows = append(rows[:i], rows[i+1:]...)
					break
				}
			}
			schedulesBox.Remove(rowBox)
			schedulesBox.Refresh()
		}

		colon := func() fyne.CanvasObject {
			return widget.NewLabelWithStyle(":", fyne.TextAlignCenter, fyne.TextStyle{Monospace: true})
		}
		timeRow := container.NewHBox(
			startHour, colon(), startMin,
			widget.NewLabel("to"),
			endHour, colon(), endMin,
		)

		rowBox = container.NewVBox(
			widget.NewSeparator(),
			container.NewBorder(nil, nil, nil, removeBtn, dayRow),
			timeRow,
		)
		schedulesBox.Add(rowBox)
	}

	existing := game.Schedules
	if len(existing) == 0 {
		existing = []config.Schedule{{StartTime: "19:00", EndTime: "21:00"}}
	}
	for _, s := range existing {
		buildRow(s)
	}

	addWindowBtn := widget.NewButton("+ Add Time Window", func() {
		buildRow(config.Schedule{StartTime: "19:00", EndTime: "21:00"})
		schedulesBox.Refresh()
	})

	enabledCheck := widget.NewCheck("Auto-launch enabled", nil)
	enabledCheck.Checked = game.Enabled

	formItems := []*widget.FormItem{
		widget.NewFormItem("Game Name", nameEntry),
		widget.NewFormItem("Path / URL", pathRow),
	}
	if !methodLocked {
		formItems = append(formItems, widget.NewFormItem("Launch Method", methodSelect))
	}
	formItems = append(formItems, widget.NewFormItem("Launch Args", argsEntry))

	form := container.NewVBox(
		widget.NewForm(formItems...),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Time Windows", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		schedulesBox,
		addWindowBtn,
		widget.NewSeparator(),
		enabledCheck,
	)

	var d *dialog.CustomDialog

	doSave := func() {
		if nameEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("game name is required"), ui.window)
			return
		}
		if pathEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("game path is required"), ui.window)
			return
		}

		var schedules []config.Schedule
		for ri, row := range rows {
			selectedDays := []string{}
			for i, on := range row.selected {
				if on {
					selectedDays = append(selectedDays, allDays[i])
				}
			}
			if len(selectedDays) == 0 {
				dialog.ShowError(fmt.Errorf("time window %d: select at least one day", ri+1), ui.window)
				return
			}
			// No HH:MM format check needed here — start/end come from Select
			// dropdowns, which can only ever hold a valid option.
			startTime := row.startHour.Selected + ":" + row.startMin.Selected
			endTime := row.endHour.Selected + ":" + row.endMin.Selected
			for pi, prev := range schedules {
				for _, pd := range prev.Days {
					for _, nd := range selectedDays {
						if strings.EqualFold(pd, nd) && scheduleOverlaps(startTime, endTime, prev.StartTime, prev.EndTime) {
							dialog.ShowError(fmt.Errorf("time windows %d and %d overlap on %s", pi+1, ri+1, nd), ui.window)
							return
						}
					}
				}
			}
			schedules = append(schedules, config.Schedule{
				Days:      selectedDays,
				StartTime: startTime,
				EndTime:   endTime,
			})
		}

		for _, s := range schedules {
			if conflict := ui.findOverlappingGame(game.GameName, s.Days, s.StartTime, s.EndTime); conflict != "" {
				dialog.ShowError(fmt.Errorf("a time window overlaps with %s", conflict), ui.window)
				return
			}
		}

		d.Hide()
		onSave(config.Game{
			GameName:     nameEntry.Text,
			GamePath:     pathEntry.Text,
			LaunchMethod: methodSelect.Selected,
			LaunchArgs:   argsEntry.Text,
			Enabled:      enabledCheck.Checked,
			Schedules:    schedules,
		})
		fyne.Do(ui.refresh)
	}

	saveBtn := widget.NewButton("Save", doSave)
	saveBtn.Importance = widget.HighImportance
	buttons := container.NewHBox(
		saveBtn,
		widget.NewButton("Cancel", func() { d.Hide() }),
	)
	if onDelete != nil {
		deleteBtn := widget.NewButton("Delete", func() {
			ui.showConfirmDialog("Delete Game",
				fmt.Sprintf("Remove %s from auto-launch?", game.GameName),
				"Delete", widget.DangerImportance,
				func() {
					d.Hide()
					onDelete()
				},
			)
		})
		deleteBtn.Importance = widget.DangerImportance
		buttons.Add(deleteBtn)
	}

	scroll := container.NewVScroll(form)
	scroll.SetMinSize(fyne.NewSize(480, 400))

	content := container.NewBorder(nil, buttons, nil, nil, scroll)

	d = dialog.NewCustomWithoutButtons("Game Schedule", content, ui.window)
	d.Resize(clampDialogSize(ui.window, fyne.NewSize(editorDialogWidth, editorDialogHeight)))
	d.Show()
}

// gameStatusLabel returns the short status shown next to a game's name in
// the list: "Disabled", "No schedule", or its next upcoming launch time.
func gameStatusLabel(app *App, game config.Game) string {
	return gameStatusLabelAt(app, game, time.Now())
}

func gameStatusLabelAt(app *App, game config.Game, now time.Time) string {
	if !game.Enabled {
		return "Disabled"
	}
	if len(game.Schedules) == 0 {
		return "No schedule"
	}
	return app.nextScheduleLabelAt(game, now)
}

func isValidTimeFormat(t string) bool {
	parts := strings.Split(t, ":")
	if len(parts) != 2 {
		return false
	}

	var hour, minute int
	if _, err := fmt.Sscanf(t, "%d:%d", &hour, &minute); err != nil {
		return false
	}

	return hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59
}
