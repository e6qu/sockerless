package bleephub

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// API insights (GET /orgs/{org}/insights/api/*) aggregate real recorded REST
// API traffic. Every matched /api/v3 route is instrumented at registration
// time (see instrumentAPIRoute, wired in Server.route): after the handler
// runs, the request is attributed to its authenticated actor and to every
// organization that actor belongs to at request time, then appended to
// Store.APIRequestRecords and persisted. The insights endpoints purely
// aggregate those records — when no attributable traffic exists the stats
// are honestly zero/empty.

// APIRequestRecord is one observed, attributed /api/v3 request.
type APIRequestRecord struct {
	ID          int64     `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Method      string    `json:"method"`
	Route       string    `json:"route"` // route template relative to /api/v3, e.g. "/repos/{owner}/{repo}"
	StatusCode  int       `json:"status_code"`
	RateLimited bool      `json:"rate_limited"`
	// Actor identifies the credential that made the request, using the
	// actor taxonomy of GitHub's API insights.
	ActorType string `json:"actor_type"` // installation | classic_pat | fine_grained_pat | oauth_app | github_app_user_to_server
	ActorID   int64  `json:"actor_id"`
	ActorName string `json:"actor_name"`
	// Subject is the account on whose behalf the request ran.
	SubjectType string `json:"subject_type"` // "user" | "installation"
	SubjectID   int64  `json:"subject_id"`
	SubjectName string `json:"subject_name"`
	// UserID is the authenticated user's ID (0 for installation tokens).
	UserID int `json:"user_id,omitempty"`
	// IntegrationID / OAuthAppID carry the GitHub App / OAuth app identity
	// behind app-derived actors, when one exists.
	IntegrationID *int64 `json:"integration_id,omitempty"`
	OAuthAppID    *int64 `json:"oauth_application_id,omitempty"`
	// OrgLogins are the organizations this request was attributed to at
	// request time (the actor's active memberships, or the installation's
	// target organization).
	OrgLogins []string `json:"org_logins,omitempty"`
}

var apiInsightsActorTypes = map[string]bool{
	"installation":              true,
	"classic_pat":               true,
	"fine_grained_pat":          true,
	"oauth_app":                 true,
	"github_app_user_to_server": true,
}

func (s *Server) registerGHAPIInsightsRoutes() {
	s.route("GET /api/v3/orgs/{org}/insights/api/route-stats/{actor_type}/{actor_id}", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleAPIInsightsRouteStats))
	s.route("GET /api/v3/orgs/{org}/insights/api/subject-stats", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleAPIInsightsSubjectStats))
	s.route("GET /api/v3/orgs/{org}/insights/api/summary-stats", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleAPIInsightsSummaryStats))
	s.route("GET /api/v3/orgs/{org}/insights/api/summary-stats/users/{user_id}", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleAPIInsightsSummaryStatsByUser))
	s.route("GET /api/v3/orgs/{org}/insights/api/summary-stats/{actor_type}/{actor_id}", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleAPIInsightsSummaryStatsByActor))
	s.route("GET /api/v3/orgs/{org}/insights/api/time-stats", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleAPIInsightsTimeStats))
	s.route("GET /api/v3/orgs/{org}/insights/api/time-stats/users/{user_id}", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleAPIInsightsTimeStatsByUser))
	s.route("GET /api/v3/orgs/{org}/insights/api/time-stats/{actor_type}/{actor_id}", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleAPIInsightsTimeStatsByActor))
	s.route("GET /api/v3/orgs/{org}/insights/api/user-stats/{user_id}", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleAPIInsightsUserStats))
}

// ─── request instrumentation ─────────────────────────────────────────────

// instrumentAPIRoute wraps a registered /api/v3 handler so the served
// request is recorded for API insights. Non-/api/v3 patterns pass through
// untouched.
func (s *Server) instrumentAPIRoute(pattern string, next http.HandlerFunc) http.HandlerFunc {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok || !strings.HasPrefix(path, "/api/v3/") {
		return next
	}
	route := strings.TrimPrefix(path, "/api/v3")
	return func(w http.ResponseWriter, r *http.Request) {
		rec := &apiInsightsStatusRecorder{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)
		s.recordAPIInsightsRequest(r, method, route, rec.status)
	}
}

// apiInsightsStatusRecorder captures the response status while passing
// writes, flushes, and hijacks through to the wrapped writer.
type apiInsightsStatusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *apiInsightsStatusRecorder) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *apiInsightsStatusRecorder) Write(b []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

func (w *apiInsightsStatusRecorder) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *apiInsightsStatusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter does not support hijacking")
	}
	return h.Hijack()
}

// recordAPIInsightsRequest attributes a served /api/v3 request to its actor
// and organizations and appends it to the store. Requests without an
// authenticated, org-attributable actor are not recorded — they can never
// appear in any organization's insights.
func (s *Server) recordAPIInsightsRequest(r *http.Request, method, route string, status int) {
	ctx := r.Context()
	rec := &APIRequestRecord{
		Timestamp:   time.Now().UTC(),
		Method:      method,
		Route:       route,
		StatusCode:  status,
		RateLimited: status == http.StatusTooManyRequests,
	}

	if instTok := ghInstallationTokenFromContext(ctx); instTok != nil {
		inst := ghInstallationFromContext(ctx)
		if inst == nil {
			return
		}
		app := s.store.GetApp(instTok.AppID)
		rec.ActorType = "installation"
		rec.ActorID = int64(inst.ID)
		rec.SubjectType = "installation"
		rec.SubjectID = int64(inst.ID)
		if app != nil {
			rec.ActorName = app.Slug
			rec.SubjectName = app.Slug
			integrationID := int64(app.ID)
			rec.IntegrationID = &integrationID
		}
		if inst.TargetType == "Organization" {
			rec.OrgLogins = []string{inst.TargetLogin}
		}
	} else if uts := ghUserToServerTokenFromContext(ctx); uts != nil {
		user := ghUserFromContext(ctx)
		if user == nil {
			return
		}
		if strings.HasPrefix(uts.Token, tokenPrefixAppUser) {
			rec.ActorType = "github_app_user_to_server"
			if app := s.store.GetApp(uts.AppID); app != nil {
				rec.ActorID = int64(app.ID)
				rec.ActorName = app.Slug
				integrationID := int64(app.ID)
				rec.IntegrationID = &integrationID
			}
		} else {
			rec.ActorType = "oauth_app"
			s.store.mu.RLock()
			if app := s.store.AppsByClientID[uts.OAuthAppClientID]; app != nil {
				rec.ActorID = int64(app.ID)
				rec.ActorName = app.Slug
				oauthID := int64(app.ID)
				rec.OAuthAppID = &oauthID
			} else if oa := s.store.OAuthApps[uts.OAuthAppClientID]; oa != nil {
				rec.ActorName = oa.Name
			}
			s.store.mu.RUnlock()
		}
		rec.SubjectType = "user"
		rec.SubjectID = int64(user.ID)
		rec.SubjectName = user.Login
		rec.UserID = user.ID
		rec.OrgLogins = s.store.ActiveOrgLoginsForUser(user.ID)
	} else if user := ghUserFromContext(ctx); user != nil && user.Type != "Bot" {
		tokenStr := ""
		if scheme, cred := authScheme(r.Header.Get("Authorization")); scheme == "token" || scheme == "bearer" {
			tokenStr = cred
		}
		if strings.HasPrefix(tokenStr, "github_pat_") {
			rec.ActorType = "fine_grained_pat"
			if tokenID, tokenName, ok := s.store.PATIdentityByTokenValue(tokenStr); ok {
				rec.ActorID = int64(tokenID)
				rec.ActorName = tokenName
			} else {
				rec.ActorName = user.Login
			}
		} else {
			rec.ActorType = "classic_pat"
			rec.ActorID = int64(user.ID)
			rec.ActorName = user.Login
		}
		rec.SubjectType = "user"
		rec.SubjectID = int64(user.ID)
		rec.SubjectName = user.Login
		rec.UserID = user.ID
		rec.OrgLogins = s.store.ActiveOrgLoginsForUser(user.ID)
	} else {
		return
	}

	if len(rec.OrgLogins) == 0 {
		return
	}
	s.store.RecordAPIRequest(rec)
}

// ActiveOrgLoginsForUser returns the logins of every organization where the
// user holds an active membership.
func (st *Store) ActiveOrgLoginsForUser(userID int) []string {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []string
	for _, m := range st.Memberships {
		if m.UserID != userID || m.State != MembershipStateActive {
			continue
		}
		if org := st.Orgs[m.OrgID]; org != nil {
			out = append(out, org.Login)
		}
	}
	sort.Strings(out)
	return out
}

// PATIdentityByTokenValue resolves a fine-grained personal access token
// value to its token ID + name via the PAT grant/request tables.
func (st *Store) PATIdentityByTokenValue(value string) (int, string, bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if token := st.Tokens[value]; token != nil && token.FineGrained {
		return token.FineGrainedID, token.Name, true
	}
	for _, m := range st.OrgPATGrants {
		for _, g := range m {
			if g.TokenValue == value {
				return g.TokenID, g.TokenName, true
			}
		}
	}
	for _, m := range st.OrgPATGrantRequests {
		for _, req := range m {
			if req.TokenValue == value {
				return req.TokenID, req.TokenName, true
			}
		}
	}
	return 0, "", false
}

// RecordAPIRequest appends an attributed request record and persists it.
func (st *Store) RecordAPIRequest(rec *APIRequestRecord) {
	st.mu.Lock()
	defer st.mu.Unlock()
	rec.ID = st.NextAPIRequestID
	st.NextAPIRequestID++
	st.APIRequestRecords = append(st.APIRequestRecords, rec)
	if st.persist != nil {
		st.persist.MustPut("api_insights_requests", strconv.FormatInt(rec.ID, 10), rec)
	}
}

// ─── query plumbing ──────────────────────────────────────────────────────

// apiInsightsWindow parses the min_timestamp (required) and max_timestamp
// (optional, default now) query parameters. On failure it writes a 422
// validation error and returns ok=false.
func apiInsightsWindow(w http.ResponseWriter, r *http.Request) (minT, maxT time.Time, ok bool) {
	minRaw := r.URL.Query().Get("min_timestamp")
	if minRaw == "" {
		writeGHValidationError(w, "ApiInsights", "min_timestamp", "missing_field")
		return time.Time{}, time.Time{}, false
	}
	minT, err := time.Parse(time.RFC3339, minRaw)
	if err != nil {
		writeGHValidationError(w, "ApiInsights", "min_timestamp", "invalid")
		return time.Time{}, time.Time{}, false
	}
	maxT = time.Now().UTC()
	if maxRaw := r.URL.Query().Get("max_timestamp"); maxRaw != "" {
		maxT, err = time.Parse(time.RFC3339, maxRaw)
		if err != nil {
			writeGHValidationError(w, "ApiInsights", "max_timestamp", "invalid")
			return time.Time{}, time.Time{}, false
		}
	}
	return minT, maxT, true
}

// apiInsightsRecords returns the org's attributed records inside [minT, maxT],
// oldest first.
func (st *Store) apiInsightsRecords(orgLogin string, minT, maxT time.Time) []*APIRequestRecord {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*APIRequestRecord
	for _, rec := range st.APIRequestRecords {
		if rec.Timestamp.Before(minT) || rec.Timestamp.After(maxT) {
			continue
		}
		for _, login := range rec.OrgLogins {
			if login == orgLogin {
				out = append(out, rec)
				break
			}
		}
	}
	return out
}

func filterRecordsByActor(records []*APIRequestRecord, actorType string, actorID int64) []*APIRequestRecord {
	var out []*APIRequestRecord
	for _, rec := range records {
		if rec.ActorType == actorType && rec.ActorID == actorID {
			out = append(out, rec)
		}
	}
	return out
}

func filterRecordsByUser(records []*APIRequestRecord, userID int) []*APIRequestRecord {
	var out []*APIRequestRecord
	for _, rec := range records {
		if rec.UserID == userID && rec.UserID != 0 {
			out = append(out, rec)
		}
	}
	return out
}

// apiInsightsAggregate is one row of grouped request counts.
type apiInsightsAggregate struct {
	total           int64
	rateLimited     int64
	lastRequest     time.Time
	lastRateLimited *time.Time
}

func (agg *apiInsightsAggregate) add(rec *APIRequestRecord) {
	agg.total++
	if rec.Timestamp.After(agg.lastRequest) {
		agg.lastRequest = rec.Timestamp
	}
	if rec.RateLimited {
		agg.rateLimited++
		if agg.lastRateLimited == nil || rec.Timestamp.After(*agg.lastRateLimited) {
			ts := rec.Timestamp
			agg.lastRateLimited = &ts
		}
	}
}

func (agg *apiInsightsAggregate) fillJSON(out map[string]interface{}) {
	out["total_request_count"] = agg.total
	out["rate_limited_request_count"] = agg.rateLimited
	out["last_request_timestamp"] = agg.lastRequest.UTC().Format(time.RFC3339)
	if agg.lastRateLimited != nil {
		out["last_rate_limited_timestamp"] = agg.lastRateLimited.UTC().Format(time.RFC3339)
	} else {
		out["last_rate_limited_timestamp"] = nil
	}
}

// apiInsightsSortValue extracts a comparable value for a sort key from a
// rendered stats row; string keys compare lexically, everything else by the
// numeric/timestamp members.
func apiInsightsLess(a, b map[string]interface{}, key string) bool {
	sv := func(m map[string]interface{}) string {
		v, _ := m[key].(string)
		return v
	}
	iv := func(m map[string]interface{}) int64 {
		v, _ := m[key].(int64)
		return v
	}
	switch key {
	case "http_method", "api_route", "subject_name", "actor_name",
		"last_rate_limited_timestamp", "last_request_timestamp":
		return sv(a) < sv(b)
	default:
		return iv(a) < iv(b)
	}
}

// sortAndPaginateStats orders rendered rows by the request's sort/direction
// parameters (default: total_request_count desc) and slices to the page.
func sortAndPaginateStats(w http.ResponseWriter, r *http.Request, rows []map[string]interface{}, allowedSorts map[string]bool) []map[string]interface{} {
	sortKey := "total_request_count"
	if v := r.URL.Query().Get("sort"); v != "" && allowedSorts[v] {
		sortKey = v
	}
	direction := r.URL.Query().Get("direction")
	if direction != "asc" {
		direction = "desc"
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if direction == "asc" {
			return apiInsightsLess(rows[i], rows[j], sortKey)
		}
		return apiInsightsLess(rows[j], rows[i], sortKey)
	})
	return paginateAndLink(w, r, rows)
}

// ─── handlers ────────────────────────────────────────────────────────────

func (s *Server) handleAPIInsightsRouteStats(w http.ResponseWriter, r *http.Request) {
	actorType := r.PathValue("actor_type")
	if !apiInsightsActorTypes[actorType] {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	actorID, err := strconv.ParseInt(r.PathValue("actor_id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	minT, maxT, ok := apiInsightsWindow(w, r)
	if !ok {
		return
	}
	records := filterRecordsByActor(s.store.apiInsightsRecords(r.PathValue("org"), minT, maxT), actorType, actorID)

	type routeKey struct{ method, route string }
	groups := map[routeKey]*apiInsightsAggregate{}
	for _, rec := range records {
		k := routeKey{rec.Method, rec.Route}
		if groups[k] == nil {
			groups[k] = &apiInsightsAggregate{}
		}
		groups[k].add(rec)
	}
	substring := strings.ToLower(r.URL.Query().Get("api_route_substring"))
	rows := make([]map[string]interface{}, 0, len(groups))
	for k, agg := range groups {
		if substring != "" && !strings.Contains(strings.ToLower(k.route), substring) {
			continue
		}
		row := map[string]interface{}{
			"http_method": k.method,
			"api_route":   k.route,
		}
		agg.fillJSON(row)
		rows = append(rows, row)
	}
	rows = sortAndPaginateStats(w, r, rows, map[string]bool{
		"last_rate_limited_timestamp": true, "last_request_timestamp": true,
		"rate_limited_request_count": true, "http_method": true,
		"api_route": true, "total_request_count": true,
	})
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAPIInsightsSubjectStats(w http.ResponseWriter, r *http.Request) {
	minT, maxT, ok := apiInsightsWindow(w, r)
	if !ok {
		return
	}
	records := s.store.apiInsightsRecords(r.PathValue("org"), minT, maxT)

	type subjectKey struct {
		subjectType string
		subjectID   int64
	}
	groups := map[subjectKey]*apiInsightsAggregate{}
	names := map[subjectKey]string{}
	for _, rec := range records {
		k := subjectKey{rec.SubjectType, rec.SubjectID}
		if groups[k] == nil {
			groups[k] = &apiInsightsAggregate{}
		}
		groups[k].add(rec)
		names[k] = rec.SubjectName
	}
	substring := strings.ToLower(r.URL.Query().Get("subject_name_substring"))
	rows := make([]map[string]interface{}, 0, len(groups))
	for k, agg := range groups {
		if substring != "" && !strings.Contains(strings.ToLower(names[k]), substring) {
			continue
		}
		row := map[string]interface{}{
			"subject_type": k.subjectType,
			"subject_name": names[k],
			"subject_id":   k.subjectID,
		}
		agg.fillJSON(row)
		rows = append(rows, row)
	}
	rows = sortAndPaginateStats(w, r, rows, map[string]bool{
		"last_rate_limited_timestamp": true, "last_request_timestamp": true,
		"rate_limited_request_count": true, "subject_name": true,
		"total_request_count": true,
	})
	writeJSON(w, http.StatusOK, rows)
}

func writeAPIInsightsSummary(w http.ResponseWriter, records []*APIRequestRecord) {
	var total, rateLimited int64
	for _, rec := range records {
		total++
		if rec.RateLimited {
			rateLimited++
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_request_count":        total,
		"rate_limited_request_count": rateLimited,
	})
}

func (s *Server) handleAPIInsightsSummaryStats(w http.ResponseWriter, r *http.Request) {
	minT, maxT, ok := apiInsightsWindow(w, r)
	if !ok {
		return
	}
	writeAPIInsightsSummary(w, s.store.apiInsightsRecords(r.PathValue("org"), minT, maxT))
}

func (s *Server) handleAPIInsightsSummaryStatsByUser(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.Atoi(r.PathValue("user_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	minT, maxT, ok := apiInsightsWindow(w, r)
	if !ok {
		return
	}
	writeAPIInsightsSummary(w, filterRecordsByUser(s.store.apiInsightsRecords(r.PathValue("org"), minT, maxT), userID))
}

func (s *Server) handleAPIInsightsSummaryStatsByActor(w http.ResponseWriter, r *http.Request) {
	actorType := r.PathValue("actor_type")
	if !apiInsightsActorTypes[actorType] {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	actorID, err := strconv.ParseInt(r.PathValue("actor_id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	minT, maxT, ok := apiInsightsWindow(w, r)
	if !ok {
		return
	}
	writeAPIInsightsSummary(w, filterRecordsByActor(s.store.apiInsightsRecords(r.PathValue("org"), minT, maxT), actorType, actorID))
}

// parseTimestampIncrement parses API insights increments like "5m", "1h",
// or "1d" (time.ParseDuration plus a day suffix).
func parseTimestampIncrement(raw string) (time.Duration, bool) {
	if strings.HasSuffix(raw, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err != nil || days <= 0 {
			return 0, false
		}
		return time.Duration(days) * 24 * time.Hour, true
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}

// maxTimeStatsBuckets bounds the generated time series so an arbitrarily
// wide window with a tiny increment cannot allocate unbounded output.
const maxTimeStatsBuckets = 10000

func writeAPIInsightsTimeStats(w http.ResponseWriter, r *http.Request, records []*APIRequestRecord, minT, maxT time.Time) {
	incRaw := r.URL.Query().Get("timestamp_increment")
	if incRaw == "" {
		writeGHValidationError(w, "ApiInsights", "timestamp_increment", "missing_field")
		return
	}
	inc, ok := parseTimestampIncrement(incRaw)
	if !ok {
		writeGHValidationError(w, "ApiInsights", "timestamp_increment", "invalid")
		return
	}
	if maxT.Before(minT) {
		writeJSON(w, http.StatusOK, []map[string]interface{}{})
		return
	}
	buckets := int(maxT.Sub(minT)/inc) + 1
	if buckets > maxTimeStatsBuckets {
		writeGHValidationError(w, "ApiInsights", "timestamp_increment", "invalid")
		return
	}
	totals := make([]int64, buckets)
	rateLimited := make([]int64, buckets)
	for _, rec := range records {
		idx := int(rec.Timestamp.Sub(minT) / inc)
		if idx < 0 || idx >= buckets {
			continue
		}
		totals[idx]++
		if rec.RateLimited {
			rateLimited[idx]++
		}
	}
	out := make([]map[string]interface{}, 0, buckets)
	for i := 0; i < buckets; i++ {
		out = append(out, map[string]interface{}{
			"timestamp":                  minT.Add(time.Duration(i) * inc).UTC().Format(time.RFC3339),
			"total_request_count":        totals[i],
			"rate_limited_request_count": rateLimited[i],
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAPIInsightsTimeStats(w http.ResponseWriter, r *http.Request) {
	minT, maxT, ok := apiInsightsWindow(w, r)
	if !ok {
		return
	}
	writeAPIInsightsTimeStats(w, r, s.store.apiInsightsRecords(r.PathValue("org"), minT, maxT), minT, maxT)
}

func (s *Server) handleAPIInsightsTimeStatsByUser(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.Atoi(r.PathValue("user_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	minT, maxT, ok := apiInsightsWindow(w, r)
	if !ok {
		return
	}
	records := filterRecordsByUser(s.store.apiInsightsRecords(r.PathValue("org"), minT, maxT), userID)
	writeAPIInsightsTimeStats(w, r, records, minT, maxT)
}

func (s *Server) handleAPIInsightsTimeStatsByActor(w http.ResponseWriter, r *http.Request) {
	actorType := r.PathValue("actor_type")
	if !apiInsightsActorTypes[actorType] {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	actorID, err := strconv.ParseInt(r.PathValue("actor_id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	minT, maxT, ok := apiInsightsWindow(w, r)
	if !ok {
		return
	}
	records := filterRecordsByActor(s.store.apiInsightsRecords(r.PathValue("org"), minT, maxT), actorType, actorID)
	writeAPIInsightsTimeStats(w, r, records, minT, maxT)
}

func (s *Server) handleAPIInsightsUserStats(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.Atoi(r.PathValue("user_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	minT, maxT, ok := apiInsightsWindow(w, r)
	if !ok {
		return
	}
	records := filterRecordsByUser(s.store.apiInsightsRecords(r.PathValue("org"), minT, maxT), userID)

	type actorKey struct {
		actorType string
		actorID   int64
	}
	type actorMeta struct {
		name          string
		integrationID *int64
		oauthAppID    *int64
	}
	groups := map[actorKey]*apiInsightsAggregate{}
	metas := map[actorKey]actorMeta{}
	for _, rec := range records {
		k := actorKey{rec.ActorType, rec.ActorID}
		if groups[k] == nil {
			groups[k] = &apiInsightsAggregate{}
		}
		groups[k].add(rec)
		metas[k] = actorMeta{name: rec.ActorName, integrationID: rec.IntegrationID, oauthAppID: rec.OAuthAppID}
	}
	substring := strings.ToLower(r.URL.Query().Get("actor_name_substring"))
	rows := make([]map[string]interface{}, 0, len(groups))
	for k, agg := range groups {
		meta := metas[k]
		if substring != "" && !strings.Contains(strings.ToLower(meta.name), substring) {
			continue
		}
		row := map[string]interface{}{
			"actor_type": k.actorType,
			"actor_name": meta.name,
			"actor_id":   k.actorID,
		}
		if meta.integrationID != nil {
			row["integration_id"] = *meta.integrationID
		} else {
			row["integration_id"] = nil
		}
		if meta.oauthAppID != nil {
			row["oauth_application_id"] = *meta.oauthAppID
		} else {
			row["oauth_application_id"] = nil
		}
		agg.fillJSON(row)
		rows = append(rows, row)
	}
	rows = sortAndPaginateStats(w, r, rows, map[string]bool{
		"last_rate_limited_timestamp": true, "last_request_timestamp": true,
		"rate_limited_request_count": true, "subject_name": true,
		"total_request_count": true,
	})
	writeJSON(w, http.StatusOK, rows)
}
