package aca

import (
	"context"
	"fmt"
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
		inv.ExitCode = 126
		inv.Error = fmt.Sprintf(
			"reverse-agent did not register for container %s within %s "+
				"(SOCKERLESS_ACA_BOOTSTRAP_TIMEOUT_SEC). The App was created and "+
				"started but the in-container bootstrap never dialled back to "+
				"SOCKERLESS_CALLBACK_URL=%s. Check egress / VNet integration / NSG.",
			id[:12], timeout, s.config.CallbackURL,
		)
		s.finishACAInitialStdinStage(id, inv, nil, []byte(inv.Error))
		return
	}

	stdout, stderr, exitCode, execErr := s.reverseAgents.RunAndCaptureWithStdin(
		id,
		"aca-stdin-"+id[:12],
		[]string{"/bin/sh"},
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
