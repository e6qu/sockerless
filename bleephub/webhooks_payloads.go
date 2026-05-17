package bleephub

import (
	"fmt"
	"time"
)

// attachInstallationBlock injects `installation: {id, node_id}` at the top
// level of every event payload, mirroring what real GH does for events
// delivered through an App installation.
func attachInstallationBlock(payload map[string]interface{}, inst *Installation) map[string]interface{} {
	if inst == nil {
		return payload
	}
	payload["installation"] = map[string]interface{}{
		"id":      inst.ID,
		"node_id": fmt.Sprintf("MDIzOkluc3RhbGxhdGlvbiVk%d", inst.ID),
	}
	return payload
}

// buildInstallationEventPayload builds the top-level `installation` event payload
// (action: created | deleted | suspend | unsuspend | new_permissions_accepted).
func buildInstallationEventPayload(app *App, action string, inst *Installation, sender *User) map[string]interface{} {
	repos := []map[string]interface{}{}
	return map[string]interface{}{
		"action":       action,
		"installation": installationToJSON(inst),
		"repositories": repos,
		"sender":       senderPayload(sender),
		"app_id":       app.ID,
	}
}

// buildInstallationRepositoriesEventPayload builds installation_repositories
// (action: added | removed).
func buildInstallationRepositoriesEventPayload(app *App, action string, inst *Installation, repoIDsChanged []int, sender *User) map[string]interface{} {
	changes := []map[string]interface{}{}
	for _, id := range repoIDsChanged {
		changes = append(changes, map[string]interface{}{"id": id})
	}
	out := map[string]interface{}{
		"action":               action,
		"installation":         installationToJSON(inst),
		"repository_selection": inst.RepositorySelection,
		"sender":               senderPayload(sender),
	}
	switch action {
	case "added":
		out["repositories_added"] = changes
		out["repositories_removed"] = []map[string]interface{}{}
	case "removed":
		out["repositories_added"] = []map[string]interface{}{}
		out["repositories_removed"] = changes
	}
	return out
}

func buildPushPayload(repo *Repo, sender *User, ref, before, after string) map[string]interface{} {
	return buildPushPayloadWithInstallation(repo, sender, ref, before, after, nil)
}

func buildPushPayloadWithInstallation(repo *Repo, sender *User, ref, before, after string, inst *Installation) map[string]interface{} {
	return attachInstallationBlock(map[string]interface{}{
		"ref":         ref,
		"before":      before,
		"after":       after,
		"created":     before == "0000000000000000000000000000000000000000",
		"deleted":     after == "0000000000000000000000000000000000000000",
		"forced":      false,
		"compare":     "",
		"commits":     []interface{}{},
		"head_commit": nil,
		"repository":  repoPayload(repo),
		"sender":      senderPayload(sender),
	}, inst)
}

func buildPullRequestPayload(repo *Repo, pr *PullRequest, sender *User, action string) map[string]interface{} {
	state := "open"
	if pr.State == "CLOSED" || pr.State == "MERGED" {
		state = "closed"
	}

	prJSON := map[string]interface{}{
		"number": pr.Number,
		"title":  pr.Title,
		"body":   pr.Body,
		"state":  state,
		"draft":  pr.IsDraft,
		"merged": pr.State == "MERGED",
		"head": map[string]interface{}{
			"ref": pr.HeadRefName,
		},
		"base": map[string]interface{}{
			"ref": pr.BaseRefName,
		},
		"created_at": pr.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": pr.UpdatedAt.UTC().Format(time.RFC3339),
	}

	if pr.MergedAt != nil {
		prJSON["merged_at"] = pr.MergedAt.UTC().Format(time.RFC3339)
	}
	if pr.ClosedAt != nil {
		prJSON["closed_at"] = pr.ClosedAt.UTC().Format(time.RFC3339)
	}

	return buildPullRequestPayloadInner(action, pr, prJSON, repo, sender, nil)
}

func buildPullRequestPayloadInner(action string, pr *PullRequest, prJSON map[string]interface{}, repo *Repo, sender *User, inst *Installation) map[string]interface{} {
	return attachInstallationBlock(map[string]interface{}{
		"action":       action,
		"number":       pr.Number,
		"pull_request": prJSON,
		"repository":   repoPayload(repo),
		"sender":       senderPayload(sender),
	}, inst)
}

func buildIssuesPayload(repo *Repo, issue *Issue, sender *User, action string) map[string]interface{} {
	state := "open"
	if issue.State == "CLOSED" {
		state = "closed"
	}

	issueJSON := map[string]interface{}{
		"number":     issue.Number,
		"title":      issue.Title,
		"body":       issue.Body,
		"state":      state,
		"created_at": issue.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": issue.UpdatedAt.UTC().Format(time.RFC3339),
	}

	if issue.ClosedAt != nil {
		issueJSON["closed_at"] = issue.ClosedAt.UTC().Format(time.RFC3339)
	}

	return attachInstallationBlock(map[string]interface{}{
		"action":     action,
		"issue":      issueJSON,
		"repository": repoPayload(repo),
		"sender":     senderPayload(sender),
	}, nil)
}

func buildPingPayload(repo *Repo, hook *Webhook) map[string]interface{} {
	payload := map[string]interface{}{
		"zen":     "Keep it logically awesome.",
		"hook_id": hook.ID,
		"hook": map[string]interface{}{
			"id":     hook.ID,
			"type":   "Repository",
			"active": hook.Active,
			"events": hook.Events,
			"config": map[string]interface{}{
				"url":          hook.URL,
				"content_type": "json",
			},
		},
	}
	if repo != nil {
		payload["repository"] = repoPayload(repo)
	}
	return payload
}

func repoPayload(repo *Repo) map[string]interface{} {
	if repo == nil {
		return nil
	}
	result := map[string]interface{}{
		"id":             repo.ID,
		"name":           repo.Name,
		"full_name":      repo.FullName,
		"private":        repo.Private,
		"description":    repo.Description,
		"fork":           repo.Fork,
		"default_branch": repo.DefaultBranch,
		"created_at":     repo.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":     repo.UpdatedAt.UTC().Format(time.RFC3339),
		"pushed_at":      repo.PushedAt.UTC().Format(time.RFC3339),
	}
	if repo.Owner != nil {
		result["owner"] = senderPayload(repo.Owner)
	}
	return result
}

func senderPayload(user *User) map[string]interface{} {
	if user == nil {
		return nil
	}
	return map[string]interface{}{
		"login":      user.Login,
		"id":         user.ID,
		"type":       user.Type,
		"avatar_url": user.AvatarURL,
	}
}
