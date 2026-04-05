package core

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// hasEnvKey checks whether an env slice already contains a variable with the given key.
func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}

// MergeEnvByKey merges env vars by key — base provides defaults, override replaces by key.
// Docker merges image and container env by key, not all-or-nothing.
func MergeEnvByKey(base, override []string) []string {
	if len(override) == 0 {
		return base
	}
	if len(base) == 0 {
		return override
	}
	keys := make(map[string]string)
	order := make([]string, 0, len(base)+len(override))
	for _, e := range base {
		k, _, _ := strings.Cut(e, "=")
		keys[k] = e
		order = append(order, k)
	}
	for _, e := range override {
		k, _, _ := strings.Cut(e, "=")
		if _, exists := keys[k]; !exists {
			order = append(order, k)
		}
		keys[k] = e
	}
	result := make([]string, 0, len(order))
	for _, k := range order {
		result = append(result, keys[k])
	}
	return result
}

// --- Default overridable container handlers (memory-like behavior) ---

func (s *BaseServer) handleContainerCreate(w http.ResponseWriter, r *http.Request) {
	var req api.ContainerCreateRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}
	req.Name = r.URL.Query().Get("name")

	// Pod association via query param (Podman convention: ?pod=<nameOrID>)
	podRef := r.URL.Query().Get("pod")
	if podRef != "" {
		if _, ok := s.Store.Pods.GetPod(podRef); !ok {
			WriteError(w, &api.NotFoundError{Resource: "pod", ID: podRef})
			return
		}
	}

	resp, err := s.self.ContainerCreate(&req)
	if err != nil {
		WriteError(w, err)
		return
	}

	// Explicit pod association (validated above)
	if podRef != "" {
		pod, _ := s.Store.Pods.GetPod(podRef)
		_ = s.Store.Pods.AddContainer(pod.ID, resp.ID)
	}

	WriteJSON(w, http.StatusCreated, resp)
}

func (s *BaseServer) handleContainerStart(w http.ResponseWriter, r *http.Request) {
	if err := s.self.ContainerStart(r.PathValue("id")); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerStop(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	var timeout *int
	if t := r.URL.Query().Get("t"); t != "" {
		v, _ := strconv.Atoi(t)
		timeout = &v
	}
	if err := s.self.ContainerStop(ref, timeout); err != nil {
		WriteError(w, err)
		return
	}
	// Adjust exit code for signal query param (Docker API v1.42+)
	if signal := r.URL.Query().Get("signal"); signal != "" {
		if id, ok := s.Store.ResolveContainerID(ref); ok {
			exitCode := SignalToExitCode(signal)
			s.Store.Containers.Update(id, func(c *api.Container) {
				c.State.ExitCode = exitCode
			})
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerKill(w http.ResponseWriter, r *http.Request) {
	signal := r.URL.Query().Get("signal")
	if err := s.self.ContainerKill(r.PathValue("id"), signal); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerRemove(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	if err := s.self.ContainerRemove(r.PathValue("id"), force); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerRestart(w http.ResponseWriter, r *http.Request) {
	var timeout *int
	if t := r.URL.Query().Get("t"); t != "" {
		v, _ := strconv.Atoi(t)
		timeout = &v
	}
	if err := s.self.ContainerRestart(r.PathValue("id"), timeout); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerWait(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")

	// Cloud-based wait: poll until stopped
	if s.CloudState != nil {
		id, ok := s.ResolveContainerIDAuto(r.Context(), ref)
		if !ok {
			WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
			return
		}
		// Check if already exited
		c, found, _ := s.CloudState.GetContainer(r.Context(), id)
		if found && (c.State.Status == "exited" || c.State.Status == "dead") {
			WriteJSON(w, http.StatusOK, api.ContainerWaitResponse{StatusCode: c.State.ExitCode})
			return
		}
		exitCode, err := s.CloudState.WaitForExit(r.Context(), id)
		if err != nil {
			WriteError(w, &api.ServerError{Message: err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, api.ContainerWaitResponse{StatusCode: exitCode})
		return
	}

	// Legacy Store-based wait
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	condition := r.URL.Query().Get("condition")
	if condition == "" {
		condition = "not-running"
	}

	c, exists := s.Store.Containers.Get(id)
	if !exists {
		if condition == "removed" {
			WriteJSON(w, http.StatusOK, api.ContainerWaitResponse{StatusCode: 0})
			return
		}
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	if condition != "next-exit" && (c.State.Status == "exited" || c.State.Status == "dead") {
		WriteJSON(w, http.StatusOK, api.ContainerWaitResponse{
			StatusCode: c.State.ExitCode,
		})
		return
	}

	ch, ok := s.Store.WaitChs.Load(id)
	if !ok {
		c, _ = s.Store.Containers.Get(id)
		WriteJSON(w, http.StatusOK, api.ContainerWaitResponse{
			StatusCode: c.State.ExitCode,
		})
		return
	}

	select {
	case <-ch.(chan struct{}):
		c, _ = s.Store.Containers.Get(id)
		// For "removed" condition, poll briefly for actual container deletion
		if condition == "removed" {
			for i := 0; i < 50; i++ {
				if _, exists := s.Store.Containers.Get(id); !exists {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
		WriteJSON(w, http.StatusOK, api.ContainerWaitResponse{
			StatusCode: c.State.ExitCode,
		})
	case <-r.Context().Done():
		return
	}
}

// SignalToExitCode maps a signal name or number to the corresponding
// exit code (128 + signal number), matching Docker's behavior.
func SignalToExitCode(signal string) int {
	signalMap := map[string]int{
		"SIGHUP": 129, "HUP": 129, "1": 129,
		"SIGINT": 130, "INT": 130, "2": 130,
		"SIGQUIT": 131, "QUIT": 131, "3": 131,
		"SIGABRT": 134, "ABRT": 134, "6": 134,
		"SIGKILL": 137, "KILL": 137, "9": 137,
		"SIGUSR1": 138, "USR1": 138, "10": 138,
		"SIGUSR2": 140, "USR2": 140, "12": 140,
		"SIGTERM": 143, "TERM": 143, "15": 143,
	}
	if code, ok := signalMap[signal]; ok {
		return code
	}
	if n, err := strconv.Atoi(signal); err == nil && n > 0 {
		return 128 + n
	}
	return 137 // default to SIGKILL
}
