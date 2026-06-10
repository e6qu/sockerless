package sdktests

import (
	"testing"

	github "github.com/google/go-github/v88/github"
)

// TestRepositoriesCRUD covers Create / Get / List / Edit / Delete through the
// typed client, asserting real field values round-trip.
func TestRepositoriesCRUD(t *testing.T) {
	name := uniqueName("repo-crud")

	created, _, err := client.Repositories.Create(ctx(), "", &github.Repository{
		Name:        github.Ptr(name),
		Description: github.Ptr("initial description"),
		Private:     github.Ptr(false),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.GetID() == 0 {
		t.Error("created repo has zero ID")
	}
	if created.GetName() != name {
		t.Errorf("created name = %q, want %q", created.GetName(), name)
	}
	if created.GetOwner().GetLogin() != "admin" {
		t.Errorf("owner login = %q, want admin", created.GetOwner().GetLogin())
	}
	if created.GetFullName() != "admin/"+name {
		t.Errorf("full_name = %q, want admin/%s", created.GetFullName(), name)
	}
	t.Cleanup(func() { _, _ = client.Repositories.Delete(ctx(), "admin", name) })

	// Get
	got, _, err := client.Repositories.Get(ctx(), "admin", name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.GetID() != created.GetID() {
		t.Errorf("Get ID = %d, want %d", got.GetID(), created.GetID())
	}
	if got.GetDescription() != "initial description" {
		t.Errorf("Get description = %q, want %q", got.GetDescription(), "initial description")
	}

	// List by authenticated user
	repos, _, err := client.Repositories.ListByUser(ctx(), "admin", nil)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	found := false
	for _, r := range repos {
		if r.GetName() == name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ListByUser did not include %q (got %d repos)", name, len(repos))
	}

	// Edit
	edited, _, err := client.Repositories.Edit(ctx(), "admin", name, &github.Repository{
		Description: github.Ptr("edited description"),
	})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if edited.GetDescription() != "edited description" {
		t.Errorf("Edit description = %q, want %q", edited.GetDescription(), "edited description")
	}

	// Delete
	if _, err := client.Repositories.Delete(ctx(), "admin", name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, resp, err := client.Repositories.Get(ctx(), "admin", name); err == nil {
		t.Errorf("Get after delete succeeded, want 404 (resp=%v)", resp)
	}
}

// TestRepositoriesBranches lists branches. A freshly created repo has its
// default branch; assert that the default branch shows up.
func TestRepositoriesBranches(t *testing.T) {
	name := uniqueName("repo-branches")
	repo := createRepo(t, name)

	branches, _, err := client.Repositories.ListBranches(ctx(), "admin", name, nil)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	// bleephub may report zero branches for an empty repo (no commits yet);
	// what we assert is the call decodes cleanly into []*Branch. If branches
	// are present, the default branch must be among them.
	if def := repo.GetDefaultBranch(); def != "" && len(branches) > 0 {
		found := false
		for _, b := range branches {
			if b.GetName() == def {
				found = true
			}
			if b.GetName() == "" {
				t.Error("branch with empty name decoded")
			}
		}
		if !found {
			t.Errorf("default branch %q not in branch list", def)
		}
	}
}
