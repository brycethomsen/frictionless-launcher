// Frictionless Launcher launches your games on a schedule.
//
// The binary runs as one of three processes, picked by its first argument
// (see docs/DESIGN.md):
//
//	(no flag)              resident tray + scheduler — no Fyne, stays cheap
//	--manage               the Manage Games window
//	--countdown SECS GAME  the launch countdown; exits 3 if cancelled
package main

import (
	"log"
	"os"
	"strconv"

	"frictionless-launcher/internal/config"
	"frictionless-launcher/internal/launcher"
	"frictionless-launcher/internal/logging"
	"frictionless-launcher/internal/tray"
	"frictionless-launcher/internal/ui"
)

func main() {
	exe, _ := os.Executable()
	store := config.NewStore(config.DefaultPath(exe), nil)

	args := os.Args[1:]
	switch {
	case len(args) >= 1 && args[0] == launcher.FlagManage:
		log.SetPrefix("[manage] ")
		closeLog := logging.Setup(false)
		store.Load()
		ui.RunManager(store)
		closeLog()

	case len(args) >= 3 && args[0] == launcher.FlagCountdown:
		log.SetPrefix("[countdown] ")
		closeLog := logging.Setup(false)
		seconds, err := strconv.Atoi(args[1])
		if err != nil {
			log.Printf("Invalid countdown seconds %q: %v", args[1], err)
			closeLog()
			os.Exit(1)
		}
		cancelled := ui.RunCountdown(args[2], seconds)
		closeLog()
		if cancelled {
			os.Exit(launcher.ExitCountdownCancelled)
		}

	default:
		closeLog := logging.Setup(true)
		log.Printf("Config path: %s", store.Path())
		app := launcher.New(store)
		app.LoadConfig()
		tray.Run(app, closeLog)
		closeLog()
	}
}
