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
