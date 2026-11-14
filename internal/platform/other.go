//go:build !darwin

package platform

// PrepareUIProcess keeps a UI process out of the Dock and brings it to the
// front. macOS-only.
func PrepareUIProcess() {}

// HideFromDock keeps the tray process out of the Dock. macOS-only.
func HideFromDock() {}

// GetFrontmostApp returns the name of the currently frontmost application,
// or "" if none can be determined. Unsupported on this platform.
func GetFrontmostApp() string { return "" }

// SendNativeNotification shows a native notification with the given title
// and body. Unsupported on this platform.
func SendNativeNotification(title, body string) {}
