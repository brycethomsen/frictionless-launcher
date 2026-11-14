// Package tray runs the resident process's system tray icon and menu. It
// uses fyne.io/systray directly and must never import Fyne itself (see
// AGENTS.md): this process stays running all day and has to stay cheap.
package tray

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os/exec"
	"slices"
	"sync"
	"time"

	"fyne.io/systray"

	"frictionless-launcher/internal/assets"
	"frictionless-launcher/internal/config"
	"frictionless-launcher/internal/launcher"
	"frictionless-launcher/internal/logging"
	"frictionless-launcher/internal/platform"
	"frictionless-launcher/internal/schedule"
)

// Compile-time check that the controller satisfies what the launcher needs.
var _ launcher.Tray = (*trayController)(nil)

// trayEntryKind identifies what a tray menu entry does when clicked. The menu
// is described as data (trayEntries) so it can be tested without a real tray.
type trayEntryKind int

const (
	entryInfo      trayEntryKind = iota // disabled, informational text
	entryCancel                         // cancels the pending launch; retitled every second
	entryGame                           // launches entry.game immediately
	entrySeparator                      //
	entryManage                         // opens the Manage Games window
	entryLogs                           // opens the log file
	entryQuit                           //
)

type trayEntry struct {
	kind  trayEntryKind
	label string
	game  config.Game
}

// trayEntries describes the tray menu: the pending launch's cancel entry if
// pendingLabel is set, otherwise the upcoming games, then the fixed entries.
func trayEntries(pendingLabel string, upcoming []config.Game, now time.Time) []trayEntry {
	var entries []trayEntry
	if pendingLabel != "" {
		entries = append(entries, trayEntry{kind: entryCancel, label: pendingLabel})
	} else {
		if len(upcoming) == 0 {
			entries = append(entries, trayEntry{kind: entryInfo, label: "No games scheduled"})
		}
		for _, game := range upcoming {
			entries = append(entries, trayEntry{
				kind:  entryGame,
				label: fmt.Sprintf("%s — %s", game.GameName, schedule.NextLabel(game, now)),
				game:  game,
			})
		}
	}
	return append(entries,
		trayEntry{kind: entrySeparator},
		trayEntry{kind: entryManage, label: "Manage Games..."},
		trayEntry{kind: entryLogs, label: "View Logs"},
		trayEntry{kind: entrySeparator},
		trayEntry{kind: entryQuit, label: "Quit"},
	)
}

// sameMenu reports whether a and b would render the same tray menu. The
// cancel entry's label is ignored: its countdown is kept current in place by
// trayController.UpdateCountdown, so a tick alone isn't a menu change.
func sameMenu(a, b []trayEntry) bool {
	return slices.EqualFunc(a, b, func(x, y trayEntry) bool {
		if x.kind != y.kind {
			return false
		}
		if x.kind == entryCancel {
			return true
		}
		return x.label == y.label && x.game.GameName == y.game.GameName
	})
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

// trayController drives the system tray icon and menu for the resident
// process. It talks to the OS tray directly through fyne.io/systray (native
// Shell_NotifyIcon / NSStatusItem / StatusNotifierItem) rather than through a
// Fyne app, so this process never creates an OpenGL context and has no render
// loop. A Fyne app would set up both just to show a tray icon, costing
// constant background CPU and a heavy startup.
type trayController struct {
	app        *launcher.App
	normalIcon []byte
	dimIcon    []byte

	mu         sync.Mutex
	shown      []trayEntry       // what the menu currently shows
	gen        chan struct{}     // closed on each rebuild to stop the previous menu's click listeners
	cancelItem *systray.MenuItem // the live countdown entry, if the menu currently has one
	manager    *exec.Cmd         // the running --manage process, if any
}

// Run shows the tray icon and runs the scheduler until the user quits. It
// blocks. onExit runs as the tray shuts down; on macOS the native loop then
// terminates the process instead of returning from Run, so cleanup that must
// happen (closing the log) belongs in onExit, not after Run.
func Run(app *launcher.App, onExit func()) {
	t := &trayController{app: app}
	t.normalIcon = platform.TrayIcon(assets.Icon)
	if dim, err := fadedIcon(assets.Icon, 80); err == nil {
		t.dimIcon = platform.TrayIcon(dim)
	}
	app.SetTray(t)
	systray.Run(t.onReady, onExit)
}

func (t *trayController) onReady() {
	platform.HideFromDock()
	systray.SetIcon(t.normalIcon)
	systray.SetTooltip("Frictionless Launcher")
	t.Refresh()
	t.app.Start()
}

// Refresh replaces the tray menu with one built from the app's current
// state, unless it would be identical to what's already shown.
func (t *trayController) Refresh() {
	pending, _ := t.app.PendingLabel()
	entries := trayEntries(pending, t.app.Upcoming(3), time.Now())

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.gen != nil && sameMenu(entries, t.shown) {
		return
	}
	if t.gen != nil {
		close(t.gen)
	}
	gen := make(chan struct{})
	t.gen = gen
	t.shown = entries
	t.cancelItem = nil

	systray.ResetMenu()
	for _, e := range entries {
		if e.kind == entrySeparator {
			systray.AddSeparator()
			continue
		}
		item := systray.AddMenuItem(e.label, "")
		switch e.kind {
		case entryInfo:
			item.Disable()
		case entryCancel:
			t.cancelItem = item
			onClick(item, gen, t.app.CancelPending)
		case entryGame:
			game := e.game
			onClick(item, gen, func() { t.app.Launch(game) })
		case entryManage:
			onClick(item, gen, t.openManager)
		case entryLogs:
			onClick(item, gen, func() { platform.OpenWithDefaultApp(logging.FilePath()) })
		case entryQuit:
			onClick(item, gen, systray.Quit)
		}
	}
}

// onClick runs f whenever item is clicked, until gen is closed.
func onClick(item *systray.MenuItem, gen <-chan struct{}, f func()) {
	go func() {
		for {
			select {
			case <-item.ClickedCh:
				f()
			case <-gen:
				return
			}
		}
	}()
}

// UpdateCountdown refreshes the countdown entry's text in place, so the
// once-a-second countdown doesn't rebuild the whole native menu.
func (t *trayController) UpdateCountdown() {
	label, ok := t.app.PendingLabel()
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancelItem != nil {
		t.cancelItem.SetTitle(label)
	}
}

// Pulse alternates the tray icon between normal and dimmed until stop is
// closed, then restores the normal icon.
func (t *trayController) Pulse(stop <-chan struct{}) {
	if t.dimIcon == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(600 * time.Millisecond)
		defer ticker.Stop()
		dim := false
		for {
			select {
			case <-stop:
				systray.SetIcon(t.normalIcon)
				return
			case <-ticker.C:
				dim = !dim
				if dim {
					systray.SetIcon(t.dimIcon)
				} else {
					systray.SetIcon(t.normalIcon)
				}
			}
		}
	}()
}

// openManager starts the --manage UI process, unless one is already open.
func (t *trayController) openManager() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.manager != nil {
		log.Println("Manage Games window is already open")
		return
	}
	cmd, err := launcher.StartSelf(launcher.FlagManage)
	if err != nil {
		log.Printf("Could not open Manage Games window: %v", err)
		return
	}
	t.manager = cmd
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("Manage Games window exited: %v", err)
		}
		t.mu.Lock()
		t.manager = nil
		t.mu.Unlock()
	}()
}
