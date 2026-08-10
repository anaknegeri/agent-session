package entities

const (
	TaskStatusInProgress = "in_progress"
	TaskStatusCompleted  = "completed"
	TaskStatusBlocked    = "blocked"
	TaskStatusCancelled  = "cancelled"
)

type Task struct {
	ID        string    `json:"id" db:"id"`
	SessionID string    `json:"session_id" db:"session_id"`
	Title     string    `json:"title" db:"title"`
	Status    string    `json:"status" db:"status"`
	CreatedAt Timestamp `json:"created_at" db:"created_at"`
	UpdatedAt Timestamp `json:"updated_at" db:"updated_at"`
}
