//go:build smoke

// Smoke tests for the real binary, run in CI against the release builds (see
// .github/workflows/build.yml) and locally with `make smoke`. Unit tests use
// Fyne's headless test driver, so they never touch what a Fyne or cgo upgrade
// is most likely to break: GLFW, the OpenGL context, the native tray, and the
// process modes in main.go. These start each mode for real and fail if it
// crashes, exits early, or logs a Fyne error or panic.
//
// They need a display (CI uses xvfb on Linux). Set SMOKE_BINARY to test an
// already-built binary; otherwise the current source is built.

package smoke

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"frictionless-launcher/internal/launcher"
)

// smokeEnv is a sandbox for one run of the binary: its own copy of the
// executable with an empty config.yaml beside it (DefaultPath prefers that),
// and a home directory of its own so the log and Fyne's preferences don't
// touch the real ones.
type smokeEnv struct {
	bin  string
	home string
}

func newSmokeEnv(t *testing.T) smokeEnv {
	t.Helper()
	dir := t.TempDir()
	name := "frictionless-launcher"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)

	if src := os.Getenv("SMOKE_BINARY"); src != "" {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("reading SMOKE_BINARY: %v", err)
		}
		if err := os.WriteFile(bin, data, 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		out, err := exec.Command("go", "build", "-o", bin, "frictionless-launcher").CombinedOutput()
		if err != nil {
			t.Fatalf("go build: %v\n%s", err, out)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("games: []\nboot_delay: 10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The cache directories every real home has (os.UserCacheDir); Fyne
	// creates its own folder inside but not these.
	home := filepath.Join(dir, "home")
	for _, d := range []string{filepath.Join("Library", "Caches"), ".cache"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return smokeEnv{bin: bin, home: home}
}

func (e smokeEnv) command(args ...string) *exec.Cmd {
	cmd := exec.Command(e.bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+e.home, "LOCALAPPDATA="+e.home, "XDG_CONFIG_HOME="+filepath.Join(e.home, ".config"), "XDG_CACHE_HOME="+filepath.Join(e.home, ".cache"))
	return cmd
}

// checkLog fails the test if the app's log records a Fyne error or a panic.
func (e smokeEnv) checkLog(t *testing.T) {
	t.Helper()
	var logPath string
	_ = filepath.WalkDir(e.home, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.Name() == "frictionless-launcher.log" {
			logPath = path
		}
		return nil
	})
	if logPath == "" {
		t.Error("the app wrote no log file; did it start at all?")
		return
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	t.Logf("app log:\n%s", log)
	for _, bad := range []string{"Fyne error", "panic:"} {
		if strings.Contains(log, bad) {
			t.Errorf("app log contains %q", bad)
		}
	}
}

// staysUp starts cmd and fails unless it is still running after d, then
// stops it. Used for the modes that run until the user closes them.
func staysUp(t *testing.T, cmd *exec.Cmd, d time.Duration) {
	t.Helper()
	var output strings.Builder
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	select {
	case err := <-exited:
		t.Fatalf("exited after less than %v (%v); output:\n%s", d, err, output.String())
	case <-time.After(d):
	}
	_ = cmd.Process.Kill()
	<-exited
	if output.Len() > 0 {
		t.Logf("output:\n%s", output.String())
	}
}

func TestSmoke_Countdown(t *testing.T) {
	env := newSmokeEnv(t)
	cmd := env.command(launcher.FlagCountdown, "2", "Smoke Test")
	start := time.Now()
	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("countdown window did not finish within 60s")
	}

	// Exit 0 is the countdown finishing; ExitCountdownCancelled would mean
	// the window closed itself, anything else is a crash.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		t.Fatalf("countdown exited with status %d; output:\n%s", exitErr.ExitCode(), out)
	} else if err != nil {
		t.Fatalf("countdown failed: %v; output:\n%s", err, out)
	}
	// Finishing much sooner than the 2s countdown means no window ran.
	if elapsed := time.Since(start); elapsed < 1500*time.Millisecond {
		t.Errorf("countdown finished after %v, want about 2s", elapsed)
	}
	env.checkLog(t)
}

func TestSmoke_ManageWindow(t *testing.T) {
	env := newSmokeEnv(t)
	staysUp(t, env.command(launcher.FlagManage), 8*time.Second)
	env.checkLog(t)
}

func TestSmoke_Tray(t *testing.T) {
	env := newSmokeEnv(t)
	staysUp(t, env.command(), 8*time.Second)
	env.checkLog(t)
}
