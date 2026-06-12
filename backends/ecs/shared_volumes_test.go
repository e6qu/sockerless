package ecs

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

	t.Run("valid 3- and 4-tuples", func(t *testing.T) {
		vols, err := parseSharedVolumes("ws=/home/runner/_work=fsap-123, ext=/home/runner/externals=fsap-456=fs-789")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(vols) != 2 {
			t.Fatalf("got %d volumes, want 2", len(vols))
		}
		if vols[0].Name != "ws" || vols[0].ContainerPath != "/home/runner/_work" || vols[0].AccessPointID != "fsap-123" || vols[0].FileSystemID != "" {
			t.Errorf("vols[0] = %+v", vols[0])
		}
		if vols[1].Name != "ext" || vols[1].ContainerPath != "/home/runner/externals" || vols[1].AccessPointID != "fsap-456" || vols[1].FileSystemID != "fs-789" {
			t.Errorf("vols[1] = %+v", vols[1])
		}
	})

	t.Run("malformed entries error", func(t *testing.T) {
		for _, in := range []string{
			"ws=/home/runner/_work",        // too few fields
			"ws=/p=fsap-1=fs-1=extra",      // too many fields
			"=/home/runner/_work=fsap-1",   // empty name
			"ws==fsap-1",                   // empty containerPath
			"ws=/home/runner/_work=",       // empty access point
			"ok=/a=fsap-1,ws=/home/runner", // one valid + one malformed
		} {
			if _, err := parseSharedVolumes(in); err == nil {
				t.Errorf("parseSharedVolumes(%q) = nil error, want parse error", in)
			}
		}
	})
}

func TestValidateSurfacesSharedVolumesParseError(t *testing.T) {
	cfg := Config{
		Cluster:          "c",
		Subnets:          []string{"subnet-1"},
		ExecutionRoleARN: "arn:aws:iam::000000000000:role/x",
		CpuArchitecture:  "ARM64",
		NetworkDiscovery: api.NetworkDiscoveryServiceMesh,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("baseline config should validate, got: %v", err)
	}
	cfg.sharedVolumesErr = errors.New("entry \"junk\" malformed")
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want shared-volumes parse error")
	}
	if !strings.Contains(err.Error(), "SOCKERLESS_ECS_SHARED_VOLUMES") {
		t.Errorf("error %q does not mention SOCKERLESS_ECS_SHARED_VOLUMES", err)
	}
}

func TestTranslateSharedVolumeBinds(t *testing.T) {
	cfg := Config{
		SharedVolumes: []SharedVolume{
			{Name: "ws", ContainerPath: "/home/runner/_work", AccessPointID: "fsap-1"},
			{Name: "ext", ContainerPath: "/home/runner/externals", AccessPointID: "fsap-2"},
		},
	}

	t.Run("mapped bind rewrites to named volume", func(t *testing.T) {
		translated, dropped, err := translateSharedVolumeBinds(cfg, []string{
			"/home/runner/_work:/__w",
			"/home/runner/externals:/__e:ro",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"ws:/__w", "ext:/__e:ro"}
		if len(translated) != len(want) || translated[0] != want[0] || translated[1] != want[1] {
			t.Errorf("translated = %v, want %v", translated, want)
		}
		if len(dropped) != 0 {
			t.Errorf("dropped = %v, want none", dropped)
		}
	})

	t.Run("sub-path binds drop", func(t *testing.T) {
		translated, dropped, err := translateSharedVolumeBinds(cfg, []string{
			"/home/runner/_work/_temp:/__w/_temp",
			"/home/runner/_work/_temp/_github_home:/github/home",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(translated) != 0 {
			t.Errorf("translated = %v, want none", translated)
		}
		if len(dropped) != 2 {
			t.Errorf("dropped = %v, want both sub-path binds", dropped)
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
		translated, _, err := translateSharedVolumeBinds(cfg, []string{"cache:/cache:ro"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(translated) != 1 || translated[0] != "cache:/cache:ro" {
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
		if !strings.Contains(err.Error(), "SOCKERLESS_ECS_SHARED_VOLUMES") {
			t.Errorf("error %q does not mention SOCKERLESS_ECS_SHARED_VOLUMES", err)
		}
	})

	t.Run("sibling path is not a sub-path", func(t *testing.T) {
		// `/home/runner/_workspace` shares a string prefix with the
		// mapped `/home/runner/_work` but is NOT under it — must reject.
		_, _, err := translateSharedVolumeBinds(cfg, []string{"/home/runner/_workspace:/x"})
		if err == nil {
			t.Fatal("want rejection for sibling path that only shares a string prefix")
		}
	})

	t.Run("invalid bind spec rejects", func(t *testing.T) {
		if _, _, err := translateSharedVolumeBinds(cfg, []string{"junk"}); err == nil {
			t.Fatal("want error for invalid bind spec")
		}
	})
}
