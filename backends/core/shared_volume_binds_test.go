package core

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

var testBindPolicy = HostBindPolicy{
	Platform: "Amazon ECS",
	Backing:  "sockerless-managed EFS access points",
	EnvVar:   "SOCKERLESS_ECS_SHARED_VOLUMES",
}

func TestTranslateSharedVolumeBinds(t *testing.T) {
	vols := SharedVolumes{
		{Name: "ws", ContainerPath: "/home/runner/_work"},
		{Name: "ext", ContainerPath: "/home/runner/externals"},
	}

	t.Run("mapped bind rewrites to named volume, keeping the mode", func(t *testing.T) {
		translated, dropped, err := TranslateSharedVolumeBinds(vols, []string{
			"/home/runner/_work:/__w",
			"/home/runner/externals:/__e:ro",
			"/home/runner/_work:/__w2:ro",
		}, testBindPolicy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"ws:/__w", "ext:/__e:ro", "ws:/__w2:ro"}; !reflect.DeepEqual(translated, want) {
			t.Errorf("translated = %v, want %v", translated, want)
		}
		if len(dropped) != 0 {
			t.Errorf("dropped = %v, want none", dropped)
		}
	})

	t.Run("sub-path binds drop", func(t *testing.T) {
		translated, dropped, err := TranslateSharedVolumeBinds(vols, []string{
			"/home/runner/_work/_temp:/__w/_temp",
			"/home/runner/_work/_temp/_github_home:/github/home",
		}, testBindPolicy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(translated) != 0 || len(dropped) != 2 {
			t.Errorf("translated=%v dropped=%v, want both sub-paths dropped", translated, dropped)
		}
	})

	t.Run("docker.sock drops", func(t *testing.T) {
		translated, dropped, err := TranslateSharedVolumeBinds(vols, []string{"/var/run/docker.sock:/var/run/docker.sock"}, testBindPolicy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(translated) != 0 || len(dropped) != 1 {
			t.Errorf("translated=%v dropped=%v, want docker.sock dropped", translated, dropped)
		}
	})

	t.Run("named volume passes through", func(t *testing.T) {
		translated, _, err := TranslateSharedVolumeBinds(vols, []string{"cache:/cache:ro", "data:/data"}, testBindPolicy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"cache:/cache:ro", "data:/data"}; !reflect.DeepEqual(translated, want) {
			t.Errorf("translated = %v, want passthrough", translated)
		}
	})

	t.Run("unmapped host bind rejects with the platform and the configure hint", func(t *testing.T) {
		for _, bind := range []string{"/not/mapped:/x", "./rel:/x", "/home/runner/_workspace:/x"} {
			_, _, err := TranslateSharedVolumeBinds(vols, []string{bind}, testBindPolicy)
			if err == nil {
				t.Fatalf("want rejection for %q", bind)
			}
			var ipe *api.InvalidParameterError
			if !errors.As(err, &ipe) {
				t.Fatalf("error %T, want *api.InvalidParameterError", err)
			}
			for _, want := range []string{"Amazon ECS", "SOCKERLESS_ECS_SHARED_VOLUMES", "sockerless-managed EFS access points", bind} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		}
	})

	t.Run("rejection with no shared volumes still names the variable", func(t *testing.T) {
		_, _, err := TranslateSharedVolumeBinds(nil, []string{"/h:/c"}, testBindPolicy)
		if err == nil || !strings.Contains(err.Error(), testBindPolicy.EnvVar) {
			t.Errorf("error %v does not mention %s", err, testBindPolicy.EnvVar)
		}
	})

	t.Run("invalid bind spec rejects", func(t *testing.T) {
		_, _, err := TranslateSharedVolumeBinds(vols, []string{"junk"}, testBindPolicy)
		var ipe *api.InvalidParameterError
		if !errors.As(err, &ipe) {
			t.Fatalf("error %v, want *api.InvalidParameterError", err)
		}
	})
}

func TestIsHostBindSource(t *testing.T) {
	for src, want := range map[string]bool{"/abs": true, "./rel": true, "..": true, "vol": false, "my-vol_1": false, "": false} {
		if got := IsHostBindSource(src); got != want {
			t.Errorf("IsHostBindSource(%q) = %v, want %v", src, got, want)
		}
	}
}
