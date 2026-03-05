package lambda

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
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
		s.EmitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	}

	s.Store.Containers.Update(id, func(c *api.Container) {
		c.RestartCount++
	})

	// Re-dispatch to start handler
	startURL := fmt.Sprintf("/internal/v1/containers/%s/start", id)
	startReq, _ := http.NewRequestWithContext(r.Context(), "POST", startURL, nil)
	startReq.SetPathValue("id", id)
	s.handleContainerStart(w, startReq)
}

func (s *Server) handleContainerPrune(w http.ResponseWriter, r *http.Request) {
	filters := core.ParseFilters(r.URL.Query().Get("filters"))
	labelFilters := filters["label"]
	untilFilters := filters["until"]
	var deleted []string
	for _, c := range s.Store.Containers.List() {
		if c.State.Status == "exited" || c.State.Status == "dead" {
			if len(labelFilters) > 0 && !core.MatchLabels(c.Config.Labels, labelFilters) {
				continue
			}
			if len(untilFilters) > 0 && !core.MatchUntil(c.Created, untilFilters) {
				continue
			}
			// Clean up Lambda cloud resources
			lambdaState, _ := s.Lambda.Get(c.ID)
			if lambdaState.FunctionName != "" {
				_, _ = s.aws.Lambda.DeleteFunction(s.ctx(), &awslambda.DeleteFunctionInput{
					FunctionName: aws.String(lambdaState.FunctionName),
				})
			}
			if lambdaState.FunctionARN != "" {
				s.Registry.MarkCleanedUp(lambdaState.FunctionARN)
			}

			s.AgentRegistry.Remove(c.ID)
			if pod, inPod := s.Store.Pods.GetPodForContainer(c.ID); inPod {
				s.Store.Pods.RemoveContainer(pod.ID, c.ID)
			}
			s.Store.Containers.Delete(c.ID)
			s.Store.ContainerNames.Delete(c.Name)
			s.Lambda.Delete(c.ID)
			if ch, ok := s.Store.WaitChs.LoadAndDelete(c.ID); ok {
				close(ch.(chan struct{}))
			}
			s.Store.LogBuffers.Delete(c.ID)
			s.Store.StagingDirs.Delete(c.ID)
			for _, eid := range c.ExecIDs {
				s.Store.Execs.Delete(eid)
			}
			s.EmitEvent("container", "destroy", c.ID, map[string]string{
				"name": strings.TrimPrefix(c.Name, "/"),
			})
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
	core.WriteError(w, &api.NotImplementedError{Message: "Lambda backend does not support pause"})
}

func (s *Server) handleContainerUnpause(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	core.WriteError(w, &api.NotImplementedError{Message: "Lambda backend does not support unpause"})
}
