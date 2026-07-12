package bleephub

import (
	"errors"
	"net/http"
)

func (s *Server) registerUIAPIRoutes() {
	s.route("GET /ui-data/repos/{owner}/{repo}/commits", s.handleUIListCommits)
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
