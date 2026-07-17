//go:build darwin

package main

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

func setupDockBehavior(fyneApp fyne.App, onStarted func()) {
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

func getFrontmostApp() string {
	return C.GoString(C.frontmostAppName())
}

func sendNativeNotification(title, body string) {
	// Notifications require a bundled app — no-op when running as a plain binary.
}
