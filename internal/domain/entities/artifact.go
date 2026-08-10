package entities

const (
	ArtifactKindTestOutput = "test_output"
	ArtifactKindDiff       = "diff"
	ArtifactKindLog        = "log"
)

type Artifact struct {
	ID        string    `json:"id" db:"id"`
	SessionID string    `json:"session_id" db:"session_id"`
	Kind      string    `json:"kind" db:"kind"`
	Path      string    `json:"path,omitempty" db:"path"`
	Content   string    `json:"content,omitempty" db:"content"`
	CreatedAt Timestamp `json:"created_at" db:"created_at"`
}
