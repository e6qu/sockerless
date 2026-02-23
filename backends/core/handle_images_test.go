package core

import (
	"encoding/base64"
	"testing"
)

func TestDecodeRegistryAuth_Valid(t *testing.T) {
	payload := `{"username":"myuser","password":"mypass"}`
	encoded := base64.URLEncoding.EncodeToString([]byte(payload))
	user, pass := decodeRegistryAuth(encoded)
	if user != "myuser" || pass != "mypass" {
		t.Errorf("expected myuser:mypass, got %q:%q", user, pass)
	}

	// Also works with StdEncoding
	encoded2 := base64.StdEncoding.EncodeToString([]byte(payload))
	user2, pass2 := decodeRegistryAuth(encoded2)
	if user2 != "myuser" || pass2 != "mypass" {
		t.Errorf("StdEncoding: expected myuser:mypass, got %q:%q", user2, pass2)
	}
}

func TestDecodeRegistryAuth_Invalid(t *testing.T) {
	user, pass := decodeRegistryAuth("not-valid-base64!!!")
	if user != "" || pass != "" {
		t.Errorf("expected empty for invalid input, got %q:%q", user, pass)
	}
}

func TestDecodeRegistryAuth_Empty(t *testing.T) {
	user, pass := decodeRegistryAuth("")
	if user != "" || pass != "" {
		t.Errorf("expected empty for empty input, got %q:%q", user, pass)
	}
}
