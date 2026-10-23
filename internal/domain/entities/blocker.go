package entities

import "encoding/json"

const (
	BlockerStatusOpen     = "open"
	BlockerStatusResolved = "resolved"
)

// Blocker is the row a recorded blocker is stored as. Every key is always
// present in its JSON, the same rule Decision/Checkpoint/Snapshot hold —
// `agent` carried `omitempty` until a shape test caught it. `resolved_at` is a
// pointer because "not yet resolved" is a real, distinct state, not an empty
// string; MarshalJSON substitutes the zero Timestamp when nil so the key is
// present either way rather than absent for every open blocker.
type Blocker struct {
	ID          string     `json:"id" db:"id"`
	SessionID   string     `json:"session_id" db:"session_id"`
	Description string     `json:"description" db:"description"`
	Status      string     `json:"status" db:"status"`
	Agent       string     `json:"agent" db:"agent"`
	CreatedAt   Timestamp  `json:"created_at" db:"created_at"`
	ResolvedAt  *Timestamp `json:"resolved_at" db:"resolved_at"`
}

// MarshalJSON keeps `resolved_at` present on an open blocker: a zero Timestamp
// (the RFC 3339 encoding of time.Time's zero value) rather than an absent key,
// so a reader told every key is always present does not have to special-case
// this one.
func (b Blocker) MarshalJSON() ([]byte, error) {
	type blocker Blocker // sheds this method, so json does not recurse
	out := blocker(b)
	if out.ResolvedAt == nil {
		out.ResolvedAt = &Timestamp{}
	}
	return json.Marshal(out)
}
