package config

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestHome creates a temporary directory and sets HOME so config I/O
// is isolated from the real ~/.miniaws.
func setupTestHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestSaveAndLoadConfig(t *testing.T) {
	setupTestHome(t)

	orig := &Config{
		ContainerName: "test-mini",
		ImageName:     "test-img:latest",
		Port:          "9999",
		EndpointURL:   "http://test:9999",
	}

	if err := SaveConfig(orig); err != nil {
		t.Fatalf("SaveConfig() error: %v", err)
	}

	// Verify file was created at the expected path
	home := os.Getenv("HOME")
	cfgPath := filepath.Join(home, ".miniaws", "config.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatalf("config file not created at %s", cfgPath)
	}

	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if got == nil {
		t.Fatal("LoadConfig() returned nil, want Config")
	}
	if got.ContainerName != orig.ContainerName {
		t.Errorf("ContainerName = %q, want %q", got.ContainerName, orig.ContainerName)
	}
	if got.ImageName != orig.ImageName {
		t.Errorf("ImageName = %q, want %q", got.ImageName, orig.ImageName)
	}
	if got.Port != orig.Port {
		t.Errorf("Port = %q, want %q", got.Port, orig.Port)
	}
	if got.EndpointURL != orig.EndpointURL {
		t.Errorf("EndpointURL = %q, want %q", got.EndpointURL, orig.EndpointURL)
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	setupTestHome(t)

	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if got != nil {
		t.Fatal("LoadConfig() should return nil when file does not exist")
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	setupTestHome(t)

	home := os.Getenv("HOME")
	dir := filepath.Join(home, ".miniaws")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() expected error for invalid JSON, got nil")
	}
}

func TestRemoveConfig(t *testing.T) {
	setupTestHome(t)

	// Save something first
	if err := SaveConfig(&Config{ContainerName: "to-remove"}); err != nil {
		t.Fatal(err)
	}

	if err := RemoveConfig(); err != nil {
		t.Fatalf("RemoveConfig() error: %v", err)
	}

	// Verify file is gone
	home := os.Getenv("HOME")
	cfgPath := filepath.Join(home, ".miniaws", "config.json")
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatalf("config file still exists after RemoveConfig")
	}

	// LoadConfig should now return nil, nil
	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() after RemoveConfig error: %v", err)
	}
	if got != nil {
		t.Fatal("LoadConfig() after RemoveConfig should return nil")
	}
}

func TestRemoveConfigNoFile(t *testing.T) {
	setupTestHome(t)

	// RemoveConfig when file doesn't exist should return nil (os.Remove is a no-op)
	if err := RemoveConfig(); err != nil {
		t.Fatalf("RemoveConfig() on missing file error: %v", err)
	}
}

func TestSaveConfigRoundTripEmptyFields(t *testing.T) {
	setupTestHome(t)

	orig := &Config{}
	if err := SaveConfig(orig); err != nil {
		t.Fatalf("SaveConfig() error: %v", err)
	}

	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if got == nil {
		t.Fatal("LoadConfig() returned nil")
	}
	if got.ContainerName != "" {
		t.Errorf("ContainerName = %q, want empty", got.ContainerName)
	}
	if got.EndpointURL != "" {
		t.Errorf("EndpointURL = %q, want empty", got.EndpointURL)
	}
}


