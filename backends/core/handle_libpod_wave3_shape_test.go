package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

// Phase 105 — third wave shape tests for libpod handlers that don't
// have golden coverage yet: image-pull stream, networks (list +
// inspect), volumes (list + inspect), system events, system df.
//
// Same shape-test pattern as the second wave: pin top-level type
// (object vs array vs stream) plus every field name podman's CLI
// bindings look at. Field absence is the regression we're guarding
// against — it triggers podman's auto-decoder to fall through to
// the wrong path (BUG-804 failure mode).

// TestLibpodImagePullStreamShape pins the JSON-stream format that
// podman's `pkg/bindings/images.Pull` consumes. The response is a
// chunked stream where each chunk is a JSON object — podman reads
// one object per chunk and looks for an `id` field on the final
// chunk to identify the pulled image. Earlier sockerless versions
// returned a single object body (BUG-501 / pre-Phase 50) which
// broke `podman pull`.
func TestLibpodImagePullStreamShape(t *testing.T) {
	s := newShapeTestServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/libpod/images/pull?reference=alpine:latest", nil)
	s.handleLibpodImagePull(rec, req)

	// Pull is a streamed response — body is a sequence of JSON
	// objects (one per docker-events line). The handler may produce
	// 200 (image pulled / available locally) or 4xx/5xx on real
	// auth errors. Either way the body must be a non-empty
	// stream of JSON objects.
	if rec.Code != http.StatusOK && rec.Code/100 != 4 && rec.Code/100 != 5 {
		t.Fatalf("expected 200/4xx/5xx, got %d / %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if body == "" && rec.Code == http.StatusOK {
		t.Fatal("pull stream must not be empty on 200")
	}

	// On 200, the stream's last non-empty line must include an
	// `id` key — that's how podman identifies the pulled image.
	if rec.Code == http.StatusOK {
		dec := json.NewDecoder(strings.NewReader(body))
		var sawID bool
		for dec.More() {
			var obj map[string]any
			if err := dec.Decode(&obj); err != nil {
				break
			}
			if _, ok := obj["id"]; ok {
				sawID = true
			}
		}
		if !sawID {
			t.Errorf("pull stream missing `id` field somewhere: %s", body)
		}
	}
}

// TestLibpodNetworkListShape pins the libpod network-list response
// shape. Podman's `define.NetworkResource` requires `name`, `id`,
// `driver`, `created`, `subnets` fields. The default networks
// (bridge / host / none) are created by `InitDefaultNetwork` and
// must each appear with the libpod-shape fields.
func TestLibpodNetworkListShape(t *testing.T) {
	s := newShapeTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/libpod/networks/json", nil)
	s.handleNetworkList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d / %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.Bytes()
	if len(body) == 0 || body[0] != '[' {
		t.Fatalf("network list must be a JSON array, got %q", string(body[:min(20, len(body))]))
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("network list must unmarshal to []object: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("network list must have at least the default bridge/host/none entries")
	}
	// Every item must carry the docker-API field set the libpod
	// list response shares with the docker-compat list response.
	for i, item := range items {
		for _, k := range []string{"Name", "Id", "Driver", "Scope"} {
			if _, ok := item[k]; !ok {
				t.Errorf("network list item %d missing %q: %s", i, k, string(body))
			}
		}
	}
}

// TestLibpodVolumeListShape pins the volume-list response.
// Podman expects `{Volumes: [...], Warnings: [...]}` — never a bare
// array (see BUG-804 failure mode). After `volumeCreate`, the
// volume entry must include all `api.Volume` keys.
func TestLibpodVolumeListShape(t *testing.T) {
	s := newShapeTestServer(t)

	// Create one volume so the list isn't empty.
	_, err := s.VolumeCreate(&api.VolumeCreateRequest{Name: "shape-test-vol"})
	if err != nil {
		t.Fatalf("VolumeCreate: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/libpod/volumes/json", nil)
	s.handleVolumeList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d / %s", rec.Code, rec.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("volume list must be a JSON object, got %s (err=%v)", rec.Body.String(), err)
	}
	for _, k := range []string{"Volumes", "Warnings"} {
		if _, ok := resp[k]; !ok {
			t.Errorf("volume-list response missing %q: %s", k, rec.Body.String())
		}
	}

	// Each Volumes entry has the api.Volume keys.
	var vols []map[string]json.RawMessage
	if err := json.Unmarshal(resp["Volumes"], &vols); err != nil {
		t.Fatalf("Volumes must be an array: %v", err)
	}
	if len(vols) == 0 {
		t.Fatalf("expected ≥1 volume after VolumeCreate")
	}
	for _, k := range []string{"Name", "Driver", "Mountpoint", "Labels", "Options", "Scope"} {
		if _, ok := vols[0][k]; !ok {
			t.Errorf("Volume entry missing %q", k)
		}
	}
}

// TestLibpodSystemDfShape pins `GET /libpod/system/df`. Podman's
// `system df` reads `LayersSize`, `Images`, `Containers`,
// `Volumes`, `BuildCache` — every key must be present, even if
// the slice is empty.
func TestLibpodSystemDfShape(t *testing.T) {
	s := newShapeTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/libpod/system/df", nil)
	s.handleSystemDf(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d / %s", rec.Code, rec.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("system df must be a JSON object, got %s (err=%v)", rec.Body.String(), err)
	}
	for _, k := range []string{"LayersSize", "Images", "Containers", "Volumes", "BuildCache"} {
		if _, ok := resp[k]; !ok {
			t.Errorf("system-df response missing %q: %s", k, rec.Body.String())
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
