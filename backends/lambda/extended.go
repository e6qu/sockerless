package lambda

import (
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// handleContainerRestart is not supported — Lambda functions run to completion.
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

func (s *Server) handleContainerPrune(w http.ResponseWriter, r *http.Request) {
	var deleted []string
	for _, c := range s.Store.Containers.List() {
		if c.State.Status == "exited" || c.State.Status == "dead" {
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

			s.Store.Containers.Delete(c.ID)
			s.Store.ContainerNames.Delete(c.Name)
			s.Lambda.Delete(c.ID)
			s.Store.WaitChs.Delete(c.ID)
			s.Store.LogBuffers.Delete(c.ID)
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
