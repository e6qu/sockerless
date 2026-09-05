package core

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// dockerAuthConfig is the credential a Docker client sends in the
// X-Registry-Auth header: the `AuthConfig` of the Docker Engine API,
// base64-encoded. Field names are the wire's.
type dockerAuthConfig struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	Auth          string `json:"auth"`
	IdentityToken string `json:"identitytoken"`
	RegistryToken string `json:"registrytoken"`
}

// RegistryAuthorizationFromDockerAuth turns the credential a Docker client
// sends in `X-Registry-Auth` into the `Authorization` header value the
// registry accepts, so a pull or push carries the credential the user logged
// in with instead of reaching the registry anonymously.
//
// The header is the Engine API's `AuthConfig`, JSON in URL-safe base64. A
// username and password (or the pre-joined `auth` field) become a Basic
// credential, which `getRegistryToken` presents to the registry's token
// service the way the Docker CLI does; a `registrytoken` is already the
// registry's own Bearer. An `identitytoken` is an OAuth2 refresh token that
// can only be redeemed by a grant this client does not perform, and it is
// refused rather than silently downgraded to an anonymous read. A value that is
// already an `Authorization` header — a cloud AuthProvider's minted token — is
// returned unchanged; an empty or credential-less header means anonymous.
func RegistryAuthorizationFromDockerAuth(encoded string) (string, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return "", nil
	}
	if strings.HasPrefix(encoded, "Bearer ") || strings.HasPrefix(encoded, "Basic ") {
		return encoded, nil
	}
	raw, err := decodeDockerAuthHeader(encoded)
	if err != nil {
		return "", fmt.Errorf("X-Registry-Auth is not base64: %w", err)
	}
	var ac dockerAuthConfig
	if err := json.Unmarshal(raw, &ac); err != nil {
		return "", fmt.Errorf("X-Registry-Auth is not a Docker AuthConfig: %w", err)
	}
	switch {
	case ac.RegistryToken != "":
		return "Bearer " + ac.RegistryToken, nil
	case ac.IdentityToken != "":
		return "", errors.New("X-Registry-Auth carries an identity token, which only the registry's OAuth2 refresh grant can redeem; log in to the registry with a username and password or an access token")
	case ac.Username != "" && ac.Password != "":
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(ac.Username+":"+ac.Password)), nil
	case ac.Auth != "":
		return "Basic " + ac.Auth, nil
	}
	return "", nil
}

// decodeDockerAuthHeader decodes the header's base64. The Docker CLI writes
// URL-safe base64 and other clients standard base64, with or without
// padding; every one of those is one credential.
func decodeDockerAuthHeader(encoded string) ([]byte, error) {
	var firstErr error
	for _, enc := range []*base64.Encoding{base64.URLEncoding, base64.RawURLEncoding, base64.StdEncoding, base64.RawStdEncoding} {
		raw, err := enc.DecodeString(encoded)
		if err == nil {
			return raw, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, firstErr
}

// registryCredential converts an `Authorization` header value into the
// credential `FetchImageMetadata` and `getRegistryToken` take: the raw
// base64 of a Basic credential (the prefix is re-added at the point of use),
// a Bearer value unchanged, "" for anonymous.
func registryCredential(authorization string) string {
	if rest, ok := strings.CutPrefix(authorization, "Basic "); ok {
		return rest
	}
	return authorization
}
