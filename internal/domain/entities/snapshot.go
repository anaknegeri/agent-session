package entities

// Snapshot is the canonical session state stored in a checkpoint (PRD §15),
// specified as Checkpoint Schema v1 in docs/spec/checkpoint-v1.md.
type Snapshot struct {
	// Version is the Checkpoint Schema version this snapshot was written
	// against. A checkpoint outlives the build that wrote it, so the reader has
	// to be told what it is holding rather than assume the current shape.
	//
	// Zero means a snapshot written before the field existed. Those are v1: the
	// field was added when the schema was specified, not when it changed.
	Version    int            `json:"version"`
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
	// Tasks carries every task with its id and status. Completed/Pending are
	// title lists for rendering and cannot express a status change, so diffing
	// task state needs this. Absent in snapshots written before it existed.
	Tasks []TaskState `json:"tasks,omitempty"`
}

type FilesState struct {
	Modified []string `json:"modified,omitempty"`
}

type TestsState struct {
	Status   string `json:"status,omitempty"`
	Failures int    `json:"failures"`
}
