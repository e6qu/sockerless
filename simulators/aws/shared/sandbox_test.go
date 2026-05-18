package simulator

import (
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
)

// TestSandboxApplies covers the core invariant: every profile sets
// the privileged + cap deny bits correctly + the user when configured.
// Regression guard for BUG-1077.
func TestSandboxApplies(t *testing.T) {
	cases := []struct {
		name           string
		profile        SandboxProfile
		wantPrivileged bool
		wantReadonly   bool
		wantUser       string
		wantDropALL    bool
		wantNoNewPriv  bool
	}{
		{"lambda", SandboxLambda, false, true, "1051:1051", true, true},
		{"fargate", SandboxFargate, false, false, "", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hc := &container.HostConfig{}
			cc := &container.Config{}
			if err := c.profile.Apply(hc, cc); err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if hc.Privileged != c.wantPrivileged {
				t.Errorf("Privileged = %v, want %v", hc.Privileged, c.wantPrivileged)
			}
			if hc.ReadonlyRootfs != c.wantReadonly {
				t.Errorf("ReadonlyRootfs = %v, want %v", hc.ReadonlyRootfs, c.wantReadonly)
			}
			if cc.User != c.wantUser {
				t.Errorf("User = %q, want %q", cc.User, c.wantUser)
			}
			gotDropAll := false
			for _, cap := range hc.CapDrop {
				if cap == "ALL" {
					gotDropAll = true
				}
			}
			if gotDropAll != c.wantDropALL {
				t.Errorf("CapDrop contains ALL = %v, want %v (CapDrop=%v)", gotDropAll, c.wantDropALL, hc.CapDrop)
			}
			gotNoNewPriv := false
			for _, opt := range hc.SecurityOpt {
				if opt == "no-new-privileges" {
					gotNoNewPriv = true
				}
			}
			if gotNoNewPriv != c.wantNoNewPriv {
				t.Errorf("no-new-privileges = %v, want %v", gotNoNewPriv, c.wantNoNewPriv)
			}
		})
	}
}

func TestSandboxDenyHostNetwork(t *testing.T) {
	hc := &container.HostConfig{NetworkMode: "host"}
	err := SandboxLambda.Apply(hc, &container.Config{})
	if err == nil {
		t.Fatal("Apply must reject NetworkMode=host")
	}
	if !errors.Is(err, errSandboxHostNet) {
		t.Errorf("err = %v, want errSandboxHostNet", err)
	}
}

func TestSandboxDenyDockerSocket(t *testing.T) {
	cases := []string{
		"/var/run/docker.sock:/var/run/docker.sock",
		"/var/run/docker.sock:/host/docker.sock:ro",
		"/run/docker.sock:/run/docker.sock",
	}
	for _, bind := range cases {
		hc := &container.HostConfig{Binds: []string{bind}}
		err := SandboxLambda.Apply(hc, &container.Config{})
		if err == nil {
			t.Errorf("Apply must reject bind %q", bind)
			continue
		}
		if !errors.Is(err, errSandboxDockerSock) {
			t.Errorf("bind %q: err = %v, want errSandboxDockerSock", bind, err)
		}
	}
}

func TestSandboxPreservesExistingUser(t *testing.T) {
	// If the caller already set User, the profile shouldn't override
	// (Fargate, Cloud Run, ACA all let the image's USER win).
	hc := &container.HostConfig{}
	cc := &container.Config{User: "appuser:appgroup"}
	if err := SandboxFargate.Apply(hc, cc); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if cc.User != "appuser:appgroup" {
		t.Errorf("User overridden to %q, want preserved", cc.User)
	}
}
