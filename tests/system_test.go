package tests

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPing(t *testing.T) {
	resp, err := http.Get("http://" + frontendAddr + "/_ping")
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK" {
		t.Errorf("expected OK, got %q", string(body))
	}

	if v := resp.Header.Get("API-Version"); v != "1.44" {
		t.Errorf("expected API-Version 1.44, got %q", v)
	}
}

func TestPingHead(t *testing.T) {
	resp, err := http.Head("http://" + frontendAddr + "/_ping")
	if err != nil {
		t.Fatalf("ping HEAD failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	if v := resp.Header.Get("API-Version"); v != "1.44" {
		t.Errorf("expected API-Version 1.44, got %q", v)
	}
}

func TestVersion(t *testing.T) {
	ver, err := dockerClient.ServerVersion(ctx)
	if err != nil {
		t.Fatalf("version failed: %v", err)
	}

	if ver.APIVersion != "1.44" {
		t.Errorf("expected APIVersion 1.44, got %q", ver.APIVersion)
	}

	if ver.Version == "" {
		t.Error("expected non-empty Version")
	}
}

func TestInfo(t *testing.T) {
	info, err := dockerClient.Info(ctx)
	if err != nil {
		t.Fatalf("info failed: %v", err)
	}

	if info.ID == "" {
		t.Error("expected non-empty ID")
	}

	if info.Name == "" {
		t.Error("expected non-empty Name")
	}

	if info.OSType == "" {
		t.Error("expected non-empty OSType")
	}

	if info.Driver == "" {
		t.Error("expected non-empty Driver")
	}
}

func TestVersionedPing(t *testing.T) {
	resp, err := http.Get("http://" + frontendAddr + "/v1.44/_ping")
	if err != nil {
		t.Fatalf("versioned ping failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK" {
		t.Errorf("expected OK, got %q", string(body))
	}
}

func TestImageBuild(t *testing.T) {
	// Create a minimal tar with a Dockerfile
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	dockerfile := []byte("FROM alpine\nENTRYPOINT [\"echo\"]\n")
	_ = tw.WriteHeader(&tar.Header{
		Name: "Dockerfile",
		Size: int64(len(dockerfile)),
		Mode: 0644,
	})
	_, _ = tw.Write(dockerfile)
	_ = tw.Close()

	resp, err := http.Post(
		"http://"+frontendAddr+"/v1.44/build?t=test-build:latest&dockerfile=Dockerfile",
		"application/x-tar",
		&buf,
	)
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Read streaming JSON, verify stream and aux messages
	body, _ := io.ReadAll(resp.Body)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	hasStream := false
	hasAux := false
	for _, line := range lines {
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if _, ok := msg["stream"]; ok {
			hasStream = true
		}
		if _, ok := msg["aux"]; ok {
			hasAux = true
		}
	}
	if !hasStream {
		t.Error("expected at least one stream message in build output")
	}
	if !hasAux {
		t.Error("expected aux message with image ID in build output")
	}

	// Verify the image exists and has correct entrypoint
	img, _, err := dockerClient.ImageInspectWithRaw(ctx, "test-build:latest")
	if err != nil {
		t.Fatalf("image inspect failed: %v", err)
	}
	if len(img.Config.Entrypoint) != 1 || img.Config.Entrypoint[0] != "echo" {
		t.Errorf("entrypoint = %v, want [echo]", img.Config.Entrypoint)
	}
}
