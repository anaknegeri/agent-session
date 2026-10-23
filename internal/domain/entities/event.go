package entities

import "encoding/json"

// EmptyPayload is the payload of an event that has nothing to say. event-v1.md
// promises `payload` is JSON text, so "nothing" has to be valid JSON a reader in
// another language can parse — an empty string is not.
const EmptyPayload = "{}"

const (
	EventSessionStarted    = "session.started"
	EventSessionResumed    = "session.resumed"
	EventTaskCreated       = "task.created"
	EventTaskUpdated       = "task.updated"
	EventFileChanged       = "file.changed"
	EventCommandExecuted   = "command.executed"
	EventTestStarted       = "test.started"
	EventTestFailed        = "test.failed"
	EventTestPassed        = "test.passed"
	EventDecisionCreated   = "decision.created"
	EventBlockerCreated    = "blocker.created"
	EventBlockerResolved   = "blocker.resolved"
	EventCheckpointCreated = "checkpoint.created"
	EventHandoffCreated    = "handoff.created"
	EventSessionCompleted  = "session.completed"
)

var canonicalEventTypes = map[string]bool{
	EventSessionStarted:    true,
	EventSessionResumed:    true,
	EventTaskCreated:       true,
	EventTaskUpdated:       true,
	EventFileChanged:       true,
	EventCommandExecuted:   true,
	EventTestStarted:       true,
	EventTestFailed:        true,
	EventTestPassed:        true,
	EventDecisionCreated:   true,
	EventBlockerCreated:    true,
	EventBlockerResolved:   true,
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
	return sortedKeys(canonicalEventTypes)
}

type SessionEvent struct {
	ID        string    `json:"id" db:"id"`
	SessionID string    `json:"session_id" db:"session_id"`
	Agent     string    `json:"agent" db:"agent"`
	Type      string    `json:"type" db:"type"`
	Payload   string    `json:"payload" db:"payload"`
	CreatedAt Timestamp `json:"timestamp" db:"created_at"`
}

// MarshalJSON emits the shape event-v1.md promises: `payload` is always present
// and always JSON text. Rows written before appends normalized it hold an empty
// string in the column, and 22 of them exist in this repo's own database; a reader
// handed `"payload": ""` has to special-case it or fail to parse, so those rows
// render as `{}` here rather than being migrated in place.
func (e SessionEvent) MarshalJSON() ([]byte, error) {
	type event SessionEvent // sheds this method, so json does not recurse
	out := event(e)
	if out.Payload == "" {
		out.Payload = EmptyPayload
	}
	return json.Marshal(out)
}
