// Package contract holds the version numbers of the interfaces agent-session
// promises to other programs, and nothing else.
//
// A session is written by one agent and read by another, often by a different
// build of this binary. Six interfaces cross that boundary and are specified in
// docs/spec:
//
//   - Format — the .agent/ directory layout (docs/spec/format-v1.md)
//   - Checkpoint — the snapshot JSON inside a checkpoint (docs/spec/checkpoint-v1.md)
//   - Event — event types and their payloads (docs/spec/event-v1.md)
//   - Context — the rendered context document (docs/spec/context-v1.md)
//   - Handoff — the rendered handoff document and its event (docs/spec/handoff-v1.md)
//   - Tools — the MCP tool and resource surface (docs/spec/mcp-tools-v1.md)
//
// Only two of them carry their version in the data: Format writes it to
// .agent/config.toml and Checkpoint writes it into every snapshot, because those
// are the two a future build reads back off disk without being able to ask
// anyone what wrote them. The other four are observed live — an MCP client lists
// the tools it is talking to, and rendered context is read, not parsed — so a
// number embedded in them would cost tokens and tell the reader nothing it did
// not already know. Their versions exist here, and in the specs, for humans
// deciding whether a change is allowed.
//
// What each version means is defined once, in docs/spec/README.md: a bump is
// required for any change that could make an existing reader wrong, and is not
// required for additive change. The tests in test/contract hold every one of the
// six to its spec, so a drift fails the build rather than surfacing as another
// agent misreading a session.
package contract

const (
	// Format is the version of the .agent/ directory layout, recorded as
	// `[format] version` in .agent/config.toml.
	Format = 1

	// Checkpoint is the version of the snapshot JSON stored in a checkpoint,
	// recorded as `version` inside the snapshot itself.
	Checkpoint = 1

	// Event is the version of the event type namespace and payload shapes.
	Event = 1

	// Context is the version of the rendered context document's structure.
	Context = 1

	// Handoff is the version of the rendered handoff document and the
	// handoff.created event payload.
	Handoff = 1

	// Tools is the version of the MCP tool and resource surface.
	Tools = 1
)
