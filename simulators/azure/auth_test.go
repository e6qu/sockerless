package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMintAzureSimJWT_StructureAndClaims(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok, err := mintAzureSimJWT("tenant-xyz", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q: want 3 segments, got %d", tok, len(parts))
	}
	if parts[2] == "" {
		t.Errorf("signature segment is empty (alg:none leak?)")
	}

	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("header decode: %v", err)
	}
	var h map[string]string
	if err := json.Unmarshal(header, &h); err != nil {
		t.Fatalf("header unmarshal: %v", err)
	}
	if h["alg"] != "RS256" {
		t.Errorf("alg = %q, want RS256 — alg:none is auth-bypass", h["alg"])
	}
	if h["kid"] == "" {
		t.Errorf("kid missing — clients that fetch JWKS rely on it")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("payload decode: %v", err)
	}
	var p map[string]any
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if p["tid"] != "tenant-xyz" {
		t.Errorf("tid = %v, want tenant-xyz", p["tid"])
	}
	if p["aud"] != "https://management.azure.com/" {
		t.Errorf("aud = %v, want management.azure.com", p["aud"])
	}
	if p["iss"] != "https://sts.windows.net/tenant-xyz/" {
		t.Errorf("iss = %v, want sts.windows.net/tenant-xyz/", p["iss"])
	}
	if iat, ok := p["iat"].(float64); !ok || int64(iat) != now.Unix() {
		t.Errorf("iat = %v, want %d", p["iat"], now.Unix())
	}
}

func TestMintAzureSimJWT_SignatureVerifies(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok, err := mintAzureSimJWT("tenant-xyz", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("want 3 segments, got %d", len(parts))
	}
	signingInput := parts[0] + "." + parts[1]

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("signature decode: %v", err)
	}
	key, err := azureSimSigningKey()
	if err != nil {
		t.Fatalf("signing key: %v", err)
	}
	digest := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Errorf("signature verify: %v", err)
	}
}

func TestAzureSimJWK_PublishesRS256PublicKey(t *testing.T) {
	jwk, err := azureSimJWK()
	if err != nil {
		t.Fatalf("jwk: %v", err)
	}
	if jwk["kty"] != "RSA" {
		t.Errorf("kty = %v, want RSA", jwk["kty"])
	}
	if jwk["alg"] != "RS256" {
		t.Errorf("alg = %v, want RS256", jwk["alg"])
	}
	if jwk["kid"] != "sockerless-sim-key-1" {
		t.Errorf("kid = %v, want sockerless-sim-key-1", jwk["kid"])
	}
	if jwk["n"] == "" || jwk["e"] == "" {
		t.Errorf("n/e missing from RSA JWK: %#v", jwk)
	}
}
