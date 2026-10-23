package errors

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrNotInitialized     = errors.New("project not initialized, run agent-session init")
	ErrNoActiveSession    = errors.New("no active session")
	ErrSessionNotFound    = errors.New("session not found")
	ErrProjectNotFound    = errors.New("project not found")
	ErrTaskNotFound       = errors.New("task not found")
	ErrBlockerNotFound    = errors.New("blocker not found")
	ErrCheckpointNotFound = errors.New("checkpoint not found")
	ErrArtifactNotFound   = errors.New("artifact not found")
	ErrNotGitRepo         = errors.New("not a git repository")
	ErrAgentNotSupported  = errors.New("agent not supported")
	ErrPluginNotFound     = errors.New("agent plugin not found")
	ErrInvalidSnapshot    = errors.New("invalid snapshot")
	ErrTaskTitleRequired  = errors.New("task title is required")
	ErrInvalidEventType   = errors.New("unknown canonical event type")
	// The vocabularies of format-v1.md are closed at the service boundary, the
	// same way the event type namespace above is. Every rejection names the
	// accepted values: a caller told only that its value was refused has to guess
	// what to send instead.
	ErrInvalidTaskStatus    = errors.New("unknown task status")
	ErrInvalidKnowledgeKind = errors.New("unknown knowledge kind")
)
