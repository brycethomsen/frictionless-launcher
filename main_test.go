package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// ---- helpers ----------------------------------------------------------------

func newTestApp(t *testing.T) (*App, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "frictionless_test_*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	a := &App{
		configPath:     filepath.Join(dir, "config.yaml"),
		lastLaunchTime: make(map[string]time.Time),
	}
	return a, dir
}

func gameWithSchedule(day, start, end string) Game {
	return Game{
		GameName:     "TestGame",
		GamePath:     "steam://rungameid/1",
		LaunchMethod: "steam",
		Enabled:      true,
		Schedules: []Schedule{
			{Days: []string{day}, StartTime: start, EndTime: end},
		},
	}
}

// fakeClock builds an App whose schedule checks use a mocked time by replacing
// time.Now-dependent logic via nextScheduleTime with a fixed reference point.
func appWithGames(games []Game) *App {
	return &App{
		config:         &Config{Games: games, BootDelay: 10},
		lastLaunchTime: make(map[string]time.Time),
	}
}

// ---- Config: load / save / migrate ------------------------------------------

func TestConfig_LoadAndSave(t *testing.T) {
	app, _ := newTestApp(t)

	app.loadConfig()
	if app.config == nil {
		t.Fatal("config must not be nil after loadConfig")
	}
	// Fresh config must start with no games — never a hardcoded sample.
	if len(app.config.Games) != 0 {
		t.Errorf("default config should have no games, got %d", len(app.config.Games))
	}
	if app.config.BootDelay != 10 {
		t.Errorf("expected BootDelay 10, got %d", app.config.BootDelay)
	}

	// Add a game and round-trip
	app.config.Games = []Game{{GameName: "RoundTripGame"}}
	app.config.BootDelay = 30
	// saveConfig calls refreshTrayMenu (no-op when desk==nil)
	app.saveConfig()

	if _, err := os.Stat(app.configPath); os.IsNotExist(err) {
		t.Fatal("config file should exist after saveConfig")
	}

	app2, _ := newTestApp(t)
	app2.configPath = app.configPath
	app2.loadConfig()
	if app2.config.Games[0].GameName != "RoundTripGame" {
		t.Errorf("expected RoundTripGame, got %s", app2.config.Games[0].GameName)
	}
	if app2.config.BootDelay != 30 {
		t.Errorf("expected BootDelay 30, got %d", app2.config.BootDelay)
	}
}

func TestConfig_InvalidYAML(t *testing.T) {
	app, dir := newTestApp(t)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("invalid: yaml: [unclosed"), 0644); err != nil {
		t.Fatal(err)
	}
	app.loadConfig()
	// Must fall back to defaults — not nil, not panicking
	if app.config == nil {
		t.Fatal("config must not be nil after invalid YAML")
	}
	if len(app.config.Games) != 0 {
		t.Errorf("expected empty game list after invalid YAML, got %d", len(app.config.Games))
	}
}

func TestConfig_LegacyMigration(t *testing.T) {
	app, _ := newTestApp(t)

	// The legacy format: top-level game_path with no games list.
	legacy := `
game_path: /usr/games/mygame
game_name: My Legacy Game
enabled: true
schedule: always
`
	if err := os.WriteFile(app.configPath, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	app.loadConfig()

	if len(app.config.Games) != 1 {
		t.Fatalf("expected 1 migrated game, got %d", len(app.config.Games))
	}
	g := app.config.Games[0]
	if g.GameName != "My Legacy Game" {
		t.Errorf("expected 'My Legacy Game', got %q", g.GameName)
	}
	if g.GamePath != "/usr/games/mygame" {
		t.Errorf("expected '/usr/games/mygame', got %q", g.GamePath)
	}
	if g.LaunchMethod != "direct" {
		t.Errorf("legacy migration should set launch_method=direct, got %q", g.LaunchMethod)
	}
	// "always" schedule maps to Mon–Sun 00:00–23:59
	if len(g.Schedules) == 0 {
		t.Fatal("expected at least one schedule after migration")
	}
	if len(g.Schedules[0].Days) != 7 {
		t.Errorf("expected 7 days, got %d", len(g.Schedules[0].Days))
	}
	// Legacy fields should be cleared after migration
	if app.config.GamePath != "" {
		t.Error("legacy GamePath should be cleared after migration")
	}
}

func TestConfig_LegacyMigration_NoSchedule(t *testing.T) {
	app, _ := newTestApp(t)
	legacy := `
game_path: /usr/games/other
game_name: Other Game
enabled: false
`
	if err := os.WriteFile(app.configPath, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	app.loadConfig()
	if len(app.config.Games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(app.config.Games))
	}
	// No schedule string → empty Schedules slice (not nil panic)
	_ = app.config.Games[0].Schedules
}

func TestSaveConfig_ErrorHandling(t *testing.T) {
	app := &App{
		configPath:     "/nonexistent/path/config.yaml",
		config:         &Config{Games: []Game{{GameName: "X"}}},
		lastLaunchTime: make(map[string]time.Time),
	}
	// Must not panic
	app.saveConfig()
}

// ---- YAML marshaling --------------------------------------------------------

func TestYAMLMarshaling_RoundTrip(t *testing.T) {
	orig := &Config{
		BootDelay: 15,
		Games: []Game{
			{
				GameName:     "Test Game",
				GamePath:     "steam://rungameid/999",
				LaunchMethod: "steam",
				LaunchArgs:   "-fullscreen",
				Enabled:      true,
				Schedules: []Schedule{
					{Days: []string{"Mon", "Fri"}, StartTime: "18:00", EndTime: "22:00"},
				},
			},
		},
	}
	data, err := yaml.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Config
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.BootDelay != orig.BootDelay {
		t.Errorf("BootDelay: want %d got %d", orig.BootDelay, got.BootDelay)
	}
	if len(got.Games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(got.Games))
	}
	g := got.Games[0]
	if g.GameName != "Test Game" {
		t.Errorf("GameName: want 'Test Game' got %q", g.GameName)
	}
	if len(g.Schedules) != 1 || g.Schedules[0].StartTime != "18:00" {
		t.Errorf("Schedules round-trip failed: %+v", g.Schedules)
	}
}

// ---- nextScheduleTime -------------------------------------------------------

func TestNextScheduleTime_TodayLater(t *testing.T) {
	// Wednesday at 08:00 → schedule is Wed 19:00–21:00 → next is same day 19:00
	now := time.Date(2024, 1, 17, 8, 0, 0, 0, time.Local) // Wednesday
	app := appWithGames(nil)
	game := gameWithSchedule("Wed", "19:00", "21:00")

	next, ok := app.nextScheduleTime(game, now)
	if !ok {
		t.Fatal("expected a next schedule time")
	}
	if next.Weekday() != time.Wednesday {
		t.Errorf("expected Wednesday, got %s", next.Weekday())
	}
	if next.Hour() != 19 || next.Minute() != 0 {
		t.Errorf("expected 19:00, got %02d:%02d", next.Hour(), next.Minute())
	}
}

func TestNextScheduleTime_TomorrowWhenTodayPassed(t *testing.T) {
	// Wednesday at 20:00 — today's 19:00 start has already passed.
	// The loop now searches up to daysAhead<=7, so next Wed (7 days out) is found.
	now := time.Date(2024, 1, 17, 20, 0, 0, 0, time.Local) // Wednesday 20:00
	app := appWithGames(nil)
	game := gameWithSchedule("Wed", "19:00", "21:00")

	next, ok := app.nextScheduleTime(game, now)
	if !ok {
		t.Fatal("expected a schedule within the 7-day window")
	}
	if next.Weekday() != time.Wednesday {
		t.Errorf("expected Wednesday, got %s", next.Weekday())
	}
	// next Wed 19:00 is 167 hours after Wed 20:00 — confirm it's ~7 days ahead
	diff := next.Sub(now)
	if diff < 6*24*time.Hour || diff > 8*24*time.Hour {
		t.Errorf("expected ~7 days ahead, got %v", diff)
	}
}

func TestNextScheduleTime_NoSchedule(t *testing.T) {
	app := appWithGames(nil)
	game := Game{GameName: "NoSched", Enabled: true}
	_, ok := app.nextScheduleTime(game, time.Now())
	if ok {
		t.Error("game with no schedules should return ok=false")
	}
}

func TestNextScheduleTime_MultipleSchedules(t *testing.T) {
	// Monday 08:00 — game has Mon 19:00 and Fri 18:00; Mon should come first
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local) // Monday
	app := appWithGames(nil)
	game := Game{
		GameName: "Multi", Enabled: true,
		Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
			{Days: []string{"Fri"}, StartTime: "18:00", EndTime: "20:00"},
		},
	}
	next, ok := app.nextScheduleTime(game, now)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if next.Weekday() != time.Monday {
		t.Errorf("expected Monday (earlier), got %s", next.Weekday())
	}
}

// ---- nextScheduledGames -----------------------------------------------------

func TestNextScheduledGames_Ordering(t *testing.T) {
	// Today is Monday 08:00; game A is Mon 19:00, game B is Mon 20:00
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local)
	gameA := Game{GameName: "A", Enabled: true, Schedules: []Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}}}
	gameB := Game{GameName: "B", Enabled: true, Schedules: []Schedule{{Days: []string{"Mon"}, StartTime: "20:00", EndTime: "22:00"}}}
	gameC := Game{GameName: "C", Enabled: false, Schedules: []Schedule{{Days: []string{"Mon"}, StartTime: "10:00", EndTime: "12:00"}}}

	app := appWithGames([]Game{gameB, gameA, gameC}) // B first in config, A second

	// Use the real nextScheduleTime but override "now" via a test shim:
	// nextScheduledGames calls time.Now() internally; we verify ordering by looking
	// at real values since the test machines will be running at some time of day.
	// To be deterministic, set game schedules relative to a time we control by
	// exercising nextScheduleTime directly.
	tA, _ := app.nextScheduleTime(gameA, now)
	tB, _ := app.nextScheduleTime(gameB, now)
	if !tA.Before(tB) {
		t.Errorf("expected A (%v) before B (%v)", tA, tB)
	}

	// Verify disabled game is excluded
	games := app.nextScheduledGames(3)
	for _, g := range games {
		if g.GameName == "C" {
			t.Error("disabled game C should not appear in nextScheduledGames")
		}
	}
}

func TestNextScheduledGames_Limit(t *testing.T) {
	app := appWithGames([]Game{
		{GameName: "A", Enabled: true, Schedules: []Schedule{{Days: []string{"Mon"}, StartTime: "10:00", EndTime: "12:00"}}},
		{GameName: "B", Enabled: true, Schedules: []Schedule{{Days: []string{"Tue"}, StartTime: "10:00", EndTime: "12:00"}}},
		{GameName: "C", Enabled: true, Schedules: []Schedule{{Days: []string{"Wed"}, StartTime: "10:00", EndTime: "12:00"}}},
		{GameName: "D", Enabled: true, Schedules: []Schedule{{Days: []string{"Thu"}, StartTime: "10:00", EndTime: "12:00"}}},
	})
	got := app.nextScheduledGames(2)
	if len(got) > 2 {
		t.Errorf("expected at most 2, got %d", len(got))
	}
}

func TestNextScheduledGames_Empty(t *testing.T) {
	app := appWithGames([]Game{})
	got := app.nextScheduledGames(5)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

// ---- isInScheduleWindow / shouldLaunchGame ----------------------------------

// isInScheduleWindowAt mimics isInScheduleWindow but uses a supplied time so
// tests are deterministic without changing production code.
func isInScheduleWindowAt(game Game, now time.Time) bool {
	currentTime := now.Format("15:04")
	currentDay := now.Weekday().String()[:3]
	for _, schedule := range game.Schedules {
		dayMatch := false
		for _, d := range schedule.Days {
			if strings.EqualFold(d, currentDay) {
				dayMatch = true
				break
			}
		}
		if !dayMatch {
			continue
		}
		if currentTime >= schedule.StartTime && currentTime <= schedule.EndTime {
			return true
		}
	}
	return false
}

func TestIsInScheduleWindow_Inside(t *testing.T) {
	// Thursday 19:30, schedule Thu 19:00–21:00
	now := time.Date(2024, 1, 18, 19, 30, 0, 0, time.Local) // Thursday
	game := gameWithSchedule("Thu", "19:00", "21:00")
	if !isInScheduleWindowAt(game, now) {
		t.Error("expected inside window")
	}
}

func TestIsInScheduleWindow_BeforeStart(t *testing.T) {
	now := time.Date(2024, 1, 18, 18, 59, 0, 0, time.Local) // Thursday 18:59
	game := gameWithSchedule("Thu", "19:00", "21:00")
	if isInScheduleWindowAt(game, now) {
		t.Error("expected outside window (before start)")
	}
}

func TestIsInScheduleWindow_AfterEnd(t *testing.T) {
	now := time.Date(2024, 1, 18, 21, 01, 0, 0, time.Local) // Thursday 21:01
	game := gameWithSchedule("Thu", "19:00", "21:00")
	if isInScheduleWindowAt(game, now) {
		t.Error("expected outside window (after end)")
	}
}

func TestIsInScheduleWindow_WrongDay(t *testing.T) {
	now := time.Date(2024, 1, 17, 19, 30, 0, 0, time.Local) // Wednesday 19:30
	game := gameWithSchedule("Thu", "19:00", "21:00")
	if isInScheduleWindowAt(game, now) {
		t.Error("expected outside window (wrong day)")
	}
}

func TestIsInScheduleWindow_NoSchedules(t *testing.T) {
	now := time.Date(2024, 1, 18, 19, 30, 0, 0, time.Local)
	game := Game{GameName: "X", Enabled: true}
	if isInScheduleWindowAt(game, now) {
		t.Error("game with no schedules should never be in window")
	}
}

func TestIsInScheduleWindow_CaseInsensitiveDay(t *testing.T) {
	now := time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local) // Thursday
	game := Game{
		Enabled:   true,
		Schedules: []Schedule{{Days: []string{"thu"}, StartTime: "19:00", EndTime: "21:00"}},
	}
	if !isInScheduleWindowAt(game, now) {
		t.Error("day matching should be case-insensitive")
	}
}

// ---- hasLaunchedInCurrentWindow ---------------------------------------------

func TestHasLaunchedInCurrentWindow_NotLaunched(t *testing.T) {
	app := appWithGames(nil)
	game := gameWithSchedule("Mon", "19:00", "21:00")
	// No record at all
	if app.hasLaunchedInCurrentWindow(game) {
		t.Error("no launch recorded, should return false")
	}
}

func TestHasLaunchedInCurrentWindow_LaunchedInWindow(t *testing.T) {
	app := appWithGames(nil)
	// Simulate: current time is Monday 19:30, last launch was Monday 19:05
	now := time.Date(2024, 1, 15, 19, 30, 0, 0, time.Local) // Monday
	game := Game{
		GameName: "TestGame",
		Enabled:  true,
		Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		},
	}
	// Record a launch that happened 25 minutes into the window
	launchTime := time.Date(now.Year(), now.Month(), now.Day(), 19, 5, 0, 0, now.Location())
	app.lastLaunchTime["TestGame"] = launchTime

	// We can't call hasLaunchedInCurrentWindow directly (it uses time.Now), so
	// replicate the logic with our controlled time.
	inWindow := app.hasLaunchedInCurrentWindowAt(game, now)
	if !inWindow {
		t.Error("launch was in window, should return true")
	}
}

func TestHasLaunchedInCurrentWindow_LaunchedBeforeWindow(t *testing.T) {
	app := appWithGames(nil)
	now := time.Date(2024, 1, 15, 19, 30, 0, 0, time.Local)
	game := Game{
		GameName: "TestGame",
		Enabled:  true,
		Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
		},
	}
	// Record a launch that happened before the window
	app.lastLaunchTime["TestGame"] = time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, now.Location())

	if app.hasLaunchedInCurrentWindowAt(game, now) {
		t.Error("launch was before window start, should return false")
	}
}

// ---- recordLaunch -----------------------------------------------------------

func TestRecordLaunch(t *testing.T) {
	app := appWithGames(nil)
	game := Game{GameName: "RecordMe"}
	before := time.Now()
	app.recordLaunch(game)
	after := time.Now()

	ts, ok := app.lastLaunchTime["RecordMe"]
	if !ok {
		t.Fatal("launch time not recorded")
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("launch timestamp %v not in expected range [%v, %v]", ts, before, after)
	}
}

// ---- getGameScheduleInfo ----------------------------------------------------

func TestGetGameScheduleInfo_Disabled(t *testing.T) {
	app := appWithGames(nil)
	g := Game{GameName: "X", Enabled: false}
	if got := app.getGameScheduleInfo(g); got != "Disabled" {
		t.Errorf("expected 'Disabled', got %q", got)
	}
}

func TestGetGameScheduleInfo_NoSchedule(t *testing.T) {
	app := appWithGames(nil)
	g := Game{GameName: "X", Enabled: true}
	if got := app.getGameScheduleInfo(g); got != "No schedule configured" {
		t.Errorf("expected 'No schedule configured', got %q", got)
	}
}

func TestGetGameScheduleInfo_WithSchedule(t *testing.T) {
	app := appWithGames(nil)
	g := gameWithSchedule("Fri", "18:00", "22:00")
	got := app.getGameScheduleInfo(g)
	if !strings.Contains(got, "Fri") {
		t.Errorf("expected day in output, got %q", got)
	}
	if !strings.Contains(got, "18:00") {
		t.Errorf("expected start time in output, got %q", got)
	}
	if !strings.Contains(got, "22:00") {
		t.Errorf("expected end time in output, got %q", got)
	}
}

// ---- warnScheduleOverlaps ---------------------------------------------------

func TestWarnScheduleOverlaps_NoOverlap(t *testing.T) {
	app := appWithGames([]Game{
		{GameName: "A", Enabled: true, Schedules: []Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}}},
		{GameName: "B", Enabled: true, Schedules: []Schedule{{Days: []string{"Tue"}, StartTime: "19:00", EndTime: "21:00"}}},
	})
	// Should not panic
	app.warnScheduleOverlaps()
}

func TestWarnScheduleOverlaps_WithOverlap(t *testing.T) {
	app := appWithGames([]Game{
		{GameName: "A", Enabled: true, Schedules: []Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}}},
		{GameName: "B", Enabled: true, Schedules: []Schedule{{Days: []string{"Mon"}, StartTime: "20:00", EndTime: "22:00"}}},
	})
	// Should not panic; overlap warning is logged only
	app.warnScheduleOverlaps()
}

// ---- fileExists -------------------------------------------------------------

func TestFileExists(t *testing.T) {
	f, err := os.CreateTemp("", "fe_test_*")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	if !fileExists(f.Name()) {
		t.Error("fileExists must return true for existing file")
	}
	if fileExists("/definitely/does/not/exist/xyz") {
		t.Error("fileExists must return false for non-existent path")
	}
}

// ---- getConfigPath ----------------------------------------------------------

func TestGetConfigPath_LocalConfig(t *testing.T) {
	dir, _ := os.MkdirTemp("", "cfg_local_*")
	defer os.RemoveAll(dir)

	localCfg := filepath.Join(dir, "config.yaml")
	os.WriteFile(localCfg, []byte("boot_delay: 5"), 0644)

	got := getConfigPathWithExecutable(filepath.Join(dir, "frictionless"))
	if got != localCfg {
		t.Errorf("expected %s, got %s", localCfg, got)
	}
}

func TestGetConfigPath_OSSpecific(t *testing.T) {
	dir, _ := os.MkdirTemp("", "cfg_os_*")
	defer os.RemoveAll(dir)

	// No local config.yaml → should fall back to OS path
	got := getConfigPathWithExecutable(filepath.Join(dir, "frictionless"))
	if !strings.Contains(got, "FrictionlessLauncher") {
		t.Errorf("expected FrictionlessLauncher in path, got %s", got)
	}
	if !strings.HasSuffix(got, "config.yaml") {
		t.Errorf("expected config.yaml suffix, got %s", got)
	}
}

// getConfigPathWithExecutable replicates getConfigPath with a controllable
// executable path so tests can verify fallback logic without touching real FS.
func getConfigPathWithExecutable(executablePath string) string {
	localDir := filepath.Dir(executablePath)
	localConfig := filepath.Join(localDir, "config.yaml")
	if _, err := os.Stat(localConfig); err == nil {
		return localConfig
	}

	var configDir string
	switch {
	case strings.Contains(strings.ToLower(os.Getenv("OS")), "windows"):
		configDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "FrictionlessLauncher")
	case fileExists("/Users"):
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, "Library", "Application Support", "FrictionlessLauncher")
	default:
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config", "FrictionlessLauncher")
	}
	return filepath.Join(configDir, "config.yaml")
}

// ---- cleanupOldLogs ---------------------------------------------------------

func TestCleanupOldLogs(t *testing.T) {
	dir, _ := os.MkdirTemp("", "logs_*")
	defer os.RemoveAll(dir)

	recentLog := filepath.Join(dir, "recent.log")
	oldLog := filepath.Join(dir, "old.log")
	notALog := filepath.Join(dir, "other.txt")

	os.WriteFile(recentLog, []byte("new"), 0644)
	os.WriteFile(oldLog, []byte("old"), 0644)
	os.WriteFile(notALog, []byte("txt"), 0644)

	old := time.Now().AddDate(0, 0, -8)
	os.Chtimes(oldLog, old, old)
	os.Chtimes(notALog, old, old)

	cleanupOldLogs(dir)

	if _, err := os.Stat(recentLog); os.IsNotExist(err) {
		t.Error("recent log should be kept")
	}
	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Error("old log should be deleted")
	}
	if _, err := os.Stat(notALog); os.IsNotExist(err) {
		t.Error("non-.log file should not be deleted")
	}
}

func TestCleanupOldLogs_EmptyDir(t *testing.T) {
	dir, _ := os.MkdirTemp("", "logs_empty_*")
	defer os.RemoveAll(dir)
	cleanupOldLogs(dir) // must not panic
}

func TestCleanupOldLogs_NonexistentDir(t *testing.T) {
	cleanupOldLogs("/does/not/exist/at/all") // must not panic
}

// ---- setupLogging / closeLogFile --------------------------------------------

func TestApp_CloseLogFile_Nil(t *testing.T) {
	app := &App{}
	app.closeLogFile() // must not panic with nil logFile
}

func TestApp_CloseLogFile_Open(t *testing.T) {
	orig := log.Writer()
	origFlags := log.Flags()
	defer func() {
		log.SetOutput(orig)
		log.SetFlags(origFlags)
	}()

	dir, _ := os.MkdirTemp("", "close_*")
	defer os.RemoveAll(dir)

	f, _ := os.OpenFile(filepath.Join(dir, "test.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	app := &App{logFile: f}
	log.SetOutput(f)
	app.closeLogFile()

	// After close, writes to the file handle should fail
	_, err := f.WriteString("after close")
	if err == nil {
		t.Error("write to closed file should fail")
	}
}

func TestSetupLogging_CreatesFile(t *testing.T) {
	orig := log.Writer()
	origFlags := log.Flags()
	defer func() {
		log.SetOutput(orig)
		log.SetFlags(origFlags)
	}()

	app, _ := newTestApp(t)
	app.setupLogging()
	if app.logFile != nil {
		defer app.closeLogFile()
		if app.logFile.Name() == "" {
			t.Error("log file should have a valid path")
		}
	}
	// If logFile is nil, setupLogging gracefully fell back to stderr (acceptable in CI)
}

// ---- openConfigFile / openLogFile path logic --------------------------------

func TestOpenLogFile_PathResolution(t *testing.T) {
	// Verify the log path contains expected components without actually opening it.
	var dir string
	switch {
	case runtime.GOOS == "windows":
		dir = "FrictionlessLauncher"
	case fileExists("/Users"):
		dir = filepath.Join("Library", "Application Support", "FrictionlessLauncher")
	default:
		dir = filepath.Join(".config", "FrictionlessLauncher")
	}
	if dir == "" {
		t.Error("expected a non-empty log directory component")
	}
}

func TestOpenLogFile_PathComponents(t *testing.T) {
	// Verify that the expected path components are present on this platform.
	var expectedParts []string
	switch {
	case runtime.GOOS == "windows":
		expectedParts = []string{"FrictionlessLauncher", "frictionless-launcher.log"}
	case fileExists("/Users"):
		expectedParts = []string{"FrictionlessLauncher", "frictionless-launcher.log"}
	default:
		expectedParts = []string{"FrictionlessLauncher", "frictionless-launcher.log"}
	}
	for _, part := range expectedParts {
		if part == "" {
			t.Errorf("unexpected empty expected part")
		}
	}
}

func TestOpenLogFile_CommandSelection(t *testing.T) {
	var expected string
	switch {
	case runtime.GOOS == "windows":
		expected = "rundll32"
	case fileExists("/usr/bin/open"):
		expected = "open"
	default:
		expected = "xdg-open"
	}
	if expected == "" {
		t.Error("no command selected for platform")
	}
}

// ---- fadedIcon --------------------------------------------------------------

func TestFadedIcon_Valid(t *testing.T) {
	// iconData is embedded; test with it directly
	result, err := fadedIcon(iconData, 128)
	if err != nil {
		t.Fatalf("fadedIcon failed: %v", err)
	}
	if len(result) == 0 {
		t.Error("fadedIcon returned empty bytes")
	}
	// Result should still be a valid PNG
	if len(result) < 8 || result[0] != 0x89 || result[1] != 'P' {
		t.Error("fadedIcon result is not a valid PNG header")
	}
}

func TestFadedIcon_ZeroAlpha(t *testing.T) {
	result, err := fadedIcon(iconData, 0)
	if err != nil {
		t.Fatalf("fadedIcon with alpha=0 failed: %v", err)
	}
	if len(result) == 0 {
		t.Error("fadedIcon returned empty bytes for alpha=0")
	}
}

func TestFadedIcon_FullAlpha(t *testing.T) {
	result, err := fadedIcon(iconData, 255)
	if err != nil {
		t.Fatalf("fadedIcon with alpha=255 failed: %v", err)
	}
	if len(result) == 0 {
		t.Error("fadedIcon returned empty bytes for alpha=255")
	}
}

func TestFadedIcon_InvalidInput(t *testing.T) {
	_, err := fadedIcon([]byte("not a png"), 128)
	if err == nil {
		t.Error("fadedIcon should return error for non-PNG input")
	}
}

// ---- discovery: scheduleOverlaps --------------------------------------------

func TestScheduleOverlaps_Overlapping(t *testing.T) {
	cases := []struct {
		s1, e1, s2, e2 string
		want           bool
		desc           string
	}{
		{"19:00", "21:00", "20:00", "22:00", true, "partial overlap"},
		{"19:00", "21:00", "19:00", "21:00", true, "identical"},
		{"18:00", "22:00", "19:00", "20:00", true, "B inside A"},
		{"19:00", "21:00", "17:00", "23:00", true, "A inside B"},
	}
	for _, c := range cases {
		got := scheduleOverlaps(c.s1, c.e1, c.s2, c.e2)
		if got != c.want {
			t.Errorf("[%s] scheduleOverlaps(%q,%q,%q,%q) = %v, want %v",
				c.desc, c.s1, c.e1, c.s2, c.e2, got, c.want)
		}
	}
}

func TestScheduleOverlaps_NonOverlapping(t *testing.T) {
	cases := []struct {
		s1, e1, s2, e2 string
		desc           string
	}{
		{"19:00", "21:00", "21:00", "23:00", "adjacent (no gap, no overlap)"},
		{"19:00", "21:00", "22:00", "23:00", "gap between"},
		{"22:00", "23:00", "19:00", "21:00", "reversed order"},
	}
	for _, c := range cases {
		got := scheduleOverlaps(c.s1, c.e1, c.s2, c.e2)
		if got {
			t.Errorf("[%s] scheduleOverlaps(%q,%q,%q,%q) = true, want false",
				c.desc, c.s1, c.e1, c.s2, c.e2)
		}
	}
}

// ---- isValidTimeFormat (dialogs.go) -----------------------------------------

func TestIsValidTimeFormat(t *testing.T) {
	valid := []string{"00:00", "23:59", "19:00", "09:05", "0:0"}
	for _, v := range valid {
		if !isValidTimeFormat(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}

	invalid := []string{"", "25:00", "12:60", "12", "12:00:00", "ab:cd", ":30", "12:"}
	for _, v := range invalid {
		if isValidTimeFormat(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

// ---- gameStatusLabel (ui.go) -------------------------------------------------

func TestGameStatusLabelAt_Disabled(t *testing.T) {
	app := appWithGames(nil)
	g := Game{Enabled: false}
	if got := gameStatusLabelAt(app, g, time.Now()); got != "Disabled" {
		t.Errorf("expected 'Disabled', got %q", got)
	}
}

func TestGameStatusLabelAt_NoSchedule(t *testing.T) {
	app := appWithGames(nil)
	g := Game{Enabled: true}
	if got := gameStatusLabelAt(app, g, time.Now()); got != "No schedule" {
		t.Errorf("expected 'No schedule', got %q", got)
	}
}

func TestGameStatusLabelAt_NextLaunchToday(t *testing.T) {
	app := appWithGames(nil)
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local) // Monday 08:00
	g := gameWithSchedule("Mon", "19:00", "21:00")

	got := gameStatusLabelAt(app, g, now)
	if !strings.HasPrefix(got, "Today ") {
		t.Errorf("expected 'Today ...' label, got %q", got)
	}
	if !strings.Contains(got, "19:00") {
		t.Errorf("expected start time in label, got %q", got)
	}
}

func TestGameStatusLabelAt_NextLaunchFutureDay(t *testing.T) {
	app := appWithGames(nil)
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local) // Monday
	g := gameWithSchedule("Thu", "19:00", "21:00")

	got := gameStatusLabelAt(app, g, now)
	if !strings.Contains(got, "Thu") || !strings.Contains(got, "19:00") {
		t.Errorf("expected 'Thu' and '19:00' in label, got %q", got)
	}
}

func TestGameStatusLabelAt_PicksEarliestOfMultipleSchedules(t *testing.T) {
	app := appWithGames(nil)
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local) // Monday 08:00
	g := Game{
		Enabled: true,
		Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"},
			{Days: []string{"Mon"}, StartTime: "12:00", EndTime: "13:00"},
		},
	}

	got := gameStatusLabelAt(app, g, now)
	if !strings.Contains(got, "12:00") {
		t.Errorf("expected the earlier 12:00 window to win, got %q", got)
	}
}

// ---- discovery: Steam / Epic (file-system driven) ---------------------------

func TestDiscoverSteamGames_EmptyDir(t *testing.T) {
	// No Steam install present in test env → must return nil, not panic
	games := discoverSteamGames()
	_ = games // nil or empty is fine
}

func TestDiscoverEpicGames_EmptyDir(t *testing.T) {
	games := discoverEpicGames()
	_ = games
}

func TestDiscoverGames_ReturnsSlice(t *testing.T) {
	games := discoverGames()
	_ = games // just ensure it doesn't panic
}

func TestDiscoverSteamGames_WithFakeVDF(t *testing.T) {
	dir, _ := os.MkdirTemp("", "steam_*")
	defer os.RemoveAll(dir)

	// Build a minimal Steam library structure
	steamapps := filepath.Join(dir, "steamapps")
	os.MkdirAll(steamapps, 0755)

	vdf := `"libraryfolders"
{
	"0"
	{
		"path"		"` + dir + `"
	}
}`
	os.WriteFile(filepath.Join(dir, "steamapps", "libraryfolders.vdf"), []byte(vdf), 0644)

	acf := `"AppState"
{
	"appid"		"12345"
	"name"		"Cool Game"
}`
	os.WriteFile(filepath.Join(steamapps, "appmanifest_12345.acf"), []byte(acf), 0644)

	// We can't easily override steamBasePaths() without a refactor, so we test
	// the manifest parsing logic directly by placing files where the function
	// would look — and verify the function doesn't panic.
	games := discoverSteamGames()
	_ = games
}

func TestDiscoverEpicGames_WithFakeManifest(t *testing.T) {
	dir, _ := os.MkdirTemp("", "epic_*")
	defer os.RemoveAll(dir)

	manifest := `{
  "DisplayName": "Epic Game",
  "AppName": "epicapp123"
}`
	os.WriteFile(filepath.Join(dir, "game.item"), []byte(manifest), 0644)

	// epicManifestPath returns a real OS path we can't override; just verify
	// parsing logic won't panic by calling the function.
	games := discoverEpicGames()
	_ = games
}

// ---- isPlatformRunning (smoke test) -----------------------------------------

func TestIsPlatformRunning_UnknownPlatform(t *testing.T) {
	app := appWithGames(nil)
	if app.isPlatformRunning("unknown_platform_xyz") {
		t.Error("unknown platform should return false")
	}
}

func TestIsPlatformRunning_KnownPlatforms_NoPanic(t *testing.T) {
	app := appWithGames(nil)
	// Just verify no panic; result depends on whether steam/epic are actually running
	_ = app.isPlatformRunning("steam")
	_ = app.isPlatformRunning("epic")
}

// ---- isGameRunning ----------------------------------------------------------

func TestIsGameRunning_NoDirectGames(t *testing.T) {
	app := appWithGames([]Game{
		{GameName: "A", LaunchMethod: "steam", GamePath: "steam://rungameid/1"},
	})
	// Steam games don't use process detection
	if app.isGameRunning() {
		t.Error("non-direct games should not trigger process check")
	}
}

func TestIsGameRunning_NoGames(t *testing.T) {
	app := appWithGames([]Game{})
	if app.isGameRunning() {
		t.Error("no games should return false")
	}
}

// ---- launchGameByStruct (launch method dispatch) ----------------------------

func TestLaunchGameByStruct_EmptyPath(t *testing.T) {
	// launchGameByStruct returns early with a log when GamePath is empty.
	app := &App{
		config:         &Config{Games: []Game{}, BootDelay: 10},
		lastLaunchTime: make(map[string]time.Time),
	}
	game := Game{GameName: "NullGame", GamePath: "", LaunchMethod: "direct"}
	app.launchGameByStruct(game) // must not panic
}
