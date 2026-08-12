package opencode

import "strings"

// GlobalSystemInstructions mirrors SystemInstructions but is scoped with a
// .agent/ guard, since ~/.config/opencode/opencode.json applies to every
// project, not just the one where agent-session was initialized.
const GlobalSystemInstructions = "If this project contains a `.agent/` directory (created by `agent-session init`), it uses Agent Session as its session layer: at the start of a session, FIRST call session.get then context.get and continue the existing task; record work with task.create/task.update, decision.create, blocker.create, and event.append; before finishing, checkpoint with session.checkpoint including next_action. Ignore this note entirely in projects without a `.agent/` directory."

// MergeGlobalInstructions appends GlobalSystemInstructions to an existing
// agent.instructions.system config map without discarding any custom
// instructions the user already has, and without duplicating on re-run.
func MergeGlobalInstructions(config map[string]any) {
	agentCfg, _ := config["agent"].(map[string]any)
	if agentCfg == nil {
		agentCfg = map[string]any{}
	}
	instructions, _ := agentCfg["instructions"].(map[string]any)
	if instructions == nil {
		instructions = map[string]any{}
	}
	existing, _ := instructions["system"].(string)
	if strings.Contains(existing, "Agent Session") {
		config["agent"] = agentCfg
		return
	}
	if existing != "" {
		existing += "\n\n"
	}
	instructions["system"] = existing + GlobalSystemInstructions
	agentCfg["instructions"] = instructions
	config["agent"] = agentCfg
}

// RemoveGlobalInstructions strips GlobalSystemInstructions from the system
// instruction string, preserving any custom instructions the user added
// alongside it. No-op if the note isn't present.
func RemoveGlobalInstructions(config map[string]any) {
	agentCfg, ok := config["agent"].(map[string]any)
	if !ok {
		return
	}
	instructions, ok := agentCfg["instructions"].(map[string]any)
	if !ok {
		return
	}
	system, _ := instructions["system"].(string)
	if !strings.Contains(system, GlobalSystemInstructions) {
		return
	}

	cleaned := strings.Replace(system, "\n\n"+GlobalSystemInstructions, "", 1)
	if cleaned == system {
		cleaned = strings.Replace(system, GlobalSystemInstructions, "", 1)
	}
	if cleaned == "" {
		delete(instructions, "system")
	} else {
		instructions["system"] = cleaned
	}
	if len(instructions) == 0 {
		delete(agentCfg, "instructions")
	} else {
		agentCfg["instructions"] = instructions
	}
	if len(agentCfg) == 0 {
		delete(config, "agent")
	} else {
		config["agent"] = agentCfg
	}
}
