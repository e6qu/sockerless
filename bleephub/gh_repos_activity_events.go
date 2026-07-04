package bleephub

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

// --- Repository activity (recorded ref updates) ---

// classifyRefUpdate maps a receive-pack ref command onto GitHub's activity
// types by inspecting the real object graph: a non-fast-forward update is a
// force push.
func classifyRefUpdate(stor gitStorage.Storer, oldHash, newHash plumbing.Hash) string {
	switch {
	case oldHash.IsZero():
		return "branch_creation"
	case newHash.IsZero():
		return "branch_deletion"
	}
	base, err := findMergeBase(stor, oldHash, newHash)
	if err == nil && base == oldHash {
		return "push"
	}
	return "force_push"
}

var validActivityTypes = map[string]bool{
	"push": true, "force_push": true, "branch_creation": true,
	"branch_deletion": true, "pr_merge": true, "merge_queue_merge": true,
}

var activityTimePeriods = map[string]time.Duration{
	"day":     24 * time.Hour,
	"week":    7 * 24 * time.Hour,
	"month":   30 * 24 * time.Hour,
	"quarter": 90 * 24 * time.Hour,
	"year":    365 * 24 * time.Hour,
}

func (s *Server) handleListRepoActivity(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	q := r.URL.Query()
	direction := q.Get("direction")
	if direction == "" {
		direction = "desc"
	}
	if direction != "asc" && direction != "desc" {
		writeGHValidationError(w, "Activity", "direction", "invalid")
		return
	}
	activityType := q.Get("activity_type")
	if activityType != "" && !validActivityTypes[activityType] {
		writeGHValidationError(w, "Activity", "activity_type", "invalid")
		return
	}
	var cutoff time.Time
	if period := q.Get("time_period"); period != "" {
		d, ok := activityTimePeriods[period]
		if !ok {
			writeGHValidationError(w, "Activity", "time_period", "invalid")
			return
		}
		cutoff = time.Now().UTC().Add(-d)
	}
	refFilter := q.Get("ref")
	actorFilter := q.Get("actor")

	acts := s.store.ListRepoActivity(repo.ID)
	out := []map[string]interface{}{}
	for _, a := range acts {
		if activityType != "" && a.ActivityType != activityType {
			continue
		}
		if !cutoff.IsZero() && a.Timestamp.Before(cutoff) {
			continue
		}
		if refFilter != "" && a.Ref != refFilter && strings.TrimPrefix(a.Ref, "refs/heads/") != refFilter {
			continue
		}
		actor := s.store.GetUserByID(a.ActorID)
		if actorFilter != "" && (actor == nil || !strings.EqualFold(actor.Login, actorFilter)) {
			continue
		}
		var actorJSON interface{}
		if actor != nil {
			actorJSON = userToJSON(actor)
		}
		out = append(out, map[string]interface{}{
			"id":            a.ID,
			"node_id":       encodeNodeID("RepositoryActivity", a.ID, ""),
			"before":        a.Before,
			"after":         a.After,
			"ref":           a.Ref,
			"timestamp":     a.Timestamp.UTC().Format(time.RFC3339),
			"activity_type": a.ActivityType,
			"actor":         actorJSON,
		})
	}
	if direction == "desc" {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

// --- Repository events ---

// repoEventEntry pairs a rendered event with its timestamp for sorting.
type repoEventEntry struct {
	when  time.Time
	event map[string]interface{}
}

// eventActorAbsJSON renders the GitHub `actor` shape with absolute URLs.
func eventActorAbsJSON(u *User, base string) map[string]interface{} {
	return map[string]interface{}{
		"id":          u.ID,
		"login":       u.Login,
		"gravatar_id": "",
		"url":         base + "/api/v3/users/" + u.Login,
		"avatar_url":  u.AvatarURL,
	}
}

// repoEvents derives the repository's event feed from real store state:
// recorded ref updates (PushEvent / CreateEvent / DeleteEvent), issues
// (IssuesEvent), pull requests (PullRequestEvent), and issue comments
// (IssueCommentEvent).
func (s *Server) repoEvents(repo *Repo, base string) []repoEventEntry {
	repoJSON := map[string]interface{}{
		"id":   repo.ID,
		"name": repo.FullName,
		"url":  base + "/api/v3/repos/" + repo.FullName,
	}
	public := !repo.Private
	var out []repoEventEntry
	add := func(id, typ string, actor *User, payload map[string]interface{}, when time.Time) {
		if actor == nil {
			return
		}
		out = append(out, repoEventEntry{when: when, event: map[string]interface{}{
			"id":         id,
			"type":       typ,
			"actor":      eventActorAbsJSON(actor, base),
			"repo":       repoJSON,
			"payload":    payload,
			"public":     public,
			"created_at": when.UTC().Format(time.RFC3339),
		}})
	}

	for _, a := range s.store.ListRepoActivity(repo.ID) {
		actor := s.store.GetUserByID(a.ActorID)
		shortRef := strings.TrimPrefix(strings.TrimPrefix(a.Ref, "refs/heads/"), "refs/tags/")
		refType := "branch"
		if strings.HasPrefix(a.Ref, "refs/tags/") {
			refType = "tag"
		}
		switch a.ActivityType {
		case "branch_creation":
			add(strconv.Itoa(a.ID), "CreateEvent", actor, map[string]interface{}{
				"ref":           shortRef,
				"ref_type":      refType,
				"full_ref":      a.Ref,
				"master_branch": repo.DefaultBranch,
				"pusher_type":   "user",
			}, a.Timestamp)
		case "branch_deletion":
			add(strconv.Itoa(a.ID), "DeleteEvent", actor, map[string]interface{}{
				"ref":         shortRef,
				"ref_type":    refType,
				"full_ref":    a.Ref,
				"pusher_type": "user",
			}, a.Timestamp)
		default:
			add(strconv.Itoa(a.ID), "PushEvent", actor, map[string]interface{}{
				"repository_id": repo.ID,
				"push_id":       a.ID,
				"ref":           a.Ref,
				"head":          a.After,
				"before":        a.Before,
			}, a.Timestamp)
		}
	}

	for _, issue := range s.store.ListIssues(repo.ID, "all") {
		author := s.store.GetUserByID(issue.AuthorID)
		add(strconv.Itoa(1_000_000_000+issue.ID), "IssuesEvent", author, map[string]interface{}{
			"action": "opened",
			"issue":  issueToJSON(issue, s.store, base, repo.FullName),
		}, issue.CreatedAt)

		for _, c := range s.store.ListComments(issue.ID) {
			commentAuthor := s.store.GetUserByID(c.AuthorID)
			add(strconv.Itoa(3_000_000_000+c.ID), "IssueCommentEvent", commentAuthor, map[string]interface{}{
				"action":  "created",
				"issue":   issueToJSON(issue, s.store, base, repo.FullName),
				"comment": commentToJSON(c, s.store, base, repo.FullName, issue.Number),
			}, c.CreatedAt)
		}
	}

	stor := s.gitStorageForRepo(repo)
	for _, pr := range s.store.ListPullRequests(repo.ID, "all") {
		author := s.store.GetUserByID(pr.AuthorID)
		add(strconv.Itoa(2_000_000_000+pr.ID), "PullRequestEvent", author, map[string]interface{}{
			"action":       "opened",
			"number":       pr.Number,
			"pull_request": pullRequestMinimalJSON(pr, repo, stor, base),
		}, pr.CreatedAt)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].when.After(out[j].when) })
	return out
}

// pullRequestMinimalJSON renders the GitHub pull-request-minimal shape, with
// head/base SHAs resolved live from git storage.
func pullRequestMinimalJSON(pr *PullRequest, repo *Repo, stor gitStorage.Storer, base string) map[string]interface{} {
	api := base + "/api/v3/repos/" + repo.FullName
	repoRef := map[string]interface{}{
		"id":   repo.ID,
		"url":  api,
		"name": repo.Name,
	}
	resolveSHA := func(ref string) string {
		if stor == nil {
			return ""
		}
		h, err := resolveGitRef(stor, ref)
		if err != nil {
			return ""
		}
		return h.String()
	}
	return map[string]interface{}{
		"id":     pr.ID,
		"number": pr.Number,
		"url":    api + "/pulls/" + strconv.Itoa(pr.Number),
		"head": map[string]interface{}{
			"ref":  pr.HeadRefName,
			"sha":  resolveSHA(pr.HeadRefName),
			"repo": repoRef,
		},
		"base": map[string]interface{}{
			"ref":  pr.BaseRefName,
			"sha":  resolveSHA(pr.BaseRefName),
			"repo": repoRef,
		},
	}
}

func writeEventEntries(w http.ResponseWriter, r *http.Request, entries []repoEventEntry) {
	out := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.event)
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func (s *Server) handleListRepoEvents(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	writeEventEntries(w, r, s.repoEvents(repo, s.baseURL(r)))
}

// handleListNetworkEvents serves the fork-network event feed: the requested
// repository plus every fork whose network root it is.
func (s *Server) handleListNetworkEvents(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	base := s.baseURL(r)
	entries := s.repoEvents(repo, base)
	for _, fork := range s.store.ListForks(repo.ID, RepoListOptions{NoPaginate: true}) {
		if fork.Private {
			continue
		}
		entries = append(entries, s.repoEvents(fork, base)...)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].when.After(entries[j].when) })
	writeEventEntries(w, r, entries)
}

// --- Traffic ---

// trafficWeekStart returns midnight UTC of the Monday beginning t's week —
// GitHub's traffic API buckets weeks from Monday.
func trafficWeekStart(t time.Time) time.Time {
	t = t.UTC()
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	offset := (int(t.Weekday()) + 6) % 7
	return t.AddDate(0, 0, -offset)
}

// trafficRepo resolves the repo and enforces GitHub's traffic access rule:
// the caller needs push access.
func (s *Server) trafficRepo(w http.ResponseWriter, r *http.Request) *Repo {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return nil
	}
	if !canPushRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusForbidden, "Must have push access to repository")
		return nil
	}
	return repo
}

func (s *Server) handleTrafficClones(w http.ResponseWriter, r *http.Request) {
	repo := s.trafficRepo(w, r)
	if repo == nil {
		return
	}
	per := r.URL.Query().Get("per")
	if per == "" {
		per = "day"
	}
	if per != "day" && per != "week" {
		writeGHValidationError(w, "Traffic", "per", "invalid")
		return
	}
	buckets := s.store.ListRepoCloneTraffic(repo.ID, time.Now().UTC().AddDate(0, 0, -14))

	total := 0
	union := map[string]bool{}
	type slot struct {
		count  int
		actors map[string]bool
	}
	slots := map[string]*slot{}
	var keys []string
	for _, b := range buckets {
		total += b.Count
		day, err := time.Parse("2006-01-02", b.Day)
		if err != nil {
			continue
		}
		ts := day
		if per == "week" {
			ts = trafficWeekStart(day)
		}
		key := ts.Format(time.RFC3339)
		sl := slots[key]
		if sl == nil {
			sl = &slot{actors: map[string]bool{}}
			slots[key] = sl
			keys = append(keys, key)
		}
		sl.count += b.Count
		for a := range b.Actors {
			sl.actors[a] = true
			union[a] = true
		}
	}
	sort.Strings(keys)
	clones := make([]map[string]interface{}, 0, len(keys))
	for _, key := range keys {
		clones = append(clones, map[string]interface{}{
			"timestamp": key,
			"count":     slots[key].count,
			"uniques":   len(slots[key].actors),
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":   total,
		"uniques": len(union),
		"clones":  clones,
	})
}

// handleTrafficViews serves view traffic. bleephub serves no repository HTML
// pages, so no views ever occur; the counters are real zeros, never
// fabricated numbers.
func (s *Server) handleTrafficViews(w http.ResponseWriter, r *http.Request) {
	repo := s.trafficRepo(w, r)
	if repo == nil {
		return
	}
	per := r.URL.Query().Get("per")
	if per != "" && per != "day" && per != "week" {
		writeGHValidationError(w, "Traffic", "per", "invalid")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":   0,
		"uniques": 0,
		"views":   []interface{}{},
	})
}

func (s *Server) handleTrafficPopularPaths(w http.ResponseWriter, r *http.Request) {
	if repo := s.trafficRepo(w, r); repo == nil {
		return
	}
	writeJSON(w, http.StatusOK, []interface{}{})
}

func (s *Server) handleTrafficPopularReferrers(w http.ResponseWriter, r *http.Request) {
	if repo := s.trafficRepo(w, r); repo == nil {
		return
	}
	writeJSON(w, http.StatusOK, []interface{}{})
}
