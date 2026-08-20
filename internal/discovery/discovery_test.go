package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- discoverSteamGamesFromVDF — mocked VDF input --------------------------

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

// ---- discoverEpicGamesFrom — mocked manifest directory ----------------------

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

// ---- discoverSteamGames / discoverEpicGames / DiscoverGames (real OS paths) -
//
// The test environment won't have a real Steam/Epic install, so these are
// smoke tests: just confirm the OS-path-driven lookups don't panic. The
// parsing logic itself is covered above via the *From/*FromVDF variants,
// which take a directory as a parameter.

func TestDiscoverSteamGames_EmptyDir(t *testing.T) {
	games := discoverSteamGames()
	_ = games // nil or empty is fine
}

func TestDiscoverEpicGames_EmptyDir(t *testing.T) {
	games := discoverEpicGames()
	_ = games
}

func TestDiscoverGames_ReturnsSlice(t *testing.T) {
	games := DiscoverGames()
	_ = games // just ensure it doesn't panic
}
