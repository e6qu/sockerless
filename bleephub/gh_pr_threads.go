package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// PR review thread resolve/unresolve.
// Real GH only exposes thread resolve as a GraphQL mutation:
//   resolveReviewThread(input: {threadId})   → ReviewThread
//   unresolveReviewThread(input: {threadId}) → ReviewThread
//
// We also expose a REST shape for the same operation so operators using
// curl / octokit / `gh api` can hit it:
//   POST   /repos/{o}/{r}/pulls/{n}/review-threads/{thread_id}/resolve
//   POST   /repos/{o}/{r}/pulls/{n}/review-threads/{thread_id}/unresolve
//
// gh CLI's `gh pr review --thread` calls the GraphQL mutations.

func (s *Server) registerGHPRThreadsRoutes() {
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/pulls/{number}/review-threads/{thread_id}/resolve",
		s.requirePerm("pull_requests", permWrite, s.handleResolveThreadREST(true)))
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/pulls/{number}/review-threads/{thread_id}/unresolve",
		s.requirePerm("pull_requests", permWrite, s.handleResolveThreadREST(false)))
}

func (s *Server) handleResolveThreadREST(resolved bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		threadID, err := strconv.Atoi(r.PathValue("thread_id"))
		if err != nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		if !s.store.PRReviewComments.ResolveThread(threadID, resolved) {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		// Return the thread + its comments to mirror real-GH's GraphQL
		// resolveReviewThread payload.
		repo := s.lookupRepoFromPath(r)
		pr := s.store.GetPullRequestByNumber(repo.ID, mustAtoi(r.PathValue("number")))
		var thread *ReviewThread
		for _, t := range s.store.PRReviewComments.ListThreads(pr.ID) {
			if t.ID == threadID {
				thread = t
				break
			}
		}
		if thread == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		// Re-shape comments for JSON.
		commentJSON := make([]map[string]interface{}, 0, len(thread.Comments))
		for _, c := range thread.Comments {
			commentJSON = append(commentJSON, prReviewCommentToJSON(c, s.store, s.baseURL(r), repo, pr))
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":         thread.ID,
			"isResolved": thread.IsResolved,
			"comments":   commentJSON,
		})
	}
}

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// graphqlMutationResolveReviewThread is invoked from the GraphQL handler
// when gh CLI sends `mutation { resolveReviewThread(input: {threadId})}`.
// Wired into the schema as part of .

// --- GraphQL mutation wiring ---

// also adds resolveReviewThread + unresolveReviewThread mutations
// to the existing graphqlSchema. Done in initGraphQLSchema via a helper to
// avoid widening schema setup further. The mutation accepts an input object
// {threadId, clientMutationId} and returns {thread, clientMutationId}.
//
// Implementation lives in initGraphQLSchema; this file just contains the
// REST surface above.

// Untyped int-parse helper to keep the REST code small; the real path always
// hits a numeric thread_id from the URL.
var _ = json.Marshal
