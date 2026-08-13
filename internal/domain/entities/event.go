package entities

import "sort"

const (
	EventSessionStarted    = "session.started"
	EventTaskCreated       = "task.created"
	EventTaskUpdated       = "task.updated"
	EventFileChanged       = "file.changed"
	EventCommandExecuted   = "command.executed"
	EventTestStarted       = "test.started"
	EventTestFailed        = "test.failed"
	EventTestPassed        = "test.passed"
	EventDecisionCreated   = "decision.created"
	EventBlockerCreated    = "blocker.created"
	EventCheckpointCreated = "checkpoint.created"
	EventHandoffCreated    = "handoff.created"
	EventSessionCompleted  = "session.completed"
)

var canonicalEventTypes = map[string]bool{
	EventSessionStarted:    true,
	EventTaskCreated:       true,
	EventTaskUpdated:       true,
	EventFileChanged:       true,
	EventCommandExecuted:   true,
	EventTestStarted:       true,
	EventTestFailed:        true,
	EventTestPassed:        true,
	EventDecisionCreated:   true,
	EventBlockerCreated:    true,
	EventCheckpointCreated: true,
	EventHandoffCreated:    true,
	EventSessionCompleted:  true,
}

// IsCanonicalEventType reports whether t is one of the documented event types (PRD §14.3).
func IsCanonicalEventType(t string) bool {
	return canonicalEventTypes[t]
}

// CanonicalEventTypes returns the documented event types in sorted order, for
// callers that have to tell a human which values are accepted.
func CanonicalEventTypes() []string {
	types := make([]string, 0, len(canonicalEventTypes))
	for t := range canonicalEventTypes {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

type SessionEvent struct {
	ID        string    `json:"id" db:"id"`
	SessionID string    `json:"session_id" db:"session_id"`
	Agent     string    `json:"agent" db:"agent"`
	Type      string    `json:"type" db:"type"`
	Payload   string    `json:"payload,omitempty" db:"payload"`
	CreatedAt Timestamp `json:"timestamp" db:"created_at"`
}
