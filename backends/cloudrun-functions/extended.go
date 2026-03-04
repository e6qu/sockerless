package gcf

import (
	"fmt"
	"net/http"
	"strings"

	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
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
	if c.State.Running {
		s.StopHealthCheck(id)
		s.AgentRegistry.Remove(id)
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

// handleContainerPrune removes all stopped containers and their GCF state.
func (s *Server) handleContainerPrune(w http.ResponseWriter, r *http.Request) {
	var deleted []string
	for _, c := range s.Store.Containers.List() {
		if c.State.Status == "exited" || c.State.Status == "dead" {
			// Clean up Cloud Run Functions cloud resources
			gcfState, _ := s.GCF.Get(c.ID)
			if gcfState.FunctionName != "" {
				fullName := fmt.Sprintf("projects/%s/locations/%s/functions/%s", s.config.Project, s.config.Region, gcfState.FunctionName)
				if op, err := s.gcp.Functions.DeleteFunction(s.ctx(), &functionspb.DeleteFunctionRequest{
					Name: fullName,
				}); err == nil {
					_ = op.Wait(s.ctx())
				}
				s.Registry.MarkCleanedUp(fullName)
			}

			s.Store.Containers.Delete(c.ID)
			s.Store.ContainerNames.Delete(c.Name)
			s.GCF.Delete(c.ID)
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
	core.WriteError(w, &api.NotImplementedError{Message: "Cloud Run Functions backend does not support pause"})
}

func (s *Server) handleContainerUnpause(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	core.WriteError(w, &api.NotImplementedError{Message: "Cloud Run Functions backend does not support unpause"})
}
