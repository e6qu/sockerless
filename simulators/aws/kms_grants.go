package main

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sort"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// KMS grants + secondary crypto ops (issue #434). AWS services (ECS, ECR,
// DynamoDB, S3) create grants on customer-managed CMKs to use them for
// encryption at rest; the Terraform `aws_kms_grant` resource is the explicit
// surface. GenerateDataKeyWithoutPlaintext and ReEncrypt are the remaining
// data-key/rotation crypto ops layered on the existing kms-sim envelope.

type KMSGrant struct {
	GrantId           string         `json:"GrantId"`
	KeyId             string         `json:"KeyId"`
	Name              string         `json:"Name,omitempty"`
	GranteePrincipal  string         `json:"GranteePrincipal"`
	RetiringPrincipal string         `json:"RetiringPrincipal,omitempty"`
	Operations        []string       `json:"Operations"`
	Constraints       map[string]any `json:"Constraints,omitempty"`
	CreationDate      float64        `json:"CreationDate"`
	IssuingAccount    string         `json:"IssuingAccount"`
}

var kmsGrants sim.Store[KMSGrant]

func registerKMSGrants(r *sim.AWSRouter, srv *sim.Server) {
	kmsGrants = sim.MakeStore[KMSGrant](srv.DB(), "kms_grants")
	r.Register("TrentService.CreateGrant", handleKMSCreateGrant)
	r.Register("TrentService.ListGrants", handleKMSListGrants)
	r.Register("TrentService.RevokeGrant", handleKMSRevokeGrant)
	r.Register("TrentService.GenerateDataKeyWithoutPlaintext", handleKMSGenerateDataKeyWithoutPlaintext)
	r.Register("TrentService.ReEncrypt", handleKMSReEncrypt)
}

func handleKMSCreateGrant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeyId             string         `json:"KeyId"`
		GranteePrincipal  string         `json:"GranteePrincipal"`
		RetiringPrincipal string         `json:"RetiringPrincipal"`
		Operations        []string       `json:"Operations"`
		Constraints       map[string]any `json:"Constraints"`
		Name              string         `json:"Name"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	keyId, ok := resolveKMSKey(req.KeyId)
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusBadRequest, "Key %q does not exist", req.KeyId)
		return
	}
	if req.GranteePrincipal == "" {
		sim.AWSError(w, "ValidationException", "GranteePrincipal is required", http.StatusBadRequest)
		return
	}
	grant := KMSGrant{
		GrantId:           generateUUID(),
		KeyId:             keyId,
		Name:              req.Name,
		GranteePrincipal:  req.GranteePrincipal,
		RetiringPrincipal: req.RetiringPrincipal,
		Operations:        req.Operations,
		Constraints:       req.Constraints,
		CreationDate:      float64(time.Now().Unix()),
		IssuingAccount:    "arn:aws:iam::" + awsAccountID() + ":root",
	}
	kmsGrants.Put(grant.GrantId, grant)
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"GrantId":    grant.GrantId,
		"GrantToken": kmsGrantToken(),
	})
}

func handleKMSListGrants(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeyId            string `json:"KeyId"`
		GrantId          string `json:"GrantId"`
		GranteePrincipal string `json:"GranteePrincipal"`
		Limit            int    `json:"Limit"`
		Marker           string `json:"Marker"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	keyId, ok := resolveKMSKey(req.KeyId)
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusBadRequest, "Key %q does not exist", req.KeyId)
		return
	}
	var matched []KMSGrant
	for _, g := range kmsGrants.List() {
		if g.KeyId != keyId {
			continue
		}
		if req.GrantId != "" && g.GrantId != req.GrantId {
			continue
		}
		if req.GranteePrincipal != "" && g.GranteePrincipal != req.GranteePrincipal {
			continue
		}
		matched = append(matched, g)
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].GrantId < matched[j].GrantId })
	page, next := awsPage(matched, req.Marker, req.Limit, 100)
	out := map[string]any{
		"Grants":    page,
		"Truncated": next != "",
	}
	if next != "" {
		out["NextMarker"] = next
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleKMSRevokeGrant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeyId   string `json:"KeyId"`
		GrantId string `json:"GrantId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	if _, ok := resolveKMSKey(req.KeyId); !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusBadRequest, "Key %q does not exist", req.KeyId)
		return
	}
	if _, ok := kmsGrants.Get(req.GrantId); !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusBadRequest, "Grant %q does not exist", req.GrantId)
		return
	}
	kmsGrants.Delete(req.GrantId)
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleKMSGenerateDataKeyWithoutPlaintext(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeyId         string `json:"KeyId"`
		NumberOfBytes int    `json:"NumberOfBytes"`
		KeySpec       string `json:"KeySpec"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	keyId, ok := resolveKMSKey(req.KeyId)
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusBadRequest, "Key %q does not exist", req.KeyId)
		return
	}
	size := req.NumberOfBytes
	if size == 0 {
		if req.KeySpec == "AES_128" {
			size = 16
		} else {
			size = 32
		}
	}
	plaintext := make([]byte, size)
	if _, err := rand.Read(plaintext); err != nil {
		sim.AWSError(w, "DependencyTimeoutException", "failed to generate random data key", http.StatusInternalServerError)
		return
	}
	// Real GenerateDataKeyWithoutPlaintext returns only the encrypted key — the
	// plaintext is never put on the wire.
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"KeyId":          kmsKeyArn(keyId),
		"CiphertextBlob": kmsEncryptEnvelope(keyId, plaintext),
	})
}

func handleKMSReEncrypt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CiphertextBlob   []byte `json:"CiphertextBlob"`
		DestinationKeyId string `json:"DestinationKeyId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	srcKeyId, plaintext, ok := kmsDecryptEnvelope(req.CiphertextBlob)
	if !ok {
		sim.AWSErrorf(w, "InvalidCiphertextException", http.StatusBadRequest,
			"The ciphertext blob is not in the expected sim envelope format.")
		return
	}
	if _, ok := kmsKeys.Get(srcKeyId); !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusBadRequest, "Source key %q does not exist", srcKeyId)
		return
	}
	destKeyId, ok := resolveKMSKey(req.DestinationKeyId)
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusBadRequest,
			"Destination key %q does not exist", req.DestinationKeyId)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"CiphertextBlob": kmsEncryptEnvelope(destKeyId, plaintext),
		"KeyId":          kmsKeyArn(destKeyId),
		"SourceKeyId":    kmsKeyArn(srcKeyId),
	})
}

// kmsEncryptEnvelope / kmsDecryptEnvelope mirror the kms-sim envelope format
// the existing Encrypt/Decrypt/GenerateDataKey handlers use:
// "kms-sim:<keyId>:<base64(plaintext)>".
func kmsEncryptEnvelope(keyId string, plaintext []byte) []byte {
	return []byte("kms-sim:" + keyId + ":" + base64.StdEncoding.EncodeToString(plaintext))
}

func kmsDecryptEnvelope(blob []byte) (keyId string, plaintext []byte, ok bool) {
	const prefix = "kms-sim:"
	envelope := string(blob)
	if !strings.HasPrefix(envelope, prefix) {
		return "", nil, false
	}
	rest := strings.TrimPrefix(envelope, prefix)
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return "", nil, false
	}
	pt, err := base64.StdEncoding.DecodeString(rest[colon+1:])
	if err != nil {
		return "", nil, false
	}
	return rest[:colon], pt, true
}

func kmsGrantToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}
