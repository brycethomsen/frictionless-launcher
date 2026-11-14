// Package launcher is the scheduler at the heart of the resident tray
// process: it watches the clock and the config, counts down to scheduled
// launches, and starts games. It never imports Fyne (see AGENTS.md); the
// tray reaches it through the Tray interface, and the countdown window runs
// as a separate process (StartSelf with FlagCountdown).
package launcher

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"frictionless-launcher/internal/config"
	"frictionless-launcher/internal/schedule"
)

// Command-line modes. With no flag the binary runs as the lightweight tray
// process. The Fyne UI only ever runs in a short-lived child process started
// with one of these flags.
const (
	FlagManage    = "--manage"    // the Manage Games (schedule board) window
	FlagCountdown = "--countdown" // --countdown <seconds> <game name>

	// ExitCountdownCancelled is the --countdown exit code meaning the user
	// cancelled the launch. Any other exit status means "no objection".
	ExitCountdownCancelled = 3
)

// Tray is what the launcher needs from the system tray. Implemented by
// internal/tray; nil (no tray) is allowed.
type Tray interface {
	Refresh()                   // rebuild the menu if anything changed
	UpdateCountdown()           // retitle the pending-launch entry in place
	Pulse(stop <-chan struct{}) // animate the icon until stop is closed
}

// App is the scheduler state for the tray process. Several goroutines use it
// (scheduler, config watcher, tray clicks, countdowns); mu guards the fields
// below it. Config access goes through the config.Store, which has its own
// locking.
type App struct {
	store *config.Store

	mu             sync.Mutex
	lastLaunchTime map[string]time.Time
	pending        *pendingLaunch
	tray           Tray
}

// pendingLaunch is an auto-launch counting down; at most one exists at a time.
type pendingLaunch struct {
	gameName    string
	secondsLeft int // guarded by App.mu
	cancelled   chan struct{}
	once        sync.Once
}

func (p *pendingLaunch) cancel() { p.once.Do(func() { close(p.cancelled) }) }

// New returns an App using the config in store.
func New(store *config.Store) *App {
	return &App{store: store, lastLaunchTime: make(map[string]time.Time)}
}

// SetTray connects the tray the App notifies about changes.
func (app *App) SetTray(t Tray) {
	app.mu.Lock()
	defer app.mu.Unlock()
	app.tray = t
}

func (app *App) currentTray() Tray {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.tray
}

func (app *App) refreshTray() {
	if t := app.currentTray(); t != nil {
		t.Refresh()
	}
}

// Config returns the current config snapshot. Treat it as read-only.
func (app *App) Config() *config.Config { return app.store.Current() }

// LoadConfig (re)loads the config file and warns about overlapping windows.
func (app *App) LoadConfig() {
	app.store.Load()
	schedule.WarnOverlaps(app.Config().Games)
}

// Start begins scheduling: the once-a-minute schedule check, config file
// hot-reload, and an immediate launch if we booted inside a window. Call it
// once the tray is ready.
func (app *App) Start() {
	go app.scheduleMonitor()
	go config.Watch(app.store.Path(), nil, func() {
		app.LoadConfig()
		app.refreshTray()
	})
	app.launchIfInWindowAtBoot()
}

// Upcoming returns up to n enabled games ordered by their next window start.
func (app *App) Upcoming(n int) []config.Game {
	return schedule.Upcoming(app.Config().Games, n, time.Now())
}

// launchIfInWindowAtBoot starts an auto-launch countdown if the app started up
// inside one of the enabled games' schedule windows.
func (app *App) launchIfInWindowAtBoot() {
	for _, game := range app.Config().Games {
		if game.Enabled && app.shouldLaunchGame(game) {
			log.Printf("Boot within schedule window for %s — queuing auto-launch", game.GameName)
			go app.autoLaunchGame(game)
			return
		}
	}
	log.Println("No games in schedule window at boot")
}

// PendingLabel returns the tray label for the pending launch, if there is one.
func (app *App) PendingLabel() (string, bool) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.pending == nil {
		return "", false
	}
	return fmt.Sprintf("⏳ %s launching in %ds... — Cancel", app.pending.gameName, app.pending.secondsLeft), true
}

// CancelPending cancels the pending launch, if any.
func (app *App) CancelPending() {
	app.mu.Lock()
	p := app.pending
	app.mu.Unlock()
	if p != nil {
		p.cancel()
	}
}

func (app *App) hasPendingLaunch() bool {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.pending != nil
}

// StartSelf starts another instance of this executable with args.
func StartSelf(args ...string) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
