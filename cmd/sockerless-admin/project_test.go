package main

import (
	"fmt"
	"testing"
)

func TestPortAllocatorAllocate(t *testing.T) {
	pa := NewPortAllocator()
	ports, err := pa.Allocate("test-project", 4)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if len(ports) != 4 {
		t.Fatalf("expected 4 ports, got %d", len(ports))
	}

	// All ports should be unique and > 0
	seen := map[int]bool{}
	for _, p := range ports {
		if p <= 0 {
			t.Errorf("expected positive port, got %d", p)
		}
		if seen[p] {
			t.Errorf("duplicate port %d", p)
		}
		seen[p] = true
	}

	// All ports should be tracked
	for _, p := range ports {
		if !pa.IsPortTaken(p) {
			t.Errorf("port %d should be taken", p)
		}
	}
}

func TestPortAllocatorRelease(t *testing.T) {
	pa := NewPortAllocator()
	ports, _ := pa.Allocate("proj-a", 2)

	pa.Release("proj-a")

	for _, p := range ports {
		if pa.IsPortTaken(p) {
			t.Errorf("port %d should be released", p)
		}
	}
}

func TestPortAllocatorReserve(t *testing.T) {
	pa := NewPortAllocator()

	if err := pa.Reserve("proj-a", []int{8080, 8081}); err != nil {
		t.Fatalf("Reserve failed: %v", err)
	}

	if !pa.IsPortTaken(8080) {
		t.Error("port 8080 should be taken")
	}
	if !pa.IsPortTaken(8081) {
		t.Error("port 8081 should be taken")
	}
}

func TestPortAllocatorReserveConflict(t *testing.T) {
	pa := NewPortAllocator()
	_ = pa.Reserve("proj-a", []int{8080})

	err := pa.Reserve("proj-b", []int{8080})
	if err == nil {
		t.Error("expected conflict error")
	}
}

func TestValidBackends(t *testing.T) {
	tests := []struct {
		cloud    CloudType
		expected []BackendType
	}{
		{CloudAWS, []BackendType{BackendECS, BackendLambda}},
		{CloudGCP, []BackendType{BackendCloudRun, BackendGCF}},
		{CloudAzure, []BackendType{BackendACA, BackendAZF}},
		{"invalid", nil},
	}

	for _, tt := range tests {
		backends := ValidBackends(tt.cloud)
		if len(backends) != len(tt.expected) {
			t.Errorf("ValidBackends(%s): expected %d backends, got %d", tt.cloud, len(tt.expected), len(backends))
			continue
		}
		for i, b := range backends {
			if b != tt.expected[i] {
				t.Errorf("ValidBackends(%s)[%d]: expected %s, got %s", tt.cloud, i, tt.expected[i], b)
			}
		}
	}
}

func TestIsValidCloud(t *testing.T) {
	if !IsValidCloud(CloudAWS) {
		t.Error("aws should be valid")
	}
	if !IsValidCloud(CloudGCP) {
		t.Error("gcp should be valid")
	}
	if !IsValidCloud(CloudAzure) {
		t.Error("azure should be valid")
	}
	if IsValidCloud("invalid") {
		t.Error("invalid should not be valid")
	}
}

func TestIsValidBackend(t *testing.T) {
	if !IsValidBackend(CloudAWS, BackendECS) {
		t.Error("ecs should be valid for aws")
	}
	if !IsValidBackend(CloudAWS, BackendLambda) {
		t.Error("lambda should be valid for aws")
	}
	if IsValidBackend(CloudAWS, BackendCloudRun) {
		t.Error("cloudrun should not be valid for aws")
	}
}

func TestSimulatorBinary(t *testing.T) {
	tests := []struct {
		cloud    CloudType
		expected string
	}{
		{CloudAWS, "simulator-aws"},
		{CloudGCP, "simulator-gcp"},
		{CloudAzure, "simulator-azure"},
		{"invalid", ""},
	}
	for _, tt := range tests {
		if got := SimulatorBinary(tt.cloud); got != tt.expected {
			t.Errorf("SimulatorBinary(%s) = %s, want %s", tt.cloud, got, tt.expected)
		}
	}
}

func TestBackendBinary(t *testing.T) {
	tests := []struct {
		backend  BackendType
		expected string
	}{
		{BackendECS, "sockerless-backend-ecs"},
		{BackendLambda, "sockerless-backend-lambda"},
		{BackendCloudRun, "sockerless-backend-cloudrun"},
		{BackendGCF, "sockerless-backend-gcf"},
		{BackendACA, "sockerless-backend-aca"},
		{BackendAZF, "sockerless-backend-azf"},
		{"invalid", ""},
	}
	for _, tt := range tests {
		if got := BackendBinary(tt.backend); got != tt.expected {
			t.Errorf("BackendBinary(%s) = %s, want %s", tt.backend, got, tt.expected)
		}
	}
}

func TestProcessNames(t *testing.T) {
	sim, backend, frontend := processNames("myapp")
	if sim != "proj-myapp-sim" {
		t.Errorf("sim = %s, want proj-myapp-sim", sim)
	}
	if backend != "proj-myapp-backend" {
		t.Errorf("backend = %s, want proj-myapp-backend", backend)
	}
	if frontend != "proj-myapp-frontend" {
		t.Errorf("frontend = %s, want proj-myapp-frontend", frontend)
	}
}

func TestProjectManagerCreateGetListDelete(t *testing.T) {
	storeDir := t.TempDir()
	reg := NewRegistry()
	pm := NewProcessManager(reg)
	projMgr := NewProjectManager(pm, reg, storeDir)

	// Create
	err := projMgr.Create(ProjectConfig{
		Name:    "test-aws",
		Cloud:   CloudAWS,
		Backend: BackendECS,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get
	status, ok := projMgr.Get("test-aws")
	if !ok {
		t.Fatal("project not found after create")
	}
	if status.Cloud != CloudAWS {
		t.Errorf("cloud = %s, want aws", status.Cloud)
	}
	if status.Backend != BackendECS {
		t.Errorf("backend = %s, want ecs", status.Backend)
	}
	if status.Status != "stopped" {
		t.Errorf("status = %s, want stopped", status.Status)
	}
	if status.SimPort == 0 {
		t.Error("expected auto-assigned sim port")
	}
	if status.BackendPort == 0 {
		t.Error("expected auto-assigned backend port")
	}
	if status.FrontendPort == 0 {
		t.Error("expected auto-assigned frontend port")
	}
	if status.FrontendMgmtPort == 0 {
		t.Error("expected auto-assigned frontend mgmt port")
	}

	// List
	list := projMgr.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 project, got %d", len(list))
	}
	if list[0].Name != "test-aws" {
		t.Errorf("list[0].Name = %s, want test-aws", list[0].Name)
	}

	// Delete
	if err := projMgr.Delete("test-aws"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, ok = projMgr.Get("test-aws")
	if ok {
		t.Error("project should be gone after delete")
	}
	list = projMgr.List()
	if len(list) != 0 {
		t.Errorf("expected 0 projects after delete, got %d", len(list))
	}
}

func TestProjectManagerCreateDuplicate(t *testing.T) {
	pm := NewProcessManager(nil)
	projMgr := NewProjectManager(pm, nil, "")

	_ = projMgr.Create(ProjectConfig{
		Name:    "dup",
		Cloud:   CloudAWS,
		Backend: BackendECS,
	})

	err := projMgr.Create(ProjectConfig{
		Name:    "dup",
		Cloud:   CloudAWS,
		Backend: BackendECS,
	})
	if err == nil {
		t.Error("expected error for duplicate project")
	}
}

func TestProjectManagerCreateInvalidCloud(t *testing.T) {
	pm := NewProcessManager(nil)
	projMgr := NewProjectManager(pm, nil, "")

	err := projMgr.Create(ProjectConfig{
		Name:    "bad",
		Cloud:   "invalid",
		Backend: BackendECS,
	})
	if err == nil {
		t.Error("expected error for invalid cloud")
	}
}

func TestProjectManagerCreateInvalidBackend(t *testing.T) {
	pm := NewProcessManager(nil)
	projMgr := NewProjectManager(pm, nil, "")

	err := projMgr.Create(ProjectConfig{
		Name:    "bad",
		Cloud:   CloudAWS,
		Backend: BackendCloudRun,
	})
	if err == nil {
		t.Error("expected error for invalid backend/cloud combination")
	}
}

func TestProjectManagerConnection(t *testing.T) {
	pm := NewProcessManager(nil)
	projMgr := NewProjectManager(pm, nil, "")

	_ = projMgr.Create(ProjectConfig{
		Name:    "conn-test",
		Cloud:   CloudGCP,
		Backend: BackendCloudRun,
	})

	conn, err := projMgr.Connection("conn-test")
	if err != nil {
		t.Fatalf("Connection failed: %v", err)
	}

	status, _ := projMgr.Get("conn-test")
	expectedHost := "tcp://localhost:" + itoa(status.FrontendPort)
	if conn.DockerHost != expectedHost {
		t.Errorf("DockerHost = %s, want %s", conn.DockerHost, expectedHost)
	}
	if conn.SimulatorAddr == "" {
		t.Error("expected non-empty simulator addr")
	}
}

func TestProjectManagerConnectionNotFound(t *testing.T) {
	pm := NewProcessManager(nil)
	projMgr := NewProjectManager(pm, nil, "")

	_, err := projMgr.Connection("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent project")
	}
}

func TestProjectManagerRemoveProcess(t *testing.T) {
	pm := NewProcessManager(nil)
	pm.AddProcess(ProcessConfig{Name: "test", Binary: "sleep", Args: []string{"1"}, Type: "backend"})

	if err := pm.RemoveProcess("test"); err != nil {
		t.Fatalf("RemoveProcess failed: %v", err)
	}

	_, ok := pm.Get("test")
	if ok {
		t.Error("process should be removed")
	}
}

func TestProjectManagerRemoveProcessNotFound(t *testing.T) {
	pm := NewProcessManager(nil)
	err := pm.RemoveProcess("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent process")
	}
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
