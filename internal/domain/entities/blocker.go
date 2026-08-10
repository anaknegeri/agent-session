package entities

const (
	BlockerStatusOpen     = "open"
	BlockerStatusResolved = "resolved"
)

type Blocker struct {
	ID          string     `json:"id" db:"id"`
	SessionID   string     `json:"session_id" db:"session_id"`
	Description string     `json:"description" db:"description"`
	Status      string     `json:"status" db:"status"`
	Agent       string     `json:"agent,omitempty" db:"agent"`
	CreatedAt   Timestamp  `json:"created_at" db:"created_at"`
	ResolvedAt  *Timestamp `json:"resolved_at,omitempty" db:"resolved_at"`
}
