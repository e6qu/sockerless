package bleephub

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
)

// testSubjectDigest derives a real sha256 subject digest from seed text.
func testSubjectDigest(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// makeSigstoreBundle builds a Sigstore bundle whose DSSE envelope wraps
// an in-toto statement for the given subject digest and predicate type.
func makeSigstoreBundle(t *testing.T, subjectDigest, predicateType string) map[string]interface{} {
	t.Helper()
	hexDigest := subjectDigest[len("sha256:"):]
	statement := map[string]interface{}{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []map[string]interface{}{
			{"name": "artifact.tar.gz", "digest": map[string]string{"sha256": hexDigest}},
		},
		"predicateType": predicateType,
		"predicate":     map[string]interface{}{"builder": map[string]interface{}{"id": "https://github.com/actions/runner"}},
	}
	payload, err := json.Marshal(statement)
	if err != nil {
		t.Fatalf("marshal statement: %v", err)
	}
	return map[string]interface{}{
		"mediaType": "application/vnd.dev.sigstore.bundle.v0.3+json",
		"verificationMaterial": map[string]interface{}{
			"tlogEntries": []map[string]interface{}{{"logIndex": "12345"}},
		},
		"dsseEnvelope": map[string]interface{}{
			"payloadType": "application/vnd.in-toto+json",
			"payload":     base64.StdEncoding.EncodeToString(payload),
			"signatures":  []map[string]interface{}{{"sig": "MEUCIQDexample"}},
		},
	}
}

func uploadAttestation(t *testing.T, ownerRepo, token string, bundle map[string]interface{}) int {
	t.Helper()
	resp := ghPost(t, "/api/v3/repos/"+ownerRepo+"/attestations", token,
		map[string]interface{}{"bundle": bundle})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("upload attestation = %d, want 201", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	return int(created["id"].(float64))
}

func TestRepoAttestations_CreateAndListRoundTrip(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "attest-repo", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}
	digest := testSubjectDigest("attest-repo-artifact")
	bundle := makeSigstoreBundle(t, digest, "https://slsa.dev/provenance/v1")
	id := uploadAttestation(t, "admin/attest-repo", defaultToken, bundle)
	if id == 0 {
		t.Fatal("attestation id missing")
	}

	// List by subject digest: the stored bundle round-trips verbatim.
	resp := ghGet(t, "/api/v3/repos/admin/attest-repo/attestations/"+digest, defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list = %d, want 200", resp.StatusCode)
	}
	body := decodeJSON(t, resp)
	attestations, _ := body["attestations"].([]interface{})
	if len(attestations) != 1 {
		t.Fatalf("attestations = %v, want 1 entry", body["attestations"])
	}
	entry := attestations[0].(map[string]interface{})
	if int(entry["repository_id"].(float64)) != repo.ID {
		t.Fatalf("repository_id = %v, want %d", entry["repository_id"], repo.ID)
	}
	gotBundle := entry["bundle"].(map[string]interface{})
	if gotBundle["mediaType"] != bundle["mediaType"] {
		t.Fatalf("bundle mediaType = %v", gotBundle["mediaType"])
	}
	gotEnvelope := gotBundle["dsseEnvelope"].(map[string]interface{})
	wantEnvelope := bundle["dsseEnvelope"].(map[string]interface{})
	if gotEnvelope["payload"] != wantEnvelope["payload"] {
		t.Fatal("dsseEnvelope.payload did not round-trip verbatim")
	}

	// predicate_type filtering: a second attestation with an SBOM
	// predicate is excluded by predicate_type=provenance.
	sbom := makeSigstoreBundle(t, digest, "https://spdx.dev/Document/v2.3")
	uploadAttestation(t, "admin/attest-repo", defaultToken, sbom)
	resp = ghGet(t, "/api/v3/repos/admin/attest-repo/attestations/"+digest+"?predicate_type=provenance", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("filtered list = %d, want 200", resp.StatusCode)
	}
	filtered := decodeJSON(t, resp)
	if got := len(filtered["attestations"].([]interface{})); got != 1 {
		t.Fatalf("predicate_type=provenance matched %d, want 1", got)
	}

	// A different digest lists nothing.
	resp = ghGet(t, "/api/v3/repos/admin/attest-repo/attestations/"+testSubjectDigest("other"), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("empty list = %d, want 200", resp.StatusCode)
	}
	empty := decodeJSON(t, resp)
	if got := len(empty["attestations"].([]interface{})); got != 0 {
		t.Fatalf("unexpected attestations for foreign digest: %d", got)
	}

	// Validation and permission failures.
	resp = ghPost(t, "/api/v3/repos/admin/attest-repo/attestations", defaultToken,
		map[string]interface{}{"bundle": map[string]interface{}{"mediaType": "x"}})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("bundle without DSSE envelope = %d, want 422", resp.StatusCode)
	}
	resp = ghPost(t, "/api/v3/repos/admin/attest-repo/attestations", defaultToken, map[string]interface{}{})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("missing bundle = %d, want 422", resp.StatusCode)
	}
	outsider := createTestUser(t, "attest-outsider")
	outsiderToken := testServer.store.CreateToken(outsider.ID, "repo").Value
	resp = ghPost(t, "/api/v3/repos/admin/attest-repo/attestations", outsiderToken,
		map[string]interface{}{"bundle": bundle})
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("non-writer upload = %d, want 403", resp.StatusCode)
	}
	resp = ghPost(t, "/api/v3/repos/admin/no-such-repo/attestations", defaultToken,
		map[string]interface{}{"bundle": bundle})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown repo upload = %d, want 404", resp.StatusCode)
	}
}

func TestRepoAttestations_BundlesUseObjectStore(t *testing.T) {
	_, byteStore := newObjectByteStoreForTest(t)
	previous := testServer.store.ObjectByteStore
	testServer.store.ObjectByteStore = byteStore
	t.Cleanup(func() { testServer.store.ObjectByteStore = previous })

	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "attest-object-repo", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}
	digest := testSubjectDigest("attest-object-repo-artifact")
	bundle := makeSigstoreBundle(t, digest, "https://slsa.dev/provenance/v1")
	id := uploadAttestation(t, "admin/attest-object-repo", defaultToken, bundle)
	key := attestationBundleDataKey(id)

	stored := testServer.store.GetAttestation(id)
	if stored == nil {
		t.Fatal("attestation metadata missing")
	}
	if stored.StoragePath != key {
		t.Fatalf("StoragePath = %q, want %q", stored.StoragePath, key)
	}
	if len(stored.Bundle) != 0 {
		t.Fatalf("metadata retained in-memory bundle bytes: %s", string(stored.Bundle))
	}
	if _, err := byteStore.Get(context.Background(), key); err != nil {
		t.Fatalf("object-backed attestation bundle missing: %v", err)
	}

	resp := ghGet(t, "/api/v3/repos/admin/attest-object-repo/attestations/"+digest, defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list = %d, want 200", resp.StatusCode)
	}
	body := decodeJSON(t, resp)
	attestations, _ := body["attestations"].([]interface{})
	if len(attestations) != 1 {
		t.Fatalf("attestations = %v, want 1 entry", body["attestations"])
	}
	gotBundle := attestations[0].(map[string]interface{})["bundle"].(map[string]interface{})
	gotEnvelope := gotBundle["dsseEnvelope"].(map[string]interface{})
	wantEnvelope := bundle["dsseEnvelope"].(map[string]interface{})
	if gotEnvelope["payload"] != wantEnvelope["payload"] {
		t.Fatal("object-backed dsseEnvelope.payload did not round-trip")
	}

	resp = ghDelete(t, "/api/v3/users/admin/attestations/"+strconv.Itoa(id), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete = %d, want 204", resp.StatusCode)
	}
	if _, err := byteStore.Get(context.Background(), key); err == nil {
		t.Fatalf("attestation object %s survived public delete", key)
	}
}

func TestOrgAttestations_ListRepositoriesBulkAndDelete(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "attest-org", "Attest Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	repo := testServer.store.CreateOrgRepo(org, admin, "signed-images", "", false)
	if repo == nil {
		t.Fatal("create org repo failed")
	}
	digest := testSubjectDigest("attest-org-artifact")
	bundle := makeSigstoreBundle(t, digest, "https://slsa.dev/provenance/v1")
	id1 := uploadAttestation(t, org.Login+"/signed-images", defaultToken, bundle)
	id2 := uploadAttestation(t, org.Login+"/signed-images", defaultToken,
		makeSigstoreBundle(t, digest, "https://spdx.dev/Document/v2.3"))

	// Org-level list by digest.
	resp := ghGet(t, "/api/v3/orgs/"+org.Login+"/attestations/"+digest, defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("org list = %d, want 200", resp.StatusCode)
	}
	listed := decodeJSON(t, resp)
	if got := len(listed["attestations"].([]interface{})); got != 2 {
		t.Fatalf("org attestations = %d, want 2", got)
	}

	// Repositories with attestations.
	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/attestations/repositories", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("repositories list = %d, want 200", resp.StatusCode)
	}
	repos := decodeJSONArray(t, resp)
	if len(repos) != 1 || int(repos[0]["id"].(float64)) != repo.ID || repos[0]["name"] != "signed-images" {
		t.Fatalf("repositories = %v", repos)
	}

	// Bulk list: known digest maps to entries, unknown digest to null.
	missing := testSubjectDigest("never-uploaded")
	resp = ghPost(t, "/api/v3/orgs/"+org.Login+"/attestations/bulk-list", defaultToken,
		map[string]interface{}{"subject_digests": []string{digest, missing}})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("bulk-list = %d, want 200", resp.StatusCode)
	}
	bulk := decodeJSON(t, resp)
	byDigest, _ := bulk["attestations_subject_digests"].(map[string]interface{})
	if entries, _ := byDigest[digest].([]interface{}); len(entries) != 2 {
		t.Fatalf("bulk entries for %s = %v", digest, byDigest[digest])
	}
	if val, present := byDigest[missing]; !present || val != nil {
		t.Fatalf("bulk entry for missing digest = %v (present=%v), want null", val, present)
	}
	pageInfo, _ := bulk["page_info"].(map[string]interface{})
	if pageInfo == nil || pageInfo["has_next"] != false {
		t.Fatalf("page_info = %v", bulk["page_info"])
	}

	// Bulk list without digests → 422.
	resp = ghPost(t, "/api/v3/orgs/"+org.Login+"/attestations/bulk-list", defaultToken,
		map[string]interface{}{"subject_digests": []string{}})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("empty bulk-list = %d, want 422", resp.StatusCode)
	}

	// Non-admin cannot delete.
	outsider := createTestUser(t, "attest-org-outsider")
	outsiderToken := testServer.store.CreateToken(outsider.ID, "repo").Value
	resp = ghDelete(t, "/api/v3/orgs/"+org.Login+"/attestations/"+strconv.Itoa(id1), outsiderToken)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("non-admin delete = %d, want 403", resp.StatusCode)
	}

	// Delete by ID.
	resp = ghDelete(t, "/api/v3/orgs/"+org.Login+"/attestations/"+strconv.Itoa(id1), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete by id = %d, want 204", resp.StatusCode)
	}
	resp = ghDelete(t, "/api/v3/orgs/"+org.Login+"/attestations/"+strconv.Itoa(id1), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("delete by id again = %d, want 404", resp.StatusCode)
	}

	// delete-request with attestation_ids removes the second one.
	resp = ghPost(t, "/api/v3/orgs/"+org.Login+"/attestations/delete-request", defaultToken,
		map[string]interface{}{"attestation_ids": []int{id2}})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("delete-request = %d, want 200", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/attestations/"+digest, defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("post-delete list = %d, want 200", resp.StatusCode)
	}
	after := decodeJSON(t, resp)
	if got := len(after["attestations"].([]interface{})); got != 0 {
		t.Fatalf("attestations after deletes = %d, want 0", got)
	}

	// delete-request matching nothing → 404; both/neither lists → 422.
	resp = ghPost(t, "/api/v3/orgs/"+org.Login+"/attestations/delete-request", defaultToken,
		map[string]interface{}{"subject_digests": []string{missing}})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("delete-request no match = %d, want 404", resp.StatusCode)
	}
	resp = ghPost(t, "/api/v3/orgs/"+org.Login+"/attestations/delete-request", defaultToken,
		map[string]interface{}{"subject_digests": []string{digest}, "attestation_ids": []int{1}})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("delete-request with both lists = %d, want 422", resp.StatusCode)
	}

	// Delete by digest: reupload then wipe the digest.
	uploadAttestation(t, org.Login+"/signed-images", defaultToken, bundle)
	resp = ghDelete(t, "/api/v3/orgs/"+org.Login+"/attestations/digest/"+digest, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete by digest = %d, want 204", resp.StatusCode)
	}
	resp = ghDelete(t, "/api/v3/orgs/"+org.Login+"/attestations/digest/"+digest, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("delete by digest again = %d, want 404", resp.StatusCode)
	}
}

func TestUserAttestations_ListBulkAndDelete(t *testing.T) {
	owner := createTestUser(t, "attest-user")
	ownerToken := testServer.store.CreateToken(owner.ID, "repo").Value
	repo := testServer.store.CreateRepo(owner, "personal-artifacts", "", false)
	if repo == nil {
		t.Fatal("create user repo failed")
	}
	digest := testSubjectDigest("attest-user-artifact")
	bundle := makeSigstoreBundle(t, digest, "https://slsa.dev/provenance/v1")
	id := uploadAttestation(t, owner.Login+"/personal-artifacts", ownerToken, bundle)

	// List by digest at the user scope.
	resp := ghGet(t, "/api/v3/users/"+owner.Login+"/attestations/"+digest, defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("user list = %d, want 200", resp.StatusCode)
	}
	listed := decodeJSON(t, resp)
	if got := len(listed["attestations"].([]interface{})); got != 1 {
		t.Fatalf("user attestations = %d, want 1", got)
	}

	// Bulk list at the user scope.
	resp = ghPost(t, "/api/v3/users/"+owner.Login+"/attestations/bulk-list", defaultToken,
		map[string]interface{}{"subject_digests": []string{digest}})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("user bulk-list = %d, want 200", resp.StatusCode)
	}
	bulk := decodeJSON(t, resp)
	byDigest, _ := bulk["attestations_subject_digests"].(map[string]interface{})
	if entries, _ := byDigest[digest].([]interface{}); len(entries) != 1 {
		t.Fatalf("user bulk entries = %v", byDigest[digest])
	}

	// A different user cannot delete another user's attestations.
	stranger := createTestUser(t, "attest-stranger")
	strangerToken := testServer.store.CreateToken(stranger.ID, "repo").Value
	resp = ghDelete(t, "/api/v3/users/"+owner.Login+"/attestations/"+strconv.Itoa(id), strangerToken)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("stranger delete = %d, want 403", resp.StatusCode)
	}

	// The owner deletes by ID.
	resp = ghDelete(t, "/api/v3/users/"+owner.Login+"/attestations/"+strconv.Itoa(id), ownerToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("owner delete = %d, want 204", resp.StatusCode)
	}

	// delete-request + delete-by-digest round-trip.
	uploadAttestation(t, owner.Login+"/personal-artifacts", ownerToken, bundle)
	resp = ghPost(t, "/api/v3/users/"+owner.Login+"/attestations/delete-request", ownerToken,
		map[string]interface{}{"subject_digests": []string{digest}})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("user delete-request = %d, want 200", resp.StatusCode)
	}
	uploadAttestation(t, owner.Login+"/personal-artifacts", ownerToken, bundle)
	resp = ghDelete(t, "/api/v3/users/"+owner.Login+"/attestations/digest/"+digest, ownerToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("user delete by digest = %d, want 204", resp.StatusCode)
	}

	// Unknown user → 404.
	resp = ghGet(t, "/api/v3/users/no-such-user/attestations/"+digest, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown user list = %d, want 404", resp.StatusCode)
	}
}

func TestAttestations_CursorPagination(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "attest-page-repo", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}
	digest := testSubjectDigest("attest-page-artifact")
	for i := 0; i < 3; i++ {
		uploadAttestation(t, "admin/attest-page-repo", defaultToken,
			makeSigstoreBundle(t, digest, fmt.Sprintf("https://example.com/predicate/v%d", i)))
	}
	resp := ghGet(t, "/api/v3/repos/admin/attest-page-repo/attestations/"+digest+"?per_page=2", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("page 1 = %d, want 200", resp.StatusCode)
	}
	link := resp.Header.Get("Link")
	page1 := decodeJSON(t, resp)
	if got := len(page1["attestations"].([]interface{})); got != 2 {
		t.Fatalf("page 1 = %d entries, want 2", got)
	}
	if !containsRel(link, "next") {
		t.Fatalf("page 1 Link = %q, want rel=next", link)
	}
	after := extractCursor(t, link, "after")
	resp = ghGet(t, "/api/v3/repos/admin/attest-page-repo/attestations/"+digest+"?per_page=2&after="+after, defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("page 2 = %d, want 200", resp.StatusCode)
	}
	page2 := decodeJSON(t, resp)
	if got := len(page2["attestations"].([]interface{})); got != 1 {
		t.Fatalf("page 2 = %d entries, want 1", got)
	}
}
