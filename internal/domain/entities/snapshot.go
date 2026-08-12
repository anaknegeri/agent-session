package entities

// Snapshot is the canonical session state stored in a checkpoint (PRD §15).
type Snapshot struct {
	Session    SessionState   `json:"session"`
	Workspace  WorkspaceState `json:"workspace"`
	Task       TaskState      `json:"task"`
	Progress   ProgressState  `json:"progress"`
	Decisions  []*Decision    `json:"decisions"`
	Files      FilesState     `json:"files"`
	Tests      TestsState     `json:"tests"`
	Blockers   []*Blocker     `json:"blockers"`
	NextAction string         `json:"next_action"`
	LastAgent  string         `json:"last_agent"`
	Nudges     []string       `json:"nudges,omitempty"`
}

type SessionState struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type WorkspaceState struct {
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	Commit     string `json:"commit,omitempty"`
	Dirty      bool   `json:"dirty"`
}

type TaskState struct {
	ID     string `json:"id,omitempty"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type ProgressState struct {
	Completed []string `json:"completed,omitempty"`
	Pending   []string `json:"pending,omitempty"`
}

type FilesState struct {
	Modified []string `json:"modified,omitempty"`
}

type TestsState struct {
	Status   string `json:"status,omitempty"`
	Failures int    `json:"failures"`
}
