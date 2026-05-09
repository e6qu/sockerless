package core

import (
	"context"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

func TestResolveAccessDriver_NoneInternalDefault(t *testing.T) {
	d, err := ResolveAccessDriver(api.AccessMechanismNoneInternal, nil)
	if err != nil {
		t.Fatalf("none-internal: unexpected err %v", err)
	}
	if d.Mechanism() != api.AccessMechanismNoneInternal {
		t.Errorf("Mechanism() = %q, want %q", d.Mechanism(), api.AccessMechanismNoneInternal)
	}
	if d.WorkloadPrincipal() != "" {
		t.Errorf("WorkloadPrincipal() = %q, want empty", d.WorkloadPrincipal())
	}
	c, err := d.AuthenticatedClient(context.Background(), "https://x")
	if err != nil || c == nil {
		t.Errorf("AuthenticatedClient: got %v, %v; want non-nil client, nil err", c, err)
	}
}

func TestResolveAccessDriver_InvalidMechanism(t *testing.T) {
	_, err := ResolveAccessDriver(api.AccessMechanism(""), nil)
	if err == nil {
		t.Fatal("empty: expected error")
	}
	if !strings.Contains(err.Error(), "invalid mechanism") {
		t.Errorf("err = %v; want 'invalid mechanism'", err)
	}
	_, err = ResolveAccessDriver(api.AccessMechanism("garbage"), nil)
	if err == nil {
		t.Fatal("unknown: expected error")
	}
}

func TestResolveAccessDriver_NotRegistered(t *testing.T) {
	_, err := ResolveAccessDriver(api.AccessMechanismIDToken, nil)
	if err == nil {
		t.Fatal("unregistered: expected error")
	}
	if !strings.Contains(err.Error(), "no constructor registered") {
		t.Errorf("err = %v; want 'no constructor registered'", err)
	}
}

func TestParseAccessMechanismEnv(t *testing.T) {
	got, err := ParseAccessMechanismEnv("", api.AccessMechanismIAMRole)
	if err != nil || got != api.AccessMechanismIAMRole {
		t.Errorf("unset: got %q, err %v; want iam-role", got, err)
	}
	got, err = ParseAccessMechanismEnv("id-token", api.AccessMechanismIAMRole)
	if err != nil || got != api.AccessMechanismIDToken {
		t.Errorf("override: got %q, err %v", got, err)
	}
	_, err = ParseAccessMechanismEnv("garbage", api.AccessMechanismIAMRole)
	if err == nil {
		t.Fatal("invalid override: expected error")
	}
	got, err = ParseAccessMechanismEnv("  none-internal  ", api.AccessMechanismIDToken)
	if err != nil || got != api.AccessMechanismNoneInternal {
		t.Errorf("trimmed: got %q, err %v", got, err)
	}
}
