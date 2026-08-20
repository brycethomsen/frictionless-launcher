package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"frictionless-launcher/internal/config"
)

func (app *App) buildTrayMenu(desk desktop.App) *fyne.Menu {
	quitItem := fyne.NewMenuItem("Quit", func() { app.ui.fyneApp.Quit() })
	quitItem.IsQuit = true

	items := []*fyne.MenuItem{}

	if app.cancelLaunch != nil {
		label := fmt.Sprintf("⏳ %s launching in %ds... — Cancel", app.pendingGameName, app.pendingSecondsLeft)
		cancelItem := fyne.NewMenuItem(label, func() {
			if app.cancelLaunch != nil {
				app.cancelLaunch()
			}
		})
		items = append(items, cancelItem)
	} else {
		upcoming := app.nextScheduledGames(3)
		if len(upcoming) == 0 {
			noGames := fyne.NewMenuItem("No games scheduled", nil)
			noGames.Disabled = true
			items = append(items, noGames)
		} else {
			for _, g := range upcoming {
				game := g
				label := fmt.Sprintf("%s — %s", game.GameName, app.nextScheduleLabel(game))
				items = append(items, fyne.NewMenuItem(label, func() {
					go app.launchGameByStruct(game)
				}))
			}
		}
	}

	items = append(items,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Manage Games...", func() { app.ui.show() }),
		fyne.NewMenuItem("View Logs", func() { app.openLogFile() }),
		fyne.NewMenuItemSeparator(),
		quitItem,
	)

	return fyne.NewMenu("Frictionless", items...)
}

// nextScheduledGames returns up to n enabled games sorted by their next upcoming schedule time.
func (app *App) nextScheduledGames(n int) []config.Game {
	now := time.Now()
	type candidate struct {
		game config.Game
		next time.Time
	}
	var candidates []candidate
	for _, game := range app.config.Games {
		if !game.Enabled {
			continue
		}
		if t, ok := app.nextScheduleTime(game, now); ok {
			candidates = append(candidates, candidate{game, t})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].next.Before(candidates[j].next)
	})
	if len(candidates) > n {
		candidates = candidates[:n]
	}
	result := make([]config.Game, len(candidates))
	for i, c := range candidates {
		result[i] = c.game
	}
	return result
}

// nextScheduleTime returns the next time a game's schedule will start, within the next 7 days.
func (app *App) nextScheduleTime(game config.Game, from time.Time) (time.Time, bool) {
	for daysAhead := 0; daysAhead <= 7; daysAhead++ {
		day := from.AddDate(0, 0, daysAhead)
		dayName := day.Weekday().String()[:3]
		var earliest time.Time
		found := false
		for _, s := range game.Schedules {
			for _, d := range s.Days {
				if !strings.EqualFold(d, dayName) {
					continue
				}
				startH, startM := 0, 0
				fmt.Sscanf(s.StartTime, "%d:%d", &startH, &startM)
				candidate := time.Date(day.Year(), day.Month(), day.Day(), startH, startM, 0, 0, day.Location())
				if !candidate.After(from) {
					continue
				}
				if !found || candidate.Before(earliest) {
					earliest = candidate
					found = true
				}
			}
		}
		if found {
			return earliest, true
		}
	}
	return time.Time{}, false
}

// nextScheduleLabel returns a human-readable label for the next schedule, e.g. "Thu 19:00".
func (app *App) nextScheduleLabel(game config.Game) string {
	return app.nextScheduleLabelAt(game, time.Now())
}

func (app *App) nextScheduleLabelAt(game config.Game, now time.Time) string {
	t, ok := app.nextScheduleTime(game, now)
	if !ok {
		return "unscheduled"
	}
	if t.Before(now.Add(24 * time.Hour)) {
		return "Today " + t.Format("15:04")
	}
	return t.Format("Mon 15:04")
}

func fadedIcon(src []byte, alpha uint8) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	out := image.NewNRGBA(bounds)
	draw.Draw(out, bounds, img, bounds.Min, draw.Src)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := out.NRGBAAt(x, y)
			out.SetNRGBA(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: uint8(uint16(c.A) * uint16(alpha) / 255)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (app *App) startIconPulse(stop <-chan struct{}) {
	if app.desk == nil {
		return
	}
	dimData, err := fadedIcon(iconData, 80)
	if err != nil {
		return
	}
	normalRes := fyne.NewStaticResource("icon.png", iconData)
	dimRes := fyne.NewStaticResource("icon-dim.png", dimData)

	go func() {
		ticker := time.NewTicker(600 * time.Millisecond)
		defer ticker.Stop()
		dim := false
		for {
			select {
			case <-stop:
				fyne.Do(func() { app.desk.SetSystemTrayIcon(normalRes) })
				return
			case <-ticker.C:
				dim = !dim
				res := normalRes
				if dim {
					res = dimRes
				}
				r := res
				fyne.Do(func() { app.desk.SetSystemTrayIcon(r) })
			}
		}
	}()
}

func (app *App) refreshTrayMenu() {
	if app.desk == nil {
		return
	}
	fyne.Do(func() {
		app.desk.SetSystemTrayMenu(app.buildTrayMenu(app.desk))
	})
}
