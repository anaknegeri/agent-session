package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/anaknegeri/agent-session/pkg/contract"
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
	// Format records which Agent Session Format the .agent/ directory follows,
	// so a build that reads it can tell whether it understands the layout.
	Format  FormatConfig  `toml:"format"`
	Project ProjectConfig `toml:"project"`
	Storage StorageConfig `toml:"storage"`
	Session SessionConfig `toml:"session"`
	Context ContextConfig `toml:"context"`
	Git     GitConfig     `toml:"git"`
	Agents  AgentsConfig  `toml:"agents"`
	Sync    SyncConfig    `toml:"sync"`
	// Retention bounds checkpoint growth. A long session produced 85 checkpoints
	// averaging 7.5 KB of snapshot each with no limit at all.
	Retention RetentionConfig `toml:"retention"`
}

// FormatConfig carries the Agent Session Format version of this .agent/
// directory (docs/spec/format-v1.md).
type FormatConfig struct {
	Version int `toml:"version"`
}

type ProjectConfig struct {
	Name string `toml:"name"`
}

type StorageConfig struct {
	Driver string `toml:"driver"` // sqlite
}

type SessionConfig struct {
	AutoCheckpoint  bool `toml:"auto_checkpoint"`
	SmartCheckpoint bool `toml:"smart_checkpoint"`
}

// ContextConfig bounds how much context is rendered (token savings).
type ContextConfig struct {
	MaxDecisions  int  `toml:"max_decisions"`
	MaxBlockers   int  `toml:"max_blockers"`
	MaxFiles      int  `toml:"max_files"`
	MaxEvents     int  `toml:"max_events"`
	MaxProgress   int  `toml:"max_progress"`
	MaxItemChars  int  `toml:"max_item_chars"`
	MaxTotalChars int  `toml:"max_total_chars"`
	InjectMemory  bool `toml:"inject_memory"`
	MaxMemory     int  `toml:"max_memory"`
}

// RetentionConfig caps how many checkpoints of each kind a session keeps. Limits
// are per kind so a burst of automatic checkpoints cannot evict the deliberate
// ones. A value of 0 or less means keep everything.
type RetentionConfig struct {
	MaxManual     int `toml:"max_manual"`
	MaxAuto       int `toml:"max_auto"`
	MaxPreCompact int `toml:"max_precompact"`
	MaxHandoff    int `toml:"max_handoff"`
}

// CheckpointLimit returns the retained count for a checkpoint kind, or 0 when the
// kind is unbounded or unknown.
func (r RetentionConfig) CheckpointLimit(kind string) int {
	switch kind {
	case "manual":
		return r.MaxManual
	case "auto":
		return r.MaxAuto
	case "precompact":
		return r.MaxPreCompact
	case "handoff":
		return r.MaxHandoff
	default:
		return 0
	}
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
		Format:  FormatConfig{Version: contract.Format},
		Project: ProjectConfig{},
		Storage: StorageConfig{Driver: "sqlite"},
		Session: SessionConfig{AutoCheckpoint: true, SmartCheckpoint: true},
		Context: ContextConfig{
			MaxDecisions:  5,
			MaxBlockers:   3,
			MaxFiles:      8,
			MaxEvents:     10,
			MaxProgress:   10,
			MaxItemChars:  200,
			MaxTotalChars: 4000,
			InjectMemory:  true,
			MaxMemory:     3,
		},
		Git: GitConfig{Enabled: true},
		Agents: AgentsConfig{
			Claude:   AgentConfig{Enabled: true},
			Codex:    AgentConfig{Enabled: true},
			OpenCode: AgentConfig{Enabled: true},
		},
		Sync: SyncConfig{Mode: SyncModeLocal},
		Retention: RetentionConfig{
			MaxManual:     50,
			MaxAuto:       20,
			MaxPreCompact: 10,
			MaxHandoff:    20,
		},
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
	// A config written before [format] existed leaves the default in place, which
	// is right: the layout it describes is v1, the version was only written down
	// once the format was specified. A higher number is a real problem — the
	// directory was laid out by a build that knows things this one does not.
	if cfg.Format.Version > contract.Format {
		return nil, fmt.Errorf("%s declares Agent Session Format v%d, this build understands v%d — upgrade agent-session",
			path, cfg.Format.Version, contract.Format)
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
