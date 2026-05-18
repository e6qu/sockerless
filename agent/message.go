package agent

// WebSocket message type constants.
const (
	// Frontend → Agent
	TypeExec       = "exec"
	TypeAttach     = "attach"
	TypeStdin      = "stdin"
	TypeCloseStdin = "close_stdin"
	TypeSignal     = "signal"
	TypeResize     = "resize"

	// Agent → Frontend
	TypeStdout = "stdout"
	TypeStderr = "stderr"
	TypeExit   = "exit"
	TypeError  = "error"
	TypeHealth = "health"

	// TypeLifetimeExpired is sent by a FaaS bootstrap (lambda / gcf /
	// cloudrun) shortly before the platform's max invocation deadline
	// would force-kill the function. The sockerless backend marks the
	// container as Stopped with reason FaaSPodLifetimeExceeded so
	// the next ExecStart returns operator-guidance ("use ECS / ACA /
	// Cloud Run Services for longer pods") rather than a generic 500
	// or hanging exec. No transparent re-invoke / warm-pool /
	// checkpoint-restart — FaaS max is a hard limit per Phase 168.
	TypeLifetimeExpired = "lifetime_expired"
)

// Message is the unified WebSocket message type.
// All fields are optional depending on the message type.
type Message struct {
	Type    string   `json:"type"`
	ID      string   `json:"id,omitempty"`
	Cmd     []string `json:"cmd,omitempty"`
	Env     []string `json:"env,omitempty"`
	WorkDir string   `json:"workdir,omitempty"`
	Tty     bool     `json:"tty,omitempty"`
	Data    string   `json:"data,omitempty"`
	Signal  string   `json:"signal,omitempty"`
	Code    *int     `json:"code,omitempty"`
	Message string   `json:"message,omitempty"`
	Status  string   `json:"status,omitempty"`
	Width   int      `json:"width,omitempty"`
	Height  int      `json:"height,omitempty"`
	Log     string   `json:"log,omitempty"`
}

// intPtr returns a pointer to an int value.
func intPtr(v int) *int {
	return &v
}
