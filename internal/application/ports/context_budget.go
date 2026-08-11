package ports

// ContextBudget bounds how much context is rendered to the agent (token savings).
type ContextBudget struct {
	MaxDecisions  int
	MaxBlockers   int
	MaxFiles      int
	MaxEvents     int
	MaxProgress   int
	MaxItemChars  int
	MaxTotalChars int
	InjectMemory  bool
	MaxMemory     int
}

// DefaultContextBudget mirrors config defaults.
func DefaultContextBudget() ContextBudget {
	return ContextBudget{
		MaxDecisions:  5,
		MaxBlockers:   3,
		MaxFiles:      8,
		MaxEvents:     10,
		MaxProgress:   10,
		MaxItemChars:  200,
		MaxTotalChars: 4000,
		InjectMemory:  true,
		MaxMemory:     3,
	}
}
