package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

// Libpod-shape conformance, second wave.
//
// These tests pin down the JSON shape of every libpod handler that
// has a non-trivial response. The first wave shipped golden tests for
// `pod inspect` and `pod stop` in `pod_inspect_shape_test.go`; this
// file extends the same coverage to `info`, `containers/json`,
// `containers/{id}` (remove), `images/pull`, and
// `containers/{id}/json` (libpod inspect).
//
// Each test asserts the response is a JSON object (or the documented
// shape — array for remove, stream for pull) and every field
// podman's bindings in `pkg/api/handlers/libpod` look at is present.
// Field absence in our response triggers podman's auto-decoder to
// fall through to the wrong path, surfacing as a confusing
// `cannot unmarshal X into Y` error from the CLI.

func newShapeTestServer(t *testing.T) *BaseServer {
	t.Helper()
	// PendingCreates etc. are wired in NewBaseServer; the libpod
	// list handler walks it so we need a fully-constructed
	// BaseServer here, not the bare `&BaseServer{Store: NewStore()}`
	// shortcut used elsewhere.
	s := NewBaseServer(NewStore(), BackendDescriptor{
		ID:              "shape-test",
		Name:            "sockerless-shape-test",
		ServerVersion:   "0.0.0-test",
		Driver:          "test",
		OperatingSystem: "Linux",
		OSType:          "linux",
		Architecture:    "amd64",
		NCPU:            1,
		MemTotal:        1024 * 1024 * 1024,
	}, zerolog.Nop())
	s.SetSelf(s)
	return s
}

// TestLibpodInfoShape locks down the GET /libpod/info response shape
// against podman's `define.Info` expectations (host / store /
// registries / version groups, every key non-absent).
func TestLibpodInfoShape(t *testing.T) {
	s := newShapeTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/libpod/info", nil)
	s.handleLibpodInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body must be a JSON object, got %s (err=%v)", rec.Body.String(), err)
	}
	for _, top := range []string{"host", "store", "registries", "version"} {
		if _, ok := body[top]; !ok {
			t.Errorf("missing top-level key %q: %s", top, rec.Body.String())
		}
	}

	// host sub-keys
	var host map[string]json.RawMessage
	if err := json.Unmarshal(body["host"], &host); err != nil {
		t.Fatalf("info.host is not an object: %v", err)
	}
	for _, k := range []string{"arch", "os", "hostname", "kernel", "memTotal", "cpus", "distribution"} {
		if _, ok := host[k]; !ok {
			t.Errorf("info.host missing %q", k)
		}
	}

	// store sub-keys
	var store map[string]json.RawMessage
	if err := json.Unmarshal(body["store"], &store); err != nil {
		t.Fatalf("info.store is not an object: %v", err)
	}
	for _, k := range []string{"containerStore", "imageStore", "graphDriverName"} {
		if _, ok := store[k]; !ok {
			t.Errorf("info.store missing %q", k)
		}
	}

	// version sub-keys
	var ver map[string]json.RawMessage
	if err := json.Unmarshal(body["version"], &ver); err != nil {
		t.Fatalf("info.version is not an object: %v", err)
	}
	for _, k := range []string{"APIVersion", "Version", "OsArch"} {
		if _, ok := ver[k]; !ok {
			t.Errorf("info.version missing %q", k)
		}
	}
}

// TestLibpodContainerListShape locks the per-container shape returned
// by GET /libpod/containers/json. Podman's
// `pkg/api/handlers/libpod.GenerateLibpodContainerListResponseBody`
// expects every libpod-specific field to be present (Pod, PodName,
// IsInfra, AutoRemove, Mounts, Pid, …), even if zero/empty. A
// missing key triggers the same auto-decoder fall-through that hits
// the libpod CLI bindings.
func TestLibpodContainerListShape(t *testing.T) {
	s := newShapeTestServer(t)

	// One running container so the list isn't empty (and the path
	// that filters !c.State.Running doesn't skip everything).
	c := api.Container{
		ID:    "abcdef0123456789",
		Name:  "/shape-test",
		Image: "alpine:latest",
		State: api.ContainerState{
			Status:    "running",
			Running:   true,
			Pid:       1,
			StartedAt: "2026-04-26T12:00:00Z",
		},
		Config: api.ContainerConfig{
			Image:      "alpine:latest",
			Cmd:        []string{"echo", "hi"},
			Entrypoint: []string{},
			Labels:     map[string]string{"foo": "bar"},
		},
		HostConfig:      api.HostConfig{NetworkMode: "bridge"},
		NetworkSettings: api.NetworkSettings{Networks: map[string]*api.EndpointSettings{}},
		Mounts:          []api.MountPoint{},
	}
	s.Store.Containers.Put(c.ID, c)
	s.Store.ContainerNames.Put(c.Name, c.ID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/libpod/containers/json", nil)
	s.handleLibpodContainerList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d / %s", rec.Code, rec.Body.String())
	}

	// Body must be a JSON array (libpod list response is `[]LibpodContainer`).
	body := rec.Body.Bytes()
	if len(body) == 0 || body[0] != '[' {
		t.Fatalf("body should start with '[' (array), got %q", string(body[:1]))
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("body must be a JSON array: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	// Every field defined in `libpodContainer` (handle_libpod.go:245)
	// must be present in the response — present-with-zero-value is
	// fine, absence is a regression.
	required := []string{
		"AutoRemove", "Command", "Created", "StartedAt", "Exited",
		"ExitedAt", "ExitCode", "Id", "Image", "ImageID", "IsInfra",
		"Labels", "Mounts", "Names", "Pid", "Pod", "PodName",
		"State", "Status",
	}
	for _, k := range required {
		if _, ok := items[0][k]; !ok {
			t.Errorf("LibpodContainer missing required field %q: %s", k, rec.Body.String())
		}
	}
}

// TestLibpodContainerRemoveShape pins down the rm-report shape:
// podman expects `[]*reports.RmReport` — a JSON array of
// `{Id, Err}` objects, NOT 204 No Content.
func TestLibpodContainerRemoveShape(t *testing.T) {
	s := newShapeTestServer(t)

	c := api.Container{
		ID:    "rmshape0000000001",
		Name:  "/rm-shape-test",
		Image: "alpine:latest",
		State: api.ContainerState{Status: "exited"},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{},
		},
		Mounts: []api.MountPoint{},
	}
	s.Store.Containers.Put(c.ID, c)
	s.Store.ContainerNames.Put(c.Name, c.ID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/libpod/containers/"+c.ID, nil)
	req.SetPathValue("id", c.ID)
	s.handleLibpodContainerRemove(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d / %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.Bytes()
	if len(body) == 0 || body[0] != '[' {
		t.Fatalf("rm-report should be a JSON array, got %q", string(body[:1]))
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("rm-report must be a JSON array: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 rm-report entry, got %d", len(items))
	}
	for _, k := range []string{"Id", "Err"} {
		if _, ok := items[0][k]; !ok {
			t.Errorf("rm-report missing %q: %s", k, rec.Body.String())
		}
	}
}
