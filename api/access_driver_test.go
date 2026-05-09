package api

import "testing"

func TestAccessMechanism_IsValid(t *testing.T) {
	cases := []struct {
		in   AccessMechanism
		want bool
	}{
		{AccessMechanismIAMRole, true},
		{AccessMechanismIDToken, true},
		{AccessMechanismMTLS, true},
		{AccessMechanismNoneInternal, true},
		{AccessMechanism(""), false},
		{AccessMechanism("garbage"), false},
	}
	for _, tc := range cases {
		if got := tc.in.IsValid(); got != tc.want {
			t.Errorf("%q.IsValid() = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestAccessMechanism_String(t *testing.T) {
	if got := AccessMechanismIDToken.String(); got != "id-token" {
		t.Errorf("String() = %q, want %q", got, "id-token")
	}
}
