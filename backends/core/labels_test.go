package core

import "testing"

func TestLabelsEnvRoundTrip(t *testing.T) {
	labels := map[string]string{"app": "web", "env": "prod"}
	v, ok := EncodeLabelsEnvValue(labels)
	if !ok {
		t.Fatal("expected ok for non-empty labels")
	}
	got, err := DecodeLabelsEnvValue(v)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["app"] != "web" || got["env"] != "prod" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// Via the env-slice scanner.
	got2, err := LabelsFromEnvSlice([]string{"FOO=bar", LabelsEnvVar + "=" + v, "BAZ=qux"})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got2["app"] != "web" {
		t.Fatalf("scan mismatch: %+v", got2)
	}
}

func TestLabelsEnvAbsentAndEmpty(t *testing.T) {
	if _, ok := EncodeLabelsEnvValue(nil); ok {
		t.Error("expected ok=false for nil labels")
	}
	if got, err := DecodeLabelsEnvValue(""); got != nil || err != nil {
		t.Errorf("empty value should be (nil,nil), got %+v err=%v", got, err)
	}
	if got, err := LabelsFromEnvSlice([]string{"A=1", "B=2"}); got != nil || err != nil {
		t.Errorf("absent var should be (nil,nil), got %+v err=%v", got, err)
	}
}

func TestLabelsEnvMalformedSurfaces(t *testing.T) {
	// Present-but-undecodable must surface (no silent empty reconstruction).
	if _, err := DecodeLabelsEnvValue("!!!not-base64!!!"); err == nil {
		t.Error("expected error for bad base64")
	}
	// Valid base64 of non-JSON.
	if _, err := DecodeLabelsEnvValue("Zm9vYmFy"); err == nil { // "foobar"
		t.Error("expected error for non-JSON payload")
	}
}
