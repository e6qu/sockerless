package cloudrun

import (
	"fmt"
	"net/http"

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
		crState, _ := s.CloudRun.Get(id)
		if crState.ExecutionName != "" {
			s.cancelExecution(crState.ExecutionName)
		}
		s.Store.StopContainer(id, 0)
	}

	// Re-dispatch to start handler
	startURL := fmt.Sprintf("/internal/v1/containers/%s/start", id)
	startReq, _ := http.NewRequestWithContext(r.Context(), "POST", startURL, nil)
	startReq.SetPathValue("id", id)
	s.handleContainerStart(w, startReq)
}

// handleContainerPrune removes all stopped containers.
func (s *Server) handleContainerPrune(w http.ResponseWriter, r *http.Request) {
	var deleted []string
	for _, c := range s.Store.Containers.List() {
		if c.State.Status == "exited" || c.State.Status == "dead" {
			// Clean up Cloud Run resources
			crState, _ := s.CloudRun.Get(c.ID)
			if crState.JobName != "" {
				s.deleteJob(crState.JobName)
			}
			s.Store.Containers.Delete(c.ID)
			s.Store.ContainerNames.Delete(c.Name)
			s.CloudRun.Delete(c.ID)
			s.Store.WaitChs.Delete(c.ID)
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

// handleContainerPause is not supported by Cloud Run backend.
func (s *Server) handleContainerPause(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	if _, ok := s.Store.ResolveContainerID(ref); !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	core.WriteError(w, &api.NotImplementedError{Message: "container pause is not supported by Cloud Run backend"})
}

// handleContainerUnpause is not supported by Cloud Run backend.
func (s *Server) handleContainerUnpause(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	if _, ok := s.Store.ResolveContainerID(ref); !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	core.WriteError(w, &api.NotImplementedError{Message: "container unpause is not supported by Cloud Run backend"})
}

// handleVolumeRemove removes a volume and its state.
func (s *Server) handleVolumeRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !s.Store.Volumes.Delete(name) {
		core.WriteError(w, &api.NotFoundError{Resource: "volume", ID: name})
		return
	}
	s.VolumeState.Delete(name)
	w.WriteHeader(http.StatusNoContent)
}

// handleVolumePrune removes unused volumes.
func (s *Server) handleVolumePrune(w http.ResponseWriter, r *http.Request) {
	core.WriteJSON(w, http.StatusOK, api.VolumePruneResponse{
		VolumesDeleted: []string{},
		SpaceReclaimed: 0,
	})
}
