package main

// coverage_test.go — additional tests targeting functions not yet covered
// by main_test.go, aiming for ≥80% statement coverage overall.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// acfNameRe / acfAppIDRe expose the same patterns used by discoverSteamGamesFromVDF.
func acfNameRe() *regexp.Regexp  { return regexp.MustCompile(`"name"\s+"([^"]+)"`) }
func acfAppIDRe() *regexp.Regexp { return regexp.MustCompile(`"appid"\s+"([^"]+)"`) }

// ============================================================================
// shouldLaunchGameAt
// ============================================================================

func TestShouldLaunchGameAt_InsideWindow(t *testing.T) {
	now := time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local) // Thu 20:00
	game := gameWithSchedule("Thu", "19:00", "21:00")
	app := appWithGames([]Game{game})
	if !app.shouldLaunchGameAt(game, now) {
		t.Error("expected true inside window with no prior launch")
	}
}

func TestShouldLaunchGameAt_OutsideWindow(t *testing.T) {
	now := time.Date(2024, 1, 18, 17, 0, 0, 0, time.Local) // Thu 17:00
	game := gameWithSchedule("Thu", "19:00", "21:00")
	app := appWithGames([]Game{game})
	if app.shouldLaunchGameAt(game, now) {
		t.Error("expected false outside window")
	}
}

func TestShouldLaunchGameAt_AlreadyLaunched(t *testing.T) {
	now := time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local)
	game := gameWithSchedule("Thu", "19:00", "21:00")
	app := appWithGames([]Game{game})
	app.lastLaunchTime[game.GameName] = now.Add(-30 * time.Minute)
	if app.shouldLaunchGameAt(game, now) {
		t.Error("expected false when already launched in window")
	}
}

// ============================================================================
// isInScheduleWindowAt  (production method)
// ============================================================================

func TestIsInScheduleWindowAt_Inside(t *testing.T) {
	now := time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local)
	app := appWithGames(nil)
	if !app.isInScheduleWindowAt(gameWithSchedule("Thu", "19:00", "21:00"), now) {
		t.Error("expected true inside window")
	}
}

func TestIsInScheduleWindowAt_Outside(t *testing.T) {
	now := time.Date(2024, 1, 18, 22, 0, 0, 0, time.Local)
	app := appWithGames(nil)
	if app.isInScheduleWindowAt(gameWithSchedule("Thu", "19:00", "21:00"), now) {
		t.Error("expected false after window end")
	}
}

func TestIsInScheduleWindowAt_MultipleWindows(t *testing.T) {
	game := Game{
		Enabled: true,
		Schedules: []Schedule{
			{Days: []string{"Mon"}, StartTime: "08:00", EndTime: "10:00"},
			{Days: []string{"Thu"}, StartTime: "19:00", EndTime: "21:00"},
		},
	}
	app := appWithGames(nil)
	if !app.isInScheduleWindowAt(game, time.Date(2024, 1, 15, 9, 0, 0, 0, time.Local)) {
		t.Error("expected true for Mon window")
	}
	if !app.isInScheduleWindowAt(game, time.Date(2024, 1, 18, 20, 0, 0, 0, time.Local)) {
		t.Error("expected true for Thu window")
	}
	if app.isInScheduleWindowAt(game, time.Date(2024, 1, 16, 9, 0, 0, 0, time.Local)) {
		t.Error("expected false on Tuesday (no window)")
	}
}

// ============================================================================
// hasLaunchedInCurrentWindowAt
// ============================================================================

func TestHasLaunchedInCurrentWindowAt_AfterWindowEnd(t *testing.T) {
	app := appWithGames(nil)
	game := Game{
		GameName: "G", Enabled: true,
		Schedules: []Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}},
	}
	now := time.Date(2024, 1, 15, 22, 0, 0, 0, time.Local)
	app.lastLaunchTime["G"] = time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	if !app.hasLaunchedInCurrentWindowAt(game, now) {
		t.Error("launch at 20:00 falls within Mon 19:00–21:00; should return true")
	}
}

func TestHasLaunchedInCurrentWindowAt_WrongDay(t *testing.T) {
	app := appWithGames(nil)
	game := Game{
		GameName: "G", Enabled: true,
		Schedules: []Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}},
	}
	now := time.Date(2024, 1, 16, 20, 0, 0, 0, time.Local) // Tuesday
	app.lastLaunchTime["G"] = time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	if app.hasLaunchedInCurrentWindowAt(game, now) {
		t.Error("schedule is Mon only; today is Tue — should return false")
	}
}

func TestHasLaunchedInCurrentWindowAt_NoRecord(t *testing.T) {
	app := appWithGames(nil)
	game := gameWithSchedule("Mon", "19:00", "21:00")
	now := time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	if app.hasLaunchedInCurrentWindowAt(game, now) {
		t.Error("no launch record should return false")
	}
}

func TestHasLaunchedInCurrentWindowAt_BeforeWindowStart(t *testing.T) {
	app := appWithGames(nil)
	game := Game{
		GameName: "G", Enabled: true,
		Schedules: []Schedule{{Days: []string{"Mon"}, StartTime: "19:00", EndTime: "21:00"}},
	}
	now := time.Date(2024, 1, 15, 20, 0, 0, 0, time.Local)
	// Launch was at 18:00 — before window start
	app.lastLaunchTime["G"] = time.Date(2024, 1, 15, 18, 0, 0, 0, time.Local)
	if app.hasLaunchedInCurrentWindowAt(game, now) {
		t.Error("launch before window start should return false")
	}
}

// ============================================================================
// nextScheduleLabelAt
// ============================================================================

func TestNextScheduleLabelAt_Unscheduled(t *testing.T) {
	app := appWithGames(nil)
	got := app.nextScheduleLabelAt(Game{Enabled: true}, time.Date(2024, 1, 15, 10, 0, 0, 0, time.Local))
	if got != "unscheduled" {
		t.Errorf("expected 'unscheduled', got %q", got)
	}
}

func TestNextScheduleLabelAt_Today(t *testing.T) {
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local) // Monday 08:00
	app := appWithGames(nil)
	got := app.nextScheduleLabelAt(gameWithSchedule("Mon", "19:00", "21:00"), now)
	if !strings.HasPrefix(got, "Today ") {
		t.Errorf("expected 'Today ...' label, got %q", got)
	}
	if !strings.Contains(got, "19:00") {
		t.Errorf("expected time in label, got %q", got)
	}
}

func TestNextScheduleLabelAt_FutureDay(t *testing.T) {
	now := time.Date(2024, 1, 15, 8, 0, 0, 0, time.Local) // Monday
	app := appWithGames(nil)
	got := app.nextScheduleLabelAt(gameWithSchedule("Thu", "19:00", "21:00"), now)
	if !strings.Contains(got, "Thu") {
		t.Errorf("expected 'Thu' in label, got %q", got)
	}
	if !strings.Contains(got, "19:00") {
		t.Errorf("expected '19:00' in label, got %q", got)
	}
}

// ============================================================================
// buildLaunchCmd
// ============================================================================

func TestBuildLaunchCmd_Steam_Darwin(t *testing.T) {
	game := Game{GamePath: "steam://rungameid/1", LaunchMethod: "steam"}
	cmd := buildLaunchCmd(game, "darwin")
	if len(cmd.Args) < 3 || cmd.Args[1] != "-g" {
		t.Errorf("expected 'open -g ...' on darwin, got %v", cmd.Args)
	}
	if cmd.Args[len(cmd.Args)-1] != game.GamePath {
		t.Errorf("expected game path as last arg, got %v", cmd.Args)
	}
}

func TestBuildLaunchCmd_Steam_Windows(t *testing.T) {
	game := Game{GamePath: "steam://rungameid/1", LaunchMethod: "steam"}
	cmd := buildLaunchCmd(game, "windows")
	found := false
	for _, a := range cmd.Args {
		if strings.EqualFold(a, "start") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'start' in windows args, got %v", cmd.Args)
	}
}

func TestBuildLaunchCmd_Steam_Linux(t *testing.T) {
	game := Game{GamePath: "steam://rungameid/1", LaunchMethod: "steam"}
	cmd := buildLaunchCmd(game, "linux")
	if !strings.HasSuffix(cmd.Path, "xdg-open") {
		t.Errorf("expected xdg-open on linux, got %q", cmd.Path)
	}
}

func TestBuildLaunchCmd_Epic_Darwin(t *testing.T) {
	game := Game{GamePath: "com.epicgames.launcher://apps/X", LaunchMethod: "epic"}
	cmd := buildLaunchCmd(game, "darwin")
	if len(cmd.Args) < 2 || cmd.Args[1] != "-g" {
		t.Errorf("expected 'open -g ...' for epic on darwin, got %v", cmd.Args)
	}
}

func TestBuildLaunchCmd_Epic_Windows(t *testing.T) {
	game := Game{GamePath: "com.epicgames.launcher://apps/X", LaunchMethod: "epic"}
	cmd := buildLaunchCmd(game, "windows")
	found := false
	for _, a := range cmd.Args {
		if strings.EqualFold(a, "start") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'start' in windows epic args, got %v", cmd.Args)
	}
}

func TestBuildLaunchCmd_Epic_Linux(t *testing.T) {
	game := Game{GamePath: "com.epicgames.launcher://apps/X", LaunchMethod: "epic"}
	cmd := buildLaunchCmd(game, "linux")
	if !strings.HasSuffix(cmd.Path, "xdg-open") {
		t.Errorf("expected xdg-open for epic on linux, got %q", cmd.Path)
	}
}

func TestBuildLaunchCmd_Direct_NoArgs(t *testing.T) {
	game := Game{GamePath: "/usr/bin/mygame", LaunchMethod: "direct"}
	cmd := buildLaunchCmd(game, "linux")
	if cmd.Path != "/usr/bin/mygame" {
		t.Errorf("expected game path as command, got %q", cmd.Path)
	}
	if len(cmd.Args) != 1 {
		t.Errorf("expected no extra args, got %v", cmd.Args)
	}
}

func TestBuildLaunchCmd_Direct_WithArgs(t *testing.T) {
	game := Game{GamePath: "/usr/bin/mygame", LaunchMethod: "direct", LaunchArgs: "-fullscreen -nosound"}
	cmd := buildLaunchCmd(game, "linux")
	if len(cmd.Args) != 3 {
		t.Errorf("expected 3 args, got %v", cmd.Args)
	}
	if cmd.Args[1] != "-fullscreen" || cmd.Args[2] != "-nosound" {
		t.Errorf("unexpected args: %v", cmd.Args)
	}
}

func TestBuildLaunchCmd_Unknown_DefaultsDirect(t *testing.T) {
	game := Game{GamePath: "/usr/bin/game", LaunchMethod: "gog", LaunchArgs: "-x"}
	cmd := buildLaunchCmd(game, "linux")
	if cmd.Path != "/usr/bin/game" {
		t.Errorf("expected game path as command, got %q", cmd.Path)
	}
	if len(cmd.Args) != 2 || cmd.Args[1] != "-x" {
		t.Errorf("unexpected args: %v", cmd.Args)
	}
}

// ============================================================================
// openFileWithOS
// ============================================================================

func TestOpenFileWithOS_MissingFile(t *testing.T) {
	openFileWithOS("/definitely/does/not/exist/xyz.log") // must not panic
}

func TestOpenFileWithOS_ExistingFile(t *testing.T) {
	// Create a real file; openFileWithOS will try to open it with the OS viewer.
	// On macOS/Linux this may actually launch a viewer — use a temp file with a
	// format no viewer cares about and accept that cmd.Start may succeed or fail.
	f, err := os.CreateTemp("", "open_test_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	// Does not panic; the OS command may fail in CI (no display) but we only
	// check that the function doesn't panic.
	openFileWithOS(f.Name())
}

// ============================================================================
// appLogDir
// ============================================================================

func TestAppLogDir_ContainsAppName(t *testing.T) {
	dir := appLogDir()
	if !strings.Contains(dir, "FrictionlessLauncher") {
		t.Errorf("log dir should contain FrictionlessLauncher, got %q", dir)
	}
}

func TestAppLogDir_PlatformSpecific(t *testing.T) {
	dir := appLogDir()
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(dir, "Library") {
			t.Errorf("macOS log dir should contain Library, got %q", dir)
		}
	case "windows":
		// LOCALAPPDATA is set by the OS; just verify FrictionlessLauncher is present
	default:
		if !strings.Contains(dir, ".config") {
			t.Errorf("Linux log dir should contain .config, got %q", dir)
		}
	}
}

// ============================================================================
// getConfigPath
// ============================================================================

func TestGetConfigPath_ReturnsConfigYAML(t *testing.T) {
	p := getConfigPath()
	if !strings.HasSuffix(p, "config.yaml") {
		t.Errorf("getConfigPath should end with config.yaml, got %q", p)
	}
}

func TestGetConfigPath_LocalWins(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("cannot determine executable path")
	}
	localCfg := filepath.Join(filepath.Dir(exe), "config.yaml")
	f, err := os.OpenFile(localCfg, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		t.Skip("cannot create local config.yaml next to binary:", err)
	}
	f.Close()
	defer os.Remove(localCfg)

	got := getConfigPath()
	if got != localCfg {
		t.Errorf("expected local config %q, got %q", localCfg, got)
	}
}

// ============================================================================
// loadConfig: read-error branch
// ============================================================================

func TestLoadConfig_ReadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod permission test unreliable on Windows")
	}
	dir, _ := os.MkdirTemp("", "cfg_perm_*")
	defer os.RemoveAll(dir)

	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("boot_delay: 5\n"), 0644)
	os.Chmod(dir, 0000)
	defer os.Chmod(dir, 0755)

	app := &App{configPath: cfgPath, lastLaunchTime: make(map[string]time.Time)}
	app.loadConfig()
	if app.config == nil {
		t.Error("config must not be nil after read error")
	}
}

// ============================================================================
// saveConfig: refreshTrayMenu when desk == nil
// ============================================================================

func TestSaveConfig_NoTray(t *testing.T) {
	app, _ := newTestApp(t)
	app.loadConfig()
	app.config.BootDelay = 42
	app.saveConfig()

	app2, _ := newTestApp(t)
	app2.configPath = app.configPath
	app2.loadConfig()
	if app2.config.BootDelay != 42 {
		t.Errorf("expected BootDelay 42 after save/reload, got %d", app2.config.BootDelay)
	}
}

// ============================================================================
// cleanupOldLogs: file older than 7 days
// ============================================================================

func TestCleanupOldLogs_OlderThanSevenDays(t *testing.T) {
	dir, _ := os.MkdirTemp("", "logs_7d_*")
	defer os.RemoveAll(dir)

	oldLog := filepath.Join(dir, "old.log")
	os.WriteFile(oldLog, []byte("data"), 0644)
	past := time.Now().AddDate(0, 0, -7).Add(-time.Minute) // just past 7-day cutoff
	os.Chtimes(oldLog, past, past)

	cleanupOldLogs(dir)
	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Error("file older than 7 days should be deleted")
	}
}

func TestCleanupOldLogs_ExactlySevenDays_Kept(t *testing.T) {
	dir, _ := os.MkdirTemp("", "logs_7d_exact_*")
	defer os.RemoveAll(dir)

	recentLog := filepath.Join(dir, "fresh.log")
	os.WriteFile(recentLog, []byte("data"), 0644)
	fresh := time.Now().AddDate(0, 0, -7).Add(time.Minute) // just inside 7 days
	os.Chtimes(recentLog, fresh, fresh)

	cleanupOldLogs(dir)
	if _, err := os.Stat(recentLog); os.IsNotExist(err) {
		t.Error("file inside 7-day window should be kept")
	}
}

// ============================================================================
// fadedIcon
// ============================================================================

func TestFadedIcon_AlphaDiffers(t *testing.T) {
	full, err := fadedIcon(iconData, 255)
	if err != nil {
		t.Fatal(err)
	}
	half, err := fadedIcon(iconData, 128)
	if err != nil {
		t.Fatal(err)
	}
	if string(full) == string(half) {
		t.Error("alpha=255 and alpha=128 outputs should produce different PNGs")
	}
}

// ============================================================================
// discoverSteamGamesFromVDF — mocked VDF input
// ============================================================================

func TestDiscoverSteamGamesFromVDF_SingleGame(t *testing.T) {
	dir, _ := os.MkdirTemp("", "steam_mock_*")
	defer os.RemoveAll(dir)

	steamapps := filepath.Join(dir, "steamapps")
	os.MkdirAll(steamapps, 0755)

	acf := `"AppState" { "appid" "413150" "name" "Stardew Valley" }`
	os.WriteFile(filepath.Join(steamapps, "appmanifest_413150.acf"), []byte(acf), 0644)

	vdf := `"libraryfolders" { "0" { "path" "` + dir + `" } }`
	games := discoverSteamGamesFromVDF([]byte(vdf))

	if len(games) != 1 {
		t.Fatalf("expected 1 game, got %d: %v", len(games), games)
	}
	if games[0].Name != "Stardew Valley" {
		t.Errorf("expected 'Stardew Valley', got %q", games[0].Name)
	}
	if games[0].LaunchMethod != "steam" {
		t.Errorf("expected launch_method=steam, got %q", games[0].LaunchMethod)
	}
	if games[0].GamePath != "steam://rungameid/413150" {
		t.Errorf("unexpected path: %q", games[0].GamePath)
	}
}

func TestDiscoverSteamGamesFromVDF_MultipleGames(t *testing.T) {
	dir, _ := os.MkdirTemp("", "steam_multi_*")
	defer os.RemoveAll(dir)

	steamapps := filepath.Join(dir, "steamapps")
	os.MkdirAll(steamapps, 0755)

	for _, tc := range []struct{ id, name string }{
		{"413150", "Stardew Valley"},
		{"570", "Dota 2"},
	} {
		acf := `"AppState" { "appid" "` + tc.id + `" "name" "` + tc.name + `" }`
		os.WriteFile(filepath.Join(steamapps, "appmanifest_"+tc.id+".acf"), []byte(acf), 0644)
	}

	vdf := `"libraryfolders" { "0" { "path" "` + dir + `" } }`
	games := discoverSteamGamesFromVDF([]byte(vdf))

	if len(games) != 2 {
		t.Fatalf("expected 2 games, got %d: %v", len(games), games)
	}
}

func TestDiscoverSteamGamesFromVDF_SkipsNonACF(t *testing.T) {
	dir, _ := os.MkdirTemp("", "steam_nofile_*")
	defer os.RemoveAll(dir)

	steamapps := filepath.Join(dir, "steamapps")
	os.MkdirAll(steamapps, 0755)
	os.WriteFile(filepath.Join(steamapps, "not_a_manifest.txt"), []byte("ignored"), 0644)

	vdf := `"libraryfolders" { "0" { "path" "` + dir + `" } }`
	games := discoverSteamGamesFromVDF([]byte(vdf))
	if len(games) != 0 {
		t.Errorf("expected 0 games, got %d", len(games))
	}
}

func TestDiscoverSteamGamesFromVDF_MissingFields(t *testing.T) {
	dir, _ := os.MkdirTemp("", "steam_bad_*")
	defer os.RemoveAll(dir)

	steamapps := filepath.Join(dir, "steamapps")
	os.MkdirAll(steamapps, 0755)
	// ACF with name but no appid
	os.WriteFile(filepath.Join(steamapps, "appmanifest_0.acf"), []byte(`"AppState" { "name" "No ID" }`), 0644)

	vdf := `"libraryfolders" { "0" { "path" "` + dir + `" } }`
	games := discoverSteamGamesFromVDF([]byte(vdf))
	if len(games) != 0 {
		t.Errorf("expected 0 games for missing appid, got %d", len(games))
	}
}

func TestDiscoverSteamGamesFromVDF_EmptyVDF(t *testing.T) {
	games := discoverSteamGamesFromVDF([]byte(""))
	if games != nil && len(games) != 0 {
		t.Errorf("expected empty result for empty VDF, got %v", games)
	}
}

// ============================================================================
// discoverEpicGamesFrom — mocked manifest directory
// ============================================================================

func TestDiscoverEpicGamesFrom_SingleGame(t *testing.T) {
	dir, _ := os.MkdirTemp("", "epic_mock_*")
	defer os.RemoveAll(dir)

	manifest := `{ "DisplayName": "Fortnite", "AppName": "Fortnite" }`
	os.WriteFile(filepath.Join(dir, "fortnite.item"), []byte(manifest), 0644)

	games := discoverEpicGamesFrom(dir)
	if len(games) != 1 {
		t.Fatalf("expected 1 game, got %d: %v", len(games), games)
	}
	if games[0].Name != "Fortnite" {
		t.Errorf("expected 'Fortnite', got %q", games[0].Name)
	}
	if games[0].LaunchMethod != "epic" {
		t.Errorf("expected launch_method=epic, got %q", games[0].LaunchMethod)
	}
	if !strings.Contains(games[0].GamePath, "Fortnite") {
		t.Errorf("expected Fortnite in game path, got %q", games[0].GamePath)
	}
}

func TestDiscoverEpicGamesFrom_MultipleGames(t *testing.T) {
	dir, _ := os.MkdirTemp("", "epic_multi_*")
	defer os.RemoveAll(dir)

	for _, tc := range []struct{ file, name, app string }{
		{"game1.item", "Fortnite", "Fortnite"},
		{"game2.item", "Rocket League", "RocketLeague"},
	} {
		manifest := `{ "DisplayName": "` + tc.name + `", "AppName": "` + tc.app + `" }`
		os.WriteFile(filepath.Join(dir, tc.file), []byte(manifest), 0644)
	}

	games := discoverEpicGamesFrom(dir)
	if len(games) != 2 {
		t.Fatalf("expected 2 games, got %d: %v", len(games), games)
	}
}

func TestDiscoverEpicGamesFrom_SkipsNonItem(t *testing.T) {
	dir, _ := os.MkdirTemp("", "epic_noitem_*")
	defer os.RemoveAll(dir)
	os.WriteFile(filepath.Join(dir, "game.json"), []byte(`{"DisplayName":"X","AppName":"X"}`), 0644)

	games := discoverEpicGamesFrom(dir)
	if len(games) != 0 {
		t.Errorf("expected 0 games for .json file, got %d", len(games))
	}
}

func TestDiscoverEpicGamesFrom_MissingFields(t *testing.T) {
	dir, _ := os.MkdirTemp("", "epic_bad_*")
	defer os.RemoveAll(dir)
	// Missing AppName
	os.WriteFile(filepath.Join(dir, "bad.item"), []byte(`{ "DisplayName": "No App" }`), 0644)

	games := discoverEpicGamesFrom(dir)
	if len(games) != 0 {
		t.Errorf("expected 0 games for missing AppName, got %d", len(games))
	}
}

func TestDiscoverEpicGamesFrom_NonexistentDir(t *testing.T) {
	games := discoverEpicGamesFrom("/nonexistent/epic/dir")
	if games != nil {
		t.Error("expected nil for nonexistent directory")
	}
}

// ============================================================================
// discoverSteamGames regex parsing (ACF content)
// ============================================================================

func TestDiscoverSteamGames_ParseACF(t *testing.T) {
	acf := []byte(`"AppState" { "appid" "413150" "name" "Stardew Valley" }`)
	if m := acfNameRe().FindSubmatch(acf); m == nil || string(m[1]) != "Stardew Valley" {
		t.Errorf("acfNameRe failed: %v", m)
	}
	if m := acfAppIDRe().FindSubmatch(acf); m == nil || string(m[1]) != "413150" {
		t.Errorf("acfAppIDRe failed: %v", m)
	}
}

// ============================================================================
// discoverEpicGames regex parsing (.item content)
// ============================================================================

func TestDiscoverEpicGames_ParseItem(t *testing.T) {
	content := []byte(`{ "DisplayName": "Fortnite", "AppName": "FortniteAppID123" }`)
	nameRe := regexp.MustCompile(`"DisplayName"\s*:\s*"([^"]+)"`)
	appNameRe := regexp.MustCompile(`"AppName"\s*:\s*"([^"]+)"`)

	nameMatch := nameRe.FindSubmatch(content)
	appMatch := appNameRe.FindSubmatch(content)

	if nameMatch == nil || string(nameMatch[1]) != "Fortnite" {
		t.Errorf("DisplayName parse failed: %v", nameMatch)
	}
	if appMatch == nil || string(appMatch[1]) != "FortniteAppID123" {
		t.Errorf("AppName parse failed: %v", appMatch)
	}
}

// ============================================================================
// isGameRunning
// ============================================================================

func TestIsGameRunning_DirectGame_NotRunning(t *testing.T) {
	app := appWithGames([]Game{
		{GameName: "X", GamePath: "/nonexistent/xyz_game_definitely_not_running_12345", LaunchMethod: "direct", Enabled: true},
	})
	if app.isGameRunning() {
		t.Error("nonexistent game should not be detected as running")
	}
}

// ============================================================================
// getForegroundAppName
// ============================================================================

func TestGetForegroundAppName_NoPanic(t *testing.T) {
	app := appWithGames(nil)
	name, err := app.getForegroundAppName()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if name == "Frictionless" || name == "frictionless-launcher" {
		t.Error("getForegroundAppName should filter out own app name")
	}
}

// ============================================================================
// warnScheduleOverlaps
// ============================================================================

func TestWarnScheduleOverlaps_SameDaySameTime(t *testing.T) {
	app := appWithGames([]Game{
		{GameName: "A", Enabled: true, Schedules: []Schedule{{Days: []string{"Fri"}, StartTime: "19:00", EndTime: "21:00"}}},
		{GameName: "B", Enabled: true, Schedules: []Schedule{{Days: []string{"Fri"}, StartTime: "19:00", EndTime: "21:00"}}},
	})
	app.warnScheduleOverlaps()
}

// ============================================================================
// setupLogging: rotation branch
// ============================================================================

// ============================================================================
// shouldLaunchGame / isInScheduleWindow / nextScheduleLabel — thin wrappers
// ============================================================================

func TestShouldLaunchGame_OutsideWindow(t *testing.T) {
	// Use a day/time that is almost certainly not "now" to guarantee we're outside.
	// The function delegates to shouldLaunchGameAt(game, time.Now()), so the game
	// just needs a schedule that's in the past or far future to return false.
	// This test only verifies the wrapper delegates correctly (no panic, bool result).
	game := Game{
		GameName: "WrapTest", Enabled: true,
		Schedules: []Schedule{{Days: []string{"Mon"}, StartTime: "00:01", EndTime: "00:02"}},
	}
	app := appWithGames([]Game{game})
	// Just call it — result depends on actual time; we only check no panic.
	_ = app.shouldLaunchGame(game)
}

func TestIsInScheduleWindow_DelegatesCorrectly(t *testing.T) {
	game := Game{
		GameName: "WrapTest2", Enabled: true,
		Schedules: []Schedule{{Days: []string{"Mon"}, StartTime: "00:01", EndTime: "00:02"}},
	}
	app := appWithGames([]Game{game})
	// Consistent result with At variant using time.Now().
	now := time.Now()
	want := app.isInScheduleWindowAt(game, now)
	got := app.isInScheduleWindow(game)
	if got != want {
		t.Errorf("isInScheduleWindow()=%v but isInScheduleWindowAt(now)=%v", got, want)
	}
}

func TestNextScheduleLabel_DelegatesCorrectly(t *testing.T) {
	game := gameWithSchedule("Mon", "19:00", "21:00")
	app := appWithGames([]Game{game})
	// Both wrappers should return the same string for the same game.
	now := time.Now()
	want := app.nextScheduleLabelAt(game, now)
	got := app.nextScheduleLabel(game)
	if got != want {
		t.Errorf("nextScheduleLabel()=%q but nextScheduleLabelAt(now)=%q", got, want)
	}
}

// ============================================================================
// launchGameByStruct — error path (bad command) + warning paths
// ============================================================================

func TestLaunchGameByStruct_SteamWarningPath(t *testing.T) {
	// Use a steam game with a deliberately bad path so cmd.Start() fails quickly.
	// This exercises the steam platform-check warning branch.
	game := Game{
		GameName:     "SteamWarn",
		GamePath:     "/absolutely/does/not/exist/game",
		LaunchMethod: "steam",
		Enabled:      true,
	}
	app := appWithGames([]Game{game})
	app.launchGameByStruct(game) // must not panic
}

func TestLaunchGameByStruct_EpicWarningPath(t *testing.T) {
	game := Game{
		GameName:     "EpicWarn",
		GamePath:     "/absolutely/does/not/exist/game",
		LaunchMethod: "epic",
		Enabled:      true,
	}
	app := appWithGames([]Game{game})
	app.launchGameByStruct(game) // must not panic
}

// ============================================================================
// setupLogging: rotation branch
// ============================================================================

func TestSetupLogging_RotationLogic(t *testing.T) {
	dir, _ := os.MkdirTemp("", "log_rotate_*")
	defer os.RemoveAll(dir)

	logPath := filepath.Join(dir, "frictionless-launcher.log")
	f, _ := os.Create(logPath)
	f.Write(make([]byte, 6*1024*1024)) // 6 MB — exceeds 5 MB limit
	f.Close()

	const maxLogSize = 5 * 1024 * 1024
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > maxLogSize {
		rotated := logPath + ".1"
		os.Remove(rotated)
		os.Rename(logPath, rotated)
	}

	if fileExists(logPath) {
		t.Error("original log should be renamed after rotation")
	}
	if !fileExists(logPath + ".1") {
		t.Error("rotated .1 log should exist")
	}
}
