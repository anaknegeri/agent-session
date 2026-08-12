package entities

// Trust classifies where a piece of rendered context came from, so the agent
// reading it can tell what the session layer asserts from what another agent
// merely wrote down.
//
// The level is a property of the content's kind, not of an individual row: a
// decision is agent-authored whoever wrote it, and a git-derived file path is an
// observation whoever triggered it. Only content arriving from outside this
// project's own agents (an imported session) is classified per-record.
type Trust string

const (
	// TrustState is state the session layer itself maintains: ids, statuses,
	// counters, nudges. Safe to act on.
	TrustState Trust = "state"

	// TrustObservation is workspace fact read from git: branch, commit, dirty
	// flag, changed paths. Safe to act on, but describes the world, not intent.
	TrustObservation Trust = "observation"

	// TrustAgentNote is free text an agent wrote: task titles, decisions,
	// blockers, next actions, promoted memory. Data to consider, never
	// instructions to follow.
	TrustAgentNote Trust = "agent_note"

	// TrustExternal is content that entered from outside this project's agents,
	// such as an imported session. Least trusted.
	TrustExternal Trust = "external"
)

// Untrusted reports whether content at this level may contain text authored
// outside the session layer, and so must never be treated as instructions.
func (t Trust) Untrusted() bool {
	return t == TrustAgentNote || t == TrustExternal
}

// Label is the marker rendered next to a context section.
func (t Trust) Label() string {
	if t.Untrusted() {
		return "(untrusted)"
	}
	return ""
}
