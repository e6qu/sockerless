package bleephub

import (
	"fmt"
	"net/http"
	"testing"
)

func TestHostedRunners_CatalogEndpoints(t *testing.T) {
	org := createTestOrg(t)
	base := "/api/v3/orgs/" + org + "/actions/hosted-runners"

	data := decodeJSONWithStatus(t, ghGet(t, base+"/images/github-owned", defaultToken), 200)
	images, _ := data["images"].([]interface{})
	if int(data["total_count"].(float64)) < 2 || len(images) < 2 {
		t.Fatalf("github-owned images = %v, want the multi-entry catalog", data)
	}
	first, _ := images[0].(map[string]interface{})
	for _, field := range []string{"id", "platform", "size_gb", "display_name", "source"} {
		if first[field] == nil {
			t.Errorf("github-owned image missing %s: %v", field, first)
		}
	}
	if first["source"] != "github" {
		t.Errorf("github-owned image source = %v, want github", first["source"])
	}

	data = decodeJSONWithStatus(t, ghGet(t, base+"/images/partner", defaultToken), 200)
	images, _ = data["images"].([]interface{})
	if len(images) == 0 {
		t.Fatalf("partner images empty: %v", data)
	}
	partner, _ := images[0].(map[string]interface{})
	if partner["source"] != "partner" {
		t.Errorf("partner image source = %v, want partner", partner["source"])
	}

	data = decodeJSONWithStatus(t, ghGet(t, base+"/machine-sizes", defaultToken), 200)
	specs, _ := data["machine_specs"].([]interface{})
	if len(specs) != 5 {
		t.Fatalf("machine_specs len = %d, want 5 (the documented 4..64-core ladder)", len(specs))
	}
	spec0, _ := specs[0].(map[string]interface{})
	if spec0["id"] != "4-core" || spec0["cpu_cores"].(float64) != 4 ||
		spec0["memory_gb"].(float64) != 16 || spec0["storage_gb"].(float64) != 150 {
		t.Errorf("4-core spec = %v, want the documented 4-core/16GB/150GB entry", spec0)
	}

	data = decodeJSONWithStatus(t, ghGet(t, base+"/platforms", defaultToken), 200)
	platforms, _ := data["platforms"].([]interface{})
	if len(platforms) != 4 {
		t.Fatalf("platforms = %v, want the 4 linux/windows x64+arm64 identifiers", platforms)
	}

	data = decodeJSONWithStatus(t, ghGet(t, base+"/limits", defaultToken), 200)
	pub, _ := data["public_ips"].(map[string]interface{})
	if pub == nil || pub["maximum"].(float64) != 50 || pub["current_usage"].(float64) != 0 {
		t.Fatalf("limits = %v, want maximum 50 / current_usage 0", data)
	}
}

func TestHostedRunners_CRUDLifecycle(t *testing.T) {
	org := createTestOrg(t)
	base := "/api/v3/orgs/" + org + "/actions/hosted-runners"

	// Validation: bad size, missing runner_group_id, unknown image.
	resp := ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "hr-bad", "image": map[string]string{"id": "ubuntu-24.04", "source": "github"},
		"size": "3-core", "runner_group_id": 1,
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("bad size status = %d, want 422", resp.StatusCode)
	}
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "hr-bad", "image": map[string]string{"id": "ubuntu-24.04", "source": "github"},
		"size": "4-core",
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("missing runner_group_id status = %d, want 422", resp.StatusCode)
	}
	resp = ghPost(t, base, defaultToken, map[string]interface{}{
		"name": "hr-bad", "image": map[string]string{"id": "no-such-image", "source": "github"},
		"size": "4-core", "runner_group_id": 1,
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("unknown image status = %d, want 422", resp.StatusCode)
	}

	// Create.
	created := decodeJSONWithStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name":  "my-hosted-runner",
		"image": map[string]string{"id": "ubuntu-24.04", "source": "github"},
		"size":  "8-core", "runner_group_id": 1,
	}), 201)
	id := int(created["id"].(float64))
	if created["platform"] != "linux-x64" || created["status"] != "Ready" {
		t.Fatalf("created runner = %v, want linux-x64 / Ready", created)
	}
	if created["maximum_runners"].(float64) != 10 {
		t.Errorf("maximum_runners = %v, want default 10", created["maximum_runners"])
	}
	imgDetails, _ := created["image_details"].(map[string]interface{})
	if imgDetails == nil || imgDetails["id"] != "ubuntu-24.04" || imgDetails["source"] != "github" {
		t.Fatalf("image_details = %v", created["image_details"])
	}
	sizeDetails, _ := created["machine_size_details"].(map[string]interface{})
	if sizeDetails == nil || sizeDetails["id"] != "8-core" || sizeDetails["cpu_cores"].(float64) != 8 {
		t.Fatalf("machine_size_details = %v", created["machine_size_details"])
	}

	// List + get round-trip.
	data := decodeJSONWithStatus(t, ghGet(t, base, defaultToken), 200)
	if int(data["total_count"].(float64)) != 1 {
		t.Fatalf("list total_count = %v, want 1", data["total_count"])
	}
	got := decodeJSONWithStatus(t, ghGet(t, fmt.Sprintf("%s/%d", base, id), defaultToken), 200)
	if got["name"] != "my-hosted-runner" {
		t.Fatalf("get name = %v", got["name"])
	}

	// Patch: rename, bump capacity, enable static IPs, change size.
	patched := decodeJSONWithStatus(t, ghPatch(t, fmt.Sprintf("%s/%d", base, id), defaultToken,
		map[string]interface{}{
			"name": "renamed-runner", "maximum_runners": 5,
			"enable_static_ip": true, "size": "4-core",
		}), 200)
	if patched["name"] != "renamed-runner" || patched["maximum_runners"].(float64) != 5 ||
		patched["public_ip_enabled"] != true {
		t.Fatalf("patched runner = %v", patched)
	}
	// Static IP reservations show up in the org limits.
	limits := decodeJSONWithStatus(t, ghGet(t, base+"/limits", defaultToken), 200)
	pub, _ := limits["public_ips"].(map[string]interface{})
	if pub["current_usage"].(float64) != 5 {
		t.Fatalf("limits current_usage = %v, want 5 (one static-IP runner x maximum_runners 5)", pub)
	}

	// The runner lists under its runner group.
	groupList := decodeJSONWithStatus(t,
		ghGet(t, "/api/v3/orgs/"+org+"/actions/runner-groups/1/hosted-runners", defaultToken), 200)
	if int(groupList["total_count"].(float64)) != 1 {
		t.Fatalf("runner-group hosted-runners total = %v, want 1", groupList["total_count"])
	}

	// Delete answers 202 with the runner in Deleting state; then 404.
	deleted := decodeJSONWithStatus(t, ghDelete(t, fmt.Sprintf("%s/%d", base, id), defaultToken), 202)
	if deleted["status"] != "Deleting" {
		t.Fatalf("delete status = %v, want Deleting", deleted["status"])
	}
	resp = ghGet(t, fmt.Sprintf("%s/%d", base, id), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("get after delete = %d, want 404", resp.StatusCode)
	}
}

func TestHostedRunners_StaticIPLimitEnforced(t *testing.T) {
	org := createTestOrg(t)
	base := "/api/v3/orgs/" + org + "/actions/hosted-runners"
	// 60 reserved addresses would exceed the 50-address limit.
	resp := ghPost(t, base, defaultToken, map[string]interface{}{
		"name":  "too-many-ips",
		"image": map[string]string{"id": "ubuntu-22.04", "source": "github"},
		"size":  "4-core", "runner_group_id": 1,
		"maximum_runners": 60, "enable_static_ip": true,
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("static-IP over-limit create = %d, want 422", resp.StatusCode)
	}
}

func TestHostedRunners_CustomImages(t *testing.T) {
	org := createTestOrg(t)
	base := "/api/v3/orgs/" + org + "/actions/hosted-runners"

	// Custom image definitions are produced by the image-generation
	// pipeline on real GitHub (no public create endpoint); seed one
	// through the store's creation entry point.
	img := testServer.store.CreateHostedRunnerCustomImage(org, "MyCustomImage", "linux-x64")
	if !testServer.store.AddHostedRunnerCustomImageVersion(img.ID, "1.0.0", 30) {
		t.Fatal("add version 1.0.0")
	}
	if !testServer.store.AddHostedRunnerCustomImageVersion(img.ID, "1.1.0", 32) {
		t.Fatal("add version 1.1.0")
	}

	data := decodeJSONWithStatus(t, ghGet(t, base+"/images/custom", defaultToken), 200)
	images, _ := data["images"].([]interface{})
	if int(data["total_count"].(float64)) != 1 || len(images) != 1 {
		t.Fatalf("custom images = %v, want 1", data)
	}
	entry, _ := images[0].(map[string]interface{})
	if entry["name"] != "MyCustomImage" || entry["versions_count"].(float64) != 2 ||
		entry["latest_version"] != "1.1.0" || entry["total_versions_size"].(float64) != 62 ||
		entry["state"] != "Ready" || entry["source"] != "custom" {
		t.Fatalf("custom image entry = %v", entry)
	}

	one := decodeJSONWithStatus(t, ghGet(t, fmt.Sprintf("%s/images/custom/%d", base, img.ID), defaultToken), 200)
	if one["platform"] != "linux-x64" {
		t.Fatalf("custom image get = %v", one)
	}

	versions := decodeJSONWithStatus(t, ghGet(t, fmt.Sprintf("%s/images/custom/%d/versions", base, img.ID), defaultToken), 200)
	vlist, _ := versions["image_versions"].([]interface{})
	if len(vlist) != 2 {
		t.Fatalf("versions = %v", versions)
	}
	newest, _ := vlist[0].(map[string]interface{})
	if newest["version"] != "1.1.0" {
		t.Fatalf("versions not newest-first: %v", vlist)
	}

	ver := decodeJSONWithStatus(t, ghGet(t, fmt.Sprintf("%s/images/custom/%d/versions/1.0.0", base, img.ID), defaultToken), 200)
	if ver["size_gb"].(float64) != 30 || ver["state"] != "Ready" || ver["state_details"] != "None" {
		t.Fatalf("version 1.0.0 = %v", ver)
	}

	// Create a hosted runner FROM the custom image (latest version).
	created := decodeJSONWithStatus(t, ghPost(t, base, defaultToken, map[string]interface{}{
		"name":  "custom-image-runner",
		"image": map[string]string{"id": fmt.Sprintf("%d", img.ID), "source": "custom"},
		"size":  "4-core", "runner_group_id": 1,
	}), 201)
	imgDetails, _ := created["image_details"].(map[string]interface{})
	if imgDetails["source"] != "custom" || imgDetails["version"] != "1.1.0" ||
		imgDetails["display_name"] != "MyCustomImage" {
		t.Fatalf("custom-image runner image_details = %v", imgDetails)
	}
	runnerID := int(created["id"].(float64))
	resp := ghDelete(t, fmt.Sprintf("%s/%d", base, runnerID), defaultToken)
	resp.Body.Close()

	// Delete one version, then the definition.
	resp = ghDelete(t, fmt.Sprintf("%s/images/custom/%d/versions/1.0.0", base, img.ID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete version = %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, fmt.Sprintf("%s/images/custom/%d/versions/1.0.0", base, img.ID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("get deleted version = %d, want 404", resp.StatusCode)
	}
	resp = ghDelete(t, fmt.Sprintf("%s/images/custom/%d", base, img.ID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete image = %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, fmt.Sprintf("%s/images/custom/%d", base, img.ID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("get deleted image = %d, want 404", resp.StatusCode)
	}
}

func TestHostedRunners_UnknownOrg404(t *testing.T) {
	resp := ghGet(t, "/api/v3/orgs/no-such-org-hr/actions/hosted-runners", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown org = %d, want 404", resp.StatusCode)
	}
}
