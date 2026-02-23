package frontend

import (
	"net/http"
	"net/url"
	"runtime"

	"github.com/sockerless/api"
)

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("API-Version", "1.44")
	w.Header().Set("Docker-Experimental", "false")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("OK"))
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	info, _ := s.backend.Info()
	version := "0.1.0"
	if info != nil && info.ServerVersion != "" {
		version = info.ServerVersion
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"Version":       version,
		"ApiVersion":    "1.44",
		"MinAPIVersion": "1.24",
		"GitCommit":     "sockerless",
		"GoVersion":     runtime.Version(),
		"Os":            runtime.GOOS,
		"Arch":          runtime.GOARCH,
		"KernelVersion": "",
		"BuildTime":     "2024-01-01T00:00:00.000000000+00:00",
		"Platform": map[string]string{
			"Name": "Sockerless",
		},
		"Components": []map[string]any{
			{
				"Name":    "Engine",
				"Version": version,
				"Details": map[string]string{
					"ApiVersion":    "1.44",
					"MinAPIVersion": "1.24",
					"GitCommit":     "sockerless",
					"GoVersion":     runtime.Version(),
					"Os":            runtime.GOOS,
					"Arch":          runtime.GOARCH,
					"BuildTime":     "2024-01-01T00:00:00.000000000+00:00",
				},
			},
		},
	})
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.backend.Info()
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ID":                info.ID,
		"Name":              info.Name,
		"ServerVersion":     info.ServerVersion,
		"Containers":        info.Containers,
		"ContainersRunning": info.ContainersRunning,
		"ContainersPaused":  info.ContainersPaused,
		"ContainersStopped": info.ContainersStopped,
		"Images":            info.Images,
		"Driver":            info.Driver,
		"OperatingSystem":   info.OperatingSystem,
		"OSType":            info.OSType,
		"Architecture":      info.Architecture,
		"NCPU":              info.NCPU,
		"MemTotal":          info.MemTotal,
		"KernelVersion":     info.KernelVersion,
		"DockerRootDir":     "/var/lib/sockerless",
		"HttpProxy":         "",
		"HttpsProxy":        "",
		"NoProxy":           "",
		"Labels":            []string{},
		"ExperimentalBuild": false,
		"LiveRestoreEnabled": false,
		"SecurityOptions":   []string{},
		"Warnings":          []string{},
		"RegistryConfig": map[string]any{
			"InsecureRegistryCIDRs": []string{},
			"IndexConfigs":          map[string]any{},
			"Mirrors":               []string{},
		},
		"Plugins": map[string]any{
			"Volume":        []string{"local"},
			"Network":       []string{"bridge", "host", "null"},
			"Authorization": nil,
			"Log":           []string{"json-file"},
		},
		"Runtimes": map[string]any{
			"runc": map[string]string{"path": "runc"},
		},
		"DefaultRuntime": "runc",
		"Swarm":          map[string]any{"LocalNodeState": "inactive"},
	})
}

func (s *Server) handleNotImplemented(w http.ResponseWriter, r *http.Request) {
	writeError(w, &api.NotImplementedError{
		Message: r.Method + " " + r.URL.Path + " is not supported",
	})
}

func (s *Server) handleNetworkConnect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req api.NetworkConnectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}
	resp, err := s.backend.post("/networks/"+id+"/connect", &req)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleVolumePrune(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	if filters := r.URL.Query().Get("filters"); filters != "" {
		query.Set("filters", filters)
	}
	resp, err := s.backend.postWithQuery("/volumes/prune", query, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleSystemEvents(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	if since := r.URL.Query().Get("since"); since != "" {
		query.Set("since", since)
	}
	if until := r.URL.Query().Get("until"); until != "" {
		query.Set("until", until)
	}
	if filters := r.URL.Query().Get("filters"); filters != "" {
		query.Set("filters", filters)
	}
	resp, err := s.backend.getWithQuery("/events", query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	flushingCopy(w, resp.Body)
}

func (s *Server) handleSystemDf(w http.ResponseWriter, r *http.Request) {
	resp, err := s.backend.get("/system/df")
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}
