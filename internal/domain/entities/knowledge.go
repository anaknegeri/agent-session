package entities

const (
	// Knowledge kinds (PRD §36: Project Knowledge, Architecture, Solutions, Preferences, Skills).
	KnowledgeKindProject      = "project_knowledge"
	KnowledgeKindArchitecture = "architecture"
	KnowledgeKindSolution     = "solution"
	KnowledgeKindPreference   = "preference"
	KnowledgeKindSkill        = "skill"

	// Source types for auto-promoted knowledge.
	KnowledgeSourceManual     = "manual"
	KnowledgeSourceDecision   = "decision"
	KnowledgeSourceBlocker    = "blocker"
	KnowledgeSourceTask       = "task"
	KnowledgeSourceCheckpoint = "checkpoint"
)

type Knowledge struct {
	ID         string    `json:"id" db:"id"`
	SessionID  string    `json:"session_id" db:"session_id"`
	Kind       string    `json:"kind" db:"kind"`
	Content    string    `json:"content" db:"content"`
	SourceType string    `json:"source_type,omitempty" db:"source_type"`
	SourceID   string    `json:"source_id,omitempty" db:"source_id"`
	Agent      string    `json:"agent,omitempty" db:"agent"`
	CreatedAt  Timestamp `json:"created_at" db:"created_at"`
}

func (Knowledge) TableName() string { return "knowledge" }

// SearchHit is a knowledge row plus an FTS snippet.
type KnowledgeHit struct {
	Knowledge
	Snippet string `json:"snippet,omitempty"`
}
