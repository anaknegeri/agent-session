package entities

// Decision is the row a recorded decision is stored as. Every key is always
// present in its JSON — the same rule Snapshot and Checkpoint hold: empty means
// empty string, never absent. `reason` and `agent` carried `omitempty` until a
// shape test learned to see past the json tag's comma and caught it.
type Decision struct {
	ID        string    `json:"id" db:"id"`
	SessionID string    `json:"session_id" db:"session_id"`
	Decision  string    `json:"decision" db:"decision"`
	Reason    string    `json:"reason" db:"reason"`
	Agent     string    `json:"agent" db:"agent"`
	CreatedAt Timestamp `json:"created_at" db:"created_at"`
}
