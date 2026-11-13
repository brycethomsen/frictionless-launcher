//go:build !windows

package platform

// TrayIcon returns icon bytes in the format the OS tray expects. The macOS and
// Linux trays take PNG directly.
func TrayIcon(pngData []byte) []byte { return pngData }
