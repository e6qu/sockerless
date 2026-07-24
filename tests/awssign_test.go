package tests

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// signAWSControlPlane signs an AWS control-plane request (awsJson / awsQuery)
// with SigV4 exactly as a real AWS SDK client does, differing only in the
// coordinates: the endpoint the request already points at and the seeded
// bootstrap admin credential the simulator provisions (access key "test",
// secret "test", region us-east-1) — the same static credential the ECS
// backend uses (backends/ecs/aws.go). The simulator verifies this signature at
// its POST / control-plane chokepoint; an unsigned request is rejected with 403
// MissingAuthenticationToken, matching real AWS.
//
// body is the exact request body bytes; the caller must supply a fresh body
// reader on the request since signing consumes nothing but the hash is computed
// over these bytes.
func signAWSControlPlane(req *http.Request, body []byte, service string) error {
	creds := aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test"}
	payloadHash := hex.EncodeToString(sha256Sum(body))
	signer := v4.NewSigner()
	if err := signer.SignHTTP(context.Background(), creds, req, payloadHash, service, "us-east-1", time.Now()); err != nil {
		return fmt.Errorf("sigv4 sign %s: %w", service, err)
	}
	return nil
}

func sha256Sum(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}
