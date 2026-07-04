package bleephub

import (
	"fmt"
	"net/http"
	"sort"
	"time"
)

// Activity event feeds — GET /orgs/{org}/events, GET /events, and the
// /users/{username}/events family — all render from one derivation over
// the real recorded state on the store: issues opened and closed, pull
// requests opened, and issue comments created on public repositories.
// Every event carries the stable identity and the recorded creation
// timestamp of the row it derives from, so feeds are identical across
// requests and honestly empty when nothing happened.

func (s *Server) registerGHOrgEventsRoutes() {
	s.route("GET /api/v3/orgs/{org}/events", s.handleListOrgEvents)
}

// activityEventID derives a stable numeric event ID from the underlying
// row: a per-event-kind prefix plus the row's own ID.
func activityEventID(kind, rowID int) string {
	return fmt.Sprintf("%d%09d", kind, rowID)
}

const (
	activityEventKindIssueOpened  = 10
	activityEventKindIssueClosed  = 11
	activityEventKindPullOpened   = 12
	activityEventKindIssueComment = 13
)

type activityEvent struct {
	actorID   int
	createdAt time.Time
	json      map[string]interface{}
}

func eventActorToJSON(u *User) map[string]interface{} {
	return map[string]interface{}{
		"id":          u.ID,
		"login":       u.Login,
		"gravatar_id": "",
		"url":         "/api/v3/users/" + u.Login,
		"avatar_url":  u.AvatarURL,
	}
}

func activityEventOrgJSON(org *Org) map[string]interface{} {
	return map[string]interface{}{
		"id":          org.ID,
		"login":       org.Login,
		"gravatar_id": "",
		"url":         "/api/v3/orgs/" + org.Login,
		"avatar_url":  org.AvatarURL,
	}
}

// deriveActivityEvents renders the activity events for the given
// repositories. When org is non-nil every event carries the org block
// (the organization feed shape). Source rows are gathered under the read
// lock and rendered outside it — the JSON builders (issueToJSON and
// friends) take the store lock themselves.
func (s *Server) deriveActivityEvents(base string, repos map[int]*Repo, org *Org) []activityEvent {
	s.store.mu.RLock()
	var issues []*Issue
	for _, issue := range s.store.Issues {
		if repos[issue.RepoID] != nil {
			issues = append(issues, issue)
		}
	}
	var pulls []*PullRequest
	for _, pr := range s.store.PullRequests {
		if repos[pr.RepoID] != nil {
			pulls = append(pulls, pr)
		}
	}
	type commentRow struct {
		comment *Comment
		issue   *Issue
		pull    *PullRequest
	}
	var comments []commentRow
	for _, c := range s.store.Comments {
		switch c.ParentType {
		case "issue":
			if issue := s.store.Issues[c.IssueID]; issue != nil && repos[issue.RepoID] != nil {
				comments = append(comments, commentRow{comment: c, issue: issue})
			}
		case "pull_request":
			if pr := s.store.PullRequests[c.IssueID]; pr != nil && repos[pr.RepoID] != nil {
				comments = append(comments, commentRow{comment: c, pull: pr})
			}
		}
	}
	s.store.mu.RUnlock()

	repoJSON := func(repo *Repo) map[string]interface{} {
		return map[string]interface{}{
			"id":   repo.ID,
			"name": repo.FullName,
			"url":  base + "/api/v3/repos/" + repo.FullName,
		}
	}
	event := func(kind, rowID int, typ string, actor *User, repo *Repo, createdAt time.Time, payload map[string]interface{}) activityEvent {
		j := map[string]interface{}{
			"id":         activityEventID(kind, rowID),
			"type":       typ,
			"actor":      eventActorToJSON(actor),
			"repo":       repoJSON(repo),
			"payload":    payload,
			"public":     true,
			"created_at": createdAt.UTC().Format(time.RFC3339),
		}
		if org != nil {
			j["org"] = activityEventOrgJSON(org)
		}
		return activityEvent{actorID: actor.ID, createdAt: createdAt, json: j}
	}

	var events []activityEvent
	for _, issue := range issues {
		repo := repos[issue.RepoID]
		author := s.store.GetUserByID(issue.AuthorID)
		if author == nil {
			continue
		}
		issueJSON := issueToJSON(issue, s.store, base, repo.FullName)
		events = append(events, event(activityEventKindIssueOpened, issue.ID, "IssuesEvent", author, repo, issue.CreatedAt, map[string]interface{}{
			"action": "opened",
			"issue":  issueJSON,
		}))
		if issue.ClosedAt != nil {
			events = append(events, event(activityEventKindIssueClosed, issue.ID, "IssuesEvent", author, repo, *issue.ClosedAt, map[string]interface{}{
				"action": "closed",
				"issue":  issueJSON,
			}))
		}
	}
	for _, pr := range pulls {
		repo := repos[pr.RepoID]
		author := s.store.GetUserByID(pr.AuthorID)
		if author == nil {
			continue
		}
		events = append(events, event(activityEventKindPullOpened, pr.ID, "PullRequestEvent", author, repo, pr.CreatedAt, map[string]interface{}{
			"action":       "opened",
			"number":       pr.Number,
			"pull_request": pullRequestToJSON(pr, s.store, base, repo.FullName),
		}))
	}
	for _, row := range comments {
		c := row.comment
		author := s.store.GetUserByID(c.AuthorID)
		if author == nil {
			continue
		}
		var repo *Repo
		var issueJSON map[string]interface{}
		var issueNumber int
		if row.issue != nil {
			repo = repos[row.issue.RepoID]
			issueJSON = issueToJSON(row.issue, s.store, base, repo.FullName)
			issueNumber = row.issue.Number
		} else {
			repo = repos[row.pull.RepoID]
			issueJSON = issueToJSONForPR(row.pull, s.store, base, repo.FullName)
			issueNumber = row.pull.Number
		}
		events = append(events, event(activityEventKindIssueComment, c.ID, "IssueCommentEvent", author, repo, c.CreatedAt, map[string]interface{}{
			"action":  "created",
			"issue":   issueJSON,
			"comment": commentToJSON(c, s.store, base, repo.FullName, issueNumber),
		}))
	}
	return events
}

// sortActivityEvents orders a feed newest-first, with the stable event ID
// as the tiebreaker for same-instant rows.
func sortActivityEvents(events []activityEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].createdAt.Equal(events[j].createdAt) {
			return events[i].createdAt.After(events[j].createdAt)
		}
		iid, _ := events[i].json["id"].(string)
		jid, _ := events[j].json["id"].(string)
		return iid > jid
	})
}

func activityEventsJSON(events []activityEvent) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.json)
	}
	return out
}

// publicReposByID returns every non-private repository keyed by ID — the
// repository universe of the public activity feeds.
func (s *Server) publicReposByID() map[int]*Repo {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	repos := map[int]*Repo{}
	for _, repo := range s.store.Repos {
		if !repo.Private {
			repos[repo.ID] = repo
		}
	}
	return repos
}

func (s *Server) handleListOrgEvents(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// The public feed covers the org's non-private repositories.
	s.store.mu.RLock()
	orgRepos := map[int]*Repo{}
	for _, repo := range s.store.Repos {
		if repo.OwnerType == "Organization" && repo.OwnerID == org.ID && !repo.Private {
			orgRepos[repo.ID] = repo
		}
	}
	s.store.mu.RUnlock()

	events := s.deriveActivityEvents(s.baseURL(r), orgRepos, org)
	sortActivityEvents(events)
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, activityEventsJSON(events)))
}
