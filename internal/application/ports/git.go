package ports

import "context"

type FileChange struct {
	Path   string `json:"path"`
	Status string `json:"status"` // M|A|D|R|?|...
}

type WorkspaceStatus struct {
	Repository string       `json:"repository,omitempty"`
	Branch     string       `json:"branch"`
	Commit     string       `json:"commit,omitempty"`
	Dirty      bool         `json:"dirty"`
	Changes    []FileChange `json:"changes,omitempty"`
}

// GitService reads workspace state from Git. Git is the source of truth
// for source code; the session layer stores only metadata (PRD §22).
type GitService interface {
	Detect(ctx context.Context, dir string) (bool, error)
	Status(ctx context.Context, dir string) (WorkspaceStatus, error)
	DiffStat(ctx context.Context, dir string) ([]FileChange, error)
	Diff(ctx context.Context, dir string) (string, error)
}
