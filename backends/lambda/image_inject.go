package lambda

import (
	"fmt"
	"strings"
)

// OverlayImageSpec describes the per-container overlay image that the
// Lambda backend builds on top of the user's requested image so the
// function can run the sockerless-agent + sockerless-lambda-bootstrap
// alongside the user's entrypoint. See docs/LAMBDA_EXEC_DESIGN.md.
//
// The Dockerfile produced by RenderOverlayDockerfile is built and
// pushed to ECR by the Lambda backend's existing image pipeline (see
// image_resolve.go), with the resulting URI used as the Lambda
// function's image URI.
type OverlayImageSpec struct {
	// BaseImageRef is the user's requested image, already resolved to
	// an ECR-compatible reference (e.g. via resolveImageURI).
	BaseImageRef string
	// AgentBinaryPath is the in-build-context path of the
	// sockerless-agent binary that should be COPYed into /opt/sockerless.
	AgentBinaryPath string
	// BootstrapBinaryPath is the in-build-context path of the
	// sockerless-lambda-bootstrap binary.
	BootstrapBinaryPath string
	// UserEntrypoint is the original Dockerfile ENTRYPOINT from the
	// container's create request. May be empty (image default used).
	UserEntrypoint []string
	// UserCmd is the original Dockerfile CMD.
	UserCmd []string
}

// RenderOverlayDockerfile returns the Dockerfile content that, when
// built against a context containing the agent + bootstrap binaries,
// produces a Lambda-compatible image with exec routed through the
// reverse agent.
//
// The user's original ENTRYPOINT and CMD are captured in env vars
// (SOCKERLESS_USER_ENTRYPOINT / SOCKERLESS_USER_CMD) rather than
// preserved directly, because the bootstrap needs to own the
// ENTRYPOINT to serve the Lambda Runtime API.
func RenderOverlayDockerfile(spec OverlayImageSpec) (string, error) {
	if spec.BaseImageRef == "" {
		return "", fmt.Errorf("BaseImageRef is required")
	}
	if spec.AgentBinaryPath == "" {
		return "", fmt.Errorf("AgentBinaryPath is required")
	}
	if spec.BootstrapBinaryPath == "" {
		return "", fmt.Errorf("BootstrapBinaryPath is required")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "FROM %s\n", spec.BaseImageRef)
	fmt.Fprintf(&b, "COPY %s /opt/sockerless/sockerless-agent\n", spec.AgentBinaryPath)
	fmt.Fprintf(&b, "COPY %s /opt/sockerless/sockerless-lambda-bootstrap\n", spec.BootstrapBinaryPath)
	fmt.Fprintln(&b, `RUN chmod +x /opt/sockerless/sockerless-agent /opt/sockerless/sockerless-lambda-bootstrap`)
	if ep := joinForEnv(spec.UserEntrypoint); ep != "" {
		fmt.Fprintf(&b, "ENV SOCKERLESS_USER_ENTRYPOINT=%s\n", ep)
	}
	if cmd := joinForEnv(spec.UserCmd); cmd != "" {
		fmt.Fprintf(&b, "ENV SOCKERLESS_USER_CMD=%s\n", cmd)
	}
	fmt.Fprintln(&b, `ENTRYPOINT ["/opt/sockerless/sockerless-lambda-bootstrap"]`)
	return b.String(), nil
}

// joinForEnv encodes a []string as colon-separated for env-var
// transport. Empty input returns empty string so the ENV line can be
// omitted entirely. Colons inside args are intentionally not escaped —
// callers controlling the entrypoint should avoid them; the bootstrap
// documents this at the receiving end.
func joinForEnv(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ":")
}
