package opencode

import "strings"

// GlobalSystemInstructions mirrors SystemInstructions but is scoped with a
// .agent/ guard, since ~/.config/opencode/opencode.json applies to every
// project, not just the one where agent-session was initialized.
const GlobalSystemInstructions = "If this project contains a `.agent/` directory (created by `agent-session init`), it uses Agent Session as its session layer: at the start of a session, FIRST call session.get then context.get and continue the existing task; record work with task.create/task.update, decision.create, blocker.create, and event.append; before finishing, checkpoint with session.checkpoint including next_action. Ignore this note entirely in projects without a `.agent/` directory."

// MergeGlobalInstructions appends GlobalSystemInstructions to an existing
// agent.instructions.system config map.
func MergeGlobalInstructions(config map[string]any) {
	MergeInstructions(config, GlobalSystemInstructions)
}

// MergeInstructions appends note to agent.instructions.system without discarding
// any custom instruction the user already wrote there, and without duplicating on
// re-run. Replacing that string instead of appending to it is silent data loss:
// it is the only place OpenCode keeps a user's own always-on prompt.
func MergeInstructions(config map[string]any, note string) {
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
	instructions["system"] = existing + note
	agentCfg["instructions"] = instructions
	config["agent"] = agentCfg
}

// RemoveGlobalInstructions strips GlobalSystemInstructions from the system
// instruction string, preserving any custom instruction the user added alongside
// it. No-op if the note isn't present.
func RemoveGlobalInstructions(config map[string]any) {
	RemoveInstructions(config, GlobalSystemInstructions)
}

// RemoveInstructions is RemoveGlobalInstructions for either note, so a
// project-scope uninstall removes what a project-scope install wrote.
func RemoveInstructions(config map[string]any, note string) {
	agentCfg, ok := config["agent"].(map[string]any)
	if !ok {
		return
	}
	instructions, ok := agentCfg["instructions"].(map[string]any)
	if !ok {
		return
	}
	system, _ := instructions["system"].(string)
	if !strings.Contains(system, note) {
		return
	}

	cleaned := strings.Replace(system, "\n\n"+note, "", 1)
	if cleaned == system {
		cleaned = strings.Replace(system, note, "", 1)
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
