package bleephub

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

func seedPullRequestBranches(t testing.TB, s *Server, repo *Repo, branches ...string) map[string]string {
	t.Helper()
	return seedStorePullRequestBranches(t, s.store, repo, branches...)
}

func seedStorePullRequestBranches(t testing.TB, st *Store, repo *Repo, branches ...string) map[string]string {
	t.Helper()
	owner, name, ok := splitRepoFullName(repo.FullName)
	if !ok {
		t.Fatalf("invalid repo name %q", repo.FullName)
	}
	stor := st.GetGitStorage(owner, name)
	sig := repoSignature(owner, owner+"@bleephub.local")
	base := repo.DefaultBranch
	if base == "" {
		base = "main"
	}
	var baseHash plumbing.Hash
	if existing := resolveBranchSha(stor, base); existing != "" {
		baseHash = plumbing.NewHash(existing)
	} else {
		var err error
		baseHash, err = initRepoWithFiles(stor, base, "initial commit", map[string]string{"README.md": "base\n"}, sig)
		if err != nil {
			t.Fatalf("init repo %s: %v", repo.FullName, err)
		}
	}

	out := map[string]string{base: baseHash.String()}
	seen := map[string]bool{base: true}
	for _, branch := range branches {
		if branch == "" || seen[branch] {
			continue
		}
		seen[branch] = true
		refName := plumbing.NewBranchReferenceName(branch)
		if err := stor.SetReference(plumbing.NewHashReference(refName, baseHash)); err != nil {
			t.Fatalf("set ref %s: %v", branch, err)
		}
		path := fmt.Sprintf("%s.txt", strings.NewReplacer("/", "-", ":", "-").Replace(branch))
		head, err := createFileCommit(stor, branch, path, "content for "+branch+"\n", "add "+branch, sig)
		if err != nil {
			t.Fatalf("commit on %s: %v", branch, err)
		}
		out[branch] = head.String()
	}
	return out
}
