package core

import (
	"encoding/json"
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

// writePodActionResponse serializes a PodActionResponse using the
// podman-compatible convention: success path emits
// `Errs: []` (the only `[]error` shape that survives podman's bindings
// json.Unmarshal); per-container stop/kill failures are surfaced via
// HTTP 409 + an ErrorModel-shaped body so the CLI prints them and
// exits non-zero. Sockerless callers that want the per-error detail
// can read the response body verbatim.
func writePodActionResponse(w http.ResponseWriter, resp *api.PodActionResponse) {
	if resp == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"Errs": []any{}, "Id": ""})
		return
	}
	if len(resp.Errs) > 0 {
		// Match podman's ErrorModel shape — `cause` + `message` + `response`.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cause":    "pod-action errors",
			"message":  strings.Join(resp.Errs, "; "),
			"response": http.StatusConflict,
		})
		return
	}
	// Success — always emit Errs: [] (never null, never populated).
	WriteJSON(w, http.StatusOK, map[string]any{
		"Errs":     []any{},
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
