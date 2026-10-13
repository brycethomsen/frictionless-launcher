package main

import (
	_ "embed"
	"log"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"frictionless-launcher/internal/config"
	"frictionless-launcher/internal/platform"
)

//go:embed icon.png
var iconData []byte

type App struct {
	config             *config.Config
	configPath         string
	logFile            *os.File
	lastLaunchTime     map[string]time.Time
	ui                 *GameManagerUI
	desk               desktop.App
	cancelLaunch       func()
	pendingGameName    string
	pendingSecondsLeft int
}

func main() {
	exe, _ := os.Executable()
	a := &App{
		configPath:     getConfigPath(exe),
		lastLaunchTime: make(map[string]time.Time),
	}

	a.setupLogging()
	defer a.closeLogFile()

	a.loadConfig()
	log.Printf("Config path: %s", a.configPath)
	a.ui = newGameManagerUI(a)

	go a.scheduleMonitor()
	go a.watchConfigFile()

	platform.SetupDockBehavior(a.ui.fyneApp, func() {
		bootLaunched := false
		for _, game := range a.config.Games {
			if game.Enabled && a.shouldLaunchGame(game) {
				log.Printf("Boot within schedule window for %s — queuing auto-launch", game.GameName)
				go a.autoLaunchGameByName(game)
				bootLaunched = true
				break
			}
		}
		if !bootLaunched {
			log.Println("No games in schedule window at boot")
		}
	})

	iconRes := fyne.NewStaticResource("icon.png", iconData)
	a.ui.fyneApp.SetIcon(iconRes)

	if desk, ok := a.ui.fyneApp.(desktop.App); ok {
		a.desk = desk
		desk.SetSystemTrayIcon(iconRes)
		desk.SetSystemTrayMenu(a.buildTrayMenu(desk))
	}

	a.ui.fyneApp.Run()
	a.closeLogFile()
}
