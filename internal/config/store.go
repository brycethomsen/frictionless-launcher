package config

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"

	"frictionless-launcher/internal/platform"
)

// DefaultBootDelay is the countdown length, in seconds, when config.yaml
// doesn't set boot_delay (or doesn't exist yet).
const DefaultBootDelay = 10

// Default returns the config used before any file exists: no games.
func Default() *Config { return &Config{BootDelay: DefaultBootDelay} }

// Read parses the config file at path, migrating the legacy single-game
// format if needed. migrated reports whether it did, so the caller can save
// the result. A missing file returns an error satisfying
// errors.Is(err, fs.ErrNotExist).
func Read(path string) (cfg *Config, migrated bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	cfg = Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, false, err
	}
	return cfg, MigrateLegacy(cfg), nil
}

// Write saves cfg to path as YAML.
func Write(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("config: nil config")
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Store holds the config loaded from one file and serialises access to it.
//
// In the tray process several goroutines read the config while reloads
// replace it, so Load always publishes a new *Config and never mutates the
// current one: a pointer returned by Current is a consistent snapshot and
// must be treated as read-only. The --manage UI process mutates Current()
// in place and then calls Save, but only from Fyne's single UI goroutine.
type Store struct {
	path string
	mu   sync.Mutex
	cfg  *Config
}

// NewStore returns a Store for the file at path, holding initial (or Default()
// if initial is nil) until Load is called.
func NewStore(path string, initial *Config) *Store {
	if initial == nil {
		initial = Default()
	}
	return &Store{path: path, cfg: initial}
}

// Path returns the config file path.
func (s *Store) Path() string { return s.path }

// Current returns the current config.
func (s *Store) Current() *Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Load (re)reads the config file. If it doesn't exist yet, the current
// config is written to create it. If it can't be read or parsed, the current
// config is kept: reloads are triggered by writes from the --manage process,
// so a reload can race a half-finished write, and swapping in a blank config
// then would drop every game. A legacy config is migrated and saved back.
func (s *Store) Load() {
	cfg, migrated, err := Read(s.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		log.Println("No config found, creating empty config.yaml")
		if err := s.Save(); err != nil {
			log.Printf("Error saving config: %v", err)
		}
		return
	case err != nil:
		log.Printf("Error loading config: %v", err)
		log.Println("Keeping previously loaded config - please check your config.yaml file for syntax errors")
		return
	}

	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()

	if migrated {
		log.Println("Migrated legacy config format to new multi-game format")
		if err := s.Save(); err != nil {
			log.Printf("Error saving config: %v", err)
		}
	}
	log.Printf("Loaded config with %d game(s)", len(cfg.Games))
}

// Save writes the current config to the file.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Write(s.path, s.cfg)
}

// reloadDebounce coalesces the burst of events a single save produces.
// os.WriteFile truncates and then writes, which arrives as two Write events;
// reloading on the first one would briefly read an empty file and publish an
// empty game list.
const reloadDebounce = 300 * time.Millisecond

// Watch calls onChange (on its own goroutine) after path changes, until stop
// is closed (never, if stop is nil). Bursts of events from one save are
// coalesced into a single call. It blocks, so run it on its own goroutine.
func Watch(path string, stop <-chan struct{}, onChange func()) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Error creating file watcher: %v", err)
		return
	}
	defer watcher.Close()

	// Watch the config file
	if err := watcher.Add(path); err != nil {
		log.Printf("Error watching config file: %v", err)
		return
	}

	log.Printf("Watching config file for changes: %s", path)

	var (
		debounce *time.Timer // only touched from this goroutine
		mu       sync.Mutex  // guards rewatch, shared with the timer goroutine
		rewatch  bool
	)
	reload := func() {
		mu.Lock()
		needRewatch := rewatch
		rewatch = false
		mu.Unlock()
		if needRewatch {
			// Editors that save by writing a new file and renaming it over the
			// old one drop our watch on the old inode; re-arm it.
			if err := watcher.Add(path); err != nil {
				log.Printf("Error re-watching config file: %v", err)
			}
		}
		log.Printf("Config file changed, reloading...")
		onChange()
	}

	for {
		select {
		case <-stop:
			if debounce != nil {
				debounce.Stop()
			}
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) &&
				!event.Has(fsnotify.Remove) && !event.Has(fsnotify.Rename) {
				continue
			}
			if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
				mu.Lock()
				rewatch = true
				mu.Unlock()
			}
			if debounce == nil {
				debounce = time.AfterFunc(reloadDebounce, reload)
			} else {
				debounce.Reset(reloadDebounce)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

// DefaultPath returns the config file path to use: a config.yaml next to
// executablePath if one exists there (portable/dev installs), otherwise an
// OS-appropriate app-support location, which it creates if needed.
func DefaultPath(executablePath string) string {
	// Try local directory first (for development/portable installs)
	localDir := filepath.Dir(executablePath)
	localConfig := filepath.Join(localDir, "config.yaml")

	if _, err := os.Stat(localConfig); err == nil {
		return localConfig
	}

	// Fall back to the per-user app data directory
	configDir := platform.DataDir()

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		log.Printf("Warning: Could not create config directory %s: %v", configDir, err)
		// Fall back to local directory
		return localConfig
	}

	return filepath.Join(configDir, "config.yaml")
}
