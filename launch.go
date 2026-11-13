package main

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	"frictionless-launcher/internal/config"
	"frictionless-launcher/internal/platform"
)

func (app *App) shouldLaunchGame(game config.Game) bool {
	return app.shouldLaunchGameAt(game, time.Now())
}

func (app *App) shouldLaunchGameAt(game config.Game, now time.Time) bool {
	if app.isGameRunning() {
		return false
	}
	if app.hasLaunchedInCurrentWindowAt(game, now) {
		return false
	}
	return app.isInScheduleWindowAt(game, now)
}

func (app *App) isInScheduleWindow(game config.Game) bool {
	return app.isInScheduleWindowAt(game, time.Now())
}

func (app *App) isInScheduleWindowAt(game config.Game, now time.Time) bool {
	currentTime := now.Format("15:04")
	currentDay := now.Weekday().String()[:3]

	for _, schedule := range game.Schedules {
		dayMatches := false
		for _, day := range schedule.Days {
			if strings.EqualFold(day, currentDay) {
				dayMatches = true
				break
			}
		}
		if !dayMatches {
			continue
		}
		if currentTime >= schedule.StartTime && currentTime <= schedule.EndTime {
			return true
		}
	}
	return false
}

func (app *App) getForegroundAppName() (string, error) {
	name := platform.GetFrontmostApp()
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

	for {
		select {
		case <-ticker.C:
			now := time.Now()

			// Skip if we already checked this minute
			if now.Minute() == lastChecked.Minute() && now.Hour() == lastChecked.Hour() {
				continue
			}
			lastChecked = now

			// The tray's "upcoming games" labels (see buildTrayMenu) are
			// only recomputed when the menu is rebuilt, which otherwise only
			// happens on discrete events — save, reload, launch, cancel.
			// Without this, a label like "Today 18:00" silently goes stale
			// once that time passes, still pointing at a window that's
			// already over instead of the actual next one.
			app.refreshTrayMenu()

			// Check each enabled game's schedule
			for _, game := range app.config.Games {
				if !game.Enabled {
					continue
				}

				// Check if we're in the schedule window
				if app.isInScheduleWindow(game) {
					// If a game is already running, skip
					if app.isGameRunning() {
						log.Printf("Game already running, skipping launch")
						continue
					}

					// If we already launched in this window, skip
					if app.hasLaunchedInCurrentWindow(game) {
						log.Printf("Already launched %s in current window, skipping", game.GameName)
						continue
					}

					// All clear or foreground app — both go through countdown popup
					log.Printf("Schedule triggered for %s", game.GameName)
					go app.autoLaunchGameByName(game)
					break // Only launch one game per check cycle
				}
			}
		}
	}
}

func (app *App) autoLaunchGameByName(game config.Game) {
	// Logged unconditionally so an unexpected relaunch is diagnosable after
	// the fact: this makes it clear from the log alone whether the guard in
	// scheduleMonitor saw a prior launch for this game and, if so, when.
	if last, ok := app.lastLaunchTime[game.GameName]; ok {
		log.Printf("autoLaunchGameByName(%s): previous launch recorded at %s", game.GameName, last.Format(time.RFC3339))
	} else {
		log.Printf("autoLaunchGameByName(%s): no previous launch recorded", game.GameName)
	}

	fgApp, _ := app.getForegroundAppName()

	cancelled := make(chan struct{})
	app.pendingGameName = game.GameName
	app.pendingSecondsLeft = app.config.BootDelay
	app.cancelLaunch = func() {
		select {
		case <-cancelled:
		default:
			close(cancelled)
		}
	}
	app.refreshTrayMenu()
	app.startIconPulse(cancelled)

	// Tick down the tray label every second
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-cancelled:
				return
			case <-ticker.C:
				app.pendingSecondsLeft--
				if app.pendingSecondsLeft <= 0 {
					return
				}
				app.refreshTrayMenu()
			}
		}
	}()

	cleanup := func() {
		app.cancelLaunch = nil
		app.pendingGameName = ""
		app.pendingSecondsLeft = 0
		app.refreshTrayMenu()
	}

	if fgApp != "" {
		log.Printf("Foreground app detected (%s) — notifying, launching %s in %ds unless cancelled via tray", fgApp, game.GameName, app.config.BootDelay)
		platform.SendNativeNotification("Frictionless", fmt.Sprintf("%s is launching in %d seconds — cancel from the menu bar if needed", game.GameName, app.config.BootDelay))

		select {
		case <-cancelled:
			log.Printf("Launch cancelled by user for %s — suppressing for remainder of schedule window", game.GameName)
			cleanup()
			app.recordLaunch(game)
			return
		case <-time.After(time.Duration(app.config.BootDelay) * time.Second):
		}

		cleanup()
		app.launchGameByStruct(game)
		return
	}

	log.Printf("Showing launch countdown for %s", game.GameName)
	platform.SendNativeNotification("Frictionless", fmt.Sprintf("Launching %s in %d seconds", game.GameName, app.config.BootDelay))

	done := make(chan bool, 1)
	app.ui.showLaunchCountdown(game.GameName, app.config.BootDelay, func(launch bool) {
		done <- launch
	})

	// Also respect tray cancel for the countdown case
	select {
	case <-cancelled:
		log.Printf("Launch cancelled via tray for %s — suppressing for remainder of schedule window", game.GameName)
		cleanup()
		app.recordLaunch(game)
		return
	case launch := <-done:
		cleanup()
		if launch {
			app.launchGameByStruct(game)
		} else {
			log.Printf("Launch cancelled by user for %s — suppressing for remainder of schedule window", game.GameName)
			app.recordLaunch(game)
		}
	}
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

func (app *App) launchGameByStruct(game config.Game) {
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
	for _, game := range app.config.Games {
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

func (app *App) hasLaunchedInCurrentWindow(game config.Game) bool {
	return app.hasLaunchedInCurrentWindowAt(game, time.Now())
}

func (app *App) hasLaunchedInCurrentWindowAt(game config.Game, now time.Time) bool {
	lastLaunch, exists := app.lastLaunchTime[game.GameName]
	if !exists {
		return false
	}

	for _, schedule := range game.Schedules {
		dayMatches := false
		for _, day := range schedule.Days {
			if strings.EqualFold(day, now.Weekday().String()[:3]) {
				dayMatches = true
				break
			}
		}

		if !dayMatches {
			continue
		}

		startTime, _ := time.Parse("15:04", schedule.StartTime)
		endTime, _ := time.Parse("15:04", schedule.EndTime)

		startTime = time.Date(now.Year(), now.Month(), now.Day(), startTime.Hour(), startTime.Minute(), 0, 0, now.Location())
		endTime = time.Date(now.Year(), now.Month(), now.Day(), endTime.Hour(), endTime.Minute(), 0, 0, now.Location())

		// A game with more than one window today only counts as "already
		// launched" against whichever window now actually falls in — a
		// launch recorded during an earlier window must not suppress a
		// later, distinct window for the same game later the same day.
		if now.Before(startTime) || now.After(endTime.Add(time.Minute)) {
			continue
		}

		if lastLaunch.After(startTime) && lastLaunch.Before(endTime.Add(time.Minute)) {
			return true
		}
	}

	return false
}

func (app *App) recordLaunch(game config.Game) {
	app.lastLaunchTime[game.GameName] = time.Now()
}
