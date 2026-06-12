package cloudrun

import (
	"errors"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

func TestParseSharedVolumes(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		for _, in := range []string{"", "   ", " , "} {
			vols, err := parseSharedVolumes(in)
			if err != nil {
				t.Fatalf("parseSharedVolumes(%q) error: %v", in, err)
			}
			if len(vols) != 0 {
				t.Fatalf("parseSharedVolumes(%q) = %v, want empty", in, vols)
			}
		}
	})

	t.Run("valid 4-tuples", func(t *testing.T) {
		vols, err := parseSharedVolumes("ws=/home/runner/_work=ws-bucket=gcs-fuse, ext=/home/runner/externals=ext-bucket=gcs-sync")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(vols) != 2 {
			t.Fatalf("got %d volumes, want 2", len(vols))
		}
		if vols[0].Name != "ws" || vols[0].ContainerPath != "/home/runner/_work" || vols[0].Bucket != "ws-bucket" || vols[0].Backing != "gcs-fuse" {
			t.Errorf("vols[0] = %+v", vols[0])
		}
		if vols[1].Backing != "gcs-sync" {
			t.Errorf("vols[1] = %+v", vols[1])
		}
	})

	t.Run("malformed entries error", func(t *testing.T) {
		for _, in := range []string{
			"ws=/home/runner/_work=bucket",       // legacy 3-tuple (backing required)
			"ws=/p=bucket=gcs-fuse=extra",        // too many fields
			"ws=/home/runner/_work=bucket=",      // empty backing
			"=/p=bucket=gcs-fuse",                // empty name
			"ok=/a=b=gcs-fuse,ws=/home/runner=b", // one valid + one malformed
		} {
			if _, err := parseSharedVolumes(in); err == nil {
				t.Errorf("parseSharedVolumes(%q) = nil error, want parse error", in)
			}
		}
	})
}

func TestValidateSurfacesSharedVolumesParseError(t *testing.T) {
	cfg := Config{
		Project:          "p",
		NetworkDiscovery: api.NetworkDiscoveryCloudDNS,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("baseline config should validate, got: %v", err)
	}
	cfg.sharedVolumesErr = errors.New("entry \"junk\" malformed")
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want shared-volumes parse error")
	}
	if !strings.Contains(err.Error(), "SOCKERLESS_GCP_SHARED_VOLUMES") {
		t.Errorf("error %q does not mention SOCKERLESS_GCP_SHARED_VOLUMES", err)
	}
}

func TestTranslateSharedVolumeBinds(t *testing.T) {
	cfg := Config{
		SharedVolumes: []SharedVolume{
			{Name: "ws", ContainerPath: "/home/runner/_work", Bucket: "ws-bucket", Backing: "gcs-fuse"},
		},
	}

	t.Run("mapped bind rewrites to named volume", func(t *testing.T) {
		translated, dropped, err := translateSharedVolumeBinds(cfg, []string{
			"/home/runner/_work:/__w",
			"/home/runner/_work:/__w2:ro",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"ws:/__w", "ws:/__w2:ro"}
		if len(translated) != 2 || translated[0] != want[0] || translated[1] != want[1] {
			t.Errorf("translated = %v, want %v", translated, want)
		}
		if len(dropped) != 0 {
			t.Errorf("dropped = %v, want none", dropped)
		}
	})

	t.Run("sub-path binds drop", func(t *testing.T) {
		translated, dropped, err := translateSharedVolumeBinds(cfg, []string{"/home/runner/_work/_temp:/__w/_temp"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(translated) != 0 || len(dropped) != 1 {
			t.Errorf("translated=%v dropped=%v, want sub-path dropped", translated, dropped)
		}
	})

	t.Run("docker.sock drops", func(t *testing.T) {
		translated, dropped, err := translateSharedVolumeBinds(cfg, []string{"/var/run/docker.sock:/var/run/docker.sock"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(translated) != 0 || len(dropped) != 1 {
			t.Errorf("translated=%v dropped=%v, want docker.sock dropped", translated, dropped)
		}
	})

	t.Run("named volume passes through", func(t *testing.T) {
		translated, _, err := translateSharedVolumeBinds(cfg, []string{"cache:/cache"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(translated) != 1 || translated[0] != "cache:/cache" {
			t.Errorf("translated = %v, want passthrough", translated)
		}
	})

	t.Run("unmapped host bind rejects with configure hint", func(t *testing.T) {
		_, _, err := translateSharedVolumeBinds(cfg, []string{"/not/mapped:/x"})
		if err == nil {
			t.Fatal("want rejection for unmapped host bind")
		}
		var ipe *api.InvalidParameterError
		if !errors.As(err, &ipe) {
			t.Fatalf("error %T, want *api.InvalidParameterError", err)
		}
		if !strings.Contains(err.Error(), "SOCKERLESS_GCP_SHARED_VOLUMES") {
			t.Errorf("error %q does not mention SOCKERLESS_GCP_SHARED_VOLUMES", err)
		}
	})

	t.Run("invalid bind spec rejects", func(t *testing.T) {
		if _, _, err := translateSharedVolumeBinds(cfg, []string{"junk"}); err == nil {
			t.Fatal("want error for invalid bind spec")
		}
	})
}
