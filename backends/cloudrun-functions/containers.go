package gcf

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/idtoken"
)

// invokeFunction does an authenticated HTTPS POST to the function's
// underlying Cloud Run Service URL. Cloud Run requires a Google ID
// token in the Authorization header (audience = service URL). idtoken
// signs the request automatically using ADC. Service-account ADC works;
// user-account ADC (`gcloud auth application-default login`) does NOT —
// the Google idtoken library refuses to sign with user creds. Operators
// must use one of:
//   - service-account JSON key via GOOGLE_APPLICATION_CREDENTIALS
//   - workload-identity (when sockerless itself runs on GCP)
//   - service-account impersonation via gcloud auth login
//     --impersonate-service-account=<sa@project.iam.gserviceaccount.com>
//
// We deliberately do NOT fall back to unauthenticated invoke or grant
// allUsers → run.invoker as a workaround: a public function URL that
// any internet caller could trigger violates the security posture
// sockerless inherits from the operator's project. Failures are loud.
func invokeFunction(ctx context.Context, audienceURL string) (*http.Response, error) {
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
	req, err := http.NewRequestWithContext(ctx, "POST", audienceURL, nil)
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
