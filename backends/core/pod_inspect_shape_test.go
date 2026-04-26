package core

import (
	"encoding/json"
	"testing"

	"github.com/sockerless/api"
)

// TestPodInspectShape_GoldenJSON locks down the wire shape returned by
// `GET /libpod/pods/<name>/json` against the libpod CLI's expectations.
// When an optional field is absent, podman's auto-decoder falls
// through to a slice path with a `cannot unmarshal array into …`
// error. The golden assertion here is:
//
//   - response is a JSON OBJECT (starts with `{`, not `[`)
//   - every `define.InspectPodData` field present in our spec is
//     emitted (zero-valued is OK, but the key must be there)
func TestPodInspectShape_GoldenJSON(t *testing.T) {
	s := &BaseServer{Store: NewStore()}
	s.SetSelf(s)

	pod := s.Store.Pods.CreatePod("shape-test", nil)
	resp, err := s.PodInspect(pod.Name)
	if err != nil {
		t.Fatalf("PodInspect: %v", err)
	}

	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// 1. Body must be a JSON object, not an array.
	if len(body) == 0 || body[0] != '{' {
		t.Fatalf("PodInspect emitted non-object JSON (first byte = %q): %s", string(body[0]), string(body))
	}

	// 2. Every libpod-shape field is present at the top level (key
	//    presence — value can be zero/null/empty).
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(body, &asMap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	required := []string{
		// sockerless core fields
		"Id", "Name", "Created", "State", "Hostname", "Labels",
		"NumContainers", "Containers", "SharedNamespaces",
		// libpod-shape additions
		"Namespace", "CreateCommand", "ExitPolicy", "InfraContainerID",
		"InfraConfig", "CgroupParent", "CgroupPath", "LockNumber",
		"RestartPolicy", "BlkioWeight", "CPUPeriod", "CPUQuota",
		"CPUShares", "CPUSetCPUs", "MemoryLimit", "MemorySwap",
		"BlkioDeviceReadBps", "BlkioDeviceWriteBps", "VolumesFrom",
		"SecurityOpts", "mounts", "devices", "device_read_bps",
	}
	for _, k := range required {
		if _, ok := asMap[k]; !ok {
			t.Errorf("missing required key %q in PodInspect response: %s", k, string(body))
		}
	}

	// 3. InfraConfig must itself be an object (not null), with all
	//    the libpod-bindings fields present.
	var infra map[string]json.RawMessage
	if err := json.Unmarshal(asMap["InfraConfig"], &infra); err != nil {
		t.Fatalf("InfraConfig is not an object: %v / raw=%s", err, string(asMap["InfraConfig"]))
	}
	for _, k := range []string{
		"PortBindings", "HostNetwork", "DNSServer", "DNSSearch",
		"DNSOption", "Networks", "NetworkOptions",
	} {
		if _, ok := infra[k]; !ok {
			t.Errorf("InfraConfig missing key %q: %s", k, string(asMap["InfraConfig"]))
		}
	}
}

// TestPodActionResponse_WireShape locks down the wire shape returned
// by `POST /libpod/pods/<name>/stop` (and kill). Podman's
// `PodStopReport.Errs` is `[]error` and never round-trips JSON. Our
// handler emits `Errs: []` on success and routes len(errs)>0 through
// HTTP 409 — the success body must always have an empty array.
func TestPodActionResponse_WireShape(t *testing.T) {
	resp := &api.PodActionResponse{ID: "abc", Errs: []string{}}
	body, _ := json.Marshal(resp)
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(body, &asMap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(asMap["Errs"]) != "[]" {
		t.Errorf("Errs should serialize as [] (empty array), got %s", string(asMap["Errs"]))
	}
}
