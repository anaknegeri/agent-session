package errors

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrNotInitialized     = errors.New("project not initialized, run agent-session init")
	ErrNoActiveSession    = errors.New("no active session")
	ErrSessionNotFound    = errors.New("session not found")
	ErrProjectNotFound    = errors.New("project not found")
	ErrTaskNotFound       = errors.New("task not found")
	ErrCheckpointNotFound = errors.New("checkpoint not found")
	ErrNotGitRepo         = errors.New("not a git repository")
	ErrAgentNotSupported  = errors.New("agent not supported")
	ErrPluginNotFound     = errors.New("agent plugin not found")
	ErrInvalidSnapshot    = errors.New("invalid snapshot")
)
