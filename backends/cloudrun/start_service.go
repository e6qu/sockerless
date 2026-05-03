package cloudrun

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
	"google.golang.org/api/idtoken"
)

// — single- and multi-container start paths for the Services
// code path. When Config.UseService is true, ContainerStart dispatches
// here instead of the Jobs flow in backend_impl.go.
// Services are long-running with MinInstanceCount=1, so there's no
// execution-exit poller — the container stays "running" until
// ContainerStop/Remove deletes the Service and closes WaitChs
// directly.

// startSingleContainerService provisions a Cloud Run Service for one
// container. Returns after the Service is created and the first
// revision has a terminal Ready condition (or we give up waiting and
// return an error with the failed condition message).
//
// Phase 122g: spawns a background goroutine that POSTs an empty body to
// the Service URL — bootstrap's default invoke runs SOCKERLESS_USER_CMD
// as a subprocess and returns combined stdout. On HTTP response (or
// error) the goroutine records the exit code in InvocationResults and
// closes the WaitCh so docker-wait unblocks. Without this, the
// bootstrap stays alive as an HTTP server forever and gitlab-runner's
// docker wait blocks indefinitely.
func (s *Server) startSingleContainerService(id string, c api.Container, crState CloudRunState, exitCh chan struct{}) error {
	// Clean up any existing resources from a previous start.
	if crState.ServiceName != "" {
		s.deleteService(crState.ServiceName)
		s.Registry.MarkCleanedUp(crState.ServiceName)
	}

	svcName := buildServiceName(id)
	svcSpec, err := s.buildServiceSpec(s.ctx(), []containerInput{
		{ID: id, Container: &c, IsMain: true},
	})
	if err != nil {
		s.Store.WaitChs.Delete(id)
		return err
	}

	createOp, err := s.gcp.Services.CreateService(s.ctx(), &runpb.CreateServiceRequest{
		Parent:    s.buildServiceParent(),
		ServiceId: svcName,
		Service:   svcSpec,
	})
	if err != nil {
		s.Logger.Error().Err(err).Str("service", svcName).Msg("failed to create Cloud Run Service")
		s.Store.WaitChs.Delete(id)
		return gcpcommon.MapGCPError(err, "service", id)
	}

	svc, err := createOp.Wait(s.ctx())
	if err != nil {
		s.deleteService(fmt.Sprintf("%s/services/%s", s.buildServiceParent(), svcName))
		s.Store.WaitChs.Delete(id)
		s.Logger.Error().Err(err).Str("service", svcName).Msg("service creation failed")
		return gcpcommon.MapGCPError(err, "service", id)
	}
	svcFullName := svc.Name

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "cloudrun",
		ResourceType: "service",
		ResourceID:   svcFullName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": c.Image, "name": c.Name, "serviceName": svcName},
	})

	s.PendingCreates.Delete(id)
	s.CloudRun.Update(id, func(state *CloudRunState) {
		state.ServiceName = svcFullName
	})

	// Phase 122g: kick off the bootstrap's default invoke so the user
	// CMD actually runs. Mirror of gcf's launch-invoke goroutine in
	// backends/cloudrun-functions/backend_impl.go::ContainerStart.
	//
	// svc.Uri from createOp.Wait is OFTEN empty even though the LRO
	// completed — Cloud Run populates Uri only after the first revision
	// is serving traffic, and createOp.Wait returns once the spec is
	// stored, not after first-revision-ready. Pass the container ID
	// alone; the goroutine uses serviceInvokeURL (GetService) which
	// re-reads the live Service object with Uri populated. Cell 7 v14
	// (BUG-929 fix attempt 1) regressed because we trusted svc.Uri ==
	// "" and bailed.
	go s.invokeServiceDefaultCmd(id, exitCh)
	return nil
}

// invokeServiceDefaultCmd POSTs an empty body to the Service URL,
// triggering the bootstrap's default invoke (which runs the env-baked
// SOCKERLESS_USER_CMD and returns combined stdout). Records the
// exit code from the X-Sockerless-Exit-Code header in
// InvocationResults so CloudState reflects exited+ExitCode and
// closes WaitCh so ContainerWait unblocks.
//
// Resolves the Service URL via serviceInvokeURL (re-reads via
// GetService until Uri populates). Polls up to 5 minutes — Cloud Run
// Service revisions can take 60-90s to reach serving state.
func (s *Server) invokeServiceDefaultCmd(id string, exitCh chan struct{}) {
	s.Logger.Info().Str("container", id).Msg("invokeServiceDefaultCmd: goroutine entered")

	// Phase 122g attach-via-stdin-pipe: if a stdinPipe was registered
	// (gitlab-runner hijacked attach with OpenStdin), wait for the
	// caller to half-close (= stdin EOF) so we can replay the captured
	// bytes as the bootstrap's `execEnvelope.Stdin`. Skip if no pipe
	// (= no attach happened, run env-baked SOCKERLESS_USER_CMD instead).
	var capturedStdin []byte
	if v, ok := s.stdinPipes.LoadAndDelete(id); ok {
		pipe := v.(*stdinPipe)
		// Wait for caller to half-close stdin OR timeout. gitlab-runner
		// pipes the script then CloseWrite()s within milliseconds. 30s
		// upper bound mirrors backends/ecs/backend_impl.go::launchAfterStdin.
		select {
		case <-pipe.Done():
		case <-time.After(30 * time.Second):
			s.Logger.Warn().Str("container", id).Msg("invokeServiceDefaultCmd: stdin pipe Done timeout — proceeding with whatever was captured")
		}
		capturedStdin = pipe.Bytes()
		s.Logger.Info().Str("container", id).Int("stdin_bytes", len(capturedStdin)).Msg("invokeServiceDefaultCmd: stdin pipe drained")
	}

	serviceURL := s.waitForServiceURL(id, 5*time.Minute)
	s.Logger.Info().Str("container", id).Str("url", serviceURL).Msg("invokeServiceDefaultCmd: waitForServiceURL returned")
	defer func() {
		closed := false
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
			closed = true
		} else if exitCh != nil {
			// Belt-and-suspenders close in case WaitChs was already
			// drained (e.g. by ContainerStop firing before invoke
			// returned). Closing the local channel is harmless if
			// nobody's reading.
			select {
			case <-exitCh:
			default:
				close(exitCh)
				closed = true
			}
		}
		s.Logger.Info().Str("container", id).Bool("waitch_closed", closed).Msg("invokeServiceDefaultCmd: defer ran (goroutine exiting)")
	}()

	inv := core.InvocationResult{}

	if serviceURL == "" {
		s.Logger.Error().Str("container", id).Msg("no Service URL available for invocation")
		inv.ExitCode = 1
		inv.Error = "no service URL available"
		s.Store.PutInvocationResult(id, inv)
		return
	}

	s.Logger.Info().Str("container", id).Msg("invokeServiceDefaultCmd: minting idtoken client")
	client, err := idtoken.NewClient(s.ctx(), serviceURL)
	if err != nil {
		s.Logger.Error().Err(err).Str("container", id).Msg("idtoken client for service invoke")
		inv.ExitCode = core.HTTPInvokeErrorExitCode(err)
		inv.Error = err.Error()
		s.Store.PutInvocationResult(id, inv)
		return
	}
	client.Timeout = 10 * time.Minute
	s.Logger.Info().Str("container", id).Msg("invokeServiceDefaultCmd: idtoken client ready")

	// Two POST shapes:
	//   - capturedStdin nil: empty body, bootstrap runs env-baked
	//     SOCKERLESS_USER_CMD as default invoke (`docker run` semantics)
	//   - capturedStdin non-nil: gcpcommon.ExecEnvelopeRequest with
	//     argv=[/bin/sh] + Stdin=<captured>, bootstrap pipes the script
	//     into sh (gitlab-runner attach pattern)
	res, err := s.postBootstrap(client, serviceURL, capturedStdin)
	if err != nil {
		s.Logger.Error().Err(err).Str("container", id).Msg("service invocation failed")
		inv.ExitCode = 1
		inv.Error = err.Error()
		s.Store.PutInvocationResult(id, inv)
		// Fan-out failure to attached caller too so it doesn't block.
		if v, ok := s.attachStreams.LoadAndDelete(id); ok {
			v.(*attachStream).publishAttachResponse(nil, []byte(err.Error()))
		}
		return
	}
	s.Logger.Info().Str("container", id).Int("exit", res.ExitCode).Int("stdout", len(res.Stdout)).Int("stderr", len(res.Stderr)).Msg("invokeServiceDefaultCmd: bootstrap response")

	if len(res.Stdout) > 0 {
		s.Store.LogBuffers.Store(id, res.Stdout)
	}
	inv.ExitCode = res.ExitCode
	if inv.ExitCode != 0 {
		inv.Error = fmt.Sprintf("subprocess exit %d", inv.ExitCode)
	}
	s.Store.PutInvocationResult(id, inv)

	// Fan-out stdout+stderr to the attached gitlab-runner (if any).
	if v, ok := s.attachStreams.LoadAndDelete(id); ok {
		v.(*attachStream).publishAttachResponse(res.Stdout, res.Stderr)
	}
}

// postBootstrap dispatches to the appropriate bootstrap call shape
// based on whether stdin was captured.
//
//   - stdin nil: empty-body POST. Bootstrap's parseExecEnvelope sees
//     no envelope -> falls through to default-invoke path running the
//     env-baked SOCKERLESS_USER_CMD (`docker run` semantics).
//   - stdin non-nil: envelope POST with argv=[/bin/sh] + Stdin=<bytes>.
//     Bootstrap pipes the script into sh (gitlab-runner attach pattern).
//
// Both shapes return ExecResult with exitCode/stdout/stderr — the
// default-invoke path's response is plain bytes (we pack into Stdout).
func (s *Server) postBootstrap(client *http.Client, url string, stdin []byte) (*gcpcommon.ExecResult, error) {
	if len(stdin) > 0 {
		return gcpcommon.PostExecEnvelope(s.ctx(), client, url, "", gcpcommon.ExecEnvelopeExec{
			Argv:  []string{"/bin/sh"},
			Stdin: gcpcommon.EncodeStdin(stdin),
		})
	}
	// Default-invoke path — empty POST. The bootstrap returns a
	// plain-text body (not envelope JSON) plus X-Sockerless-Exit-Code
	// header. Read both manually.
	req, err := http.NewRequestWithContext(s.ctx(), http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	exitCode := 0
	if exitHeader := resp.Header.Get("X-Sockerless-Exit-Code"); exitHeader != "" {
		if code, perr := strconv.Atoi(exitHeader); perr == nil {
			exitCode = code
		} else {
			exitCode = core.HTTPStatusToExitCode(resp.StatusCode)
		}
	} else {
		exitCode = core.HTTPStatusToExitCode(resp.StatusCode)
	}
	return &gcpcommon.ExecResult{ExitCode: exitCode, Stdout: body}, nil
}

// startMultiContainerServiceTyped provisions a single Cloud Run Service
// whose revision template holds all pod containers. Parallels
// startMultiContainerJobTyped.
func (s *Server) startMultiContainerServiceTyped(_ string, podContainers []api.Container, _ chan struct{}) error {
	var inputs []containerInput
	for i, pc := range podContainers {
		pcCopy := pc
		inputs = append(inputs, containerInput{
			ID:        pc.ID,
			Container: &pcCopy,
			IsMain:    i == 0,
		})
	}
	mainID := podContainers[0].ID

	svcName := buildServiceName(mainID)
	svcSpec, err := s.buildServiceSpec(s.ctx(), inputs)
	if err != nil {
		return err
	}

	createOp, err := s.gcp.Services.CreateService(s.ctx(), &runpb.CreateServiceRequest{
		Parent:    s.buildServiceParent(),
		ServiceId: svcName,
		Service:   svcSpec,
	})
	if err != nil {
		for _, pc := range podContainers {
			s.Store.WaitChs.Delete(pc.ID)
		}
		s.Logger.Error().Err(err).Str("service", svcName).Msg("failed to create multi-container Cloud Run Service")
		return gcpcommon.MapGCPError(err, "service", mainID)
	}

	svc, err := createOp.Wait(s.ctx())
	if err != nil {
		s.deleteService(fmt.Sprintf("%s/services/%s", s.buildServiceParent(), svcName))
		for _, pc := range podContainers {
			s.Store.WaitChs.Delete(pc.ID)
		}
		s.Logger.Error().Err(err).Str("service", svcName).Msg("service creation failed")
		return gcpcommon.MapGCPError(err, "service", mainID)
	}
	svcFullName := svc.Name

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  mainID,
		Backend:      "cloudrun",
		ResourceType: "service",
		ResourceID:   svcFullName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": podContainers[0].Image, "name": podContainers[0].Name, "serviceName": svcName},
	})

	for _, pc := range podContainers {
		// Keep service-style sidecars in PendingCreates so subsequent
		// script-runner stages joining the same network can re-bundle
		// them (gitlab-runner v17.5 spawns a new script-runner per
		// stage; each stage's revision needs its own copy of the
		// sidecar). The script-runner itself (OpenStdin=true) gets
		// deleted normally — it's per-stage and not re-bundled.
		if pc.Config.OpenStdin {
			s.PendingCreates.Delete(pc.ID)
		}
		s.CloudRun.Update(pc.ID, func(state *CloudRunState) {
			state.ServiceName = svcFullName
		})
	}

	// Trigger the bootstrap's default invoke (which drains any pending
	// stdinPipe captured by ContainerAttach + POSTs an exec envelope).
	// Mirrors startSingleContainerService — without this, gitlab-runner's
	// script bytes piped via /attach never reach the bootstrap subprocess
	// and the BUILD container sits idle until Cloud Run kills it.
	mainExitCh, _ := s.Store.WaitChs.Load(mainID)
	exitCh, _ := mainExitCh.(chan struct{})
	go s.invokeServiceDefaultCmd(mainID, exitCh)
	return nil
}

// waitForServiceURL polls the Service via GetService until Uri is
// populated (revision serving traffic) or the deadline expires. The
// LRO returned by CreateService completes BEFORE first-revision-ready,
// so Uri is often empty at create-completion time. Returns the URL or
// empty string on timeout — caller should treat empty as failure.
func (s *Server) waitForServiceURL(containerID string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		url, ok := s.serviceInvokeURL(s.ctx(), containerID)
		if attempt%5 == 1 {
			s.Logger.Debug().Str("container", containerID).Int("attempt", attempt).Bool("ok", ok).Str("url", url).Msg("waitForServiceURL poll")
		}
		if ok && url != "" {
			return url
		}
		time.Sleep(2 * time.Second)
	}
	return ""
}

// deleteService deletes a Cloud Run Service and waits for the LRO to
// complete. Best-effort: errors are logged, not propagated, so caller
// cleanup paths stay simple.
func (s *Server) deleteService(serviceName string) {
	if serviceName == "" || s.gcp == nil || s.gcp.Services == nil {
		return
	}
	op, err := s.gcp.Services.DeleteService(context.Background(), &runpb.DeleteServiceRequest{
		Name: serviceName,
	})
	if err != nil {
		s.Logger.Warn().Err(err).Str("service", serviceName).Msg("failed to initiate service delete")
		return
	}
	if _, err := op.Wait(context.Background()); err != nil {
		s.Logger.Warn().Err(err).Str("service", serviceName).Msg("service delete wait failed")
	}
}
