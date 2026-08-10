package entities

const (
	SessionStatusActive    = "active"
	SessionStatusPaused    = "paused"
	SessionStatusCompleted = "completed"
)

type Session struct {
	ID            string    `json:"id" db:"id"`
	ProjectID     string    `json:"project_id" db:"project_id"`
	Title         string    `json:"title" db:"title"`
	Status        string    `json:"status" db:"status"`
	Branch        string    `json:"branch" db:"branch"`
	Commit        string    `json:"commit,omitempty" db:"commit"`
	Dirty         bool      `json:"dirty" db:"dirty"`
	LastAgent     string    `json:"last_agent,omitempty" db:"last_agent"`
	CurrentTaskID string    `json:"current_task_id,omitempty" db:"current_task_id"`
	CreatedAt     Timestamp `json:"created_at" db:"created_at"`
	UpdatedAt     Timestamp `json:"updated_at" db:"updated_at"`
}
