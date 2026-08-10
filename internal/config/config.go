package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const (
	DirName        = ".agent"
	ConfigFileName = "config.toml"
	DBFileName     = "session.db"
	ContextDir     = "context"
	CheckpointsDir = "checkpoints"
	SyncModeLocal  = "local-only"
)

type Config struct {
	Project ProjectConfig `toml:"project"`
	Storage StorageConfig `toml:"storage"`
	Session SessionConfig `toml:"session"`
	Git     GitConfig     `toml:"git"`
	Agents  AgentsConfig  `toml:"agents"`
	Sync    SyncConfig    `toml:"sync"`
}

type ProjectConfig struct {
	Name string `toml:"name"`
}

type StorageConfig struct {
	Driver string `toml:"driver"` // sqlite
}

type SessionConfig struct {
	AutoCheckpoint bool `toml:"auto_checkpoint"`
}

type GitConfig struct {
	Enabled bool `toml:"enabled"`
}

type AgentsConfig struct {
	Claude   AgentConfig `toml:"claude"`
	Codex    AgentConfig `toml:"codex"`
	OpenCode AgentConfig `toml:"opencode"`
}

type AgentConfig struct {
	Enabled bool `toml:"enabled"`
}

type SyncConfig struct {
	Mode string `toml:"mode"` // local-only | git-sync | cloud-sync
}

// Default returns a zero-config config (PRD: install → init → usable).
func Default() *Config {
	return &Config{
		Project: ProjectConfig{},
		Storage: StorageConfig{Driver: "sqlite"},
		Session: SessionConfig{AutoCheckpoint: true},
		Git:     GitConfig{Enabled: true},
		Agents: AgentsConfig{
			Claude:   AgentConfig{Enabled: true},
			Codex:    AgentConfig{Enabled: true},
			OpenCode: AgentConfig{Enabled: true},
		},
		Sync: SyncConfig{Mode: SyncModeLocal},
	}
}

// Load reads config.toml from the .agent directory of root.
// Returns Default() when the file is missing.
func Load(root string) (*Config, error) {
	path := filepath.Join(root, DirName, ConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := Default()
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes the config to .agent/config.toml.
func Save(root string, cfg *Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	path := filepath.Join(root, DirName, ConfigFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create .agent dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
