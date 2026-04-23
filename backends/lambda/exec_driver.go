package lambda

// Exec/stream drivers live in backend-core. The aliases below keep
// existing Server.Drivers wiring compiling without a churn commit.

import (
	core "github.com/sockerless/backend-core"
)

type lambdaExecDriver = core.ReverseAgentExecDriver

type lambdaStreamDriver = core.ReverseAgentStreamDriver
