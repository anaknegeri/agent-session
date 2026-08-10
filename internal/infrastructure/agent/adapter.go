package agent

import "context"

// Capability describes what an agent adapter can do.
type Capability string

const (
	CapMCP  Capability = "mcp"
	CapHook Capability = "hooks"
)

// Adapter integrates Agent Session with a specific coding agent (PRD §20).
// Adapters configure the client but never modify the core session model.
type Adapter interface {
	Name() string
	Detect(ctx context.Context) (bool, error)
	Configure(ctx context.Context, mcpCommand string) error
	Install(ctx context.Context) error
	Uninstall(ctx context.Context) error
}
