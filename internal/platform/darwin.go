//go:build darwin

// Package platform wraps the bits of OS integration that differ per
// platform: hiding the dock icon, reading the frontmost app, and sending
// native notifications.
package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>

void setupApp() {
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    for (NSWindow *w in [[NSApp windows] copy]) {
        if ([[w title] isEqualToString:@"SystrayMonitor"]) {
            [w close];
        }
    }
}

const char* frontmostAppName() {
    NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
    if (app == nil) return "";
    const char *name = [[app localizedName] UTF8String];
    return name ? name : "";
}
*/
import "C"

import (
	"time"

	"fyne.io/fyne/v2"
)

// SetupDockBehavior hides the dock icon (NSApplicationActivationPolicyAccessory)
// once the Fyne app lifecycle starts, then calls onStarted.
func SetupDockBehavior(fyneApp fyne.App, onStarted func()) {
	fyneApp.Lifecycle().SetOnStarted(func() {
		C.setupApp()
		go func() {
			time.Sleep(500 * time.Millisecond)
			fyne.Do(func() { C.setupApp() })
		}()
		if onStarted != nil {
			onStarted()
		}
	})
}

// GetFrontmostApp returns the name of the currently frontmost application,
// or "" if none can be determined.
func GetFrontmostApp() string {
	return C.GoString(C.frontmostAppName())
}

// SendNativeNotification shows a native notification with the given title
// and body. Notifications require a bundled .app — this is a no-op when
// running as a plain binary.
func SendNativeNotification(title, body string) {
}
