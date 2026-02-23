package frontend

import (
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/rs/zerolog"
)

var versionPrefix = regexp.MustCompile(`^/v\d+\.\d+/`)

// Server is the Docker REST API frontend server.
type Server struct {
	logger  zerolog.Logger
	backend *BackendClient
	mux     *http.ServeMux
	Mgmt    *MgmtServer
}

// NewServer creates a new Docker frontend server.
func NewServer(logger zerolog.Logger, backendAddr string) *Server {
	s := &Server{
		logger:  logger,
		backend: NewBackendClient(backendAddr),
		mux:     http.NewServeMux(),
	}

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// System
	s.mux.HandleFunc("GET /_ping", s.handlePing)
	s.mux.HandleFunc("HEAD /_ping", s.handlePing)
	s.mux.HandleFunc("GET /version", s.handleVersion)
	s.mux.HandleFunc("GET /info", s.handleInfo)

	// Containers
	s.mux.HandleFunc("POST /containers/create", s.handleContainerCreate)
	s.mux.HandleFunc("GET /containers/json", s.handleContainerList)
	s.mux.HandleFunc("GET /containers/{id}/json", s.handleContainerInspect)
	s.mux.HandleFunc("POST /containers/{id}/start", s.handleContainerStart)
	s.mux.HandleFunc("POST /containers/{id}/stop", s.handleContainerStop)
	s.mux.HandleFunc("POST /containers/{id}/restart", s.handleContainerRestart)
	s.mux.HandleFunc("POST /containers/{id}/kill", s.handleContainerKill)
	s.mux.HandleFunc("DELETE /containers/{id}", s.handleContainerRemove)
	s.mux.HandleFunc("GET /containers/{id}/logs", s.handleContainerLogs)
	s.mux.HandleFunc("POST /containers/{id}/wait", s.handleContainerWait)
	s.mux.HandleFunc("POST /containers/{id}/attach", s.handleContainerAttach)
	s.mux.HandleFunc("POST /containers/{id}/resize", s.handleContainerResize)
	s.mux.HandleFunc("GET /containers/{id}/top", s.handleContainerTop)
	s.mux.HandleFunc("GET /containers/{id}/stats", s.handleContainerStats)
	s.mux.HandleFunc("POST /containers/{id}/rename", s.handleContainerRename)
	s.mux.HandleFunc("POST /containers/{id}/pause", s.handleContainerPause)
	s.mux.HandleFunc("POST /containers/{id}/unpause", s.handleContainerUnpause)
	s.mux.HandleFunc("POST /containers/prune", s.handleContainerPrune)
	s.mux.HandleFunc("PUT /containers/{id}/archive", s.handleContainerPutArchive)
	s.mux.HandleFunc("HEAD /containers/{id}/archive", s.handleContainerHeadArchive)
	s.mux.HandleFunc("GET /containers/{id}/archive", s.handleContainerGetArchive)
	s.mux.HandleFunc("GET /containers/{id}/changes", s.handleContainerChanges)
	s.mux.HandleFunc("GET /containers/{id}/export", s.handleContainerExport)
	s.mux.HandleFunc("POST /containers/{id}/update", s.handleContainerUpdate)

	// Exec
	s.mux.HandleFunc("POST /containers/{id}/exec", s.handleExecCreate)
	s.mux.HandleFunc("GET /exec/{id}/json", s.handleExecInspect)
	s.mux.HandleFunc("POST /exec/{id}/start", s.handleExecStart)
	s.mux.HandleFunc("POST /exec/{id}/resize", s.handleExecResize)

	// Images — specific routes first, catch-all last
	s.mux.HandleFunc("POST /images/create", s.handleImageCreate)
	s.mux.HandleFunc("GET /images/json", s.handleImageList)
	s.mux.HandleFunc("POST /images/load", s.handleImageLoad)
	s.mux.HandleFunc("GET /images/search", s.handleNotImplemented)
	s.mux.HandleFunc("POST /images/prune", s.handleImagePrune)
	// Catch-all for /images/{name}/json, /images/{name}/tag, etc.
	s.mux.HandleFunc("GET /images/", s.handleImageCatchAll)
	s.mux.HandleFunc("POST /images/", s.handleImageCatchAll)
	s.mux.HandleFunc("DELETE /images/", s.handleImageCatchAll)

	// Auth
	s.mux.HandleFunc("POST /auth", s.handleAuth)

	// Networks
	s.mux.HandleFunc("POST /networks/create", s.handleNetworkCreate)
	s.mux.HandleFunc("GET /networks", s.handleNetworkList)
	s.mux.HandleFunc("GET /networks/{id}", s.handleNetworkInspect)
	s.mux.HandleFunc("POST /networks/{id}/connect", s.handleNetworkConnect)
	s.mux.HandleFunc("POST /networks/{id}/disconnect", s.handleNetworkDisconnect)
	s.mux.HandleFunc("DELETE /networks/{id}", s.handleNetworkRemove)
	s.mux.HandleFunc("POST /networks/prune", s.handleNetworkPrune)

	// Volumes
	s.mux.HandleFunc("POST /volumes/create", s.handleVolumeCreate)
	s.mux.HandleFunc("GET /volumes", s.handleVolumeList)
	s.mux.HandleFunc("GET /volumes/{name}", s.handleVolumeInspect)
	s.mux.HandleFunc("DELETE /volumes/{name}", s.handleVolumeRemove)
	s.mux.HandleFunc("POST /volumes/prune", s.handleVolumePrune)

	// System events and disk usage
	s.mux.HandleFunc("GET /events", s.handleSystemEvents)
	s.mux.HandleFunc("GET /system/df", s.handleSystemDf)

	// Podman Libpod pod API (Sockerless extension)
	s.mux.HandleFunc("POST /libpod/pods/create", s.handlePodCreate)
	s.mux.HandleFunc("GET /libpod/pods/json", s.handlePodList)
	s.mux.HandleFunc("GET /libpod/pods/{name}/json", s.handlePodInspect)
	s.mux.HandleFunc("GET /libpod/pods/{name}/exists", s.handlePodExists)
	s.mux.HandleFunc("POST /libpod/pods/{name}/start", s.handlePodStart)
	s.mux.HandleFunc("POST /libpod/pods/{name}/stop", s.handlePodStop)
	s.mux.HandleFunc("POST /libpod/pods/{name}/kill", s.handlePodKill)
	s.mux.HandleFunc("DELETE /libpod/pods/{name}", s.handlePodRemove)

	// Unsupported endpoints (501)
	s.mux.HandleFunc("POST /build", s.handleImageBuild)
	s.mux.HandleFunc("POST /commit", s.handleContainerCommit)
	s.mux.HandleFunc("POST /swarm/", s.handleNotImplemented)
	s.mux.HandleFunc("GET /swarm", s.handleNotImplemented)
	s.mux.HandleFunc("GET /nodes", s.handleNotImplemented)
	s.mux.HandleFunc("GET /services", s.handleNotImplemented)
	s.mux.HandleFunc("GET /tasks", s.handleNotImplemented)
	s.mux.HandleFunc("GET /secrets", s.handleNotImplemented)
	s.mux.HandleFunc("GET /configs", s.handleNotImplemented)
	s.mux.HandleFunc("GET /plugins", s.handleNotImplemented)
	s.mux.HandleFunc("POST /session", s.handleNotImplemented)
	s.mux.HandleFunc("GET /distribution/", s.handleNotImplemented)
}

// stripVersionPrefix is middleware that removes /v1.XX/ prefix from request paths.
func stripVersionPrefix(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if loc := versionPrefix.FindStringIndex(r.URL.Path); loc != nil {
			r.URL.Path = r.URL.Path[loc[1]-1:] // keep the leading /
			if r.URL.RawPath != "" {
				if loc := versionPrefix.FindStringIndex(r.URL.RawPath); loc != nil {
					r.URL.RawPath = r.URL.RawPath[loc[1]-1:]
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requestLogger is middleware that logs every incoming request.
func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.logger.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("query", r.URL.RawQuery).
			Msg("docker-api")
		if s.Mgmt != nil {
			s.Mgmt.IncrementRequests()
		}
		next.ServeHTTP(w, r)
	})
}

// ListenAndServe starts the server on the given address.
// If addr starts with /, it listens on a Unix socket (TLS is ignored).
// If certFile and keyFile are both non-empty, the TCP listener uses TLS.
func (s *Server) ListenAndServe(addr, certFile, keyFile string) error {
	handler := s.requestLogger(stripVersionPrefix(s.mux))

	if strings.HasPrefix(addr, "/") {
		os.Remove(addr)
		listener, err := net.Listen("unix", addr)
		if err != nil {
			return err
		}
		defer func() { _ = listener.Close() }()
		srv := &http.Server{Handler: handler}
		return srv.Serve(listener)
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return err
		}
		srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
		return srv.ListenAndServeTLS("", "")
	}
	return srv.ListenAndServe()
}
