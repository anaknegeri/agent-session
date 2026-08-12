package opencode

import "testing"

func TestMergeGlobalInstructions(t *testing.T) {
	config := map[string]any{}
	MergeGlobalInstructions(config)

	agentCfg, _ := config["agent"].(map[string]any)
	instructions, _ := agentCfg["instructions"].(map[string]any)
	system, _ := instructions["system"].(string)
	if system != GlobalSystemInstructions {
		t.Fatalf("expected global instructions, got %q", system)
	}

	// idempotent: re-merging does not duplicate the note
	MergeGlobalInstructions(config)
	agentCfg2, _ := config["agent"].(map[string]any)
	instructions2, _ := agentCfg2["instructions"].(map[string]any)
	system2, _ := instructions2["system"].(string)
	if system2 != GlobalSystemInstructions {
		t.Fatalf("expected no duplication on re-merge, got %q", system2)
	}

	// preserves an existing custom system instruction
	custom := map[string]any{
		"agent": map[string]any{
			"instructions": map[string]any{
				"system": "Always write tests first.",
			},
		},
	}
	MergeGlobalInstructions(custom)
	agentCfg3, _ := custom["agent"].(map[string]any)
	instructions3, _ := agentCfg3["instructions"].(map[string]any)
	system3, _ := instructions3["system"].(string)
	if system3 == GlobalSystemInstructions {
		t.Fatalf("expected custom instruction to be preserved, got only global text")
	}
	if len(system3) <= len(GlobalSystemInstructions) {
		t.Fatalf("expected custom instruction to be kept alongside global text, got %q", system3)
	}
}

func TestRemoveGlobalInstructions(t *testing.T) {
	// no-op on a config that never had the note
	empty := map[string]any{}
	RemoveGlobalInstructions(empty)
	if len(empty) != 0 {
		t.Fatalf("expected no-op on empty config, got %v", empty)
	}

	// removes the note, preserving a custom instruction added alongside it
	config := map[string]any{
		"agent": map[string]any{
			"instructions": map[string]any{
				"system": "Always write tests first.",
			},
		},
	}
	MergeGlobalInstructions(config)
	RemoveGlobalInstructions(config)

	agentCfg, ok := config["agent"].(map[string]any)
	if !ok {
		t.Fatalf("expected agent config with custom instruction to survive, got %v", config)
	}
	instructions, _ := agentCfg["instructions"].(map[string]any)
	system, _ := instructions["system"].(string)
	if system != "Always write tests first." {
		t.Fatalf("expected only the custom instruction to remain, got %q", system)
	}

	// removes the note entirely when there was no custom instruction
	soleNote := map[string]any{}
	MergeGlobalInstructions(soleNote)
	RemoveGlobalInstructions(soleNote)
	if len(soleNote) != 0 {
		t.Fatalf("expected agent key to be removed entirely, got %v", soleNote)
	}
}
