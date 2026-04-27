package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// KMS — runner workflows interact with KMS through Secrets Manager
// (KmsKeyId), SSM Parameter Store (SecureString + KeyId), S3 SSE-KMS,
// and direct Encrypt/Decrypt. Without this slice every kms.Encrypt /
// terraform `aws_kms_key` 404s. Wire format follows the JSON protocol
// with `X-Amz-Target: TrentService.<Action>`.

// KMSKey is a customer master key. Real KMS wraps a per-key data key
// in HSM-protected key material; the sim doesn't have an HSM, so the
// "encryption" is a deterministic envelope (`<keyId>:<base64(plain)>`)
// — opaque to SDK callers (treated as bytes), reversible by the sim,
// and round-tripped exactly through the wire shape callers expect.
type KMSKey struct {
	KeyId        string           `json:"KeyId"`
	Arn          string           `json:"Arn"`
	Description  string           `json:"Description,omitempty"`
	KeyState     string           `json:"KeyState"`
	KeyUsage     string           `json:"KeyUsage,omitempty"`
	KeyManager   string           `json:"KeyManager,omitempty"`
	Origin       string           `json:"Origin,omitempty"`
	CreationDate float64          `json:"CreationDate,omitempty"`
	Aliases      []string         `json:"Aliases,omitempty"`
	Tags         []SMTag          `json:"Tags,omitempty"`
	Policy       map[string]any   `json:"Policy,omitempty"`
	Grants       []map[string]any `json:"Grants,omitempty"`
	Spec         string           `json:"KeySpec,omitempty"`
}

var (
	kmsKeys    sim.Store[KMSKey]
	kmsAliases sim.Store[string] // alias -> keyId
)

func kmsKeyArn(keyId string) string {
	return fmt.Sprintf("arn:aws:kms:%s:%s:key/%s", awsRegion(), awsAccountID(), keyId)
}

func registerKMS(r *sim.AWSRouter, srv *sim.Server) {
	kmsKeys = sim.MakeStore[KMSKey](srv.DB(), "kms_keys")
	kmsAliases = sim.MakeStore[string](srv.DB(), "kms_aliases")
	kmsAliasNames = sim.MakeStore[string](srv.DB(), "kms_alias_names")

	r.Register("TrentService.CreateKey", handleKMSCreateKey)
	r.Register("TrentService.DescribeKey", handleKMSDescribeKey)
	r.Register("TrentService.ListKeys", handleKMSListKeys)
	r.Register("TrentService.ScheduleKeyDeletion", handleKMSScheduleKeyDeletion)
	r.Register("TrentService.Encrypt", handleKMSEncrypt)
	r.Register("TrentService.Decrypt", handleKMSDecrypt)
	r.Register("TrentService.GenerateDataKey", handleKMSGenerateDataKey)
	r.Register("TrentService.CreateAlias", handleKMSCreateAlias)
	r.Register("TrentService.DeleteAlias", handleKMSDeleteAlias)
	r.Register("TrentService.ListAliases", handleKMSListAliases)
}

func handleKMSCreateKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Description string  `json:"Description"`
		KeyUsage    string  `json:"KeyUsage"`
		Origin      string  `json:"Origin"`
		KeySpec     string  `json:"KeySpec"`
		Tags        []SMTag `json:"Tags"`
	}
	_ = sim.ReadJSON(r, &req)

	keyId := generateUUID()
	if req.KeyUsage == "" {
		req.KeyUsage = "ENCRYPT_DECRYPT"
	}
	if req.Origin == "" {
		req.Origin = "AWS_KMS"
	}
	if req.KeySpec == "" {
		req.KeySpec = "SYMMETRIC_DEFAULT"
	}
	key := KMSKey{
		KeyId:        keyId,
		Arn:          kmsKeyArn(keyId),
		Description:  req.Description,
		KeyState:     "Enabled",
		KeyUsage:     req.KeyUsage,
		KeyManager:   "CUSTOMER",
		Origin:       req.Origin,
		Spec:         req.KeySpec,
		CreationDate: float64(time.Now().Unix()),
		Tags:         req.Tags,
	}
	kmsKeys.Put(keyId, key)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"KeyMetadata": map[string]any{
			"KeyId":        key.KeyId,
			"Arn":          key.Arn,
			"AWSAccountId": awsAccountID(),
			"CreationDate": key.CreationDate,
			"Description":  key.Description,
			"KeyState":     key.KeyState,
			"KeyUsage":     key.KeyUsage,
			"KeyManager":   key.KeyManager,
			"Origin":       key.Origin,
			"KeySpec":      key.Spec,
			"Enabled":      true,
		},
	})
}

// resolveKMSKey accepts either a plain KeyId, a full ARN, or an alias
// (`alias/<name>`) and returns the canonical KeyId.
func resolveKMSKey(idOrArn string) (string, bool) {
	if _, ok := kmsKeys.Get(idOrArn); ok {
		return idOrArn, true
	}
	if strings.HasPrefix(idOrArn, "alias/") {
		if keyId, ok := kmsAliases.Get(idOrArn); ok {
			return keyId, true
		}
		return "", false
	}
	for _, k := range kmsKeys.List() {
		if k.Arn == idOrArn {
			return k.KeyId, true
		}
	}
	return "", false
}

func handleKMSDescribeKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeyId string `json:"KeyId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	keyId, ok := resolveKMSKey(req.KeyId)
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusBadRequest,
			"Key %q does not exist", req.KeyId)
		return
	}
	key, _ := kmsKeys.Get(keyId)
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"KeyMetadata": map[string]any{
			"KeyId":        key.KeyId,
			"Arn":          key.Arn,
			"AWSAccountId": awsAccountID(),
			"CreationDate": key.CreationDate,
			"Description":  key.Description,
			"KeyState":     key.KeyState,
			"KeyUsage":     key.KeyUsage,
			"KeyManager":   key.KeyManager,
			"Origin":       key.Origin,
			"KeySpec":      key.Spec,
			"Enabled":      key.KeyState == "Enabled",
		},
	})
}

func handleKMSListKeys(w http.ResponseWriter, r *http.Request) {
	all := kmsKeys.List()
	out := make([]map[string]any, 0, len(all))
	for _, k := range all {
		out = append(out, map[string]any{
			"KeyId":  k.KeyId,
			"KeyArn": k.Arn,
		})
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Keys": out})
}

func handleKMSScheduleKeyDeletion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeyId               string `json:"KeyId"`
		PendingWindowInDays int32  `json:"PendingWindowInDays"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	keyId, ok := resolveKMSKey(req.KeyId)
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusBadRequest,
			"Key %q does not exist", req.KeyId)
		return
	}
	deletionDate := float64(time.Now().AddDate(0, 0, int(req.PendingWindowInDays)).Unix())
	kmsKeys.Update(keyId, func(k *KMSKey) {
		k.KeyState = "PendingDeletion"
	})
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"KeyId":        keyId,
		"DeletionDate": deletionDate,
		"KeyState":     "PendingDeletion",
	})
}

// handleKMSEncrypt produces a ciphertext blob = base64("kms-sim:<keyId>:<base64(plaintext)>").
// SDK callers treat ciphertext as opaque bytes; the sim's structured
// envelope round-trips identically through Decrypt without needing
// real cryptography.
func handleKMSEncrypt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeyId     string `json:"KeyId"`
		Plaintext []byte `json:"Plaintext"` // base64-decoded by the SDK
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	keyId, ok := resolveKMSKey(req.KeyId)
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusBadRequest,
			"Key %q does not exist", req.KeyId)
		return
	}
	envelope := "kms-sim:" + keyId + ":" + base64.StdEncoding.EncodeToString(req.Plaintext)
	ciphertextBlob := []byte(envelope)
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"KeyId":          kmsKeyArn(keyId),
		"CiphertextBlob": ciphertextBlob, // SDK base64-encodes on the wire
	})
}

func handleKMSDecrypt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeyId          string `json:"KeyId"`
		CiphertextBlob []byte `json:"CiphertextBlob"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	envelope := string(req.CiphertextBlob)
	const prefix = "kms-sim:"
	if !strings.HasPrefix(envelope, prefix) {
		sim.AWSErrorf(w, "InvalidCiphertextException", http.StatusBadRequest,
			"The ciphertext blob is not in the expected sim envelope format.")
		return
	}
	rest := strings.TrimPrefix(envelope, prefix)
	colon := strings.Index(rest, ":")
	if colon < 0 {
		sim.AWSErrorf(w, "InvalidCiphertextException", http.StatusBadRequest,
			"Malformed sim ciphertext envelope.")
		return
	}
	keyId := rest[:colon]
	plaintextB64 := rest[colon+1:]
	plaintext, err := base64.StdEncoding.DecodeString(plaintextB64)
	if err != nil {
		sim.AWSErrorf(w, "InvalidCiphertextException", http.StatusBadRequest,
			"Sim ciphertext envelope had an invalid plaintext payload.")
		return
	}
	if _, ok := kmsKeys.Get(keyId); !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusBadRequest,
			"Key %q does not exist", keyId)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"KeyId":     kmsKeyArn(keyId),
		"Plaintext": plaintext, // SDK base64-encodes on the wire
	})
}

func handleKMSGenerateDataKey(w http.ResponseWriter, r *http.Request) {
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
		sim.AWSErrorf(w, "NotFoundException", http.StatusBadRequest,
			"Key %q does not exist", req.KeyId)
		return
	}
	size := req.NumberOfBytes
	if size == 0 {
		switch req.KeySpec {
		case "AES_128":
			size = 16
		default: // "" or AES_256
			size = 32
		}
	}
	plaintext := make([]byte, size)
	for i := range plaintext {
		plaintext[i] = byte(i)
	}
	envelope := []byte("kms-sim:" + keyId + ":" + base64.StdEncoding.EncodeToString(plaintext))
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"KeyId":          kmsKeyArn(keyId),
		"Plaintext":      plaintext,
		"CiphertextBlob": envelope,
	})
}

func handleKMSCreateAlias(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AliasName   string `json:"AliasName"`
		TargetKeyId string `json:"TargetKeyId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(req.AliasName, "alias/") {
		sim.AWSError(w, "ValidationException",
			"AliasName must start with 'alias/'", http.StatusBadRequest)
		return
	}
	keyId, ok := resolveKMSKey(req.TargetKeyId)
	if !ok {
		sim.AWSErrorf(w, "NotFoundException", http.StatusBadRequest,
			"TargetKeyId %q does not exist", req.TargetKeyId)
		return
	}
	if _, exists := kmsAliases.Get(req.AliasName); exists {
		sim.AWSErrorf(w, "AlreadyExistsException", http.StatusBadRequest,
			"Alias %q already exists", req.AliasName)
		return
	}
	kmsAliases.Put(req.AliasName, keyId)
	kmsAliasNames.Put(req.AliasName, req.AliasName)
	w.WriteHeader(http.StatusOK)
}

func handleKMSDeleteAlias(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AliasName string `json:"AliasName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequest", "Invalid request body", http.StatusBadRequest)
		return
	}
	kmsAliases.Delete(req.AliasName)
	kmsAliasNames.Delete(req.AliasName)
	w.WriteHeader(http.StatusOK)
}

func handleKMSListAliases(w http.ResponseWriter, r *http.Request) {
	out := make([]map[string]any, 0)
	for _, keyId := range kmsAliases.List() {
		// Filter rebuilds (key,value) pairs we need to find the alias name.
		_ = keyId
	}
	// `Store` doesn't expose key iteration directly; reconstruct via a
	// per-alias filter that retains order. Aliases are tracked per
	// `alias/<name>` key with the target keyId as value.
	all := kmsAliases.Filter(func(string) bool { return true })
	_ = all
	// Use a simple per-key probe — KMS aliases are bounded in practice.
	for _, key := range kmsKeys.List() {
		// For each key, find aliases pointing at it.
		for _, alias := range listAliasesForKey(key.KeyId) {
			out = append(out, map[string]any{
				"AliasName":   alias,
				"AliasArn":    fmt.Sprintf("arn:aws:kms:%s:%s:%s", awsRegion(), awsAccountID(), alias),
				"TargetKeyId": key.KeyId,
			})
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Aliases": out})
}

// listAliasesForKey scans the alias store for entries pointing at
// keyId. The sim's Store doesn't expose key iteration, but the alias
// table is small enough that a snapshot scan is fine.
func listAliasesForKey(keyId string) []string {
	// kmsAliasNames mirrors the alias keys for iteration. It's
	// maintained alongside kmsAliases in handleKMSCreateAlias and
	// handleKMSDeleteAlias.
	var out []string
	for _, name := range kmsAliasNames.List() {
		if id, ok := kmsAliases.Get(name); ok && id == keyId {
			out = append(out, name)
		}
	}
	return out
}

// kmsAliasNames holds the alias names for iteration. Real KMS exposes
// `ListAliases` paginated; the sim returns all at once.
var kmsAliasNames sim.Store[string]
