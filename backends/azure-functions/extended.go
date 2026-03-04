package azf

import (
	"fmt"
	"net/http"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// handleContainerRestart is not meaningful for Azure Functions (run-to-completion).
func (s *Server) handleContainerRestart(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		core.WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("Container %s is not running", ref),
		})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleContainerPrune removes all stopped containers and their AZF state.
func (s *Server) handleContainerPrune(w http.ResponseWriter, r *http.Request) {
	var deleted []string
	for _, c := range s.Store.Containers.List() {
		if c.State.Status == "exited" || c.State.Status == "dead" {
			// Clean up Azure Functions cloud resources
			azfState, _ := s.AZF.Get(c.ID)
			if azfState.FunctionAppName != "" {
				_, _ = s.azure.WebApps.Delete(s.ctx(), s.config.ResourceGroup, azfState.FunctionAppName, nil)
			}
			if azfState.ResourceID != "" {
				s.Registry.MarkCleanedUp(azfState.ResourceID)
			}

			s.Store.Containers.Delete(c.ID)
			s.Store.ContainerNames.Delete(c.Name)
			s.AZF.Delete(c.ID)
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
	core.WriteError(w, &api.NotImplementedError{Message: "Azure Functions backend does not support pause"})
}

func (s *Server) handleContainerUnpause(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	core.WriteError(w, &api.NotImplementedError{Message: "Azure Functions backend does not support unpause"})
}
