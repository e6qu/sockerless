package bleephub

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"time"
)

// signAppJWT creates an RS256 JWT for use in tests.
// Lives here (not in gh_apps_jwt.go) so the production binary doesn't
// carry test-only signing logic.
func signAppJWT(privateKeyPEM string, appID int, now time.Time) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM")
	}
	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	header := testBase64urlEncode([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := fmt.Sprintf(`{"iss":"%d","iat":%d,"exp":%d}`, appID, now.Unix(), now.Unix()+600)
	payloadEnc := testBase64urlEncode([]byte(payload))

	signInput := header + "." + payloadEnc
	hash := sha256.Sum256([]byte(signInput))
	sig, err := rsa.SignPKCS1v15(nil, privKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign: %w", err)
	}

	return signInput + "." + testBase64urlEncode(sig), nil
}

// testBase64urlEncode encodes bytes as unpadded base64url (test-only helper).
func testBase64urlEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
