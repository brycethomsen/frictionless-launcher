//go:build windows

package platform

import (
	"bytes"
	"image/png"
	"log"

	"github.com/fyne-io/image/ico"
)

// TrayIcon converts PNG bytes to the .ico format the Windows tray requires.
// On failure it logs and returns the input unchanged.
func TrayIcon(pngData []byte) []byte {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		log.Printf("Error decoding tray icon: %v", err)
		return pngData
	}
	var buf bytes.Buffer
	if err := ico.Encode(&buf, img); err != nil {
		log.Printf("Error encoding tray icon: %v", err)
		return pngData
	}
	return buf.Bytes()
}
