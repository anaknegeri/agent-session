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

// knowledgeKinds is the frozen knowledge kind vocabulary of format-v1.md. It is
// closed because kind is how knowledge is retrieved: `memory.get`-by-kind and the
// context injection both filter on it, so a row stored under an undocumented kind
// is a row nothing ever reads back.
var knowledgeKinds = map[string]bool{
	KnowledgeKindProject:      true,
	KnowledgeKindArchitecture: true,
	KnowledgeKindSolution:     true,
	KnowledgeKindPreference:   true,
	KnowledgeKindSkill:        true,
}

// ValidKnowledgeKind reports whether kind is one of the documented knowledge kinds.
func ValidKnowledgeKind(kind string) bool {
	return knowledgeKinds[kind]
}

// KnowledgeKinds returns the documented knowledge kinds in sorted order, for
// callers that have to tell a human which values are accepted.
func KnowledgeKinds() []string {
	return sortedKeys(knowledgeKinds)
}

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
