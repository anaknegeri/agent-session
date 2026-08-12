package ports

import "github.com/anaknegeri/agent-session/internal/domain/entities"

// ContextRenderer renders a canonical snapshot to human-readable markdown
// (context.md) and handoff text (PRD §4.5, §24).
type ContextRenderer interface {
	RenderContext(snapshot *entities.Snapshot, budget ContextBudget) (string, error)
	RenderHandoff(snapshot *entities.Snapshot, to string, budget ContextBudget) (string, error)
}
