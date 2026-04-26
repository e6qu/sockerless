package azf

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// PodStart starts all containers in a pod by calling ContainerStart for each,
// which triggers the Azure Function App HTTP invocation.
func (s *Server) PodStart(name string) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	var errs []string
	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(context.Background(), cid)
		if !ok || c.State.Running {
			continue
		}
		if err := s.ContainerStart(cid); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %s", cid[:12], err.Error()))
		}
	}

	s.Store.Pods.SetStatus(pod.ID, "running")
	if errs == nil {
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodStop stops all running containers in a pod.
func (s *Server) PodStop(name string, timeout *int) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(context.Background(), cid)
		if !ok || !c.State.Running {
			continue
		}
		s.StopHealthCheck(cid)
		// Close wait channel so ContainerWait unblocks
		if ch, ok := s.Store.WaitChs.LoadAndDelete(cid); ok {
			close(ch.(chan struct{}))
		}
		// `pod stop` sends SIGTERM → exit 143 (BUG-826).
		stopExitCode := core.SignalToExitCode("SIGTERM")
		s.EmitEvent("container", "die", cid, map[string]string{
			"exitCode": fmt.Sprintf("%d", stopExitCode),
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		s.EmitEvent("container", "stop", cid, map[string]string{
			"name": strings.TrimPrefix(c.Name, "/"),
		})
	}

	s.Store.Pods.SetStatus(pod.ID, "stopped")
	return &api.PodActionResponse{ID: pod.ID, Errs: []string{}}, nil
}

// PodKill sends a signal to all running containers in a pod.
func (s *Server) PodKill(name string, signal string) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	if signal == "" {
		signal = "SIGKILL"
	}
	exitCode := core.SignalToExitCode(signal)

	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(context.Background(), cid)
		if !ok || !c.State.Running {
			continue
		}
		s.StopHealthCheck(cid)

		s.EmitEvent("container", "kill", cid, map[string]string{
			"name": strings.TrimPrefix(c.Name, "/"),
		})
		s.EmitEvent("container", "die", cid, map[string]string{
			"exitCode": fmt.Sprintf("%d", exitCode),
			"name":     strings.TrimPrefix(c.Name, "/"),
		})

		if ch, ok := s.Store.WaitChs.LoadAndDelete(cid); ok {
			close(ch.(chan struct{}))
		}
	}

	s.Store.Pods.SetStatus(pod.ID, "exited")
	return &api.PodActionResponse{ID: pod.ID, Errs: []string{}}, nil
}

// PodRemove removes a pod and all its containers, cleaning up Azure Function
// App resources.
func (s *Server) PodRemove(name string, force bool) error {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return &api.NotFoundError{Resource: "pod", ID: name}
	}

	ctx := context.Background()

	// Without force, reject if any containers are running
	if !force {
		for _, cid := range pod.ContainerIDs {
			c, ok := s.ResolveContainerAuto(ctx, cid)
			if ok && c.State.Running {
				return &api.ConflictError{
					Message: fmt.Sprintf("pod %s has running containers, cannot remove without force", name),
				}
			}
		}
	}

	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(ctx, cid)
		if !ok {
			continue
		}

		if force && c.State.Running {
			// `pod rm -f` is SIGKILL → exit 137 (BUG-826).
			killExitCode := core.SignalToExitCode("SIGKILL")
			s.EmitEvent("container", "kill", cid, map[string]string{
				"name": strings.TrimPrefix(c.Name, "/"),
			})
			s.EmitEvent("container", "die", cid, map[string]string{
				"exitCode": fmt.Sprintf("%d", killExitCode),
				"name":     strings.TrimPrefix(c.Name, "/"),
			})
			if ch, ok := s.Store.WaitChs.LoadAndDelete(cid); ok {
				close(ch.(chan struct{}))
			}
		}

		s.StopHealthCheck(cid)

		// Delete Function App (best-effort)
		azfState, _ := s.AZF.Get(cid)
		if azfState.FunctionAppName != "" {
			_, err := s.azure.WebApps.Delete(ctx, s.config.ResourceGroup, azfState.FunctionAppName, nil)
			if err != nil {
				s.Logger.Debug().Err(err).Str("functionApp", azfState.FunctionAppName).Msg("failed to delete Function App during pod remove")
			}
		}
		if azfState.ResourceID != "" {
			s.Registry.MarkCleanedUp(azfState.ResourceID)
		}

		// Clean up network associations
		for _, ep := range c.NetworkSettings.Networks {
			if ep != nil && ep.NetworkID != "" {
				_ = s.Drivers.Network.Disconnect(ctx, ep.NetworkID, cid)
			}
		}

		s.PendingCreates.Delete(cid)
		s.AZF.Delete(cid)
		if ch, ok := s.Store.WaitChs.LoadAndDelete(cid); ok {
			close(ch.(chan struct{}))
		}
		s.Store.LogBuffers.Delete(cid)
		s.Store.StagingDirs.Delete(cid)
		if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(cid); ok {
			for _, d := range dirs.([]string) {
				os.RemoveAll(d)
			}
		}
		for _, eid := range c.ExecIDs {
			s.Store.Execs.Delete(eid)
		}

		s.EmitEvent("container", "destroy", cid, map[string]string{
			"name": strings.TrimPrefix(c.Name, "/"),
		})
	}

	s.Store.Pods.DeletePod(pod.ID)
	return nil
}
