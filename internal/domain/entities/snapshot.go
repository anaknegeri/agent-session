package entities

import "encoding/json"

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

// MarshalJSON emits the shape checkpoint-v1.md promises: `nudges` omitted when
// empty, every other key present, and an empty list as `[]` rather than `null`.
//
// Neither half is free otherwise. Snapshots are built from stores that return a
// nil slice for "nothing recorded yet", so without this a fresh session's
// checkpoint carried `"decisions": null` — and `omitempty` on the optional-looking
// fields dropped seven keys entirely. Both contradict the one clause the spec
// states for readers in other languages: empty means empty, never absent, never
// null. Doing it in MarshalJSON rather than at each construction site means a new
// caller cannot reintroduce the difference.
func (s Snapshot) MarshalJSON() ([]byte, error) {
	type snapshot Snapshot // sheds this method, so json does not recurse
	out := snapshot(s)
	if out.Decisions == nil {
		out.Decisions = []*Decision{}
	}
	if out.Blockers == nil {
		out.Blockers = []*Blocker{}
	}
	if out.Progress.Completed == nil {
		out.Progress.Completed = []string{}
	}
	if out.Progress.Pending == nil {
		out.Progress.Pending = []string{}
	}
	if out.Progress.Tasks == nil {
		out.Progress.Tasks = []TaskState{}
	}
	if out.Files.Modified == nil {
		out.Files.Modified = []string{}
	}
	return json.Marshal(out)
}

type SessionState struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type WorkspaceState struct {
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	Commit     string `json:"commit"`
	Dirty      bool   `json:"dirty"`
}

type TaskState struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type ProgressState struct {
	Completed []string `json:"completed"`
	Pending   []string `json:"pending"`
	// Tasks carries every task with its id and status. Completed/Pending are
	// title lists for rendering and cannot express a status change, so diffing
	// task state needs this. Absent in snapshots written before it existed.
	Tasks []TaskState `json:"tasks"`
}

type FilesState struct {
	Modified []string `json:"modified"`
}

type TestsState struct {
	Status   string `json:"status"`
	Failures int    `json:"failures"`
}
