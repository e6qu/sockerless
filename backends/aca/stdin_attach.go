package aca

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sockerless/api"
	azurecommon "github.com/sockerless/azure-common"
	core "github.com/sockerless/backend-core"
)

func (s *Server) runACAInitialStdinStage(id string, c api.Container) {
	stdin, ok := s.captureACAStdin(id)
	if !ok {
		return
	}
	s.runACAStageInvoke(id, c, stdin)
}

// runACAOneShotCommand runs a one-shot command container's command once via the
// reverse-agent and closes its wait channel. The gitlab-runner cache-volume
// permission container (a `--rm` container that chmods the cache mount and
// exits) needs `docker wait` to return its exit code; a long-lived App that
// only serves the reverse-agent never produces one, so prepare-stage hangs.
func (s *Server) runACAOneShotCommand(id string, c api.Container) {
	s.runACAStageInvoke(id, c, nil)
}

// runACAStageInvoke runs the container's resolved command (optionally piping
// `stdin`) through the App's bootstrap HTTP buffered-invoke (`/aca-app-invoke`
// → the bootstrap's :8080 listener), then publishes the result and closes the
// wait channel via finishACAInitialStdinStage. The runner stage uses HTTP —
// not the reverse-agent WebSocket — so it never depends on a persistent
// backend→container connection (the same robustness reason cloudrun + gcf
// route their stages through the bootstrap's HTTP listener); the reverse agent
// remains for interactive `docker exec`.
func (s *Server) runACAStageInvoke(id string, c api.Container, stdin []byte) {
	inv := core.InvocationResult{}

	var argv []string
	if len(stdin) > 0 {
		// gitlab-runner attach-stdin pattern: the captured bytes are a shell
		// script the runner pipes to the container's process. It must run under
		// /bin/sh — not the image's own entrypoint+cmd (e.g. the gitlab-runner
		// helper's `gitlab-runner-build`, which reads stdin in its own protocol
		// and silently ignores a raw script), which would leave the prepare /
		// stage producing no output and the runner looping. Mirror gcf/cloudrun,
		// which force argv=[/bin/sh] whenever stdin is captured.
		argv = []string{"/bin/sh"}
	} else {
		var argvErr error
		argv, argvErr = acaInitialStdinArgv(c)
		if argvErr != nil {
			inv.ExitCode = 126
			inv.Error = argvErr.Error()
			s.finishACAInitialStdinStage(id, inv, nil, []byte(inv.Error))
			return
		}
	}

	result, err := s.invokeStageHTTP(id, c, argv, stdin)
	if err != nil {
		logTail := s.recentACAAppLogTail(id)
		inv.ExitCode = 126
		inv.Error = fmt.Sprintf("ACA App HTTP invoke for container %s failed: %v%s", id[:12], err, logTail)
		s.finishACAInitialStdinStage(id, inv, nil, []byte(inv.Error))
		return
	}
	inv.ExitCode = result.ExitCode
	if result.ExitCode != 0 {
		inv.Error = fmt.Sprintf("subprocess exit %d", result.ExitCode)
	}
	s.finishACAInitialStdinStage(id, inv, result.Stdout, result.Stderr)
}

// invokeStageHTTP POSTs the exec envelope to the App's ingress (real ACA) or
// the sim's App-invoke proxy (harness), reached via the EndpointURL coordinate
// with the App's LatestRevisionFqdn carried as the Host header — the same
// virtual-host shape azf's invokeURLForHost uses. The bootstrap runs argv with
// the (optional) stdin piped in, and returns the captured stdout/stderr/exit.
func (s *Server) invokeStageHTTP(id string, c api.Container, argv []string, stdin []byte) (*azurecommon.ExecResult, error) {
	appState, ok := s.resolveAppACAState(s.ctx(), id)
	if !ok || appState.AppName == "" {
		return nil, fmt.Errorf("no ACA App resolved for container %s", id[:12])
	}
	fqdn, err := s.resolveAppFqdn(appState.AppName)
	if err != nil {
		return nil, err
	}
	if fqdn == "" {
		return nil, fmt.Errorf("ACA App %s has no ingress FQDN yet", appState.AppName)
	}

	timeout, terr := core.BootstrapTimeoutFromEnv("aca")
	if terr != nil {
		return nil, fmt.Errorf("invalid bootstrap-timeout env: %w", terr)
	}
	// The budget covers the bootstrap-ready wait (the sim's invoke proxy retries
	// the container's :8080 until it accepts) PLUS the stage run.
	runBudget := time.Duration(core.JobTimeoutDefault()) * time.Second
	if runBudget <= 0 {
		runBudget = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(s.ctx(), timeout+runBudget)
	defer cancel()

	client := &http.Client{Timeout: timeout + runBudget}
	return azurecommon.PostExecEnvelope(ctx, client, acaInvokeURL(s.config.EndpointURL, fqdn), fqdn, azurecommon.ExecEnvelopeExec{
		Argv:    argv,
		Workdir: c.Config.WorkingDir,
		Stdin:   azurecommon.EncodeStdin(stdin),
	})
}

// resolveAppFqdn returns the ContainerApp's LatestRevisionFqdn — the App's
// internal ingress hostname, used as the invoke Host header.
func (s *Server) resolveAppFqdn(appName string) (string, error) {
	resp, err := s.azure.ContainerApps.Get(s.ctx(), s.config.ResourceGroup, appName, nil)
	if err != nil {
		return "", fmt.Errorf("get containerapp %s for invoke: %w", appName, err)
	}
	if resp.Properties != nil && resp.Properties.LatestRevisionFqdn != nil {
		return *resp.Properties.LatestRevisionFqdn, nil
	}
	return "", nil
}

// acaInvokeURL builds the App ingress URL. An ACA App with ingress is reached
// at `https://<fqdn>/` — its ingress hostname routes to the container. Against
// real ACA the URL host IS the FQDN; against the sim the host is the EndpointURL
// coordinate and the FQDN rides in the Host header (set by PostExecEnvelope) so
// the sim's ingress (virtual-host) routing reaches the right App. The two forms
// differ only in that coordinate. Mirrors azf's invokeURLForHost.
func acaInvokeURL(endpointURL, fqdn string) string {
	scheme := "https"
	tcpHost := fqdn
	if endpointURL != "" {
		scheme = "https"
		if strings.HasPrefix(endpointURL, "http://") {
			scheme = "http"
		}
		tcpHost = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(endpointURL, "http://"), "https://"), "/")
	}
	return scheme + "://" + tcpHost + "/"
}

// acaCommandRunsViaAgent reports whether a started container carries a user
// command (in the overlay's SOCKERLESS_USER_* env) that is NOT run at App
// startup — i.e. a command container the client will `docker wait` on, whose
// command must therefore be executed via the reverse-agent. serviceLike service
// workloads carry SOCKERLESS_RUN_USER_WORKLOAD=1 (run at startup by the
// bootstrap) and are excluded.
func acaCommandRunsViaAgent(c api.Container) bool {
	hasUserCommand := false
	runsAtStartup := false
	for _, e := range c.Config.Env {
		switch {
		case e == "SOCKERLESS_RUN_USER_WORKLOAD=1":
			runsAtStartup = true
		case strings.HasPrefix(e, "SOCKERLESS_USER_CMD=") && len(e) > len("SOCKERLESS_USER_CMD="):
			hasUserCommand = true
		case strings.HasPrefix(e, "SOCKERLESS_USER_ENTRYPOINT=") && len(e) > len("SOCKERLESS_USER_ENTRYPOINT="):
			hasUserCommand = true
		}
	}
	return hasUserCommand && !runsAtStartup
}

func (s *Server) captureACAStdin(id string) ([]byte, bool) {
	v, ok := s.stdinPipes.LoadAndDelete(id)
	if !ok {
		return nil, false
	}
	pipe := v.(*stdinPipe)
	select {
	case <-pipe.Done():
	case <-time.After(30 * time.Second):
		s.Logger.Warn().Str("container", id).Msg("ACA stdin pipe Done timeout; proceeding with captured bytes")
	case <-s.ctx().Done():
		return nil, true
	}
	return pipe.Bytes(), true
}

func (s *Server) finishACAInitialStdinStage(id string, inv core.InvocationResult, stdout, stderr []byte) {
	if len(stdout) > 0 || len(stderr) > 0 {
		combined := append(append([]byte{}, stdout...), stderr...)
		s.Store.LogBuffers.Store(id, combined)
	}
	s.Store.PutInvocationResult(id, inv)
	if v, ok := s.attachStreams.LoadAndDelete(id); ok {
		v.(*attachStream).publishAttachResponse(stdout, stderr)
	}
	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": fmt.Sprintf("%d", inv.ExitCode)})
}

func acaInitialStdinArgv(c api.Container) ([]string, error) {
	argv := append([]string{}, c.Config.Entrypoint...)
	argv = append(argv, c.Config.Cmd...)
	if len(argv) > 0 {
		return argv, nil
	}

	env := map[string]string{}
	for _, item := range c.Config.Env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	entrypoint, err := decodeACAOverlayArgv("SOCKERLESS_USER_ENTRYPOINT", env["SOCKERLESS_USER_ENTRYPOINT"])
	if err != nil {
		return nil, err
	}
	cmd, err := decodeACAOverlayArgv("SOCKERLESS_USER_CMD", env["SOCKERLESS_USER_CMD"])
	if err != nil {
		return nil, err
	}
	argv = append(argv, entrypoint...)
	argv = append(argv, cmd...)
	if len(argv) == 0 {
		shortID := c.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		return nil, fmt.Errorf("container %s has no configured command for attached stdin", shortID)
	}
	return argv, nil
}

func decodeACAOverlayArgv(name, value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	var argv []string
	if err := json.Unmarshal(data, &argv); err != nil {
		return nil, fmt.Errorf("decode %s JSON: %w", name, err)
	}
	return argv, nil
}
