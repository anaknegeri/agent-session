package entities

type Decision struct {
	ID        string    `json:"id" db:"id"`
	SessionID string    `json:"session_id" db:"session_id"`
	Decision  string    `json:"decision" db:"decision"`
	Reason    string    `json:"reason,omitempty" db:"reason"`
	Agent     string    `json:"agent,omitempty" db:"agent"`
	CreatedAt Timestamp `json:"created_at" db:"created_at"`
}
