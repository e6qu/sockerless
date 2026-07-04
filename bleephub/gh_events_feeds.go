package bleephub

import (
	"net/http"
)

// GET /events (the global public event feed) and GET /feeds (the feeds
// catalog).
//
// The public event feed shares the activity-event derivation with the
// org and user feeds: every issue, pull request, and issue comment in a
// public repository yields the corresponding IssuesEvent /
// PullRequestEvent / IssueCommentEvent with its real actor, repository,
// payload, and timestamp. Event IDs are deterministic per source entity
// so pollers see stable IDs across requests.

func (s *Server) registerGHEventsFeedsRoutes() {
	s.route("GET /api/v3/events", s.handleListPublicEvents)
	s.route("GET /api/v3/feeds", s.handleGetFeeds)
}

func (s *Server) handleListPublicEvents(w http.ResponseWriter, r *http.Request) {
	events := s.deriveActivityEvents(s.baseURL(r), s.publicReposByID(), nil)
	sortActivityEvents(events)
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, activityEventsJSON(events)))
}

// handleGetFeeds serves the feeds catalog. bleephub advertises the feeds
// that resolve against this instance; the authenticated-user feed links
// appear only for an authenticated caller, as on real GitHub.
func (s *Server) handleGetFeeds(w http.ResponseWriter, r *http.Request) {
	base := s.baseURL(r)
	atomLink := func(href string) map[string]interface{} {
		return map[string]interface{}{"href": href, "type": "application/atom+xml"}
	}
	links := map[string]interface{}{
		"timeline":            atomLink(base + "/timeline"),
		"user":                atomLink(base + "/{user}"),
		"security_advisories": atomLink(base + "/security-advisories"),
	}
	out := map[string]interface{}{
		"timeline_url":            base + "/timeline",
		"user_url":                base + "/{user}",
		"security_advisories_url": base + "/security-advisories",
		"_links":                  links,
	}
	if user := ghUserFromContext(r.Context()); user != nil {
		out["current_user_public_url"] = base + "/" + user.Login
		links["current_user_public"] = atomLink(base + "/" + user.Login)
	}
	writeJSON(w, http.StatusOK, out)
}
