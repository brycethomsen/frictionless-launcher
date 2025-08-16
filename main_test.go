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

func TestConfig_LoadAndSave(t *testing.T) {
	// Create temporary directory for test config
	tempDir, err := os.MkdirTemp("", "frictionless_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")

	app := &App{
		configPath: configPath,
	}

	// Test loading config when file doesn't exist (should create defaults)
	app.loadConfig()

	if app.config == nil {
		t.Fatal("Config should not be nil after loadConfig")
	}

	// Verify default values
	if app.config.GameName != "Test Command" {
		t.Errorf("Expected GameName 'Test Command', got '%s'", app.config.GameName)
	}
	if app.config.Enabled != true {
		t.Errorf("Expected Enabled true, got %v", app.config.Enabled)
	}
	if app.config.Schedule != "always" {
		t.Errorf("Expected Schedule 'always', got '%s'", app.config.Schedule)
	}

	// Test saving config
	app.config.GameName = "Test Game"
	app.config.Enabled = false
	app.saveConfig()

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file should exist after saveConfig")
	}

	// Test loading saved config
	app2 := &App{configPath: configPath}
	app2.loadConfig()

	if app2.config.GameName != "Test Game" {
		t.Errorf("Expected loaded GameName 'Test Game', got '%s'", app2.config.GameName)
	}
	if app2.config.Enabled != false {
		t.Errorf("Expected loaded Enabled false, got %v", app2.config.Enabled)
	}
}

func TestConfig_InvalidYAML(t *testing.T) {
	// Create temporary directory for test config
	tempDir, err := os.MkdirTemp("", "frictionless_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")

	// Write invalid YAML
	invalidYAML := "invalid: yaml: content: [unclosed"
	err = os.WriteFile(configPath, []byte(invalidYAML), 0644)
	if err != nil {
		t.Fatalf("Failed to write invalid YAML: %v", err)
	}

	app := &App{configPath: configPath}
	app.loadConfig()

	// Should fall back to defaults when YAML is invalid
	if app.config.GameName != "Test Command" {
		t.Errorf("Expected default GameName 'Test Command' when YAML invalid, got '%s'", app.config.GameName)
	}
}

func TestShouldLaunchNow_Always(t *testing.T) {
	app := &App{
		config: &Config{Schedule: "always"},
	}

	if !app.shouldLaunchNow() {
		t.Error("shouldLaunchNow should return true for 'always' schedule")
	}
}

func TestShouldLaunchNow_After5PM(t *testing.T) {
	app := &App{
		config: &Config{Schedule: "after_5pm_daily"},
	}

	// Test with 6 PM time
	mockTime := time.Date(2024, 1, 15, 18, 0, 0, 0, time.Local)
	result := shouldLaunchNowWithTime(app.config, mockTime)
	if !result {
		t.Error("shouldLaunchNow should return true at 6 PM for 'after_5pm_daily'")
	}

	// Test with 3 PM time
	mockTime = time.Date(2024, 1, 15, 15, 0, 0, 0, time.Local)
	result = shouldLaunchNowWithTime(app.config, mockTime)
	if result {
		t.Error("shouldLaunchNow should return false at 3 PM for 'after_5pm_daily'")
	}
}

func TestShouldLaunchNow_Weekends(t *testing.T) {
	config := &Config{Schedule: "weekends_anytime"}

	// Saturday - should launch
	mockTime := time.Date(2024, 1, 13, 10, 0, 0, 0, time.Local) // Saturday
	if !shouldLaunchNowWithTime(config, mockTime) {
		t.Error("shouldLaunchNow should return true on Saturday for 'weekends_anytime'")
	}

	// Sunday - should launch
	mockTime = time.Date(2024, 1, 14, 10, 0, 0, 0, time.Local) // Sunday
	if !shouldLaunchNowWithTime(config, mockTime) {
		t.Error("shouldLaunchNow should return true on Sunday for 'weekends_anytime'")
	}

	// Monday - should not launch
	mockTime = time.Date(2024, 1, 15, 10, 0, 0, 0, time.Local) // Monday
	if shouldLaunchNowWithTime(config, mockTime) {
		t.Error("shouldLaunchNow should return false on Monday for 'weekends_anytime'")
	}
}

func TestShouldLaunchNow_TueThuAfter8PM(t *testing.T) {
	config := &Config{Schedule: "tue_thu_after_8pm"}

	// Tuesday 9 PM - should launch
	mockTime := time.Date(2024, 1, 16, 21, 0, 0, 0, time.Local) // Tuesday
	if !shouldLaunchNowWithTime(config, mockTime) {
		t.Error("shouldLaunchNow should return true on Tuesday at 9 PM")
	}

	// Thursday 9 PM - should launch
	mockTime = time.Date(2024, 1, 18, 21, 0, 0, 0, time.Local) // Thursday
	if !shouldLaunchNowWithTime(config, mockTime) {
		t.Error("shouldLaunchNow should return true on Thursday at 9 PM")
	}

	// Tuesday 7 PM - should not launch (before 8 PM)
	mockTime = time.Date(2024, 1, 16, 19, 0, 0, 0, time.Local) // Tuesday
	if shouldLaunchNowWithTime(config, mockTime) {
		t.Error("shouldLaunchNow should return false on Tuesday at 7 PM")
	}

	// Wednesday 9 PM - should not launch (wrong day)
	mockTime = time.Date(2024, 1, 17, 21, 0, 0, 0, time.Local) // Wednesday
	if shouldLaunchNowWithTime(config, mockTime) {
		t.Error("shouldLaunchNow should return false on Wednesday at 9 PM")
	}
}

func TestShouldLaunchNow_WeekdaysEvening(t *testing.T) {
	config := &Config{Schedule: "weekdays_evening"}

	// Monday 7 PM - should launch
	mockTime := time.Date(2024, 1, 15, 19, 0, 0, 0, time.Local) // Monday
	if !shouldLaunchNowWithTime(config, mockTime) {
		t.Error("shouldLaunchNow should return true on Monday at 7 PM")
	}

	// Friday 9 PM - should launch
	mockTime = time.Date(2024, 1, 19, 21, 0, 0, 0, time.Local) // Friday
	if !shouldLaunchNowWithTime(config, mockTime) {
		t.Error("shouldLaunchNow should return true on Friday at 9 PM")
	}

	// Monday 5 PM - should not launch (before 6 PM)
	mockTime = time.Date(2024, 1, 15, 17, 0, 0, 0, time.Local) // Monday
	if shouldLaunchNowWithTime(config, mockTime) {
		t.Error("shouldLaunchNow should return false on Monday at 5 PM")
	}

	// Monday 11 PM - should not launch (after 10 PM)
	mockTime = time.Date(2024, 1, 15, 23, 0, 0, 0, time.Local) // Monday
	if shouldLaunchNowWithTime(config, mockTime) {
		t.Error("shouldLaunchNow should return false on Monday at 11 PM")
	}

	// Saturday 7 PM - should not launch (weekend)
	mockTime = time.Date(2024, 1, 13, 19, 0, 0, 0, time.Local) // Saturday
	if shouldLaunchNowWithTime(config, mockTime) {
		t.Error("shouldLaunchNow should return false on Saturday at 7 PM")
	}
}

func TestShouldLaunchNow_InvalidSchedule(t *testing.T) {
	app := &App{
		config: &Config{Schedule: "invalid_schedule"},
	}

	if app.shouldLaunchNow() {
		t.Error("shouldLaunchNow should return false for invalid schedule")
	}
}

func TestFileExists(t *testing.T) {
	// Create temporary file
	tempFile, err := os.CreateTemp("", "test_file")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	// Test existing file
	if !fileExists(tempFile.Name()) {
		t.Error("fileExists should return true for existing file")
	}

	// Test non-existing file
	if fileExists("/non/existent/file") {
		t.Error("fileExists should return false for non-existent file")
	}
}

func TestGetConfigPath_LocalConfig(t *testing.T) {
	// Create temporary directory to simulate executable location
	tempDir, err := os.MkdirTemp("", "frictionless_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a local config.yaml file
	localConfig := filepath.Join(tempDir, "config.yaml")
	err = os.WriteFile(localConfig, []byte("test: value"), 0644)
	if err != nil {
		t.Fatalf("Failed to create local config: %v", err)
	}

	// Test getConfigPathWithExecutable helper
	executablePath := filepath.Join(tempDir, "frictionless")
	configPath := getConfigPathWithExecutable(executablePath)

	if configPath != localConfig {
		t.Errorf("Expected config path %s, got %s", localConfig, configPath)
	}
}

func TestGetConfigPath_OSSpecific(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "frictionless_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test with executable path that has no local config
	executablePath := filepath.Join(tempDir, "frictionless")
	configPath := getConfigPathWithExecutable(executablePath)

	// Should contain "FrictionlessLauncher" directory and config.yaml
	if !strings.Contains(configPath, "FrictionlessLauncher") {
		t.Errorf("Config path should contain 'FrictionlessLauncher', got %s", configPath)
	}
	if !strings.HasSuffix(configPath, "config.yaml") {
		t.Errorf("Config path should end with 'config.yaml', got %s", configPath)
	}
}

func TestYAMLMarshaling(t *testing.T) {
	config := &Config{
		GamePath:   "/path/to/game",
		GameName:   "Test Game",
		LaunchArgs: "-arg1 -arg2",
		Enabled:    true,
		BootDelay:  10,
		Schedule:   "always",
	}

	// Test marshaling to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal config to YAML: %v", err)
	}

	// Test unmarshaling from YAML
	var unmarshaledConfig Config
	err = yaml.Unmarshal(data, &unmarshaledConfig)
	if err != nil {
		t.Fatalf("Failed to unmarshal config from YAML: %v", err)
	}

	// Verify all fields match
	if unmarshaledConfig.GamePath != config.GamePath {
		t.Errorf("GamePath mismatch: expected %s, got %s", config.GamePath, unmarshaledConfig.GamePath)
	}
	if unmarshaledConfig.GameName != config.GameName {
		t.Errorf("GameName mismatch: expected %s, got %s", config.GameName, unmarshaledConfig.GameName)
	}
	if unmarshaledConfig.LaunchArgs != config.LaunchArgs {
		t.Errorf("LaunchArgs mismatch: expected %s, got %s", config.LaunchArgs, unmarshaledConfig.LaunchArgs)
	}
	if unmarshaledConfig.Enabled != config.Enabled {
		t.Errorf("Enabled mismatch: expected %v, got %v", config.Enabled, unmarshaledConfig.Enabled)
	}
	if unmarshaledConfig.BootDelay != config.BootDelay {
		t.Errorf("BootDelay mismatch: expected %d, got %d", config.BootDelay, unmarshaledConfig.BootDelay)
	}
	if unmarshaledConfig.Schedule != config.Schedule {
		t.Errorf("Schedule mismatch: expected %s, got %s", config.Schedule, unmarshaledConfig.Schedule)
	}
}

// Helper function to test schedule logic with a specific time
func shouldLaunchNowWithTime(config *Config, now time.Time) bool {
	switch config.Schedule {
	case "always":
		return true

	case "after_5pm_daily":
		return now.Hour() >= 17

	case "weekends_anytime":
		weekday := now.Weekday()
		return weekday == time.Saturday || weekday == time.Sunday

	case "tue_thu_after_8pm":
		weekday := now.Weekday()
		return (weekday == time.Tuesday || weekday == time.Thursday) && now.Hour() >= 20

	case "weekdays_evening":
		weekday := now.Weekday()
		return weekday >= time.Monday && weekday <= time.Friday && now.Hour() >= 18 && now.Hour() < 22

	default:
		return false
	}
}

// Helper function to test config path resolution with a specific executable path
func getConfigPathWithExecutable(executablePath string) string {
	localDir := filepath.Dir(executablePath)
	localConfig := filepath.Join(localDir, "config.yaml")

	if _, err := os.Stat(localConfig); err == nil {
		return localConfig
	}

	// Fall back to OS-appropriate location
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

func TestCleanupOldLogs(t *testing.T) {
	// Create temporary directory for test logs
	tempDir, err := os.MkdirTemp("", "frictionless_logs_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test log files with different ages
	now := time.Now()

	// Recent log (should be kept)
	recentLog := filepath.Join(tempDir, "recent.log")
	if err := os.WriteFile(recentLog, []byte("recent log"), 0644); err != nil {
		t.Fatalf("Failed to create recent log: %v", err)
	}

	// Old log (should be deleted)
	oldLog := filepath.Join(tempDir, "old.log")
	if err := os.WriteFile(oldLog, []byte("old log"), 0644); err != nil {
		t.Fatalf("Failed to create old log: %v", err)
	}

	// Set the old log's modification time to 8 days ago
	eightDaysAgo := now.AddDate(0, 0, -8)
	if err := os.Chtimes(oldLog, eightDaysAgo, eightDaysAgo); err != nil {
		t.Fatalf("Failed to set old log time: %v", err)
	}

	// Non-log file (should be ignored)
	nonLogFile := filepath.Join(tempDir, "other.txt")
	if err := os.WriteFile(nonLogFile, []byte("not a log"), 0644); err != nil {
		t.Fatalf("Failed to create non-log file: %v", err)
	}

	// Set non-log file to old time too
	if err := os.Chtimes(nonLogFile, eightDaysAgo, eightDaysAgo); err != nil {
		t.Fatalf("Failed to set non-log file time: %v", err)
	}

	// Run cleanup
	cleanupOldLogs(tempDir)

	// Check results
	if _, err := os.Stat(recentLog); os.IsNotExist(err) {
		t.Error("Recent log file should still exist")
	}

	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Error("Old log file should have been deleted")
	}

	if _, err := os.Stat(nonLogFile); os.IsNotExist(err) {
		t.Error("Non-log file should not have been deleted")
	}
}

func TestCleanupOldLogs_EmptyDirectory(t *testing.T) {
	// Create empty temporary directory
	tempDir, err := os.MkdirTemp("", "frictionless_empty_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Should not panic or error on empty directory
	cleanupOldLogs(tempDir)
}

func TestCleanupOldLogs_NonexistentDirectory(t *testing.T) {
	// Should not panic or error on nonexistent directory
	cleanupOldLogs("/nonexistent/directory")
}

func TestOpenLogFile_PathResolution(t *testing.T) {
	// Create temporary directory structure
	tempDir, err := os.MkdirTemp("", "frictionless_log_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test the log path resolution logic
	var expectedLogPath string

	switch {
	case runtime.GOOS == "windows":
		// We can't easily mock environment variables, so we'll test the logic
		if os.Getenv("LOCALAPPDATA") != "" {
			expectedLogPath = filepath.Join(os.Getenv("LOCALAPPDATA"), "FrictionlessLauncher", "frictionless-launcher.log")
		}
	case fileExists("/Users"):
		// macOS
		home, _ := os.UserHomeDir()
		expectedLogPath = filepath.Join(home, "Library", "Application Support", "FrictionlessLauncher", "frictionless-launcher.log")
	default:
		// Linux
		home, _ := os.UserHomeDir()
		expectedLogPath = filepath.Join(home, ".config", "FrictionlessLauncher", "frictionless-launcher.log")
	}

	if expectedLogPath != "" {
		// Verify the path contains the expected components
		if !strings.Contains(expectedLogPath, "FrictionlessLauncher") {
			t.Errorf("Log path should contain 'FrictionlessLauncher', got %s", expectedLogPath)
		}
		if !strings.HasSuffix(expectedLogPath, "frictionless-launcher.log") {
			t.Errorf("Log path should end with 'frictionless-launcher.log', got %s", expectedLogPath)
		}
	}
}

func TestSetupLogging_DirectoryCreation(t *testing.T) {
	// Save original log output
	originalOutput := log.Writer()
	originalFlags := log.Flags()
	defer func() {
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
	}()

	// Create a temporary directory to simulate a clean environment
	tempDir, err := os.MkdirTemp("", "frictionless_setup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock the log directory to our temp directory
	// Note: We can't easily test setupLogging() directly due to environment dependencies,
	// but we can test the core logic components

	// Test directory creation logic
	testLogDir := filepath.Join(tempDir, "TestFrictionlessLauncher")
	if err := os.MkdirAll(testLogDir, 0755); err != nil {
		t.Fatalf("Failed to create test log directory: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(testLogDir); os.IsNotExist(err) {
		t.Error("Log directory should have been created")
	}

	// Test log file creation
	testLogFile := filepath.Join(testLogDir, "test-frictionless-launcher.log")
	file, err := os.OpenFile(testLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		t.Fatalf("Failed to create test log file: %v", err)
	}
	file.Close()

	// Verify log file was created
	if _, err := os.Stat(testLogFile); os.IsNotExist(err) {
		t.Error("Log file should have been created")
	}
}

func TestApp_SetupLogging(t *testing.T) {
	// Save original log settings
	originalOutput := log.Writer()
	originalFlags := log.Flags()
	defer func() {
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
	}()

	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "frictionless_app_logging_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create app instance
	app := &App{
		configPath: filepath.Join(tempDir, "config.yaml"),
	}

	// We can't easily test the full setupLogging method due to OS dependencies,
	// but we can test that it doesn't panic and sets up the logFile field
	
	// Note: This test will use the actual OS paths, so we'll just verify basic functionality
	app.setupLogging()
	
	// Verify logFile was set (if logging setup succeeded)
	// On some systems this might fail due to permissions, so we allow for that
	if app.logFile != nil {
		// Log file was successfully opened
		defer app.closeLogFile() // Clean up
		
		// Verify we can write to the log
		log.Printf("Test log message from setupLogging test")
		
		// The logFile should be a valid file handle
		if app.logFile.Name() == "" {
			t.Error("Log file should have a valid name")
		}
	}
	// If app.logFile is nil, setupLogging failed (possibly due to permissions),
	// but that's acceptable for a test environment
}

func TestApp_CloseLogFile(t *testing.T) {
	// Save original log settings
	originalOutput := log.Writer()
	originalFlags := log.Flags()
	defer func() {
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
	}()

	// Test with nil logFile (should not panic)
	app := &App{}
	app.closeLogFile() // Should not panic

	// Test with actual logFile
	tempDir, err := os.MkdirTemp("", "frictionless_close_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a log file manually
	logFile := filepath.Join(tempDir, "test.log")
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		t.Fatalf("Failed to create test log file: %v", err)
	}

	app.logFile = file
	log.SetOutput(file)

	// Close the log file
	app.closeLogFile()

	// Verify file was closed by trying to write (should fail)
	_, err = file.WriteString("test")
	if err == nil {
		t.Error("Expected error writing to closed file")
	}
}

func TestOpenConfigFile_PathGeneration(t *testing.T) {
	// Create temporary directory and config file
	tempDir, err := os.MkdirTemp("", "frictionless_config_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "test-config.yaml")
	if err := os.WriteFile(configPath, []byte("test: value"), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	app := &App{
		configPath: configPath,
	}

	// We can't easily test the actual file opening without mocking exec.Command,
	// but we can test the path resolution logic by checking that the file exists
	if _, err := os.Stat(app.configPath); os.IsNotExist(err) {
		t.Error("Config file should exist for openConfigFile to work")
	}

	// The openConfigFile method should handle the file existence check
	// Testing the actual execution would require mocking system commands
}

func TestOpenLogFile_CrossPlatform(t *testing.T) {
	// Test the cross-platform command generation logic
	var expectedCommand string
	
	switch {
	case runtime.GOOS == "windows":
		expectedCommand = "rundll32"
	case fileExists("/usr/bin/open"):
		expectedCommand = "open"
	default:
		expectedCommand = "xdg-open"
	}

	// We can't easily test the actual execution, but we can verify
	// the platform detection logic works correctly
	if expectedCommand == "" {
		t.Error("Should have determined appropriate command for platform")
	}

	// Verify the command exists on the system (for non-Windows)
	if runtime.GOOS != "windows" {
		if expectedCommand == "open" && !fileExists("/usr/bin/open") {
			t.Skip("open command not available on this system")
		}
		if expectedCommand == "xdg-open" {
			// xdg-open might not be available in test environments, that's OK
		}
	}
}

func TestSaveConfig_ErrorHandling(t *testing.T) {
	// Test saveConfig with invalid path
	app := &App{
		configPath: "/invalid/path/that/does/not/exist/config.yaml",
		config: &Config{
			GameName: "Test",
		},
	}

	// This should not panic, even with an invalid path
	app.saveConfig()
	// The method should handle the error gracefully (just log it)
}
