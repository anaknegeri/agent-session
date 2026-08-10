package entities

type Checkpoint struct {
	ID         string    `json:"id" db:"id"`
	SessionID  string    `json:"session_id" db:"session_id"`
	TaskID     string    `json:"task_id,omitempty" db:"task_id"`
	Label      string    `json:"label,omitempty" db:"label"`
	Snapshot   string    `json:"snapshot" db:"snapshot"`
	NextAction string    `json:"next_action,omitempty" db:"next_action"`
	Agent      string    `json:"agent,omitempty" db:"agent"`
	CreatedAt  Timestamp `json:"created_at" db:"created_at"`
}
