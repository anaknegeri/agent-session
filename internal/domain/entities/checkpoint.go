package entities

// Checkpoint kinds record what triggered a checkpoint. Retention is applied per
// kind, so a burst of automatic checkpoints cannot evict the deliberate ones.
const (
	// CheckpointKindManual is an explicit checkpoint from an agent, the CLI, or
	// session.record.
	CheckpointKindManual = "manual"
	// CheckpointKindAuto is one the session layer took by itself: a Stop hook or
	// a stale-context checkpoint.
	CheckpointKindAuto = "auto"
	// CheckpointKindPreCompact is taken before an agent compacts its context.
	CheckpointKindPreCompact = "precompact"
	// CheckpointKindHandoff accompanies a handoff to another agent.
	CheckpointKindHandoff = "handoff"
)

// CheckpointKinds lists every valid kind.
var CheckpointKinds = []string{
	CheckpointKindManual,
	CheckpointKindAuto,
	CheckpointKindPreCompact,
	CheckpointKindHandoff,
}

// CheckpointKindFor maps a checkpoint label to a kind. Hooks installed before
// the kind column existed pass only `--label auto` or `--label precompact`, so
// those labels still have to resolve to the right kind.
func CheckpointKindFor(label string) string {
	switch {
	case label == CheckpointKindAuto || hasPrefix(label, "auto-checkpoint"):
		return CheckpointKindAuto
	case label == CheckpointKindPreCompact:
		return CheckpointKindPreCompact
	case label == CheckpointKindHandoff:
		return CheckpointKindHandoff
	default:
		return CheckpointKindManual
	}
}

// ValidCheckpointKind reports whether kind is one this project writes.
func ValidCheckpointKind(kind string) bool {
	for _, k := range CheckpointKinds {
		if k == kind {
			return true
		}
	}
	return false
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

type Checkpoint struct {
	ID         string    `json:"id" db:"id"`
	SessionID  string    `json:"session_id" db:"session_id"`
	TaskID     string    `json:"task_id,omitempty" db:"task_id"`
	Kind       string    `json:"kind" db:"kind"`
	Label      string    `json:"label,omitempty" db:"label"`
	Snapshot   string    `json:"snapshot" db:"snapshot"`
	NextAction string    `json:"next_action,omitempty" db:"next_action"`
	Agent      string    `json:"agent,omitempty" db:"agent"`
	CreatedAt  Timestamp `json:"created_at" db:"created_at"`
}
