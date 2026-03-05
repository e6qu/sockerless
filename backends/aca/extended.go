package aca

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

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
		acaState, _ := s.ACA.Get(id)
		if acaState.JobName != "" && acaState.ExecutionName != "" {
			s.stopExecution(acaState.JobName, acaState.ExecutionName)
		}
		s.Store.ForceStopContainer(id, 0)
		if acaState.JobName != "" {
			s.deleteJob(acaState.JobName)
			s.Registry.MarkCleanedUp(acaState.JobName)
		}
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		s.EmitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	}

	s.Store.Containers.Update(id, func(c *api.Container) {
		c.RestartCount++
	})

	// BUG-387: Use ResponseRecorder to capture start result, only emit restart on success
	rec := httptest.NewRecorder()
	startURL := fmt.Sprintf("/internal/v1/containers/%s/start", id)
	startReq, _ := http.NewRequestWithContext(r.Context(), "POST", startURL, nil)
	startReq.SetPathValue("id", id)
	s.handleContainerStart(rec, startReq)

	for k, vv := range rec.Header() {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(rec.Code)
	w.Write(rec.Body.Bytes())

	if rec.Code == http.StatusNoContent || rec.Code == http.StatusOK {
		s.EmitEvent("container", "restart", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	}
}

// handleContainerPrune removes all stopped containers.
func (s *Server) handleContainerPrune(w http.ResponseWriter, r *http.Request) {
	filters := core.ParseFilters(r.URL.Query().Get("filters"))
	labelFilters := filters["label"]
	untilFilters := filters["until"]
	var deleted []string
	var spaceReclaimed uint64
	for _, c := range s.Store.Containers.List() {
		if c.State.Status == "exited" || c.State.Status == "dead" {
			if len(labelFilters) > 0 && !core.MatchLabels(c.Config.Labels, labelFilters) {
				continue
			}
			if len(untilFilters) > 0 && !core.MatchUntil(c.Created, untilFilters) {
				continue
			}
			// BUG-480: Sum image sizes for SpaceReclaimed
			if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
				spaceReclaimed += uint64(img.Size)
			}
			// Clean up ACA resources
			acaState, _ := s.ACA.Get(c.ID)
			if acaState.JobName != "" {
				s.deleteJob(acaState.JobName)
				s.Registry.MarkCleanedUp(acaState.JobName)
			}
			s.StopHealthCheck(c.ID)
			s.AgentRegistry.Remove(c.ID)
			// Clean up network associations
			for _, ep := range c.NetworkSettings.Networks {
				if ep != nil && ep.NetworkID != "" {
					_ = s.Drivers.Network.Disconnect(r.Context(), ep.NetworkID, c.ID)
				}
			}
			if pod, inPod := s.Store.Pods.GetPodForContainer(c.ID); inPod {
				s.Store.Pods.RemoveContainer(pod.ID, c.ID)
			}
			s.Store.Containers.Delete(c.ID)
			s.Store.ContainerNames.Delete(c.Name)
			s.ACA.Delete(c.ID)
			if ch, ok := s.Store.WaitChs.LoadAndDelete(c.ID); ok {
				close(ch.(chan struct{}))
			}
			s.Store.LogBuffers.Delete(c.ID)
			s.Store.StagingDirs.Delete(c.ID)
			if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(c.ID); ok {
				for _, d := range dirs.([]string) {
					os.RemoveAll(d)
				}
			}
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
		SpaceReclaimed:    spaceReclaimed,
	})
}

// handleContainerPause is not supported by ACA backend.
func (s *Server) handleContainerPause(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	if _, ok := s.Store.ResolveContainerID(ref); !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	core.WriteError(w, &api.NotImplementedError{Message: "container pause is not supported by ACA backend"})
}

// handleContainerUnpause is not supported by ACA backend.
func (s *Server) handleContainerUnpause(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	if _, ok := s.Store.ResolveContainerID(ref); !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	core.WriteError(w, &api.NotImplementedError{Message: "container unpause is not supported by ACA backend"})
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
	var deleted []string
	var spaceReclaimed uint64
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
			// BUG-486: Sum volume dir sizes for SpaceReclaimed
			if dir, ok := s.Store.VolumeDirs.Load(v.Name); ok {
				spaceReclaimed += uint64(core.DirSize(dir.(string)))
			}
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
		SpaceReclaimed: spaceReclaimed,
	})
}
