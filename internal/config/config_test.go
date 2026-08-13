package config

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
)

// contextBudgetV1 is the context budget docs/spec/format-v1.md:57 freezes:
//
//	| `[context]` | context budget: `max_decisions` 5, `max_blockers` 3,
//	  `max_files` 8, `max_events` 10, `max_progress` 10, `max_item_chars` 200,
//	  `max_total_chars` 4000, `inject_memory` true, `max_memory` 3 |
//
// The numbers are written out here rather than taken from Default(), because a
// test that asks the loader for its own defaults agrees with whatever it returns.
// Every project that never wrote a config.toml renders context under exactly
// these bounds, so a changed default changes every rendered context in place.
var contextBudgetV1 = map[string]int{
	"max_decisions":   5,
	"max_blockers":    3,
	"max_files":       8,
	"max_events":      10,
	"max_progress":    10,
	"max_item_chars":  200,
	"max_total_chars": 4000,
	"max_memory":      3,
}

// retentionV1 is docs/spec/format-v1.md:58, keyed by the checkpoint kind the
// limit applies to:
//
//	| `[retention]` | checkpoints kept per kind: manual 50, auto 20,
//	  precompact 10, handoff 20 |
//
// Read through CheckpointLimit, because that is how pruning reaches the number:
// a kind that stops matching keeps everything instead of its limit.
var retentionV1 = map[string]int{
	"manual":     50,
	"auto":       20,
	"precompact": 10,
	"handoff":    20,
}

func TestLoadWithoutConfigFileUsesFrozenDefaults(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("a directory with no config.toml must load: %v", err)
	}

	assertContextBudget(t, cfg.Context, contextBudgetV1)
	assertRetention(t, cfg.Retention, retentionV1)
	if !cfg.Context.InjectMemory {
		t.Error("inject_memory defaults to false, format-v1.md:57 says true")
	}
}

// TestLoadIgnoresUnknownKeys covers format-v1.md:61 — "Unknown keys are ignored
// rather than rejected, so a config written by a newer build of the same format
// version still loads." Rejecting them makes a project unopenable by the build
// that did not write it.
func TestLoadIgnoresUnknownKeys(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `[format]
version = 1

[context]
max_events = 4
max_nudges = 99

[nudges]
enabled = true
`)

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("a config carrying keys this build does not know must still load: %v", err)
	}
	// Ignored, not skipped over: the known key sitting next to the unknown one
	// still has to be applied.
	if cfg.Context.MaxEvents != 4 {
		t.Errorf("max_events = %d, the config next to the unknown keys set 4", cfg.Context.MaxEvents)
	}
	assertRetention(t, cfg.Retention, retentionV1)
}

// TestLoadOverrideKeepsRemainingDefaults is the property that makes the defaults
// table usable: a config naming two keys opts out of two bounds, not out of the
// budget. Unmarshalling into a zero Config instead of Default() would leave every
// unnamed bound at 0, which renders nothing at all.
func TestLoadOverrideKeepsRemainingDefaults(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `[context]
max_total_chars = 12000

[retention]
max_auto = 3
`)

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	wantBudget := maps.Clone(contextBudgetV1)
	wantBudget["max_total_chars"] = 12000
	assertContextBudget(t, cfg.Context, wantBudget)

	wantRetention := maps.Clone(retentionV1)
	wantRetention["auto"] = 3
	assertRetention(t, cfg.Retention, wantRetention)

	if !cfg.Context.InjectMemory {
		t.Error("inject_memory was turned off by a config that never mentioned it")
	}
}

func assertContextBudget(t *testing.T, got ContextConfig, want map[string]int) {
	t.Helper()
	have := map[string]int{
		"max_decisions":   got.MaxDecisions,
		"max_blockers":    got.MaxBlockers,
		"max_files":       got.MaxFiles,
		"max_events":      got.MaxEvents,
		"max_progress":    got.MaxProgress,
		"max_item_chars":  got.MaxItemChars,
		"max_total_chars": got.MaxTotalChars,
		"max_memory":      got.MaxMemory,
	}
	for key, wantValue := range want {
		if have[key] != wantValue {
			t.Errorf("context %s = %d, want %d", key, have[key], wantValue)
		}
	}
}

func assertRetention(t *testing.T, got RetentionConfig, want map[string]int) {
	t.Helper()
	for kind, wantValue := range want {
		if limit := got.CheckpointLimit(kind); limit != wantValue {
			t.Errorf("retention for %s checkpoints = %d, want %d", kind, limit, wantValue)
		}
	}
}

func writeConfig(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
