package bleephub

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Server is the bleephub HTTP server implementing the GitHub Actions
// runner service API (GHES-style endpoints).
type Server struct {
	addr                   string
	mux                    *http.ServeMux
	logger                 zerolog.Logger
	store                  *Store
	graphqlSchema          graphql.Schema
	actionCache            *ActionCache
	artifactStore          *ArtifactStore
	metrics                *Metrics
	maxConcurrentWorkflows int
	scheduleFired          scheduleFiredKeys // cron-firing dedup (on: schedule)
	actionsEvents          actionsEventLoop  // checks/webhook fan-out for run+job transitions
	routePatterns          []string          // every pattern registered via route(), for fidelity enumeration
	externalURL            string            // BLEEPHUB_EXTERNAL_URL; when set, overrides request-Host URL derivation (job messages, action URLs) — the GHES "external URL" knob
	// responseObserver, when set before ListenAndServe, sees every
	// request/response pair in the handler chain. The test harness
	// assigns it (same package) to validate /api/v3 response shapes
	// against the vendored GitHub OpenAPI description; nil costs nothing.
	responseObserver func(req *http.Request, status int, header http.Header, body []byte)
}

// route registers a handler AND records its "METHOD /path" pattern so the
// registered surface can be enumerated and validated directly (e.g. against
// GitHub's API definition) rather than inferred by probing the catch-all
// fallback. The catch-all is intentionally NOT registered through here, so a
// route that should exist but doesn't is a visible gap in RegisteredRoutes(),
// never silently swallowed by the fallback.
func (s *Server) route(pattern string, handler http.HandlerFunc) {
	s.routePatterns = append(s.routePatterns, pattern)
	s.mux.HandleFunc(pattern, handler)
}

// NewServer creates a bleephub server with all routes registered.
//
// Honors the persistence-related env vars:
//   - BLEEPHUB_DATA_DIR     — directory for SQLite DB + artifact store.
//   - BLEEPHUB_PERSIST      — when "true", enables SQLite-backed state.
//   - BLEEPHUB_DATABASE_URL — PostgreSQL DSN (takes priority over SQLite).
//
// Operator-requested persistence that fails to open will log.Fatalf.
//
// When persistence is enabled, the full metadata surface persists: users,
// tokens, apps (incl. credentials + webhook config), OAuth apps,
// installations (incl. selected repos), installation / user-to-server /
// refresh tokens, repos, orgs, teams, memberships, issues, labels,
// milestones, comments, pull requests, PR reviews + review comments,
// hooks (incl. secrets) + deliveries, app hook deliveries, repo secrets
// (incl. values), check suites/runs/prefs, workflow files, releases,
// deployments + statuses + environments (incl. reviewers/wait timer),
// reactions, Projects v2, user SSH/GPG keys, Pages, branch protection,
// the audit log, and marketplace plans.
//
// Persistence requires durable git storage (BLEEPHUB_GIT_DIR or
// BLEEPHUB_S3_BUCKET): reloading repo metadata against in-memory git
// storage would resurrect every repo empty, so that combination is a
// startup error rather than a silent degraded mode.
//
// Intentionally NOT persisted: in-flight workflow/session/agent state
// (a restart abandons in-flight runs) and the OIDC signing key
// (gh_misc_endpoints.go oidcKey), which rotates on restart — consumers
// must re-fetch the JWKS, exactly as they must against real GitHub key
// rotation.
func NewServer(addr string, logger zerolog.Logger) *Server {
	maxWF := 10
	if v := os.Getenv("BLEEPHUB_MAX_WORKFLOWS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxWF = n
		}
	}
	dataDir := os.Getenv("BLEEPHUB_DATA_DIR")
	var artifactStore *ArtifactStore
	if dataDir != "" {
		artifactStore = NewArtifactStore(dataDir)
	} else {
		artifactStore = NewArtifactStore()
	}

	s := &Server{
		addr:                   addr,
		mux:                    http.NewServeMux(),
		logger:                 logger,
		store:                  NewStore(),
		actionCache:            NewActionCache(),
		artifactStore:          artifactStore,
		metrics:                NewMetrics(),
		maxConcurrentWorkflows: maxWF,
		externalURL:            strings.TrimRight(os.Getenv("BLEEPHUB_EXTERNAL_URL"), "/"),
	}

	// Wire persistence. PostgreSQL takes priority via BLEEPHUB_DATABASE_URL;
	// otherwise BLEEPHUB_PERSIST=true enables SQLite. Both fail-loud on open failure.
	persist := MustNewPersistence()
	if persist != nil {
		// Metadata persistence with in-memory git storage is a silent
		// degraded mode: every repo would reload with an empty git side.
		// Refuse to start rather than serve hollow repos.
		if GitDataDir() == "" && !IsS3GitStorage() {
			logger.Fatal().Msg("persistence is enabled (BLEEPHUB_PERSIST/BLEEPHUB_DATABASE_URL) but git storage is in-memory: " +
				"repo metadata would survive a restart while every git repo reloads empty. " +
				"Configure durable git storage (BLEEPHUB_GIT_DIR=<dir> or BLEEPHUB_S3_BUCKET=<bucket>) or disable persistence.")
		}
		if err := s.store.SetPersistence(persist); err != nil {
			logger.Fatal().Err(err).Msg("failed to load persisted state")
		}
		s.logger.Info().Str("dialect", persist.dialect.name).Str("data_dir", dataDir).Msg("bleephub persistence enabled")
	}

	// Seed default user only if the store didn't load one from disk.
	if s.store.LookupUserByLogin("admin") == nil {
		s.store.SeedDefaultUser()
	}
	// Seed pre-registered GitHub Apps from config (BLEEPHUB_SEED_APPS /
	// BLEEPHUB_SEED_APPS_FILE) so a coordinate-only consumer can hold a fixed
	// app id + private key + org, exactly as it would against real GitHub.
	if err := s.seedConfiguredApps(); err != nil {
		logger.Fatal().Err(err).Msg("failed to seed configured GitHub Apps")
	}
	s.initGraphQLSchema()
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// Health check
	s.route("GET /health", s.handleHealth)

	// Auth + connection data (auth.go)
	s.registerAuthRoutes()

	// Agent management (agents.go)
	s.registerAgentRoutes()

	// Broker: sessions + message poll (broker.go)
	s.registerBrokerRoutes()

	// Job submission (jobs.go)
	s.registerJobRoutes()

	// Action resolution + tarball proxy (actions.go)
	s.registerActionRoutes()

	// Artifact + cache stubs (artifacts.go)
	s.registerArtifactRoutes()

	// Run service: acquire/renew/complete (run_service.go)
	s.registerRunServiceRoutes()

	// Timeline + logs (timeline.go)
	s.registerTimelineRoutes()

	// Secrets API (secrets.go)
	s.registerSecretsRoutes()

	// Webhooks API (gh_hooks_rest.go)
	s.registerGHHookRoutes()

	// GitHub Apps API (gh_apps_rest.go)
	s.registerGHAppsRoutes()

	// GitHub Apps webhook config + deliveries (gh_app_hooks_rest.go)
	s.registerGHAppHookRoutes()

	// /applications/{client_id}/* (gh_apps_oauth_mgmt.go) + OAuth Apps mgmt
	s.registerGHAppsOAuthMgmtRoutes()

	// Checks API (gh_checks_rest.go)
	s.registerGHChecksRoutes()

	// Reactions API (gh_reactions.go)
	s.registerGHReactionsRoutes()

	// Releases API (gh_releases.go)
	s.registerGHReleasesRoutes()

	// Actions extras (gh_actions_extras.go) — repository_dispatch, logs, timing
	s.registerGHActionsExtrasRoutes()

	// Deployments + Environments (gh_deployments.go)
	s.registerGHDeploymentsRoutes()

	// PR review comments (gh_pr_comments.go) — inline / file-line / threads
	s.registerGHPRCommentsRoutes()

	// PR thread resolve/unresolve (gh_pr_threads.go)
	s.registerGHPRThreadsRoutes()

	// Long-tail surfaces (gh_misc_endpoints.go) — Users keys/follow, OIDC,
	// Pages, branch protection, org members, marketplace.
	s.registerGHMiscEndpoints()
	s.seedDefaultMarketplacePlans()

	// GitHub API: REST, GraphQL, OAuth (gh_*.go)
	s.registerGHRestRoutes()
	s.registerGHRepoRoutes()
	s.registerGHOrgRoutes()
	s.registerGHIssueRoutes()
	s.registerGHPullRoutes()
	s.registerGHOAuthRoutes()
	s.registerGHGraphQLRoutes()
	s.registerGHActionsRoutes()
	s.registerGHWorkflowsRoutes()

	// Org runner groups (gh_runner_groups.go)
	s.registerRunnerGroupRoutes()

	// Management API (metrics, status, dashboard data)
	s.route("GET /internal/metrics", s.handleInternalMetrics)
	s.route("GET /internal/status", s.handleInternalStatus)
	s.registerMgmtRoutes()

	s.route("GET /internal/storage", s.handleInternalStorage)

	// UI dashboard
	s.registerUI()
	s.route("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
	})

	// Catch-all: tries smart HTTP git protocol, then logs unmatched
	s.mux.HandleFunc("/", s.handleCatchAll)
}

func (s *Server) handleCatchAll(w http.ResponseWriter, r *http.Request) {
	// Try smart HTTP git protocol
	if s.tryHandleGitRequest(w, r) {
		return
	}

	s.logger.Warn().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Str("query", r.URL.RawQuery).
		Msg("UNHANDLED REQUEST")
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "bleephub"})
}

func (s *Server) handleInternalMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.metrics.Snapshot())
}

func (s *Server) handleInternalStatus(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	activeWfs := 0
	jobsByStatus := make(map[string]int)
	for _, wf := range s.store.Workflows {
		if wf.Status == "running" {
			activeWfs++
		}
		for _, j := range wf.Jobs {
			jobsByStatus[string(j.Status)]++
		}
	}
	sessions := len(s.store.Sessions)
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"active_workflows":  activeWfs,
		"jobs_by_status":    jobsByStatus,
		"connected_runners": sessions,
		"uptime_seconds":    int(time.Since(s.metrics.StartedAt).Seconds()),
	})
}

func (s *Server) handleInternalStorage(w http.ResponseWriter, r *http.Request) {
	persistenceBackend := "none"
	dialectName := ""
	if s.store.persist != nil {
		persistenceBackend = s.store.persist.dialect.name
		dialectName = s.store.persist.dialect.name
	}

	gitBackend := "memory"
	gitDetails := map[string]string{}
	gitDir := GitDataDir()
	if IsS3GitStorage() {
		gitBackend = "s3"
		if bucket := os.Getenv("BLEEPHUB_S3_BUCKET"); bucket != "" {
			gitDetails["bucket"] = bucket
		}
		if endpoint := os.Getenv("BLEEPHUB_S3_ENDPOINT"); endpoint != "" {
			gitDetails["endpoint"] = endpoint
		}
		if prefix := os.Getenv("BLEEPHUB_S3_PREFIX"); prefix != "" {
			gitDetails["prefix"] = prefix
		}
	} else if gitDir != "" {
		gitBackend = "filesystem"
		gitDetails["dir"] = gitDir
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"persistence": persistenceBackend,
		"dialect":     dialectName,
		"git":         gitBackend,
		"git_details": gitDetails,
	})
}

// ListenAndServe starts the HTTP server (crash-only, no graceful shutdown).
func (s *Server) ListenAndServe() error {
	s.startScheduleDispatcher()
	inner := s.prefixStripMiddleware(s.internalAuthMiddleware(s.mux))
	ghWrapped := s.ghHeadersMiddleware(inner)
	observed := ghWrapped
	if s.responseObserver != nil {
		observed = s.observeMiddleware(ghWrapped)
	}
	handler := otelhttp.NewHandler(s.loggingMiddleware(observed), "bleephub")

	srv := &http.Server{
		Addr:    s.addr,
		Handler: handler,
		// Bound only the header read (slowloris protection). A fixed
		// ReadTimeout/WriteTimeout caps the WHOLE body, which cuts off large
		// git push/pull + artifact uploads/downloads under load.
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Resolve addr for log output
	host, port, _ := net.SplitHostPort(s.addr)
	if host == "" {
		host = "localhost"
	}

	// TLS support via environment variables
	certFile := os.Getenv("BPH_TLS_CERT")
	keyFile := os.Getenv("BPH_TLS_KEY")
	if certFile != "" && keyFile != "" {
		s.logger.Info().Msgf("bleephub listening on https://%s:%s", host, port)
		return srv.ListenAndServeTLS(certFile, keyFile)
	}

	s.logger.Info().Msgf("bleephub listening on http://%s:%s", host, port)
	return srv.ListenAndServe()
}

// prefixStripMiddleware removes any path segments before known API prefixes.
// The runner prepends the tenant URL path to all API calls, e.g.
// /owner/repo/_apis/... instead of /_apis/...
// internalAuthMiddleware gates the operator-facing /internal/* surface — the
// sim-control + dashboard endpoints that have no GitHub API equivalent
// (job/workflow submission + status under /internal/exec, app/oauth-app
// management, and the dashboard aggregations). These are NOT part of the
// GitHub-compatible /api/ surface, so they live here rather than under
// /api/v3/. They require a valid token (the UI sends the admin token as a
// Bearer credential); the resolved user is injected into the request context
// so management handlers can attribute ownership via ghUserFromContext.
// /health stays open for liveness probes.
func (s *Server) internalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/internal/") {
			next.ServeHTTP(w, r)
			return
		}
		user := s.internalTokenUser(r)
		if user == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Requires authentication"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUser, user)))
	})
}

// internalTokenUser resolves the user for a token recognized on the internal
// surface: any PAT in the store (which includes the seeded admin token).
// Returns nil when absent/unknown. ghs_/gho_/ghu_ installation/OAuth tokens
// are intentionally not accepted here — the internal surface is operator-only.
func (s *Server) internalTokenUser(r *http.Request) *User {
	scheme, cred := authScheme(r.Header.Get("Authorization"))
	var tok string
	if scheme == "bearer" || scheme == "token" {
		tok = cred
	}
	if tok == "" {
		return nil
	}
	t, user := s.store.LookupToken(tok)
	if t == nil {
		return nil
	}
	return user
}

func (s *Server) prefixStripMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Strip everything before /_apis/ or /api/
		for _, prefix := range []string{"/_apis/", "/api/"} {
			if idx := strings.Index(path, prefix); idx > 0 {
				r.URL.Path = path[idx:]
				r.URL.RawPath = ""
				break
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		s.logger.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", rw.status).
			Dur("dur", time.Since(start)).
			Msg("request")
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// writeJSON marshals v as JSON and writes it to w.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
