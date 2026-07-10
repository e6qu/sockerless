package bleephub

import (
	"testing"
)

func TestPRGraphQL_OrgOwnedHeadRepositoryOwner(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	orgLogin := "sweep-org-head-owner"
	org := testServer.store.CreateOrg(admin, orgLogin, "Sweep Org Head Owner", "")
	if org == nil {
		t.Fatal("failed to create org")
	}
	defer func() {
		testServer.store.mu.Lock()
		delete(testServer.store.Orgs, org.ID)
		delete(testServer.store.OrgsByLogin, orgLogin)
		delete(testServer.store.Memberships, membershipKey(orgLogin, admin.ID))
		testServer.store.mu.Unlock()
	}()

	repo := testServer.store.CreateOrgRepo(org, admin, "sweep-org-repo", "", false)
	if repo == nil {
		t.Fatal("failed to create org repo")
	}
	seedPullRequestBranches(t, testServer, repo, "feature")
	defer func() {
		if _, err := testServer.store.DeleteRepo(orgLogin, "sweep-org-repo"); err != nil {
			t.Fatalf("DeleteRepo: %v", err)
		}
	}()

	prNum, _ := sweepPR(t, orgLogin, "sweep-org-repo", "org pr")

	query := `query PR($owner: String!, $repo: String!, $num: Int!) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $num) {
				headRepositoryOwner { login }
				headRepository { nameWithOwner }
			}
		}
	}`
	d := gqlData(t, query, map[string]interface{}{"owner": orgLogin, "repo": "sweep-org-repo", "num": prNum})
	repoData, _ := d["repository"].(map[string]interface{})
	prData, _ := repoData["pullRequest"].(map[string]interface{})
	hro, _ := prData["headRepositoryOwner"].(map[string]interface{})
	if hro == nil || hro["login"] != orgLogin {
		t.Errorf("headRepositoryOwner.login = %v, want %q", prData["headRepositoryOwner"], orgLogin)
	}
}
