package bleephub

import (
	"context"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
)

func (s *Server) afterCommittedRefUpdate(repo *Repo, sender *User, ref, before, after, baseURL string) {
	if repo == nil {
		return
	}
	stor, _ := s.store.GitStorageForRepoID(repo.ID)
	if owner, name, ok := splitRepoFullName(repo.FullName); ok {
		s.store.UpdateRepo(owner, name, func(current *Repo) {
			current.PushedAt = time.Now().UTC()
		})
	}
	actorID := 0
	if sender != nil {
		actorID = sender.ID
	}
	oldHash := plumbing.NewHash(before)
	newHash := plumbing.NewHash(after)
	s.store.RecordRepoActivity(repo.ID, ref, before, after, actorID, classifyRefUpdate(stor, oldHash, newHash))
	payload := buildPushPayload(s.store, repo, sender, ref, before, after)
	s.emitWebhookEvent(repo.FullName, "push", "", payload)
	s.triggerWorkflowsForEvent(repo.FullName, "push", "", ref, payload)
	if branch := strings.TrimPrefix(ref, "refs/heads/"); branch != ref {
		s.firePullRequestSynchronize(repo, repo.FullName, branch)
	}
	s.triggerPagesBuildForRef(repo, sender, ref, baseURL)
}

func (s *Server) triggerPagesBuildForRef(repo *Repo, sender *User, ref, baseURL string) {
	s.store.Misc.mu.RLock()
	site := s.store.Misc.pagesByRepo[repo.ID]
	buildType := ""
	branch := ""
	if site != nil {
		if site.BuildType != nil {
			buildType = *site.BuildType
		}
		branch, _ = site.Source["branch"].(string)
	}
	s.store.Misc.mu.RUnlock()
	if site == nil || buildType != "legacy" || ref != "refs/heads/"+branch {
		return
	}
	actor := "bleephub-system"
	var pusher *PagesPusher
	if sender != nil {
		actor = sender.Login
		pusher = &PagesPusher{Login: sender.Login, ID: sender.ID, Type: coalesceStr(sender.Type, "User")}
	}
	_, _ = s.runPagesBuild(context.Background(), repo, pusher, actor, baseURL)
}
