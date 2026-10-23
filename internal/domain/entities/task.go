package entities

const (
	TaskStatusInProgress = "in_progress"
	TaskStatusCompleted  = "completed"
	TaskStatusBlocked    = "blocked"
	TaskStatusCancelled  = "cancelled"
)

// taskStatuses is the frozen task status vocabulary of format-v1.md. It is closed
// for the same reason the event type namespace is: the value is written by one
// agent and matched on by another, and a status outside the set is filed as
// neither done nor outstanding by every reader that classifies it — so it goes
// missing from progress rather than showing up wrong.
var taskStatuses = map[string]bool{
	TaskStatusInProgress: true,
	TaskStatusCompleted:  true,
	TaskStatusBlocked:    true,
	TaskStatusCancelled:  true,
}

// ValidTaskStatus reports whether status is one of the documented task statuses.
func ValidTaskStatus(status string) bool {
	return taskStatuses[status]
}

// TaskStatuses returns the documented task statuses in sorted order, for callers
// that have to tell a human which values are accepted.
func TaskStatuses() []string {
	return sortedKeys(taskStatuses)
}

type Task struct {
	ID        string    `json:"id" db:"id"`
	SessionID string    `json:"session_id" db:"session_id"`
	Title     string    `json:"title" db:"title"`
	Status    string    `json:"status" db:"status"`
	CreatedAt Timestamp `json:"created_at" db:"created_at"`
	UpdatedAt Timestamp `json:"updated_at" db:"updated_at"`
}
