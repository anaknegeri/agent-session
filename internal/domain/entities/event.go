package entities

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

type SessionEvent struct {
	ID        string    `json:"id" db:"id"`
	SessionID string    `json:"session_id" db:"session_id"`
	Agent     string    `json:"agent" db:"agent"`
	Type      string    `json:"type" db:"type"`
	Payload   string    `json:"payload,omitempty" db:"payload"`
	CreatedAt Timestamp `json:"timestamp" db:"created_at"`
}
