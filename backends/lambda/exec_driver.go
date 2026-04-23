package lambda

// Phase 96 lifted the exec/stream drivers into backend-core.
// Lambda now uses the shared types directly; the types below are just
// aliases so existing Server.Drivers wiring keeps compiling.

import (
	core "github.com/sockerless/backend-core"
)

type lambdaExecDriver = core.ReverseAgentExecDriver

type lambdaStreamDriver = core.ReverseAgentStreamDriver
