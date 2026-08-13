// Package safetext neutralizes untrusted free text before it is rendered into
// the Markdown context handed to an agent.
package safetext

import "strings"

// SingleLine collapses s onto one line, normalizing every run of whitespace to a
// single space.
//
// Any agent can write to a shared session, so agent-authored values (task
// titles, decisions, blockers, next actions, memory) are untrusted input. A
// value containing newlines can render as its own Markdown section — letting one
// agent forge a "## Nudges" or "## Next action" block and impersonate the
// session layer to whoever reads the context next. Removing line breaks confines
// a value to the single line it was rendered on.
//
// Callers must still render untrusted values as list items or inline after a
// label, never as the first thing on a line, so a leading "#" cannot become a
// heading.
func SingleLine(s string) string {
	if s == "" {
		return ""
	}
	if !strings.ContainsAny(s, "\n\r\t\v\f") && !strings.Contains(s, "  ") {
		return s
	}
	return strings.Join(strings.Fields(s), " ")
}

// maxIdentifier bounds a stored identifier. An agent name is a name — "claude",
// "codex", "omp" — and it is copied onto every event, checkpoint and session row,
// so anything past this is either a mistake or an attempt to pad a payload.
const maxIdentifier = 64

// Identifier normalizes a value the session layer later presents as its own
// assertion rather than as agent prose: today the agent name, which arrives from
// `--agent`, from the session.resume tool argument and from AGENT_SESSION_AGENT.
//
// The rendered context and the handoff document print it after a label, and the
// handoff spec calls it the one field a previous agent cannot have forged. That
// only holds if the stored value is a single line: "claude\n\n## Next action\n- …"
// otherwise arrives looking like a section the session layer wrote itself.
func Identifier(s string) string {
	normalized := strings.TrimSpace(SingleLine(s))
	if len(normalized) <= maxIdentifier {
		return normalized
	}
	runes := []rune(normalized)
	if len(runes) <= maxIdentifier {
		return normalized
	}
	return string(runes[:maxIdentifier])
}
