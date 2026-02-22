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
