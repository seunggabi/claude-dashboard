package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds application configuration.
type Config struct {
	RefreshInterval time.Duration `yaml:"refresh_interval"`
	SessionPrefix   string        `yaml:"session_prefix"`
	DefaultDir      string        `yaml:"default_dir"`
	LogHistory      int           `yaml:"log_history"`
}

// configFile is the YAML representation.
type configFile struct {
	RefreshInterval string `yaml:"refresh_interval"`
	SessionPrefix   string `yaml:"session_prefix"`
	DefaultDir      string `yaml:"default_dir"`
	LogHistory      int    `yaml:"log_history"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		RefreshInterval: 2 * time.Second,
		SessionPrefix:   "cd-",
		DefaultDir:      "",
		LogHistory:      1000,
	}
}

// ConfigDir returns the config directory path.
func ConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude-dashboard")
}

// ConfigPath returns the config file path.
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// Load reads configuration from file, falling back to defaults.
func Load() *Config {
	cfg := DefaultConfig()

	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		return cfg
	}

	var cf configFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return cfg
	}

	if cf.RefreshInterval != "" {
		if d, err := time.ParseDuration(cf.RefreshInterval); err == nil {
			cfg.RefreshInterval = d
		}
	}
	if cf.SessionPrefix != "" {
		cfg.SessionPrefix = cf.SessionPrefix
	}
	if cf.DefaultDir != "" {
		cfg.DefaultDir = cf.DefaultDir
	}
	if cf.LogHistory > 0 {
		cfg.LogHistory = cf.LogHistory
	}

	return cfg
}

// Save writes the configuration to file.
func Save(cfg *Config) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	cf := configFile{
		RefreshInterval: cfg.RefreshInterval.String(),
		SessionPrefix:   cfg.SessionPrefix,
		DefaultDir:      cfg.DefaultDir,
		LogHistory:      cfg.LogHistory,
	}

	data, err := yaml.Marshal(&cf)
	if err != nil {
		return err
	}

	return os.WriteFile(ConfigPath(), data, 0644)
}
