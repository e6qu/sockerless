package core

import (
	"encoding/json"
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
	var podMeta *PodContext
	if podRef != "" {
		pod, ok := s.Store.Pods.GetPod(podRef)
		if !ok {
			WriteError(w, &api.NotFoundError{Resource: "pod", ID: podRef})
			return
		}
		podMeta = pod
		// Tag the container's labels with sockerless-pod=<name> BEFORE
		// ContainerCreate so every backend (Docker included) preserves
		// pod membership on the underlying cloud resource, not just in
		// Store.Pods.
		if req.ContainerConfig == nil {
			req.ContainerConfig = &api.ContainerConfig{}
		}
		if req.Labels == nil {
			req.Labels = make(map[string]string)
		}
		req.Labels["sockerless-pod"] = pod.Name
	}

	resp, err := s.self.ContainerCreate(&req)
	if err != nil {
		WriteError(w, err)
		return
	}

	// Explicit pod association (validated above)
	if podMeta != nil {
		_ = s.Store.Pods.AddContainer(podMeta.ID, resp.ID)
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
		if id, ok := s.ResolveContainerIDAuto(r.Context(), ref); ok {
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
	ref := r.PathValue("id")
	c, _ := s.ResolveContainerAuto(r.Context(), ref)
	if c.ID == "" {
		c.ID = ref
	}
	dctx := DriverContext{
		Ctx:       r.Context(),
		Container: c,
		Backend:   s.Desc.Driver,
		Logger:    s.Logger,
	}
	if err := s.Typed.Signal.Kill(dctx, signal); err != nil {
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

	// Phase 122g BUG-936 fast-path: if WaitChs has a channel for this
	// ref AND InvocationResult is already recorded, return immediately
	// without ResolveContainerIDAuto + CloudState.GetContainer (which
	// for cloudrun lists ALL Cloud Run Services in the project — minutes
	// per call with N services). Most /wait callers send the canonical
	// 64-char hex ID; PendingCreates / WaitChs maps it directly.
	if ch, hasChannel := s.Store.WaitChs.Load(ref); hasChannel {
		if inv, ok := s.Store.GetInvocationResult(ref); ok {
			// Container already exited via invoke goroutine — no point
			// blocking. Drain channel so we drop the entry from WaitChs.
			select {
			case <-ch.(chan struct{}):
			default:
			}
			WriteJSON(w, http.StatusOK, api.ContainerWaitResponse{StatusCode: inv.ExitCode})
			return
		}
	}

	// Try local wait channel first (simulator mode runs containers locally)
	if s.CloudState != nil {
		id, ok := s.ResolveContainerIDAuto(r.Context(), ref)
		if !ok {
			WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
			return
		}

		// Check if already exited (cloud or local)
		c, found, _ := s.CloudState.GetContainer(r.Context(), id)
		if !found {
			if lc, lok := s.Store.Containers.Get(id); lok {
				c = lc
				found = true
			}
		}
		if found && (c.State.Status == "exited" || c.State.Status == "dead") {
			WriteJSON(w, http.StatusOK, api.ContainerWaitResponse{StatusCode: c.State.ExitCode})
			return
		}

		// Flush 200 OK headers to the client BEFORE any blocking path.
		// Docker CLI's docker-run-d flow sends POST /wait?condition=next-exit
		// *before* POST /start and blocks on reading the response
		// status line. Without this early flush, cli.post() never
		// returns, /start is never sent, and the container never runs.
		// The body is written after the exit event lands.
		flushWaitHeaders(w)

		// If there's a local wait channel, use it then query CloudState for exit code
		if ch, hasChannel := s.Store.WaitChs.Load(id); hasChannel {
			select {
			case <-ch.(chan struct{}):
				// Phase 122g BUG-934: prefer InvocationResult over
				// CloudState.GetContainer here. CloudState.GetContainer
				// for cloudrun's Service-path containers calls
				// queryServices which iterates ALL services in the
				// project + filters; with even ~50 stale services this
				// took ~15 minutes (cell 7 v20 evidence: /wait
				// duration=902811ms despite WaitCh closing in 2s).
				// The invoke goroutine that closed WaitCh just stored
				// InvocationResult — that IS the truth for FaaS / Service
				// path containers; reading cloud state again wastes the
				// fast-path the channel signal was meant to provide.
				if inv, ok := s.Store.GetInvocationResult(id); ok {
					writeWaitBody(w, inv.ExitCode)
				} else if cc, found, _ := s.CloudState.GetContainer(r.Context(), id); found {
					writeWaitBody(w, cc.State.ExitCode)
				} else if lc, lok := s.Store.Containers.Get(id); lok {
					writeWaitBody(w, lc.State.ExitCode)
				} else {
					writeWaitBody(w, 0)
				}
			case <-r.Context().Done():
				// Headers already sent; best we can do is send an
				// empty body and let the client's body read error.
				writeWaitBody(w, -1)
			}
			return
		}

		// Cloud-based wait: poll until stopped
		exitCode, err := s.CloudState.WaitForExit(r.Context(), id)
		if err != nil {
			writeWaitBody(w, -1)
			return
		}
		writeWaitBody(w, exitCode)
		return
	}

	id, ok := s.ResolveContainerIDAuto(r.Context(), ref)
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

	// Flush headers before blocking — see note above.
	flushWaitHeaders(w)

	ch, ok := s.Store.WaitChs.Load(id)
	if !ok {
		c, _ = s.Store.Containers.Get(id)
		writeWaitBody(w, c.State.ExitCode)
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
		writeWaitBody(w, c.State.ExitCode)
	case <-r.Context().Done():
		writeWaitBody(w, -1)
	}
}

// flushWaitHeaders sends 200 OK + JSON content-type headers to the
// client before the wait handler blocks on the container-exit event.
// docker CLI's ContainerWait sends the request via cli.post() and
// blocks on reading the response status line; without this early
// flush, /start never gets sent in the docker-run-d flow.
func flushWaitHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// writeWaitBody writes the ContainerWaitResponse JSON body. Called
// after flushWaitHeaders + exit event. Uses raw Encoder since headers
// are already committed.
func writeWaitBody(w http.ResponseWriter, exitCode int) {
	_ = json.NewEncoder(w).Encode(api.ContainerWaitResponse{StatusCode: exitCode})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
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
