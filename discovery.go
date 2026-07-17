package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type DiscoveredGame struct {
	Name         string
	LaunchMethod string
	GamePath     string
}

func discoverGames() []DiscoveredGame {
	var games []DiscoveredGame
	games = append(games, discoverSteamGames()...)
	games = append(games, discoverEpicGames()...)
	return games
}

// --- Steam ---

func steamBasePaths() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return []string{filepath.Join(home, "Library", "Application Support", "Steam")}
	case "windows":
		return []string{
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Steam"),
			filepath.Join(os.Getenv("ProgramFiles"), "Steam"),
		}
	default: // Linux — Steam can be in many places
		return []string{
			filepath.Join(home, ".steam", "debian-installation"),
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".local", "share", "Steam"),
			filepath.Join(home, "snap", "steam", "common", ".local", "share", "Steam"),
			filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam"),
		}
	}
}

func discoverSteamGames() []DiscoveredGame {
	for _, base := range steamBasePaths() {
		vdf := filepath.Join(base, "steamapps", "libraryfolders.vdf")
		if data, err := os.ReadFile(vdf); err == nil {
			return discoverSteamGamesFromVDF(data)
		}
	}
	return nil
}

func discoverSteamGamesFromVDF(vdfData []byte) []DiscoveredGame {
	pathRe := regexp.MustCompile(`"path"\s+"([^"]+)"`)
	nameRe := regexp.MustCompile(`"name"\s+"([^"]+)"`)
	appidRe := regexp.MustCompile(`"appid"\s+"([^"]+)"`)

	var libraryPaths []string
	for _, m := range pathRe.FindAllSubmatch(vdfData, -1) {
		libraryPaths = append(libraryPaths, string(m[1]))
	}

	var games []DiscoveredGame
	for _, lib := range libraryPaths {
		steamapps := filepath.Join(lib, "steamapps")
		entries, err := os.ReadDir(steamapps)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "appmanifest_") || !strings.HasSuffix(e.Name(), ".acf") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(steamapps, e.Name()))
			if err != nil {
				continue
			}
			nameMatch := nameRe.FindSubmatch(content)
			appidMatch := appidRe.FindSubmatch(content)
			if nameMatch == nil || appidMatch == nil {
				continue
			}
			games = append(games, DiscoveredGame{
				Name:         string(nameMatch[1]),
				LaunchMethod: "steam",
				GamePath:     fmt.Sprintf("steam://rungameid/%s", appidMatch[1]),
			})
		}
	}
	return games
}

// --- Epic ---

func epicManifestPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Epic", "EpicGamesLauncher", "Data", "Manifests")
	case "windows":
		// Epic stores manifests in ProgramData, not the user's AppData
		return filepath.Join(os.Getenv("ProgramData"), "Epic", "EpicGamesLauncher", "Data", "Manifests")
	default:
		// Linux via Heroic or Lutris (native Epic launcher doesn't exist on Linux)
		return filepath.Join(home, ".config", "legendary", "metadata")
	}
}

func discoverEpicGames() []DiscoveredGame {
	return discoverEpicGamesFrom(epicManifestPath())
}

func discoverEpicGamesFrom(manifestDir string) []DiscoveredGame {
	entries, err := os.ReadDir(manifestDir)
	if err != nil {
		return nil
	}

	nameRe := regexp.MustCompile(`"DisplayName"\s*:\s*"([^"]+)"`)
	appNameRe := regexp.MustCompile(`"AppName"\s*:\s*"([^"]+)"`)

	var games []DiscoveredGame
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".item") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(manifestDir, e.Name()))
		if err != nil {
			continue
		}
		nameMatch := nameRe.FindSubmatch(content)
		appNameMatch := appNameRe.FindSubmatch(content)
		if nameMatch == nil || appNameMatch == nil {
			continue
		}
		games = append(games, DiscoveredGame{
			Name:         string(nameMatch[1]),
			LaunchMethod: "epic",
			GamePath:     fmt.Sprintf("com.epicgames.launcher://apps/%s?action=launch&silent=true", appNameMatch[1]),
		})
	}
	return games
}
