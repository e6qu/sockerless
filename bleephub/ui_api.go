package bleephub

import (
	"errors"
	"net/http"
)

func (s *Server) registerUIAPIRoutes() {
	s.route("GET /ui-data/repos/{owner}/{repo}/commits", s.handleUIListCommits)
	s.route("GET /ui-data/repos/{owner}/{repo}/viewer", s.handleUIRepoViewer)
}

// handleUIRepoViewer gives the browser one successful read for viewer-specific
// repository chrome. GitHub's public Star and Subscription existence checks
// correctly return 404 when absent; issuing those expected checks as page
// resources would still produce browser console errors. Mutations continue to
// use the public GitHub REST endpoints.
func (s *Server) handleUIRepoViewer(w http.ResponseWriter, r *http.Request) {
	ctx := s.authenticateRequest(r)
	user := ghUserFromContext(ctx)
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	if suspended, _ := ctx.Value(ctxSuspendedUser).(bool); suspended {
		writeGHError(w, http.StatusForbidden, "This account has been suspended")
		return
	}

	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil || (repo.Private && !canReadRepo(s.store, user, repo)) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	subscription := s.store.GetRepoSubscription(user.ID, repo.ID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"starred":    s.store.IsRepoStarredBy(user.ID, owner, repoName),
		"subscribed": subscription != nil && subscription.Subscribed,
	})
}

func (s *Server) handleUIListCommits(w http.ResponseWriter, r *http.Request) {
	ctx := s.authenticateRequest(r)
	if ghUserFromContext(ctx) == nil && ghInstallationTokenFromContext(ctx) == nil && ghUserToServerTokenFromContext(ctx) == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	r = r.WithContext(ctx)
	if suspended, _ := ctx.Value(ctxSuspendedInstallation).(bool); suspended {
		writeGHError(w, http.StatusForbidden, "This installation has been suspended")
		return
	}
	if suspended, _ := ctx.Value(ctxSuspendedUser).(bool); suspended {
		writeGHError(w, http.StatusForbidden, "This account has been suspended")
		return
	}

	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	commits, err := s.listRepoCommits(repo, owner, repoName, s.baseURL(r))
	if err != nil {
		switch {
		case errors.Is(err, errRepoGitRepositoryEmpty):
			writeJSON(w, http.StatusOK, []map[string]interface{}{})
		case errors.Is(err, errRepoGitObjectUnavailable):
			writeGHError(w, http.StatusInternalServerError, "Git object unavailable")
		default:
			writeGHError(w, http.StatusInternalServerError, "Git storage unavailable")
		}
		return
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, commits))
}
