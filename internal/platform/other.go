//go:build !darwin

package platform

import "fyne.io/fyne/v2"

// SetupDockBehavior hides the dock icon (macOS-only) once the Fyne app
// lifecycle starts, then calls onStarted. No-op on this platform beyond that.
func SetupDockBehavior(fyneApp fyne.App, onStarted func()) {
	fyneApp.Lifecycle().SetOnStarted(func() {
		if onStarted != nil {
			onStarted()
		}
	})
}

// GetFrontmostApp returns the name of the currently frontmost application,
// or "" if none can be determined. Unsupported on this platform.
func GetFrontmostApp() string { return "" }

// SendNativeNotification shows a native notification with the given title
// and body. Unsupported on this platform.
func SendNativeNotification(title, body string) {}
