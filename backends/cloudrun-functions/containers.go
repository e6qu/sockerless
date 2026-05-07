package gcf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/idtoken"
)

// execEnvelope mirrors the bootstrap's expected request body for the
// "Path B" exec route. Sending entrypoint/cmd/workdir/env in the
// request body lets pool-claimed Functions execute the right user
// command WITHOUT requiring an UpdateService rollout: the Cloud Run
// regional CPU quota only debits on revision creation, not on invoke,
// so envelope-driven dispatch keeps the warm pool warm.
type execEnvelope struct {
	Sockerless struct {
		Exec struct {
			Argv    []string `json:"argv"`
			Workdir string   `json:"workdir,omitempty"`
			Env     []string `json:"env,omitempty"`
		} `json:"exec"`
	} `json:"sockerless"`
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
func invokeFunction(ctx context.Context, audienceURL string, argv []string, workdir string, env []string) (*http.Response, error) {
	client, err := idtoken.NewClient(ctx, audienceURL)
	if err != nil {
		if isUnsupportedCredsErr(err) {
			return nil, fmt.Errorf(
				"gcf invoke: ADC user credentials cannot sign ID tokens for Cloud Run; configure service-account ADC via GOOGLE_APPLICATION_CREDENTIALS or `gcloud auth login --impersonate-service-account=<sa>`. underlying: %w",
				err,
			)
		}
		return nil, fmt.Errorf("idtoken.NewClient(%s): %w", audienceURL, err)
	}
	client.Timeout = 10 * time.Minute

	var body []byte
	if len(argv) > 0 {
		var env_ execEnvelope
		env_.Sockerless.Exec.Argv = argv
		env_.Sockerless.Exec.Workdir = workdir
		env_.Sockerless.Exec.Env = env
		body, err = json.Marshal(&env_)
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
