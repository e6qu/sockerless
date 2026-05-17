package lambda

// execEnvelopeRequest is the JSON shape sent as the payload of the
// main `lambda.Invoke` that holds a container alive. Mirrors
// `agent/cmd/sockerless-lambda-bootstrap/main.go::execEnvelope`.
//
// Used by GitLab-runner's stdin-piped scripts: the runner sends a raw
// bash script via /attach + /start; sockerless wraps it as
// `{"sockerless":{"exec":{"argv":["sh","-c","<script>"]}}}` so the
// bootstrap parses it as a docker-exec dispatch, runs the script, and
// returns the result envelope.
//
// Per-step `docker exec` (`ExecStart`) does NOT route through this
// envelope — it uses the reverse-agent WebSocket dispatch
// (`backends/core/handle_exec.go` → `s.Typed.Exec.Exec(...)`).
type execEnvelopeRequest struct {
	Sockerless struct {
		Exec execEnvelopeExec `json:"exec"`
	} `json:"sockerless"`
}

type execEnvelopeExec struct {
	Argv    []string `json:"argv"`
	Tty     bool     `json:"tty,omitempty"`
	Workdir string   `json:"workdir,omitempty"`
	Env     []string `json:"env,omitempty"`
	Stdin   string   `json:"stdin,omitempty"`
}
