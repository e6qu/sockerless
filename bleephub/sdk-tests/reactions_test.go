package sdktests

import (
	"testing"

	github "github.com/google/go-github/v88/github"
)

// TestIssueReactions covers CreateIssueReaction across the eight content types
// and ListIssueReactions reading them back.
func TestIssueReactions(t *testing.T) {
	name := uniqueName("reactions")
	createRepo(t, name)

	issue, _, err := client.Issues.Create(ctx(), "admin", name, &github.IssueRequest{
		Title: github.Ptr("react to me"),
	})
	if err != nil {
		t.Fatalf("Create issue: %v", err)
	}
	num := issue.GetNumber()

	// The eight GitHub reaction contents.
	contents := []string{"+1", "-1", "laugh", "confused", "heart", "hooray", "rocket", "eyes"}
	for _, c := range contents {
		r, _, err := client.Reactions.CreateIssueReaction(ctx(), "admin", name, num, c)
		if err != nil {
			t.Fatalf("CreateIssueReaction(%q): %v", c, err)
		}
		if r.GetID() == 0 {
			t.Errorf("reaction %q has zero ID", c)
		}
		if r.GetContent() != c {
			t.Errorf("reaction content = %q, want %q", r.GetContent(), c)
		}
		if r.GetUser().GetLogin() != "admin" {
			t.Errorf("reaction user = %q, want admin", r.GetUser().GetLogin())
		}
	}

	got, _, err := client.Reactions.ListIssueReactions(ctx(), "admin", name, num, nil)
	if err != nil {
		t.Fatalf("ListIssueReactions: %v", err)
	}
	if len(got) != len(contents) {
		t.Errorf("ListIssueReactions len = %d, want %d", len(got), len(contents))
	}
	seen := map[string]bool{}
	for _, r := range got {
		seen[r.GetContent()] = true
	}
	for _, c := range contents {
		if !seen[c] {
			t.Errorf("reaction %q missing from list", c)
		}
	}
}
