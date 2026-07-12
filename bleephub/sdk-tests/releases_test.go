package sdktests

import (
	"testing"

	github "github.com/google/go-github/v88/github"
)

// TestReleasesLifecycle covers CreateRelease / GetRelease / ListReleases /
// GetLatestRelease / GetReleaseByTag / EditRelease / GenerateReleaseNotes /
// DeleteRelease.
func TestReleasesLifecycle(t *testing.T) {
	name := uniqueName("releases")
	createRepo(t, name)
	createSDKDefaultBranch(t, name)

	rel, _, err := client.Repositories.CreateRelease(ctx(), "admin", name, &github.RepositoryRelease{
		TagName: github.Ptr("v1.0.0"),
		Name:    github.Ptr("Release 1.0.0"),
		Body:    github.Ptr("first release"),
	})
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if rel.GetID() == 0 {
		t.Error("release ID is zero")
	}
	if rel.GetTagName() != "v1.0.0" {
		t.Errorf("tag = %q, want v1.0.0", rel.GetTagName())
	}
	id := rel.GetID()

	got, _, err := client.Repositories.GetRelease(ctx(), "admin", name, id)
	if err != nil {
		t.Fatalf("GetRelease: %v", err)
	}
	if got.GetName() != "Release 1.0.0" {
		t.Errorf("GetRelease name = %q", got.GetName())
	}

	list, _, err := client.Repositories.ListReleases(ctx(), "admin", name, nil)
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if len(list) == 0 {
		t.Error("ListReleases returned none")
	}

	latest, _, err := client.Repositories.GetLatestRelease(ctx(), "admin", name)
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	if latest.GetTagName() != "v1.0.0" {
		t.Errorf("latest tag = %q, want v1.0.0", latest.GetTagName())
	}

	byTag, _, err := client.Repositories.GetReleaseByTag(ctx(), "admin", name, "v1.0.0")
	if err != nil {
		t.Fatalf("GetReleaseByTag: %v", err)
	}
	if byTag.GetID() != id {
		t.Errorf("GetReleaseByTag ID = %d, want %d", byTag.GetID(), id)
	}

	edited, _, err := client.Repositories.EditRelease(ctx(), "admin", name, id, &github.RepositoryRelease{
		Name: github.Ptr("Renamed Release"),
	})
	if err != nil {
		t.Fatalf("EditRelease: %v", err)
	}
	if edited.GetName() != "Renamed Release" {
		t.Errorf("edited name = %q, want Renamed Release", edited.GetName())
	}

	notes, _, err := client.Repositories.GenerateReleaseNotes(ctx(), "admin", name, &github.GenerateNotesOptions{
		TagName: "v2.0.0",
	})
	if err != nil {
		t.Fatalf("GenerateReleaseNotes: %v", err)
	}
	if notes.Name == "" && notes.Body == "" {
		t.Error("GenerateReleaseNotes returned empty name and body")
	}

	if _, err := client.Repositories.DeleteRelease(ctx(), "admin", name, id); err != nil {
		t.Fatalf("DeleteRelease: %v", err)
	}
	if _, _, err := client.Repositories.GetRelease(ctx(), "admin", name, id); err == nil {
		t.Error("GetRelease after delete succeeded, want 404")
	}
}
