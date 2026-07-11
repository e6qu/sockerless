package bleephub

import (
	"strings"
	"testing"
)

func TestOrgArtifactMetadata_StorageRecords(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "artifact-meta-org", "Artifact Meta", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	digest := testSubjectDigest("storage-artifact")
	base := "/api/v3/orgs/" + org.Login + "/artifacts"

	// Create a storage record.
	resp := ghPost(t, base+"/metadata/storage-record", defaultToken, map[string]interface{}{
		"name":         "libfoo",
		"digest":       digest,
		"version":      "1.2.3",
		"artifact_url": "https://reg.example.com/artifactory/bar/libfoo-1.2.3",
		"registry_url": "https://reg.example.com/artifactory/",
		"repository":   "bar",
		"status":       "active",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("create storage record = %d, want 200", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	if created["total_count"] != float64(1) {
		t.Fatalf("total_count = %v, want 1", created["total_count"])
	}
	records, _ := created["storage_records"].([]interface{})
	if len(records) != 1 {
		t.Fatalf("storage_records = %v, want 1 entry", created["storage_records"])
	}
	rec := records[0].(map[string]interface{})
	if rec["name"] != "libfoo" || rec["digest"] != digest || rec["registry_url"] != "https://reg.example.com/artifactory/" {
		t.Fatalf("record = %v", rec)
	}
	if rec["status"] != "active" || rec["repository"] != "bar" {
		t.Fatalf("record status/repository = %v/%v", rec["status"], rec["repository"])
	}

	// return_records=false omits the records array.
	resp = ghPost(t, base+"/metadata/storage-record", defaultToken, map[string]interface{}{
		"name":           "libbar",
		"digest":         testSubjectDigest("storage-artifact-2"),
		"registry_url":   "https://reg.example.com/artifactory/",
		"return_records": false,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("create without return_records = %d, want 200", resp.StatusCode)
	}
	silent := decodeJSON(t, resp)
	if _, present := silent["storage_records"]; present {
		t.Fatalf("return_records=false still returned records: %v", silent)
	}

	// List by digest.
	resp = ghGet(t, base+"/"+digest+"/metadata/storage-records", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list storage records = %d, want 200", resp.StatusCode)
	}
	listed := decodeJSON(t, resp)
	if listed["total_count"] != float64(1) {
		t.Fatalf("list total_count = %v, want 1", listed["total_count"])
	}

	// Validation failures.
	for i, body := range []map[string]interface{}{
		{"digest": digest, "registry_url": "https://x/"},                                // missing name
		{"name": "x", "digest": "sha256:short", "registry_url": "https://x/"},           // bad digest
		{"name": "x", "digest": digest},                                                 // missing registry_url
		{"name": "x", "digest": digest, "registry_url": "https://x/", "status": "gone"}, // bad status
	} {
		resp = ghPost(t, base+"/metadata/storage-record", defaultToken, body)
		resp.Body.Close()
		if resp.StatusCode != 422 {
			t.Fatalf("invalid storage body #%d = %d, want 422", i, resp.StatusCode)
		}
	}

	// Unknown org → 404; non-admin → 403; org-outsider read → 404.
	resp = ghPost(t, "/api/v3/orgs/no-such-org/artifacts/metadata/storage-record", defaultToken,
		map[string]interface{}{"name": "x", "digest": digest, "registry_url": "https://x/"})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown org = %d, want 404", resp.StatusCode)
	}
	outsider := createTestUser(t, "artifact-meta-outsider")
	outsiderToken := testServer.store.CreateToken(outsider.ID, "repo").Value
	resp = ghPost(t, base+"/metadata/storage-record", outsiderToken,
		map[string]interface{}{"name": "x", "digest": digest, "registry_url": "https://x/"})
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("non-admin create = %d, want 403", resp.StatusCode)
	}
	resp = ghGet(t, base+"/"+digest+"/metadata/storage-records", outsiderToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("outsider list = %d, want 404", resp.StatusCode)
	}
}

func TestOrgArtifactMetadata_DeploymentRecords(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "deploy-meta-org", "Deploy Meta", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	digest := testSubjectDigest("deploy-artifact")
	base := "/api/v3/orgs/" + org.Login + "/artifacts"

	// Create a deployment record. No provenance attestation exists yet,
	// so attestation_id is null.
	resp := ghPost(t, base+"/metadata/deployment-record", defaultToken, map[string]interface{}{
		"name":                 "libfoo",
		"digest":               digest,
		"version":              "1.2.3",
		"status":               "deployed",
		"logical_environment":  "production",
		"physical_environment": "eu-west-1",
		"cluster":              "cluster-a",
		"deployment_name":      "default-libfoo-app",
		"tags":                 map[string]string{"team": "platform"},
		"runtime_risks":        []string{"internet-exposed"},
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("create deployment record = %d, want 200", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	records, _ := created["deployment_records"].([]interface{})
	if len(records) != 1 {
		t.Fatalf("deployment_records = %v", created["deployment_records"])
	}
	rec := records[0].(map[string]interface{})
	recID := int(rec["id"].(float64))
	if rec["deployment_name"] != "default-libfoo-app" || rec["cluster"] != "cluster-a" {
		t.Fatalf("record = %v", rec)
	}
	if rec["attestation_id"] != nil {
		t.Fatalf("attestation_id = %v, want null before any attestation", rec["attestation_id"])
	}

	// Same deployment identity again → the record is updated in place
	// (same ID, new status), not duplicated.
	resp = ghPost(t, base+"/metadata/deployment-record", defaultToken, map[string]interface{}{
		"name":                 "libfoo",
		"digest":               digest,
		"status":               "decommissioned",
		"logical_environment":  "production",
		"physical_environment": "eu-west-1",
		"cluster":              "cluster-a",
		"deployment_name":      "default-libfoo-app",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("upsert deployment record = %d, want 200", resp.StatusCode)
	}
	updated := decodeJSON(t, resp)
	upRec := updated["deployment_records"].([]interface{})[0].(map[string]interface{})
	if int(upRec["id"].(float64)) != recID {
		t.Fatalf("upsert created a new record: %v vs %d", upRec["id"], recID)
	}

	// Once a provenance attestation for the digest lands in an org repo,
	// the record links to it.
	repo := testServer.store.CreateOrgRepo(org, admin, "deploy-src", "", false)
	if repo == nil {
		t.Fatal("create org repo failed")
	}
	attID := uploadAttestation(t, org.Login+"/deploy-src", defaultToken,
		makeSigstoreBundle(t, digest, "https://slsa.dev/provenance/v1"))
	resp = ghGet(t, base+"/"+digest+"/metadata/deployment-records", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list deployment records = %d, want 200", resp.StatusCode)
	}
	listed := decodeJSON(t, resp)
	if listed["total_count"] != float64(1) {
		t.Fatalf("list total_count = %v, want 1", listed["total_count"])
	}
	linked := listed["deployment_records"].([]interface{})[0].(map[string]interface{})
	if got, _ := linked["attestation_id"].(float64); int(got) != attID {
		t.Fatalf("attestation_id = %v, want %d", linked["attestation_id"], attID)
	}

	// Validation failures.
	for i, body := range []map[string]interface{}{
		{"digest": digest, "status": "deployed", "logical_environment": "p", "deployment_name": "d"}, // missing name
		{"name": "x", "digest": "sha256:nope", "status": "deployed", "logical_environment": "p", "deployment_name": "d"},
		{"name": "x", "digest": digest, "status": "running", "logical_environment": "p", "deployment_name": "d"},
		{"name": "x", "digest": digest, "status": "deployed", "deployment_name": "d"},     // missing logical_environment
		{"name": "x", "digest": digest, "status": "deployed", "logical_environment": "p"}, // missing deployment_name
		{"name": "x", "digest": digest, "status": "deployed", "logical_environment": "p", "deployment_name": "d",
			"runtime_risks": []string{"volcano"}},
	} {
		resp = ghPost(t, base+"/metadata/deployment-record", defaultToken, body)
		resp.Body.Close()
		if resp.StatusCode != 422 {
			t.Fatalf("invalid deployment body #%d = %d, want 422", i, resp.StatusCode)
		}
	}
}

func TestOrgArtifactMetadata_ClusterDeploymentRecords(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "cluster-meta-org", "Cluster Meta", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	d1 := testSubjectDigest("cluster-artifact-1")
	d2 := testSubjectDigest("cluster-artifact-2")
	base := "/api/v3/orgs/" + org.Login + "/artifacts/metadata/deployment-record/cluster/prod-cluster"

	resp := ghPost(t, base, defaultToken, map[string]interface{}{
		"logical_environment":  "production",
		"physical_environment": "us-east-1",
		"deployments": []map[string]interface{}{
			{"name": "svc-a", "digest": d1, "deployment_name": "ns-a-svc-a"},
			{"name": "svc-b", "digest": d2, "deployment_name": "ns-b-svc-b", "status": "decommissioned"},
		},
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("set cluster records = %d, want 200", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	if created["total_count"] != float64(2) {
		t.Fatalf("total_count = %v, want 2", created["total_count"])
	}
	records := created["deployment_records"].([]interface{})
	if len(records) != 2 {
		t.Fatalf("deployment_records = %d, want 2", len(records))
	}
	for _, raw := range records {
		rec := raw.(map[string]interface{})
		if rec["cluster"] != "prod-cluster" {
			t.Fatalf("cluster = %v, want prod-cluster (from the path)", rec["cluster"])
		}
	}

	// Re-posting the same identities updates in place.
	firstID := int(records[0].(map[string]interface{})["id"].(float64))
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"logical_environment":  "production",
		"physical_environment": "us-east-1",
		"deployments": []map[string]interface{}{
			{"name": "svc-a", "digest": d1, "deployment_name": "ns-a-svc-a", "status": "decommissioned"},
		},
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("re-set cluster records = %d, want 200", resp.StatusCode)
	}
	again := decodeJSON(t, resp)
	againRec := again["deployment_records"].([]interface{})[0].(map[string]interface{})
	if int(againRec["id"].(float64)) != firstID {
		t.Fatalf("cluster upsert minted a new record: %v vs %d", againRec["id"], firstID)
	}

	// Duplicate deployment_name in one request → 422; empty list → 422;
	// missing logical_environment → 422.
	for i, body := range []map[string]interface{}{
		{"logical_environment": "production", "deployments": []map[string]interface{}{
			{"name": "a", "digest": d1, "deployment_name": "dup"},
			{"name": "b", "digest": d2, "deployment_name": "dup"},
		}},
		{"logical_environment": "production", "deployments": []map[string]interface{}{}},
		{"deployments": []map[string]interface{}{{"name": "a", "digest": d1, "deployment_name": "x"}}},
	} {
		resp = ghPost(t, base, defaultToken, body)
		resp.Body.Close()
		if resp.StatusCode != 422 {
			t.Fatalf("invalid cluster body #%d = %d, want 422", i, resp.StatusCode)
		}
	}
}

// TestArtifactMetadataAndAttestationPersistenceReload verifies the new
// buckets (attestations, artifact records, project views) survive a
// store reload.
func TestArtifactMetadataAndAttestationPersistenceReload(t *testing.T) {
	var attID, storageID, deployID, viewID, viewNumber, projID int
	digest := testSubjectDigest("reload-artifact")
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		_, byteStore := newObjectByteStoreForTest(t)
		st.ObjectByteStore = byteStore
		st.SeedDefaultUser()
		admin := st.UsersByLogin["admin"]
		repo := st.CreateRepo(admin, "reload-repo", "", false)
		bundle := []byte(`{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json","verificationMaterial":{},"dsseEnvelope":{"payload":"x"}}`)
		att, err := st.CreateAttestation(repo.ID, bundle, []string{digest}, "https://slsa.dev/provenance/v1", admin.Login)
		if err != nil {
			t.Fatalf("CreateAttestation: %v", err)
		}
		attID = att.ID
		storage := st.CreateArtifactStorageRecord(&ArtifactStorageRecord{
			OrgID: 42, Name: "libfoo", Digest: digest, RegistryURL: "https://reg/", Status: "active",
		})
		storageID = storage.ID
		deploy := st.UpsertArtifactDeploymentRecord(&ArtifactDeploymentRecord{
			OrgID: 42, Name: "libfoo", Digest: digest, Status: "deployed",
			LogicalEnvironment: "prod", Cluster: "c1", DeploymentName: "d1",
		})
		deployID = deploy.ID
		proj := st.ProjectsV2.CreateProject(admin.ID, "User", "Reload", admin.ID)
		projID = proj.ID
		filter := "is:issue"
		view := st.ProjectsV2.CreateView(proj.ID, "Board", "board", &filter, []int{}, admin.ID)
		viewID = view.ID
		viewNumber = view.Number
	})

	if a := st2.GetAttestation(attID); a == nil || len(a.SubjectDigests) != 1 || a.SubjectDigests[0] != strings.ToLower(digest) {
		t.Fatalf("attestation did not survive reload: %+v", st2.GetAttestation(attID))
	}
	if recs := st2.ListArtifactStorageRecords(42, digest); len(recs) != 1 || recs[0].ID != storageID {
		t.Fatalf("storage record did not survive reload: %v", recs)
	}
	if recs := st2.ListArtifactDeploymentRecords(42, digest); len(recs) != 1 || recs[0].ID != deployID {
		t.Fatalf("deployment record did not survive reload: %v", recs)
	}
	view := st2.ProjectsV2.GetViewByNumber(projID, viewNumber)
	if view == nil || view.ID != viewID || view.Filter == nil || *view.Filter != "is:issue" {
		t.Fatalf("project view did not survive reload: %+v", view)
	}
	// A post-reload view keeps numbering past the loaded one.
	next := st2.ProjectsV2.CreateView(projID, "Table", "table", nil, []int{}, 1)
	if next.ID <= viewID || next.Number <= viewNumber {
		t.Fatalf("post-reload view numbering regressed: %+v", next)
	}

	// Upserting the same deployment identity after reload updates, not
	// duplicates.
	updated := st2.UpsertArtifactDeploymentRecord(&ArtifactDeploymentRecord{
		OrgID: 42, Name: "libfoo", Digest: digest, Status: "decommissioned",
		LogicalEnvironment: "prod", Cluster: "c1", DeploymentName: "d1",
	})
	if updated.ID != deployID {
		t.Fatalf("post-reload upsert minted a new record: %d vs %d", updated.ID, deployID)
	}
}
