package core

import (
	"fmt"
	"time"

	"github.com/sockerless/api"
)

// PodDeferredStart handles deferred start logic for pod containers.
// Returns:
//   - shouldDefer=true: container is in a multi-container pod, not all started yet
//   - shouldDefer=false, podContainers=nil: not in a pod or single-container pod
//   - shouldDefer=false, podContainers!=nil: all pod containers started, materialize now
func (s *BaseServer) PodDeferredStart(containerID string) (shouldDefer bool, podContainers []api.Container) {
	pod, inPod := s.Store.Pods.GetPodForContainer(containerID)
	if !inPod || len(pod.ContainerIDs) <= 1 {
		return false, nil
	}

	shouldDefer, allIDs := s.Store.Pods.MarkStarted(pod.ID, containerID)
	if shouldDefer {
		return true, nil
	}

	// Collect all container objects for materialization
	containers := make([]api.Container, 0, len(allIDs))
	for _, cid := range allIDs {
		if c, ok := s.Store.Containers.Get(cid); ok {
			containers = append(containers, c)
		}
	}
	return false, containers
}

// WaitForServiceHealth polls pod containers that have Healthcheck configs
// until they become "healthy" or the timeout expires. Containers without
// health checks are skipped. This is used by cloud backends after
// multi-container pod start, before signaling the job as ready.
func (s *BaseServer) WaitForServiceHealth(podID string, timeout time.Duration) error {
	pod, ok := s.Store.Pods.GetPod(podID)
	if !ok {
		return fmt.Errorf("pod %q not found", podID)
	}

	// Collect containers that have health checks
	var healthIDs []string
	for _, cid := range pod.ContainerIDs {
		c, ok := s.Store.Containers.Get(cid)
		if !ok {
			continue
		}
		if c.Config.Healthcheck != nil && len(c.Config.Healthcheck.Test) > 0 {
			// Skip NONE
			if len(c.Config.Healthcheck.Test) == 1 && c.Config.Healthcheck.Test[0] == "NONE" {
				continue
			}
			healthIDs = append(healthIDs, cid)
		}
	}

	if len(healthIDs) == 0 {
		return nil
	}

	deadline := time.After(timeout)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()

	for {
		allHealthy := true
		for _, cid := range healthIDs {
			c, ok := s.Store.Containers.Get(cid)
			if !ok {
				continue
			}
			if c.State.Health == nil || c.State.Health.Status != "healthy" {
				allHealthy = false
				break
			}
		}
		if allHealthy {
			return nil
		}

		select {
		case <-deadline:
			return fmt.Errorf("timeout waiting for service containers to become healthy")
		case <-tick.C:
		}
	}
}
