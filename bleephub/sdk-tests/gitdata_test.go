package sdktests

import (
	"testing"

	github "github.com/google/go-github/v88/github"
)

func createSDKCommit(t *testing.T, repoName, message, path, content string, parents []*github.Commit) *github.Commit {
	t.Helper()
	tree, _, err := client.Git.CreateTree(ctx(), "admin", repoName, "", []*github.TreeEntry{{
		Path:    github.Ptr(path),
		Mode:    github.Ptr("100644"),
		Type:    github.Ptr("blob"),
		Content: github.Ptr(content),
	}})
	if err != nil {
		t.Fatalf("Git.CreateTree(%s): %v", path, err)
	}
	commit, _, err := client.Git.CreateCommit(ctx(), "admin", repoName, github.Commit{
		Message: github.Ptr(message),
		Tree:    tree,
		Parents: parents,
		Author: &github.CommitAuthor{
			Name:  github.Ptr("SDK Test"),
			Email: github.Ptr("sdk@example.test"),
		},
		Committer: &github.CommitAuthor{
			Name:  github.Ptr("SDK Test"),
			Email: github.Ptr("sdk@example.test"),
		},
	}, nil)
	if err != nil {
		t.Fatalf("Git.CreateCommit(%s): %v", message, err)
	}
	if commit.GetSHA() == "" {
		t.Fatalf("Git.CreateCommit(%s) returned empty SHA", message)
	}
	return commit
}

func createSDKDefaultBranch(t *testing.T, repoName string) *github.Commit {
	t.Helper()
	commit := createSDKCommit(t, repoName, "initial commit", "README.md", "# "+repoName+"\n", nil)
	if _, _, err := client.Git.CreateRef(ctx(), "admin", repoName, github.CreateRef{
		Ref: "refs/heads/main",
		SHA: commit.GetSHA(),
	}); err != nil {
		t.Fatalf("Git.CreateRef(main): %v", err)
	}
	return commit
}

func createPullRequestBranches(t *testing.T, repoName string) (base, head *github.Commit) {
	t.Helper()
	base = createSDKDefaultBranch(t, repoName)

	head = createSDKCommit(t, repoName, "feature commit", "feature.txt", "feature\n", []*github.Commit{base})
	if _, _, err := client.Git.CreateRef(ctx(), "admin", repoName, github.CreateRef{
		Ref: "refs/heads/feature",
		SHA: head.GetSHA(),
	}); err != nil {
		t.Fatalf("Git.CreateRef(feature): %v", err)
	}
	return base, head
}

func TestGitData(t *testing.T) {
	name := uniqueName("git-data")
	createRepo(t, name)

	blob, _, err := client.Git.CreateBlob(ctx(), "admin", name, github.Blob{
		Content:  github.Ptr("hello from the SDK\n"),
		Encoding: github.Ptr("utf-8"),
	})
	if err != nil {
		t.Fatalf("Git.CreateBlob: %v", err)
	}
	if blob.GetSHA() == "" {
		t.Fatal("Git.CreateBlob returned empty SHA")
	}

	tree, _, err := client.Git.CreateTree(ctx(), "admin", name, "", []*github.TreeEntry{{
		Path: github.Ptr("hello.txt"),
		Mode: github.Ptr("100644"),
		Type: github.Ptr("blob"),
		SHA:  blob.SHA,
	}})
	if err != nil {
		t.Fatalf("Git.CreateTree: %v", err)
	}
	if tree.GetSHA() == "" {
		t.Fatal("Git.CreateTree returned empty SHA")
	}

	commit, _, err := client.Git.CreateCommit(ctx(), "admin", name, github.Commit{
		Message: github.Ptr("add hello"),
		Tree:    tree,
		Author: &github.CommitAuthor{
			Name:  github.Ptr("SDK Test"),
			Email: github.Ptr("sdk@example.test"),
		},
		Committer: &github.CommitAuthor{
			Name:  github.Ptr("SDK Test"),
			Email: github.Ptr("sdk@example.test"),
		},
	}, nil)
	if err != nil {
		t.Fatalf("Git.CreateCommit: %v", err)
	}

	ref, _, err := client.Git.CreateRef(ctx(), "admin", name, github.CreateRef{
		Ref: "refs/heads/main",
		SHA: commit.GetSHA(),
	})
	if err != nil {
		t.Fatalf("Git.CreateRef: %v", err)
	}
	if ref.GetRef() != "refs/heads/main" || ref.GetObject().GetSHA() != commit.GetSHA() {
		t.Fatalf("created ref = %q %q, want refs/heads/main %s", ref.GetRef(), ref.GetObject().GetSHA(), commit.GetSHA())
	}

	got, _, err := client.Git.GetRef(ctx(), "admin", name, "heads/main")
	if err != nil {
		t.Fatalf("Git.GetRef: %v", err)
	}
	if got.GetObject().GetSHA() != commit.GetSHA() {
		t.Fatalf("Git.GetRef sha = %q, want %s", got.GetObject().GetSHA(), commit.GetSHA())
	}

	next := createSDKCommit(t, name, "advance main", "next.txt", "next\n", []*github.Commit{commit})
	updated, _, err := client.Git.UpdateRef(ctx(), "admin", name, "heads/main", github.UpdateRef{
		SHA: next.GetSHA(),
	})
	if err != nil {
		t.Fatalf("Git.UpdateRef: %v", err)
	}
	if updated.GetObject().GetSHA() != next.GetSHA() {
		t.Fatalf("Git.UpdateRef sha = %q, want %s", updated.GetObject().GetSHA(), next.GetSHA())
	}

	refs, _, err := client.Git.ListMatchingRefs(ctx(), "admin", name, "heads")
	if err != nil {
		t.Fatalf("Git.ListMatchingRefs: %v", err)
	}
	if len(refs) != 1 || refs[0].GetRef() != "refs/heads/main" {
		t.Fatalf("matching refs = %+v, want only refs/heads/main", refs)
	}
}
