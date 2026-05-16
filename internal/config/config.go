// Package config provides shared configuration for the miniaws CLI.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds the miniaws container configuration persisted to disk.
type Config struct {
	ContainerName string `json:"containerName"`
	ImageName     string `json:"imageName"`
	Port          string `json:"port"`
	EndpointURL   string `json:"endpointUrl"`
}

func configFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(home, ".miniaws")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.json"), nil
}

// LoadConfig reads the config file from ~/.miniaws/config.json.
// Returns nil, nil if the file doesn't exist.
func LoadConfig() (*Config, error) {
	path, err := configFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// SaveConfig writes config to ~/.miniaws/config.json.
func SaveConfig(c *Config) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// RemoveConfig deletes the config file.
func RemoveConfig() error {
	path, err := configFilePath()
	if err != nil {
		return err
	}
	return os.Remove(path)
}
