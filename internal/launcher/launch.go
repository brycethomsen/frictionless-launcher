package launcher

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	"frictionless-launcher/internal/config"
	"frictionless-launcher/internal/platform"
	"frictionless-launcher/internal/schedule"
)

func (app *App) shouldLaunchGame(game config.Game) bool {
	return app.shouldLaunchGameAt(game, time.Now())
}

func (app *App) shouldLaunchGameAt(game config.Game, now time.Time) bool {
	// Cheapest checks first: isGameRunning enumerates every process on the
	// system, so only pay for it when the game would otherwise launch.
	if !schedule.InWindow(game, now) {
		return false
	}
	if app.launchedInCurrentWindow(game, now) {
		return false
	}
	return !app.isGameRunning()
}

// frontmostApp is platform.GetFrontmostApp, swappable in tests.
var frontmostApp = platform.GetFrontmostApp

func (app *App) getForegroundAppName() (string, error) {
	name := frontmostApp()
	// Ignore our own app and empty results
	if name == "Frictionless" || name == "frictionless-launcher" {
		return "", nil
	}
	return name, nil
}

func (app *App) scheduleMonitor() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	var lastChecked time.Time

	for now := range ticker.C {
		// Skip if we already checked this minute
		if now.Minute() == lastChecked.Minute() && now.Hour() == lastChecked.Hour() {
			continue
		}
		lastChecked = now

		// The tray's "upcoming games" labels (see internal/tray) are only
		// recomputed when the menu is rebuilt, which otherwise only happens
		// on discrete events — save, reload, launch, cancel. Without this, a
		// label like "Today 18:00" silently goes stale once that time passes,
		// still pointing at a window that's already over instead of the
		// actual next one. (rebuild is a no-op when nothing changed.)
		app.refreshTray()

		// A countdown is already running (e.g. boot_delay is longer than the
		// tick interval); don't start a second one.
		if app.hasPendingLaunch() {
			continue
		}

		for _, game := range app.Config().Games {
			// shouldLaunchGameAt checks the cheap conditions (schedule window,
			// already launched this window) before enumerating processes.
			if game.Enabled && app.shouldLaunchGameAt(game, now) {
				log.Printf("Schedule triggered for %s", game.GameName)
				go app.autoLaunchGame(game)
				break // Only launch one game per check cycle
			}
		}
	}
}

// autoLaunchGame counts down BootDelay seconds and then launches game, unless
// the user cancels first, either from the tray menu or from the countdown
// popup. The tray process owns the timer; the popup (a separate --countdown
// process) can only cancel, so a popup that fails to start or crashes can
// neither block nor double-fire the launch.
func (app *App) autoLaunchGame(game config.Game) {
	// Logged unconditionally so an unexpected relaunch is diagnosable after
	// the fact: this makes it clear from the log alone whether the guard in
	// scheduleMonitor saw a prior launch for this game and, if so, when.
	app.mu.Lock()
	last, launchedBefore := app.lastLaunchTime[game.GameName]
	app.mu.Unlock()
	if launchedBefore {
		log.Printf("autoLaunchGame(%s): previous launch recorded at %s", game.GameName, last.Format(time.RFC3339))
	} else {
		log.Printf("autoLaunchGame(%s): no previous launch recorded", game.GameName)
	}

	delay := app.Config().BootDelay
	p := &pendingLaunch{gameName: game.GameName, secondsLeft: delay, cancelled: make(chan struct{})}

	app.mu.Lock()
	if app.pending != nil {
		app.mu.Unlock()
		log.Printf("A launch is already pending, not queuing %s", game.GameName)
		return
	}
	app.pending = p
	app.mu.Unlock()

	app.refreshTray()
	done := make(chan struct{})
	tray := app.currentTray()
	if tray != nil {
		tray.Pulse(done)
	}

	var popup *exec.Cmd
	if fgApp, _ := app.getForegroundAppName(); fgApp != "" {
		log.Printf("Foreground app detected (%s) — notifying, launching %s in %ds unless cancelled via tray", fgApp, game.GameName, delay)
		platform.SendNativeNotification("Frictionless", fmt.Sprintf("%s is launching in %d seconds — cancel from the menu bar if needed", game.GameName, delay))
	} else {
		log.Printf("Showing launch countdown for %s", game.GameName)
		platform.SendNativeNotification("Frictionless", fmt.Sprintf("Launching %s in %d seconds", game.GameName, delay))
		if delay > 0 {
			popup = countdownPopup(game.GameName, delay, p.cancel)
		}
	}

	deadline := time.NewTimer(time.Duration(delay) * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	cancelled := false
wait:
	for {
		select {
		case <-p.cancelled:
			cancelled = true
			break wait
		case <-deadline.C:
			break wait
		case <-ticker.C:
			app.mu.Lock()
			if p.secondsLeft > 0 {
				p.secondsLeft--
			}
			app.mu.Unlock()
			if tray != nil {
				tray.UpdateCountdown()
			}
		}
	}

	if popup != nil {
		// Normally the popup has already exited on its own; this only matters
		// when the launch was cancelled from the tray, or the clocks drifted.
		_ = popup.Process.Kill()
	}

	app.mu.Lock()
	app.pending = nil
	app.mu.Unlock()
	close(done)
	app.refreshTray()

	if cancelled {
		log.Printf("Launch cancelled by user for %s — suppressing for remainder of schedule window", game.GameName)
		app.recordLaunch(game)
		return
	}
	app.Launch(game)
}

// countdownPopup is startCountdownPopup, swappable in tests.
var countdownPopup = startCountdownPopup

// startCountdownPopup starts the --countdown UI process and calls onCancel if
// the user cancels from it. It returns nil if the process couldn't be started;
// the launch then proceeds on the tray's timer with only the tray cancel item.
func startCountdownPopup(gameName string, seconds int, onCancel func()) *exec.Cmd {
	cmd, err := StartSelf(FlagCountdown, strconv.Itoa(seconds), gameName)
	if err != nil {
		log.Printf("Could not show countdown window: %v", err)
		return nil
	}
	go func() {
		err := cmd.Wait()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == ExitCountdownCancelled {
			onCancel()
		}
	}()
	return cmd
}

// buildLaunchCmd returns the exec.Cmd that would launch the given game on the
// current OS. It does not start the command.
func buildLaunchCmd(game config.Game, goos string) *exec.Cmd {
	switch game.LaunchMethod {
	case "steam", "epic":
		switch goos {
		case "darwin":
			return exec.Command("open", "-g", game.GamePath)
		case "windows":
			return exec.Command("cmd", "/c", "start", game.GamePath)
		default:
			return exec.Command("xdg-open", game.GamePath)
		}
	default: // "direct" and unknown methods
		var args []string
		if game.LaunchArgs != "" {
			args = strings.Fields(game.LaunchArgs)
		}
		return exec.Command(game.GamePath, args...)
	}
}

// Launch starts game now, outside any countdown (e.g. from the tray menu).
func (app *App) Launch(game config.Game) {
	if game.GamePath == "" {
		log.Println("No game path configured")
		return
	}

	log.Printf("Launching %s via %s", game.GameName, game.LaunchMethod)

	switch game.LaunchMethod {
	case "steam":
		if !app.isPlatformRunning("steam") {
			log.Printf("Warning: Steam does not appear to be running — it will launch first, adding delay")
		}
	case "epic":
		if !app.isPlatformRunning("epic") {
			log.Printf("Warning: Epic Games Launcher does not appear to be running — it will launch first, adding delay")
		}
	}

	cmd := buildLaunchCmd(game, runtime.GOOS)
	if err := cmd.Start(); err != nil {
		log.Printf("Error launching game: %v", err)
		return
	}

	app.recordLaunch(game)
	log.Printf("%s launched successfully", game.GameName)
}

func (app *App) isPlatformRunning(platformName string) bool {
	var names []string
	switch platformName {
	case "steam":
		names = []string{"steam", "steam.exe", "Steam"}
	case "epic":
		names = []string{"EpicGamesLauncher", "EpicGamesLauncher.exe"}
	default:
		return false
	}
	procs, err := process.Processes()
	if err != nil {
		return false
	}
	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			continue
		}
		for _, n := range names {
			if strings.EqualFold(name, n) {
				return true
			}
		}
	}
	return false
}

func (app *App) isGameRunning() bool {
	// For steam/epic, hasLaunchedInCurrentWindow prevents double-launches.
	// Only check processes for direct-launch games where we have a real executable path.
	var directGames []config.Game
	for _, game := range app.Config().Games {
		if game.LaunchMethod == "direct" && game.GamePath != "" {
			directGames = append(directGames, game)
		}
	}
	if len(directGames) == 0 {
		return false
	}

	processes, err := process.Processes()
	if err != nil {
		log.Printf("Error checking processes: %v", err)
		return false
	}

	for _, game := range directGames {
		exeName := filepath.Base(game.GamePath)
		for _, proc := range processes {
			name, err := proc.Name()
			if err != nil {
				continue
			}
			if strings.EqualFold(name, exeName) {
				log.Printf("Game process found: %s", name)
				return true
			}
			exe, err := proc.Exe()
			if err != nil {
				continue
			}
			if strings.EqualFold(exe, game.GamePath) {
				log.Printf("Game process found: %s", exe)
				return true
			}
		}
	}

	return false
}

// launchedInCurrentWindow reports whether game was already launched (or its
// launch cancelled) during the schedule window now is in.
func (app *App) launchedInCurrentWindow(game config.Game, now time.Time) bool {
	app.mu.Lock()
	last, ok := app.lastLaunchTime[game.GameName]
	app.mu.Unlock()
	return ok && schedule.LaunchedInWindow(game, last, now)
}

func (app *App) recordLaunch(game config.Game) {
	app.mu.Lock()
	defer app.mu.Unlock()
	app.lastLaunchTime[game.GameName] = time.Now()
}
