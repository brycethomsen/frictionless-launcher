//go:build smoke

// Smoke tests for the real binary, run in CI against the release builds (see
// .github/workflows/build.yml) and locally with `make smoke`. Unit tests use
// Fyne's headless test driver, so they never touch what a Fyne or cgo upgrade
// is most likely to break: GLFW, the OpenGL context, the native tray, and the
// process modes in main.go. These start each mode for real and fail if it
// crashes, exits early, or logs a Fyne error or panic.
//
// They need a display (CI uses xvfb on Linux). Set SMOKE_BINARY to test an
// already-built binary; otherwise the current source is built. Set
// SMOKE_NO_OPENGL=1 where no OpenGL context can be created (GitHub's
// Apple-silicon macOS runners are GPU-less VMs, where GLFW finds no pixel
// format): the window modes are then skipped and only the tray is tested.

package smoke

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"frictionless-launcher/internal/config"
	"frictionless-launcher/internal/launcher"
)

// smokeEnv is a sandbox for one run of the binary: its own copy of the
// executable with an empty config.yaml beside it (DefaultPath prefers that),
// and a home directory of its own so the log and Fyne's preferences don't
// touch the real ones.
type smokeEnv struct {
	dir  string
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
	return smokeEnv{dir: dir, bin: bin, home: home}
}

// writeConfig replaces the sandbox's config.yaml with cfg.
func (e smokeEnv) writeConfig(t *testing.T, cfg config.Config) {
	t.Helper()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.dir, "config.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e smokeEnv) command(args ...string) *exec.Cmd {
	cmd := exec.Command(e.bin, args...)
	cmd.Env = append(os.Environ(), "HOME="+e.home, "LOCALAPPDATA="+e.home, "XDG_CONFIG_HOME="+filepath.Join(e.home, ".config"), "XDG_CACHE_HOME="+filepath.Join(e.home, ".cache"))
	return cmd
}

// readLog returns the app's log so far, or "" if it hasn't written one.
func (e smokeEnv) readLog() string {
	var logPath string
	_ = filepath.WalkDir(e.home, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.Name() == "frictionless-launcher.log" {
			logPath = path
		}
		return nil
	})
	if logPath == "" {
		return ""
	}
	data, _ := os.ReadFile(logPath)
	return string(data)
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
	checkVersionLine(t, log)
}

// startLine matches the log's first line for each process, capturing the
// version it reports.
var startLine = regexp.MustCompile(`=== Frictionless Launcher (\S+) \(\w+/\w+\) started ===`)

// checkVersionLine fails the test unless the log names the build's version.
// A release build (SMOKE_BINARY, built by goreleaser) must report the
// version goreleaser stamped in, not the "dev" fallback: that would mean
// the build-time setting no longer reaches internal/version.
func checkVersionLine(t *testing.T, log string) {
	t.Helper()
	m := startLine.FindStringSubmatch(log)
	if m == nil {
		t.Error("app log has no versioned startup line")
		return
	}
	if os.Getenv("SMOKE_BINARY") != "" && strings.HasPrefix(m[1], "dev") {
		t.Errorf("release build reports version %q; the build-time version wasn't applied", m[1])
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

// needsOpenGL skips a test of a Fyne window mode where OpenGL is unavailable.
func needsOpenGL(t *testing.T) {
	t.Helper()
	if os.Getenv("SMOKE_NO_OPENGL") != "" {
		t.Skip("SMOKE_NO_OPENGL is set: this machine can't create an OpenGL context")
	}
}

func TestSmoke_Countdown(t *testing.T) {
	needsOpenGL(t)
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
	needsOpenGL(t)
	env := newSmokeEnv(t)
	staysUp(t, env.command(launcher.FlagManage), 8*time.Second)
	env.checkLog(t)
}

func TestSmoke_Tray(t *testing.T) {
	env := newSmokeEnv(t)
	staysUp(t, env.command(), 8*time.Second)
	env.checkLog(t)
}

// TestSmokeHelperGame is not a real test: it is the "game" that
// TestSmoke_ScheduledLaunch has the app launch. It proves it ran by writing
// the file named in FL_SMOKE_GAME_MARKER (inherited from the tray process).
func TestSmokeHelperGame(t *testing.T) {
	marker := os.Getenv("FL_SMOKE_GAME_MARKER")
	if marker == "" {
		return
	}
	_ = os.WriteFile(marker, []byte("launched"), 0o644)
	os.Exit(0)
}

// TestSmoke_ScheduledLaunch is the app's whole job, end to end in the real
// binary: the tray process starts inside a game's schedule window, counts
// down the boot delay (with the countdown window, where one can open), and
// launches the game. The game is a copy of this test binary that writes a
// marker file.
func TestSmoke_ScheduledLaunch(t *testing.T) {
	now := time.Now()
	if now.Hour() == 23 && now.Minute() >= 58 {
		t.Skip("too close to midnight for a today-only schedule window")
	}
	env := newSmokeEnv(t)

	// The game gets its own name: the app skips launching a direct game
	// whose executable name is already running, and this test binary is.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(env.dir, "fake-game")
	if runtime.GOOS == "windows" {
		game += ".exe"
	}
	if err := os.WriteFile(game, data, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(env.dir, "game-launched")

	env.writeConfig(t, config.Config{
		BootDelay: 2,
		Games: []config.Game{{
			GameName:     "Smoke Game",
			GamePath:     game,
			LaunchMethod: "direct",
			LaunchArgs:   "-test.run=^TestSmokeHelperGame$",
			Enabled:      true,
			Schedules: []config.Schedule{{
				Days:      []string{now.Weekday().String()[:3]},
				StartTime: "00:00",
				EndTime:   "23:59",
			}},
		}},
	})

	cmd := env.command()
	cmd.Env = append(cmd.Env, "FL_SMOKE_GAME_MARKER="+marker)
	var output strings.Builder
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-exited
	})

	deadline := time.After(60 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		select {
		case err := <-exited:
			t.Fatalf("tray process exited (%v) before launching the game; output:\n%s\nlog:\n%s", err, output.String(), env.readLog())
		case <-deadline:
			t.Fatalf("game was not launched within 60s; log:\n%s", env.readLog())
		case <-time.After(200 * time.Millisecond):
		}
	}

	// The marker shows the game ran; the log shows it was the scheduler
	// that ran it, after the countdown.
	log := env.readLog()
	t.Logf("app log:\n%s", log)
	for _, want := range []string{"Boot within schedule window for Smoke Game", "Smoke Game launched successfully"} {
		if !strings.Contains(log, want) {
			t.Errorf("app log is missing %q", want)
		}
	}
	if strings.Contains(log, "panic:") {
		t.Error("app log contains a panic")
	}
}
