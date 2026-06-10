package sdktests

import (
	"testing"

	github "github.com/google/go-github/v88/github"
)

// TestIssuesLifecycle covers Create / Get / List / Edit (close+reopen),
// comments Create/List/Edit, and lock/unlock through the typed client.
func TestIssuesLifecycle(t *testing.T) {
	name := uniqueName("issues")
	createRepo(t, name)

	issue, _, err := client.Issues.Create(ctx(), "admin", name, &github.IssueRequest{
		Title: github.Ptr("first issue"),
		Body:  github.Ptr("issue body"),
	})
	if err != nil {
		t.Fatalf("Issues.Create: %v", err)
	}
	if issue.GetNumber() == 0 {
		t.Error("issue number is zero")
	}
	if issue.GetTitle() != "first issue" {
		t.Errorf("title = %q, want %q", issue.GetTitle(), "first issue")
	}
	if issue.GetState() != "open" {
		t.Errorf("state = %q, want open", issue.GetState())
	}
	if issue.GetUser().GetLogin() != "admin" {
		t.Errorf("user.login = %q, want admin", issue.GetUser().GetLogin())
	}
	num := issue.GetNumber()

	// Get
	got, _, err := client.Issues.Get(ctx(), "admin", name, num)
	if err != nil {
		t.Fatalf("Issues.Get: %v", err)
	}
	if got.GetTitle() != "first issue" {
		t.Errorf("Get title = %q", got.GetTitle())
	}

	// List
	issues, _, err := client.Issues.ListByRepo(ctx(), "admin", name, nil)
	if err != nil {
		t.Fatalf("ListByRepo: %v", err)
	}
	if len(issues) == 0 {
		t.Error("ListByRepo returned no issues")
	}

	// Edit: close
	closed, _, err := client.Issues.Edit(ctx(), "admin", name, num, &github.IssueRequest{
		State: github.Ptr("closed"),
	})
	if err != nil {
		t.Fatalf("Edit close: %v", err)
	}
	if closed.GetState() != "closed" {
		t.Errorf("after close state = %q, want closed", closed.GetState())
	}

	// Edit: reopen
	reopened, _, err := client.Issues.Edit(ctx(), "admin", name, num, &github.IssueRequest{
		State: github.Ptr("open"),
	})
	if err != nil {
		t.Fatalf("Edit reopen: %v", err)
	}
	if reopened.GetState() != "open" {
		t.Errorf("after reopen state = %q, want open", reopened.GetState())
	}

	// Comments
	comment, _, err := client.Issues.CreateComment(ctx(), "admin", name, num, &github.IssueComment{
		Body: github.Ptr("a comment"),
	})
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if comment.GetID() == 0 {
		t.Error("comment ID is zero")
	}
	if comment.GetBody() != "a comment" {
		t.Errorf("comment body = %q", comment.GetBody())
	}

	comments, _, err := client.Issues.ListComments(ctx(), "admin", name, num, nil)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 1 {
		t.Errorf("ListComments len = %d, want 1", len(comments))
	}

	edited, _, err := client.Issues.EditComment(ctx(), "admin", name, comment.GetID(), &github.IssueComment{
		Body: github.Ptr("edited comment"),
	})
	if err != nil {
		t.Fatalf("EditComment: %v", err)
	}
	if edited.GetBody() != "edited comment" {
		t.Errorf("edited comment body = %q", edited.GetBody())
	}

	// Lock / Unlock
	if _, err := client.Issues.Lock(ctx(), "admin", name, num, &github.LockIssueOptions{
		LockReason: "resolved",
	}); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	locked, _, err := client.Issues.Get(ctx(), "admin", name, num)
	if err != nil {
		t.Fatalf("Get after lock: %v", err)
	}
	if !locked.GetLocked() {
		t.Error("issue not locked after Lock")
	}
	if _, err := client.Issues.Unlock(ctx(), "admin", name, num); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
}

// TestIssuesLabels covers label Create / AddToIssue / ListByRepo.
func TestIssuesLabels(t *testing.T) {
	name := uniqueName("labels")
	createRepo(t, name)

	label, _, err := client.Issues.CreateLabel(ctx(), "admin", name, &github.Label{
		Name:        github.Ptr("bug"),
		Color:       github.Ptr("ff0000"),
		Description: github.Ptr("Something is broken"),
	})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if label.GetName() != "bug" {
		t.Errorf("label name = %q, want bug", label.GetName())
	}
	if label.GetColor() != "ff0000" {
		t.Errorf("label color = %q, want ff0000", label.GetColor())
	}

	all, _, err := client.Issues.ListLabels(ctx(), "admin", name, nil)
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	found := false
	for _, l := range all {
		if l.GetName() == "bug" {
			found = true
		}
	}
	if !found {
		t.Errorf("ListLabels missing 'bug'")
	}

	// AddLabelsToIssue is split out because it triggers a real bleephub
	// fidelity bug (see TestIssuesAddLabelsToIssue).
}

// TestIssuesAddLabelsToIssue verifies go-github's Issues.AddLabelsToIssue,
// which sends the request body as a bare JSON array (`["bug"]`) — one of the
// two shapes GitHub's "Add labels to an issue" endpoint accepts (the other
// being the object form `{"labels":["bug"]}`). bleephub must accept both.
func TestIssuesAddLabelsToIssue(t *testing.T) {
	name := uniqueName("addlabels")
	createRepo(t, name)

	if _, _, err := client.Issues.CreateLabel(ctx(), "admin", name, &github.Label{
		Name:  github.Ptr("bug"),
		Color: github.Ptr("ff0000"),
	}); err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	issue, _, err := client.Issues.Create(ctx(), "admin", name, &github.IssueRequest{
		Title: github.Ptr("needs a label"),
	})
	if err != nil {
		t.Fatalf("Create issue: %v", err)
	}

	// go-github sends the bare array form here.
	labels, _, err := client.Issues.AddLabelsToIssue(ctx(), "admin", name, issue.GetNumber(), []string{"bug"})
	if err != nil {
		t.Fatalf("AddLabelsToIssue: %v", err)
	}
	found := false
	for _, l := range labels {
		if l.GetName() == "bug" {
			found = true
		}
	}
	if !found {
		t.Errorf("AddLabelsToIssue: 'bug' not in resulting labels %v", labels)
	}
}

// TestIssuesMilestones covers milestone Create / List.
func TestIssuesMilestones(t *testing.T) {
	name := uniqueName("milestones")
	createRepo(t, name)

	ms, _, err := client.Issues.CreateMilestone(ctx(), "admin", name, &github.Milestone{
		Title:       github.Ptr("v1.0"),
		Description: github.Ptr("first release"),
	})
	if err != nil {
		t.Fatalf("CreateMilestone: %v", err)
	}
	if ms.GetNumber() == 0 {
		t.Error("milestone number is zero")
	}
	if ms.GetTitle() != "v1.0" {
		t.Errorf("milestone title = %q, want v1.0", ms.GetTitle())
	}

	list, _, err := client.Issues.ListMilestones(ctx(), "admin", name, nil)
	if err != nil {
		t.Fatalf("ListMilestones: %v", err)
	}
	if len(list) == 0 {
		t.Error("ListMilestones returned none")
	}
}
