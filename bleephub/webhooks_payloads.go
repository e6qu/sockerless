package bleephub

import "time"

func buildPushPayload(repo *Repo, sender *User, ref, before, after string) map[string]interface{} {
	return map[string]interface{}{
		"ref":        ref,
		"before":     before,
		"after":      after,
		"created":    before == "0000000000000000000000000000000000000000",
		"deleted":    after == "0000000000000000000000000000000000000000",
		"forced":     false,
		"compare":    "",
		"commits":    []interface{}{},
		"head_commit": nil,
		"repository": repoPayload(repo),
		"sender":     senderPayload(sender),
	}
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

	return map[string]interface{}{
		"action":       action,
		"number":       pr.Number,
		"pull_request": prJSON,
		"repository":   repoPayload(repo),
		"sender":       senderPayload(sender),
	}
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

	return map[string]interface{}{
		"action":     action,
		"issue":      issueJSON,
		"repository": repoPayload(repo),
		"sender":     senderPayload(sender),
	}
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
