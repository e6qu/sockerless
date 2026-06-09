package aca

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

func (s *Server) runACAInitialStdinStage(id string, c api.Container) {
	stdin, ok := s.captureACAStdin(id)
	if !ok {
		return
	}

	inv := core.InvocationResult{}
	timeout, err := core.BootstrapTimeoutFromEnv("aca")
	if err != nil {
		inv.ExitCode = 126
		inv.Error = fmt.Sprintf("invalid bootstrap-timeout env: %v", err)
		s.finishACAInitialStdinStage(id, inv, nil, []byte(inv.Error))
		return
	}

	waitCtx, cancel := context.WithTimeout(s.ctx(), timeout)
	defer cancel()
	if werr := s.reverseAgents.WaitForAgent(waitCtx, id); werr != nil {
		logTail := s.recentACAAppLogTail(id)
		inv.ExitCode = 126
		inv.Error = fmt.Sprintf(
			"reverse-agent did not register for container %s within %s "+
				"(SOCKERLESS_ACA_BOOTSTRAP_TIMEOUT_SEC). The App was created and "+
				"started but the in-container bootstrap never dialled back to "+
				"SOCKERLESS_CALLBACK_URL=%s. Check egress / VNet integration / NSG.%s",
			id[:12], timeout, s.config.CallbackURL, logTail,
		)
		s.finishACAInitialStdinStage(id, inv, nil, []byte(inv.Error))
		return
	}

	argv, argvErr := acaInitialStdinArgv(c)
	if argvErr != nil {
		inv.ExitCode = 126
		inv.Error = argvErr.Error()
		s.finishACAInitialStdinStage(id, inv, nil, []byte(inv.Error))
		return
	}

	stdout, stderr, exitCode, execErr := s.reverseAgents.RunAndCaptureWithStdin(
		id,
		"aca-stdin-"+id[:12],
		argv,
		nil,
		c.Config.WorkingDir,
		stdin,
	)
	inv.ExitCode = exitCode
	if execErr != nil {
		inv.ExitCode = 126
		inv.Error = execErr.Error()
		stderr = append(stderr, []byte(execErr.Error())...)
	} else if exitCode != 0 {
		inv.Error = fmt.Sprintf("subprocess exit %d", exitCode)
	}
	s.finishACAInitialStdinStage(id, inv, stdout, stderr)
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
