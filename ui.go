package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"gopkg.in/yaml.v3"
)

type GameManagerUI struct {
	fyneApp fyne.App
	window  fyne.Window
	appRef  *App
}

func newGameManagerUI(appRef *App) *GameManagerUI {
	fyneApp := app.NewWithID("com.frictionless.launcher")

	win := fyneApp.NewWindow("Frictionless Launcher")
	win.Resize(fyne.NewSize(640, 640))
	win.CenterOnScreen()
	win.SetCloseIntercept(func() { win.Hide() })

	ui := &GameManagerUI{
		fyneApp: fyneApp,
		window:  win,
		appRef:  appRef,
	}
	return ui
}

// showLaunchCountdown shows a countdown window. Calls onDone(true) if countdown
// completes, onDone(false) if the user cancels. Safe to call from any goroutine.
func (ui *GameManagerUI) showLaunchCountdown(gameName string, seconds int, onDone func(launch bool)) {
	cancelled := make(chan struct{})

	countdownLabel := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	cancelBtn := widget.NewButton("Cancel", func() {
		close(cancelled)
	})

	updateLabel := func(remaining int) {
		countdownLabel.SetText(fmt.Sprintf("Time to play %s!\nLaunching in %d second%s...",
			gameName, remaining, map[bool]string{true: "", false: "s"}[remaining == 1]))
	}
	updateLabel(seconds)

	content := container.NewBorder(nil, cancelBtn, nil, nil, countdownLabel)

	var win fyne.Window
	fyne.Do(func() {
		win = ui.fyneApp.NewWindow("Frictionless")
		win.SetContent(content)
		win.Resize(fyne.NewSize(300, 120))
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
				fyne.Do(func() { updateLabel(r) })
			}
		}
	}()
}

func (ui *GameManagerUI) show() {
	ui.refresh()
	ui.window.Show()
	ui.window.RequestFocus()
}

func (ui *GameManagerUI) refresh() {
	games := ui.appRef.config.Games

	var gameList *widget.List
	gameList = widget.NewList(
		func() int { return len(games) },
		func() fyne.CanvasObject {
			return container.NewBorder(
				nil, nil,
				widget.NewCheck("", nil),
				container.NewHBox(
					widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), nil),
					widget.NewButtonWithIcon("", theme.DeleteIcon(), nil),
				),
				container.NewHBox(
					widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
					widget.NewLabel(""),
				),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= len(games) {
				return
			}
			game := games[id]

			border := obj.(*fyne.Container)
			left := border.Objects[0].(*fyne.Container)
			enabledCheck := border.Objects[1].(*widget.Check)
			right := border.Objects[2].(*fyne.Container)

			left.Objects[0].(*widget.Label).SetText(game.GameName)
			left.Objects[1].(*widget.Label).SetText("— " + gameStatusLabel(ui.appRef, game))

			enabledCheck.Checked = game.Enabled
			enabledCheck.Refresh()
			enabledCheck.OnChanged = func(checked bool) {
				ui.appRef.config.Games[id].Enabled = checked
				ui.appRef.saveConfig()
				fyne.Do(ui.refresh)
			}

			right.Objects[0].(*widget.Button).OnTapped = func() {
				ui.showGameEditor(&game, false, func(updated Game) {
					ui.appRef.config.Games[id] = updated
					ui.appRef.saveConfig()
				})
			}
			right.Objects[1].(*widget.Button).OnTapped = func() {
				dialog.ShowConfirm("Delete Game",
					fmt.Sprintf("Remove %s from auto-launch?", game.GameName),
					func(ok bool) {
						if ok {
							ui.appRef.config.Games = append(
								ui.appRef.config.Games[:id],
								ui.appRef.config.Games[id+1:]...,
							)
							ui.appRef.saveConfig()
							fyne.Do(ui.refresh)
						}
					},
					ui.window,
				)
			}
		},
	)
	// Rows have their own tap targets (checkbox/edit/delete); the list itself
	// shouldn't retain a "selected" highlight when a row is tapped elsewhere.
	gameList.OnSelected = func(id widget.ListItemID) {
		gameList.Unselect(id)
	}

	addBtn := widget.NewButtonWithIcon("Add Game", theme.ContentAddIcon(), func() {
		ui.showGamePicker(func(created Game) {
			ui.appRef.config.Games = append(ui.appRef.config.Games, created)
			ui.appRef.saveConfig()
		})
	})

	exportBtn := widget.NewButtonWithIcon("Export", theme.DocumentSaveIcon(), func() {
		d := dialog.NewFileSave(func(f fyne.URIWriteCloser, err error) {
			if err != nil || f == nil {
				return
			}
			defer f.Close()
			data, err := yaml.Marshal(struct {
				Games []Game `yaml:"games"`
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

	importBtn := widget.NewButtonWithIcon("Import", theme.FolderOpenIcon(), func() {
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
				Games []Game `yaml:"games"`
			}
			if err := yaml.Unmarshal(data, &imported); err != nil {
				dialog.ShowError(fmt.Errorf("invalid YAML: %w", err), ui.window)
				return
			}
			if len(imported.Games) == 0 {
				dialog.ShowError(fmt.Errorf("no games found in file"), ui.window)
				return
			}
			dialog.ShowConfirm("Import Games",
				fmt.Sprintf("Replace all %d current game(s) with %d imported game(s)?", len(ui.appRef.config.Games), len(imported.Games)),
				func(ok bool) {
					if !ok {
						return
					}
					ui.appRef.config.Games = imported.Games
					ui.appRef.saveConfig()
					fyne.Do(ui.refresh)
				},
				ui.window,
			)
		}, ui.window)
	})

	footer := container.NewVBox(
		widget.NewSeparator(),
		container.NewHBox(addBtn, exportBtn, importBtn),
	)

	ui.window.SetContent(container.NewBorder(nil, footer, nil, nil, gameList))
}

func (ui *GameManagerUI) showGamePicker(onSave func(Game)) {
	discovered := discoverGames()

	if len(discovered) == 0 {
		// No games found — fall straight through to manual entry
		blank := Game{Enabled: true}
		ui.showGameEditor(&blank, false, onSave)
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
			blank := Game{Enabled: true}
			ui.showGameEditor(&blank, false, onSave)
			return
		}
		d := discovered[id]
		g := Game{
			GameName:     d.Name,
			GamePath:     d.GamePath,
			LaunchMethod: d.LaunchMethod,
			Enabled:      true,
		}
		ui.showGameEditor(&g, true, onSave)
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
	const margin float32 = 40

	size := want
	if maxW := avail.Width - margin; maxW < size.Width {
		size.Width = maxW
	}
	if maxH := avail.Height - margin; maxH < size.Height {
		size.Height = maxH
	}
	return size
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

// showGameEditor opens the game edit form. When methodLocked is true, the
// Launch Method field is omitted entirely — the user already picked a
// discovered game (and thus its launch method) in the picker dialog, so
// re-asking here would just be redundant.
func (ui *GameManagerUI) showGameEditor(game *Game, methodLocked bool, onSave func(Game)) {
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

	allDays := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

	type scheduleRow struct {
		checks []*widget.Check
		start  *widget.Entry
		end    *widget.Entry
	}

	schedulesBox := container.NewVBox()

	var rows []scheduleRow

	buildRow := func(s Schedule) {
		checks := make([]*widget.Check, len(allDays))
		grid := container.NewGridWithColumns(4)
		for i, day := range allDays {
			checked := false
			for _, d := range s.Days {
				if strings.EqualFold(d, day) {
					checked = true
					break
				}
			}
			checks[i] = widget.NewCheck(day, nil)
			checks[i].Checked = checked
			grid.Add(checks[i])
		}

		startE := widget.NewEntry()
		startE.SetText(s.StartTime)
		startE.SetPlaceHolder("19:00")

		endE := widget.NewEntry()
		endE.SetText(s.EndTime)
		endE.SetPlaceHolder("21:00")

		row := scheduleRow{checks: checks, start: startE, end: endE}
		rows = append(rows, row)
		idx := len(rows) - 1

		timeRow := container.NewHBox(
			container.NewGridWrap(fyne.NewSize(70, 36), startE),
			widget.NewLabel("to"),
			container.NewGridWrap(fyne.NewSize(70, 36), endE),
		)

		var rowBox *fyne.Container
		removeBtn := widget.NewButton("Remove", func() {
			if len(rows) <= 1 {
				return
			}
			rows = append(rows[:idx], rows[idx+1:]...)
			schedulesBox.Remove(rowBox)
			schedulesBox.Refresh()
		})

		rowBox = container.NewVBox(
			widget.NewSeparator(),
			grid,
			container.NewBorder(nil, nil, nil, removeBtn, timeRow),
		)
		schedulesBox.Add(rowBox)
	}

	existing := game.Schedules
	if len(existing) == 0 {
		existing = []Schedule{{StartTime: "19:00", EndTime: "21:00"}}
	}
	for _, s := range existing {
		buildRow(s)
	}

	addWindowBtn := widget.NewButton("+ Add Time Window", func() {
		buildRow(Schedule{StartTime: "19:00", EndTime: "21:00"})
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

		var schedules []Schedule
		for ri, row := range rows {
			selectedDays := []string{}
			for i, c := range row.checks {
				if c.Checked {
					selectedDays = append(selectedDays, allDays[i])
				}
			}
			if len(selectedDays) == 0 {
				dialog.ShowError(fmt.Errorf("time window %d: select at least one day", ri+1), ui.window)
				return
			}
			if !isValidTimeFormat(row.start.Text) || !isValidTimeFormat(row.end.Text) {
				dialog.ShowError(fmt.Errorf("time window %d: time must be in HH:MM format", ri+1), ui.window)
				return
			}
			for pi, prev := range schedules {
				for _, pd := range prev.Days {
					for _, nd := range selectedDays {
						if strings.EqualFold(pd, nd) && scheduleOverlaps(row.start.Text, row.end.Text, prev.StartTime, prev.EndTime) {
							dialog.ShowError(fmt.Errorf("time windows %d and %d overlap on %s", pi+1, ri+1, nd), ui.window)
							return
						}
					}
				}
			}
			schedules = append(schedules, Schedule{
				Days:      selectedDays,
				StartTime: row.start.Text,
				EndTime:   row.end.Text,
			})
		}

		for _, s := range schedules {
			if conflict := ui.findOverlappingGame(game.GameName, s.Days, s.StartTime, s.EndTime); conflict != "" {
				dialog.ShowError(fmt.Errorf("a time window overlaps with %s", conflict), ui.window)
				return
			}
		}

		d.Hide()
		onSave(Game{
			GameName:     nameEntry.Text,
			GamePath:     pathEntry.Text,
			LaunchMethod: methodSelect.Selected,
			LaunchArgs:   argsEntry.Text,
			Enabled:      enabledCheck.Checked,
			Schedules:    schedules,
		})
		fyne.Do(ui.refresh)
	}

	buttons := container.NewHBox(
		widget.NewButton("Cancel", func() { d.Hide() }),
		widget.NewButton("Save", doSave),
	)

	scroll := container.NewVScroll(form)
	scroll.SetMinSize(fyne.NewSize(480, 400))

	content := container.NewBorder(nil, buttons, nil, nil, scroll)

	d = dialog.NewCustomWithoutButtons("Game Schedule", content, ui.window)
	d.Resize(clampDialogSize(ui.window, fyne.NewSize(520, 600)))
	d.Show()
}

// gameStatusLabel returns the short status shown next to a game's name in
// the list: "Disabled", "No schedule", or its next upcoming launch time.
func gameStatusLabel(app *App, game Game) string {
	return gameStatusLabelAt(app, game, time.Now())
}

func gameStatusLabelAt(app *App, game Game, now time.Time) string {
	if !game.Enabled {
		return "Disabled"
	}
	if len(game.Schedules) == 0 {
		return "No schedule"
	}
	return app.nextScheduleLabelAt(game, now)
}
