//go:build !darwin

package main

import "fyne.io/fyne/v2"

func setupDockBehavior(fyneApp fyne.App, onStarted func()) {
	fyneApp.Lifecycle().SetOnStarted(func() {
		if onStarted != nil {
			onStarted()
		}
	})
}

func getFrontmostApp() string { return "" }

func sendNativeNotification(title, body string) {}
