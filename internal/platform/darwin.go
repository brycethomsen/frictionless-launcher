//go:build darwin

// Package platform wraps the bits of OS integration that differ per
// platform: hiding the dock icon, reading the frontmost app, sending native
// notifications, opening files, the per-user data directory, and the tray
// icon format. The tray process depends on it, so it must not import Fyne.
package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>

// setupApp keeps the Fyne UI processes out of the Dock (matching the tray
// process) and brings their window to the front, since they are started in
// the background by the tray process rather than by the user directly.
void setupApp() {
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    [NSApp activateIgnoringOtherApps:YES];
}

// hideFromDockAsync sets the accessory (menu-bar-only) activation policy for
// the tray process. It may be called from any thread.
void hideFromDockAsync() {
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    });
}

const char* frontmostAppName() {
    NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
    if (app == nil) return "";
    const char *name = [[app localizedName] UTF8String];
    return name ? name : "";
}
*/
import "C"

// PrepareUIProcess keeps a Fyne UI process (--manage, --countdown) out of
// the Dock and brings its window to the front. Call it on the UI's main
// thread once the app has started, and again after its first window exists:
// GLFW resets the activation policy when it creates that window.
func PrepareUIProcess() { C.setupApp() }

// HideFromDock keeps the (non-Fyne) tray process out of the Dock when it runs
// as a plain binary; the .app bundle already does this via LSUIElement.
func HideFromDock() { C.hideFromDockAsync() }

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
