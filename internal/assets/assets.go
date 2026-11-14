// Package assets embeds the app's static files.
package assets

import _ "embed"

// Icon is the app icon (256x256 PNG), used for the tray, the Fyne windows,
// and (via packaging/macos/build-app.sh) the macOS bundle icon.
//
//go:embed icon.png
var Icon []byte
