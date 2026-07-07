package sdktests

import (
	"testing"

	github "github.com/google/go-github/v88/github"
)

// createTestPR creates a repo and a single open PR, returning the repo name and
// PR number. The branches are created through the public Git Data API so the
// PR points at real repository objects.
func createTestPR(t *testing.T, repoName string) int {
	t.Helper()
	createRepo(t, repoName)
	createPullRequestBranches(t, repoName)
	pr, _, err := client.PullRequests.Create(ctx(), "admin", repoName, &github.NewPullRequest{
		Title: github.Ptr("a PR"),
		Body:  github.Ptr("PR body"),
		Head:  github.Ptr("feature"),
		Base:  github.Ptr("main"),
	})
	if err != nil {
		t.Fatalf("PullRequests.Create: %v", err)
	}
	return pr.GetNumber()
}

// TestPullRequestsLifecycle covers Create / Get / List / Edit.
func TestPullRequestsLifecycle(t *testing.T) {
	name := uniqueName("pr-life")
	createRepo(t, name)
	createPullRequestBranches(t, name)

	pr, _, err := client.PullRequests.Create(ctx(), "admin", name, &github.NewPullRequest{
		Title: github.Ptr("feature PR"),
		Body:  github.Ptr("does a thing"),
		Head:  github.Ptr("feature"),
		Base:  github.Ptr("main"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if pr.GetNumber() == 0 {
		t.Error("PR number is zero")
	}
	if pr.GetState() != "open" {
		t.Errorf("state = %q, want open", pr.GetState())
	}
	if pr.GetHead().GetRef() != "feature" {
		t.Errorf("head.ref = %q, want feature", pr.GetHead().GetRef())
	}
	if pr.GetBase().GetRef() != "main" {
		t.Errorf("base.ref = %q, want main", pr.GetBase().GetRef())
	}
	if pr.GetUser().GetLogin() != "admin" {
		t.Errorf("user.login = %q, want admin", pr.GetUser().GetLogin())
	}
	num := pr.GetNumber()

	got, _, err := client.PullRequests.Get(ctx(), "admin", name, num)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.GetTitle() != "feature PR" {
		t.Errorf("Get title = %q", got.GetTitle())
	}

	list, _, err := client.PullRequests.List(ctx(), "admin", name, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) == 0 {
		t.Error("List returned no PRs")
	}

	edited, _, err := client.PullRequests.Edit(ctx(), "admin", name, num, &github.PullRequest{
		Title: github.Ptr("renamed PR"),
	})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if edited.GetTitle() != "renamed PR" {
		t.Errorf("Edit title = %q, want renamed PR", edited.GetTitle())
	}
}

// TestPullRequestsReviews covers CreateReview and review-comment Create/List.
func TestPullRequestsReviews(t *testing.T) {
	name := uniqueName("pr-review")
	num := createTestPR(t, name)

	review, _, err := client.PullRequests.CreateReview(ctx(), "admin", name, num, &github.PullRequestReviewRequest{
		Body:  github.Ptr("looks good"),
		Event: github.Ptr("COMMENT"),
	})
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if review.GetID() == 0 {
		t.Error("review ID is zero")
	}
	if review.GetBody() != "looks good" {
		t.Errorf("review body = %q", review.GetBody())
	}

	// Review comment (inline). bleephub requires a non-empty body; commit_id
	// and path are accepted as-is.
	comment, _, err := client.PullRequests.CreateComment(ctx(), "admin", name, num, &github.PullRequestComment{
		Body:     github.Ptr("nit: rename this"),
		CommitID: github.Ptr("0000000000000000000000000000000000000000"),
		Path:     github.Ptr("main.go"),
		Position: github.Ptr(1),
	})
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if comment.GetID() == 0 {
		t.Error("review comment ID is zero")
	}
	if comment.GetBody() != "nit: rename this" {
		t.Errorf("review comment body = %q", comment.GetBody())
	}

	comments, _, err := client.PullRequests.ListComments(ctx(), "admin", name, num, nil)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) == 0 {
		t.Error("ListComments returned none")
	}
}

// TestPullRequestsRequestReviewers covers RequestReviewers and Merge.
func TestPullRequestsRequestReviewers(t *testing.T) {
	name := uniqueName("pr-reviewers")
	num := createTestPR(t, name)

	// RequestReviewers returns the PR object; assert it decodes and the PR
	// identity is intact. (bleephub does not currently echo the requested
	// reviewers back into requested_reviewers — see report.)
	pr, _, err := client.PullRequests.RequestReviewers(ctx(), "admin", name, num, github.ReviewersRequest{
		Reviewers: []string{"admin"},
	})
	if err != nil {
		t.Fatalf("RequestReviewers: %v", err)
	}
	if pr.GetNumber() != num {
		t.Errorf("RequestReviewers PR number = %d, want %d", pr.GetNumber(), num)
	}

	// Merge
	res, _, err := client.PullRequests.Merge(ctx(), "admin", name, num, "merge it", nil)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if !res.GetMerged() {
		t.Error("Merge result merged = false, want true")
	}
	if res.GetSHA() == "" {
		t.Error("Merge result SHA empty")
	}
}
