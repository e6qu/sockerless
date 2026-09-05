package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/sockerless/api"
)

// Pod lifecycle on a cloud backend is a loop over the pod's members that
// calls the backend's own cloud-aware container operations. The pod
// registry is the only local state: membership and status. The per-member
// operation is passed in, so each backend's method stays the explicit
// implementation the api.Backend contract requires while the loop is
// written once.

// CloudPodStart starts every member that is not already running. The pod
// is marked running only when every member started.
func (s *BaseServer) CloudPodStart(name string, start func(containerID string) error) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}
	errs := []string{}
	for _, cid := range pod.ContainerIDs {
		if c, ok := s.PendingCreates.Get(cid); ok {
			if c.State.Running {
				continue
			}
		} else if c, ok := s.ResolveContainerAuto(context.Background(), cid); ok && c.State.Running {
			continue
		}
		if err := start(cid); err != nil {
			errs = append(errs, fmt.Sprintf("container %s: %v", shortID(cid), err))
		}
	}
	if len(errs) == 0 {
		s.Store.Pods.SetStatus(pod.ID, "running")
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// CloudPodStop stops every running member. A member that was already
// stopped (NotModifiedError) is not an error.
func (s *BaseServer) CloudPodStop(name string, timeout *int, stop func(containerID string, timeout *int) error) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}
	errs := []string{}
	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(context.Background(), cid)
		if !ok || !c.State.Running {
			continue
		}
		if err := stop(cid, timeout); err != nil {
			var notModified *api.NotModifiedError
			if !errors.As(err, &notModified) {
				errs = append(errs, fmt.Sprintf("container %s: %v", shortID(cid), err))
			}
		}
	}
	s.Store.Pods.SetStatus(pod.ID, "stopped")
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// CloudPodKill signals every running member. An empty signal means SIGKILL.
func (s *BaseServer) CloudPodKill(name, signal string, kill func(containerID, signal string) error) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}
	if signal == "" {
		signal = "SIGKILL"
	}
	errs := []string{}
	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(context.Background(), cid)
		if !ok || !c.State.Running {
			continue
		}
		if err := kill(cid, signal); err != nil {
			errs = append(errs, fmt.Sprintf("container %s: %v", shortID(cid), err))
		}
	}
	s.Store.Pods.SetStatus(pod.ID, "exited")
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// CloudPodRemove removes every member and then the pod. Without force a
// pod with a running member is refused. Every member is attempted even
// when one fails, and the joined error is returned, because a member
// whose cloud resource survived must not read as a removed pod.
func (s *BaseServer) CloudPodRemove(name string, force bool, remove func(containerID string, force bool) error) error {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return &api.NotFoundError{Resource: "pod", ID: name}
	}
	if !force {
		for _, cid := range pod.ContainerIDs {
			if c, ok := s.ResolveContainerAuto(context.Background(), cid); ok && c.State.Running {
				return &api.ConflictError{Message: fmt.Sprintf("pod %s has running containers, cannot remove without force", name)}
			}
		}
	}
	var errs []error
	for _, cid := range pod.ContainerIDs {
		if _, ok := s.ResolveContainerAuto(context.Background(), cid); !ok {
			continue
		}
		if err := remove(cid, force); err != nil {
			errs = append(errs, fmt.Errorf("remove pod member %s: %w", cid, err))
		}
	}
	s.Store.Pods.DeletePod(pod.ID)
	return errors.Join(errs...)
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
