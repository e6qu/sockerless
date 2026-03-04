package ecs

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// handleContainerRestart stops and then starts a container.
func (s *Server) handleContainerRestart(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)

	// Stop if running
	if c.State.Running {
		s.StopHealthCheck(id)
		s.AgentRegistry.Remove(id)
		ecsState, _ := s.ECS.Get(id)
		if ecsState.TaskARN != "" {
			_, _ = s.aws.ECS.StopTask(s.ctx(), &awsecs.StopTaskInput{
				Cluster: aws.String(s.config.Cluster),
				Task:    aws.String(ecsState.TaskARN),
				Reason:  aws.String("Container restarted via API"),
			})
			s.Registry.MarkCleanedUp(ecsState.TaskARN)
		}
		s.Store.ForceStopContainer(id, 0)
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
	}

	// Re-dispatch to start handler
	startURL := fmt.Sprintf("/internal/v1/containers/%s/start", id)
	startReq, _ := http.NewRequestWithContext(r.Context(), "POST", startURL, nil)
	startReq.SetPathValue("id", id)
	s.handleContainerStart(w, startReq)
}

func (s *Server) handleContainerPrune(w http.ResponseWriter, r *http.Request) {
	var deleted []string
	for _, c := range s.Store.Containers.List() {
		if c.State.Status == "exited" || c.State.Status == "dead" {
			// Clean up ECS cloud resources
			ecsState, _ := s.ECS.Get(c.ID)
			if ecsState.TaskDefARN != "" {
				_, _ = s.aws.ECS.DeregisterTaskDefinition(s.ctx(), &awsecs.DeregisterTaskDefinitionInput{
					TaskDefinition: aws.String(ecsState.TaskDefARN),
				})
			}
			if ecsState.TaskARN != "" {
				s.Registry.MarkCleanedUp(ecsState.TaskARN)
			}

			s.AgentRegistry.Remove(c.ID)
			s.Store.Containers.Delete(c.ID)
			s.Store.ContainerNames.Delete(c.Name)
			s.ECS.Delete(c.ID)
			s.Store.WaitChs.Delete(c.ID)
			s.Store.LogBuffers.Delete(c.ID)
			s.Store.StagingDirs.Delete(c.ID)
			for _, eid := range c.ExecIDs {
				s.Store.Execs.Delete(eid)
			}
			deleted = append(deleted, c.ID)
		}
	}
	if deleted == nil {
		deleted = []string{}
	}
	core.WriteJSON(w, http.StatusOK, api.ContainerPruneResponse{
		ContainersDeleted: deleted,
		SpaceReclaimed:    0,
	})
}

func (s *Server) handleContainerPause(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	core.WriteError(w, &api.NotImplementedError{Message: "ECS backend does not support pause"})
}

func (s *Server) handleContainerUnpause(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	core.WriteError(w, &api.NotImplementedError{Message: "ECS backend does not support unpause"})
}

func (s *Server) handleVolumeRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.Store.Volumes.Delete(name) {
		core.WriteError(w, &api.NotFoundError{Resource: "volume", ID: name})
		return
	}
	s.VolumeState.Delete(name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleVolumePrune(w http.ResponseWriter, r *http.Request) {
	var deleted []string
	for _, v := range s.Store.Volumes.List() {
		inUse := false
		for _, c := range s.Store.Containers.List() {
			for _, m := range c.Mounts {
				if m.Name == v.Name {
					inUse = true
					break
				}
			}
			if inUse {
				break
			}
		}
		if !inUse {
			s.Store.Volumes.Delete(v.Name)
			s.VolumeState.Delete(v.Name)
			deleted = append(deleted, v.Name)
		}
	}
	if deleted == nil {
		deleted = []string{}
	}
	core.WriteJSON(w, http.StatusOK, api.VolumePruneResponse{
		VolumesDeleted: deleted,
		SpaceReclaimed: 0,
	})
}
