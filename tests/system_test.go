package tests

import (
	"archive/tar"
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPing(t *testing.T) {
	resp, err := http.Get("http://" + serverAddr + "/_ping")
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
	resp, err := http.Head("http://" + serverAddr + "/_ping")
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
	resp, err := http.Get("http://" + serverAddr + "/v1.44/_ping")
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
	// The e2e harness runs the ECS backend with no CodeBuild project
	// configured, so /build returns NotImplementedError naming the missing
	// prerequisite. Real Docker behavior: a backend that can't build is
	// expected to fail this call cleanly, not produce a synthetic image.
	// Asserting the error contract here ensures the no-fakes guarantee
	// (BUG-822) stays honored as the build path is refactored.
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
		"http://"+serverAddr+"/v1.44/build?t=test-build:latest&dockerfile=Dockerfile",
		"application/x-tar",
		&buf,
	)
	if err != nil {
		t.Fatalf("build request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotImplemented {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 501 (no cloud build service configured), got %d: %s", resp.StatusCode, body)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "cloud build service") {
		t.Errorf("expected error to name the missing build service, got: %s", body)
	}
}
