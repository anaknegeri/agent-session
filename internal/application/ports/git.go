package ports

import (
	"context"
	"encoding/json"
)

type FileChange struct {
	Path   string `json:"path"`
	Status string `json:"status"` // M|A|D|R|?|...
}

type WorkspaceStatus struct {
	Repository string       `json:"repository,omitempty"`
	Branch     string       `json:"branch"`
	Commit     string       `json:"commit,omitempty"`
	Dirty      bool         `json:"dirty"`
	Changes    []FileChange `json:"changes"`
}

// MarshalJSON keeps `changes` present and a list. workspace.status and
// session://workspace hand this to an agent over MCP: a clean tree used to drop
// the key entirely, so a client had to treat absent, null and empty as one thing.
// Same reason entities.Snapshot marshals itself.
func (w WorkspaceStatus) MarshalJSON() ([]byte, error) {
	type status WorkspaceStatus // sheds this method, so json does not recurse
	out := status(w)
	if out.Changes == nil {
		out.Changes = []FileChange{}
	}
	return json.Marshal(out)
}

// DiffScope selects how much of the diff a caller wants. An agent asking for
// DiffScopeStat is bounding its token spend, so returning the full patch instead
// spends what it was trying not to.
type DiffScope string

const (
	DiffScopeFull DiffScope = "full"
	DiffScopeStat DiffScope = "stat"
)

// GitService reads workspace state from Git. Git is the source of truth
// for source code; the session layer stores only metadata (PRD §22).
type GitService interface {
	Detect(ctx context.Context, dir string) (bool, error)
	Status(ctx context.Context, dir string) (WorkspaceStatus, error)
	DiffStat(ctx context.Context, dir string) ([]FileChange, error)
	Diff(ctx context.Context, dir string, scope DiffScope) (string, error)
}
