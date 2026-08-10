package entities

type AgentSession struct {
	ID           string     `json:"id" db:"id"`
	SessionID    string     `json:"session_id" db:"session_id"`
	Agent        string     `json:"agent" db:"agent"`
	StartedAt    Timestamp  `json:"started_at" db:"started_at"`
	EndedAt      *Timestamp `json:"ended_at,omitempty" db:"ended_at"`
	CheckpointID string     `json:"checkpoint_id,omitempty" db:"checkpoint_id"`
}
