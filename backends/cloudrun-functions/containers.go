package gcf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sockerless/agent/envelope"
)

// bootstrapReadyPath cold-starts a scale-to-zero Service instance without
// running the user's keepalive command. The overlay bootstrap serves it
// (HTTP 204); the gcp sim's Cloud Run Services invoke handler forwards the
// request path to the bootstrap.
const bootstrapReadyPath = "/_sockerless/ready"

// warmBootstrap POSTs to the bootstrap's readiness route so a scale-to-zero
// Cloud Run Service creates its first instance — starting the overlay
// bootstrap so it dials the reverse-agent — WITHOUT running the container's
// long-lived keepalive command as a request. Used by the GH actions/runner
// materialize path, where the JOB container must stay alive for `docker exec`.
func (s *Server) warmBootstrap(ctx context.Context, audienceURL string) error {
	client, err := s.Access.AuthenticatedClient(ctx, audienceURL)
	if err != nil {
		return err
	}
	client.Timeout = 10 * time.Minute
	readyURL := strings.TrimRight(audienceURL, "/") + bootstrapReadyPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, readyURL, nil)
	if err != nil {
		return fmt.Errorf("build warmup request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send warmup request: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("warmup returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// invokeFunction does an authenticated HTTPS POST to the function's
// underlying Cloud Run Service URL. Cloud Run requires a Google ID
// token in the Authorization header (audience = service URL). idtoken
// signs the request automatically using ADC. Service-account ADC works;
// user-account ADC (`gcloud auth application-default login`) does NOT —
// the Google idtoken library refuses to sign with user creds.
//
// When `argv` is non-empty, the request body carries an exec envelope
// so the bootstrap runs Path B (envelope.argv) instead of Path A
// (env-baked SOCKERLESS_USER_*). Pool-claimed Functions don't need
// their env updated on each claim — the user's entrypoint+cmd+workdir
// flow through the request body and an immutable pool entry can serve
// any user command.
func (s *Server) invokeFunction(ctx context.Context, audienceURL string, argv []string, workdir string, env []string, stdin []byte) (*http.Response, error) {
	client, err := s.Access.AuthenticatedClient(ctx, audienceURL)
	if err != nil {
		if isUnsupportedCredsErr(err) {
			return nil, fmt.Errorf(
				"gcf invoke: ADC user credentials cannot sign ID tokens for Cloud Run; configure service-account ADC via GOOGLE_APPLICATION_CREDENTIALS or `gcloud auth login --impersonate-service-account=<sa>`. underlying: %w",
				err,
			)
		}
		return nil, err
	}
	client.Timeout = 10 * time.Minute

	var body []byte
	if len(argv) > 0 {
		body, err = json.Marshal(envelope.NewRequest(envelope.Exec{
			Argv:    argv,
			Workdir: workdir,
			Env:     env,
			Stdin:   envelope.EncodeStdin(stdin),
		}))
		if err != nil {
			return nil, fmt.Errorf("marshal exec envelope: %w", err)
		}
	}

	var reqBody *bytes.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	var req *http.Request
	if reqBody != nil {
		req, err = http.NewRequestWithContext(ctx, "POST", audienceURL, reqBody)
	} else {
		req, err = http.NewRequestWithContext(ctx, "POST", audienceURL, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return client.Do(req)
}

func isUnsupportedCredsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unsupported credentials type") || strings.Contains(msg, "authorized_user")
}
