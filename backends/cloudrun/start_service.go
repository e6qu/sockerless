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
	serviceURL := s.waitForServiceURL(id, 5*time.Minute)
	s.Logger.Info().Str("container", id).Str("url", serviceURL).Msg("invokeServiceDefaultCmd: waitForServiceURL returned")
	defer func() {
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		} else if exitCh != nil {
			// Belt-and-suspenders close in case WaitChs was already
			// drained (e.g. by ContainerStop firing before invoke
			// returned). Closing the local channel is harmless if
			// nobody's reading.
			select {
			case <-exitCh:
			default:
				close(exitCh)
			}
		}
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

	req, err := http.NewRequestWithContext(s.ctx(), http.MethodPost, serviceURL, nil)
	if err != nil {
		inv.ExitCode = 1
		inv.Error = err.Error()
		s.Store.PutInvocationResult(id, inv)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	s.Logger.Info().Str("container", id).Str("url", serviceURL).Msg("invokeServiceDefaultCmd: POST starting")
	postStart := time.Now()
	resp, err := client.Do(req)
	postDur := time.Since(postStart)
	s.Logger.Info().Str("container", id).Dur("dur", postDur).Err(err).Msg("invokeServiceDefaultCmd: POST returned")
	if err != nil {
		s.Logger.Error().Err(err).Str("container", id).Msg("service invocation failed")
		inv.ExitCode = core.HTTPInvokeErrorExitCode(err)
		inv.Error = err.Error()
		s.Store.PutInvocationResult(id, inv)
		return
	}
	defer resp.Body.Close()
	s.Logger.Info().Str("container", id).Int("status", resp.StatusCode).Msg("invokeServiceDefaultCmd: response status")

	body, _ := io.ReadAll(resp.Body)
	if len(body) > 0 {
		s.Store.LogBuffers.Store(id, body)
	}

	// Bootstrap rides the real subprocess exit code in this header
	// (mirrors sockerless-gcf-bootstrap). Falls back to HTTP status
	// code mapping when the header is absent / unparseable.
	if exitHeader := resp.Header.Get("X-Sockerless-Exit-Code"); exitHeader != "" {
		if code, perr := strconv.Atoi(exitHeader); perr == nil {
			inv.ExitCode = code
		} else {
			inv.ExitCode = core.HTTPStatusToExitCode(resp.StatusCode)
		}
	} else {
		inv.ExitCode = core.HTTPStatusToExitCode(resp.StatusCode)
	}
	if inv.ExitCode != 0 {
		inv.Error = fmt.Sprintf("HTTP %d (exit-code %d)", resp.StatusCode, inv.ExitCode)
	}
	s.Store.PutInvocationResult(id, inv)
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
		s.PendingCreates.Delete(pc.ID)
		s.CloudRun.Update(pc.ID, func(state *CloudRunState) {
			state.ServiceName = svcFullName
		})
	}
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
