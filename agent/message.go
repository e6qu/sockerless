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
	// checkpoint-restart — FaaS max is a hard limit.
	TypeLifetimeExpired = "lifetime_expired"
)

// maxWSMessageBytes bounds the size of a single inbound WebSocket message on
// every connection the agent reads (server, reverse-connect, and the
// backend-side bridge/reverse readers). gorilla/websocket defaults to NO read
// limit, so without this a peer can declare a multi-gigabyte frame and OOM the
// process. Legitimate messages are JSON envelopes carrying at most one 32 KiB
// stdin/stdout chunk base64-encoded (~44 KiB) plus small fields, so 4 MiB is far
// above any real message yet bounds the attack surface. A message past the limit
// makes ReadMessage return an error and the read loop tears the connection down.
const maxWSMessageBytes = 4 << 20 // 4 MiB

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
