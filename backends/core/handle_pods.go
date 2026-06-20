package core

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/sockerless/api"
)

func (s *BaseServer) handlePodCreate(w http.ResponseWriter, r *http.Request) {
	var req api.PodCreateRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	resp, err := s.self.PodCreate(&req)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, resp)
}

func (s *BaseServer) handlePodList(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))
	result, err := s.self.PodList(api.PodListOptions{Filters: filters})
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

func matchPodFilters(pod *PodContext, filters map[string][]string) bool {
	for key, values := range filters {
		switch key {
		case "name":
			matched := false
			for _, v := range values {
				if strings.Contains(pod.Name, v) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "id":
			matched := false
			for _, v := range values {
				if strings.HasPrefix(pod.ID, v) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "label":
			if !MatchLabels(pod.Labels, values) {
				return false
			}
		case "status":
			matched := false
			for _, v := range values {
				if pod.Status == v {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}
	return true
}

func (s *BaseServer) handlePodInspect(w http.ResponseWriter, r *http.Request) {
	resp, err := s.self.PodInspect(r.PathValue("name"))
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *BaseServer) handlePodExists(w http.ResponseWriter, r *http.Request) {
	exists, err := s.self.PodExists(r.PathValue("name"))
	if err != nil {
		WriteError(w, err)
		return
	}
	if exists {
		w.WriteHeader(http.StatusNoContent)
	} else {
		WriteError(w, &api.NotFoundError{Resource: "pod", ID: r.PathValue("name")})
	}
}

func (s *BaseServer) handlePodStart(w http.ResponseWriter, r *http.Request) {
	resp, err := s.self.PodStart(r.PathValue("name"))
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *BaseServer) handlePodStop(w http.ResponseWriter, r *http.Request) {
	var timeout *int
	if t := r.URL.Query().Get("t"); t != "" {
		v, _ := strconv.Atoi(t)
		timeout = &v
	}
	resp, err := s.self.PodStop(r.PathValue("name"), timeout)
	if err != nil {
		WriteError(w, err)
		return
	}
	writePodActionResponse(w, resp)
}

func (s *BaseServer) handlePodKill(w http.ResponseWriter, r *http.Request) {
	signal := r.URL.Query().Get("signal")
	resp, err := s.self.PodKill(r.PathValue("name"), signal)
	if err != nil {
		WriteError(w, err)
		return
	}
	writePodActionResponse(w, resp)
}

// writePodActionResponse serializes a PodActionResponse in podman's
// PodStopReport / PodKillReport shape: `{ "Id": ..., "Errs": [...] }`
// returned with HTTP 200, regardless of whether any per-container action
// failed. Real podman reports per-container stop/kill failures through
// the report's `Errs` array with a 200 status — not an HTTP error — so
// the CLI can present the pod's outcome along with the partial errors.
// `Errs` is always a non-nil array (never null) so podman's bindings
// json.Unmarshal succeeds.
func writePodActionResponse(w http.ResponseWriter, resp *api.PodActionResponse) {
	if resp == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"Errs": []any{}, "Id": ""})
		return
	}
	errs := make([]string, 0, len(resp.Errs))
	errs = append(errs, resp.Errs...)
	WriteJSON(w, http.StatusOK, map[string]any{
		"Errs":     errs,
		"Id":       resp.ID,
		"RawInput": resp.RawInput,
	})
}

func (s *BaseServer) handlePodRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	force := r.URL.Query().Get("force") == "true" || r.URL.Query().Get("force") == "1"

	// Resolve pod ID before removal for the response
	pod, _ := s.self.PodInspect(name)
	if err := s.self.PodRemove(name, force); err != nil {
		WriteError(w, err)
		return
	}
	// Podman expects PodRmReport JSON, not 204 No Content
	podID := name
	if pod != nil {
		podID = pod.ID
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"Id":          podID,
		"Err":         nil,
		"RemovedCtrs": map[string]any{},
	})
}
