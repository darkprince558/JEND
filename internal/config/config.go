package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds persistent user settings
type Config struct {
	RelayURL  string `json:"relay_url,omitempty"`
	RelayUser string `json:"relay_user,omitempty"`
	RelayPass string `json:"relay_pass,omitempty"`

	// Theme settings
	Theme       string            `json:"theme,omitempty"`        // Named theme: dark, light, dracula, nord, catppuccin, solarized
	ThemeColors map[string]string `json:"theme_colors,omitempty"` // Per-color hex overrides (e.g. "primary": "#FF6B6B")

	// Update checker state
	LastUpdateCheck int64  `json:"last_update_check,omitempty"` // Unix timestamp
	LatestVersion   string `json:"latest_version,omitempty"`    // e.g. "v2.3.0"
}

func GetConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(home, ".jend")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.json"), nil
}

// Load reads the config file
func Load() (*Config, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil // Default empty config
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes the config file
func Save(cfg *Config) error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
