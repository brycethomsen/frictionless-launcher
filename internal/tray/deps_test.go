package tray

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoFyneDependency enforces the rule the tray/UI process split depends
// on: nothing the resident tray process imports may pull in Fyne, whose
// driver creates an OpenGL context and a 60 Hz event loop at startup (see
// docs/DESIGN.md). fyne.io/systray is fine; it's a separate module.
func TestNoFyneDependency(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	out, err := exec.Command(goBin, "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if pkg == "fyne.io/fyne/v2" || strings.HasPrefix(pkg, "fyne.io/fyne/v2/") {
			t.Errorf("tray process depends on %s; Fyne may only be used from internal/ui", pkg)
		}
	}
}
