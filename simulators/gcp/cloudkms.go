package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Cloud KMS v1 slice (cloudkms.googleapis.com). Models keyRings,
// cryptoKeys, cryptoKeyVersions, and symmetric encrypt/decrypt so a
// key-management client (the google.golang.org/api/cloudkms REST client
// and `gcloud kms`) has a real endpoint to hit. AWS KMS and Azure Key
// Vault keys are already simulated; this brings GCP to parity.
//
// Crypto is real, not faked: each ENABLED cryptoKeyVersion gets a random
// AES-256 key (non-exportable, like real KMS), and encrypt/decrypt use
// AES-256-GCM. The ciphertext is opaque to the client and round-trips.
// Real API: https://cloud.google.com/kms/docs/reference/rest

const (
	kmsDefaultProtectionLevel = "SOFTWARE"
	kmsSymmetricAlgorithm     = "GOOGLE_SYMMETRIC_ENCRYPTION"
	kmsPurposeEncryptDecrypt  = "ENCRYPT_DECRYPT"
	// kmsDestroyScheduledDelay is how far in the future a destroyed
	// version's destroyTime is set (real KMS default is 24h–30d; the
	// sim uses 24h).
	kmsDestroyScheduledDelay = 24 * time.Hour
)

// kmsKeyRing is the wire shape for a KeyRing resource.
type kmsKeyRing struct {
	Name       string `json:"name"`
	CreateTime string `json:"createTime"`
}

// kmsCryptoKeyVersionTemplate is the versionTemplate on a CryptoKey.
type kmsCryptoKeyVersionTemplate struct {
	ProtectionLevel string `json:"protectionLevel,omitempty"`
	Algorithm       string `json:"algorithm,omitempty"`
}

// kmsCryptoKeyVersion is the wire shape for a CryptoKeyVersion.
type kmsCryptoKeyVersion struct {
	Name             string `json:"name"`
	State            string `json:"state,omitempty"`
	ProtectionLevel  string `json:"protectionLevel,omitempty"`
	Algorithm        string `json:"algorithm,omitempty"`
	CreateTime       string `json:"createTime,omitempty"`
	GenerateTime     string `json:"generateTime,omitempty"`
	DestroyTime      string `json:"destroyTime,omitempty"`
	DestroyEventTime string `json:"destroyEventTime,omitempty"`
}

// kmsCryptoKey is the wire shape for a CryptoKey. `primary` is assembled
// from the live primary version on read so destroy/disable state shows
// through.
type kmsCryptoKey struct {
	Name             string                       `json:"name"`
	Primary          *kmsCryptoKeyVersion         `json:"primary,omitempty"`
	Purpose          string                       `json:"purpose,omitempty"`
	CreateTime       string                       `json:"createTime,omitempty"`
	NextRotationTime string                       `json:"nextRotationTime,omitempty"`
	RotationPeriod   string                       `json:"rotationPeriod,omitempty"`
	VersionTemplate  *kmsCryptoKeyVersionTemplate `json:"versionTemplate,omitempty"`
	Labels           map[string]string            `json:"labels,omitempty"`
}

// kmsStoredCryptoKey is the persisted CryptoKey metadata. Versions live
// in their own store; the primary is resolved by ID at read time.
type kmsStoredCryptoKey struct {
	Name             string            `json:"name"`
	Purpose          string            `json:"purpose"`
	CreateTime       string            `json:"createTime"`
	NextRotationTime string            `json:"nextRotationTime,omitempty"`
	RotationPeriod   string            `json:"rotationPeriod,omitempty"`
	ProtectionLevel  string            `json:"protectionLevel"`
	Algorithm        string            `json:"algorithm"`
	Labels           map[string]string `json:"labels,omitempty"`
	PrimaryVersionID string            `json:"primaryVersionId"`
}

// kmsKeyMaterialRecord holds the (non-exportable) AES key bytes for a
// version, keyed by full version name. Never surfaced on the wire.
type kmsKeyMaterialRecord struct {
	Key []byte `json:"key"`
}

var (
	kmsKeyRings          sim.Store[kmsKeyRing]
	kmsCryptoKeys        sim.Store[kmsStoredCryptoKey]
	kmsCryptoKeyVersions sim.Store[kmsCryptoKeyVersion]
	kmsKeyMaterial       sim.Store[kmsKeyMaterialRecord]
	kmsCRC32CTable       = crc32.MakeTable(crc32.Castagnoli)
)

func registerCloudKMS(srv *sim.Server) {
	kmsKeyRings = sim.MakeStore[kmsKeyRing](srv.DB(), "kms_key_rings")
	kmsCryptoKeys = sim.MakeStore[kmsStoredCryptoKey](srv.DB(), "kms_crypto_keys")
	kmsCryptoKeyVersions = sim.MakeStore[kmsCryptoKeyVersion](srv.DB(), "kms_crypto_key_versions")
	kmsKeyMaterial = sim.MakeStore[kmsKeyMaterialRecord](srv.DB(), "kms_key_material")

	// ----- KeyRings -----

	// CreateKeyRing: POST .../keyRings?keyRingId=X
	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/keyRings", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		id := r.URL.Query().Get("keyRingId")
		if id == "" {
			sim.GCPError(w, http.StatusBadRequest, "keyRingId query parameter is required", "INVALID_ARGUMENT")
			return
		}
		name := kmsKeyRingName(project, location, id)
		if _, exists := kmsKeyRings.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "KeyRing %s already exists", name)
			return
		}
		kr := kmsKeyRing{Name: name, CreateTime: kmsNow()}
		kmsKeyRings.Put(name, kr)
		sim.WriteJSON(w, http.StatusOK, kr)
	})

	// ListKeyRings: GET .../keyRings
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/keyRings", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		prefix := kmsKeyRingName(project, location, "")
		var all []kmsKeyRing
		for _, kr := range kmsKeyRings.List() {
			if strings.HasPrefix(kr.Name, prefix) {
				all = append(all, kr)
			}
		}
		sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
		page, next, ok := paginateList(w, r, all)
		if !ok {
			return
		}
		resp := map[string]any{"keyRings": page, "totalSize": len(all)}
		if next != "" {
			resp["nextPageToken"] = next
		}
		sim.WriteJSON(w, http.StatusOK, resp)
	})

	// GetKeyRing: GET .../keyRings/{keyRing}
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/keyRings/{keyRing}", func(w http.ResponseWriter, r *http.Request) {
		name := kmsKeyRingName(sim.PathParam(r, "project"), sim.PathParam(r, "location"), sim.PathParam(r, "keyRing"))
		kr, ok := kmsKeyRings.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "KeyRing %s not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, kr)
	})

	// ----- CryptoKeys -----

	// CreateCryptoKey: POST .../keyRings/{keyRing}/cryptoKeys?cryptoKeyId=X
	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/keyRings/{keyRing}/cryptoKeys", func(w http.ResponseWriter, r *http.Request) {
		project, location, keyRing := sim.PathParam(r, "project"), sim.PathParam(r, "location"), sim.PathParam(r, "keyRing")
		ringName := kmsKeyRingName(project, location, keyRing)
		if _, ok := kmsKeyRings.Get(ringName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "KeyRing %s not found", ringName)
			return
		}
		id := r.URL.Query().Get("cryptoKeyId")
		if id == "" {
			sim.GCPError(w, http.StatusBadRequest, "cryptoKeyId query parameter is required", "INVALID_ARGUMENT")
			return
		}
		var req kmsCryptoKey
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		name := ringName + "/cryptoKeys/" + id
		if _, exists := kmsCryptoKeys.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "CryptoKey %s already exists", name)
			return
		}
		purpose := req.Purpose
		if purpose == "" {
			purpose = kmsPurposeEncryptDecrypt
		}
		protection, algorithm := kmsDefaultProtectionLevel, kmsSymmetricAlgorithm
		if req.VersionTemplate != nil {
			if req.VersionTemplate.ProtectionLevel != "" {
				protection = req.VersionTemplate.ProtectionLevel
			}
			if req.VersionTemplate.Algorithm != "" {
				algorithm = req.VersionTemplate.Algorithm
			}
		}
		stored := kmsStoredCryptoKey{
			Name:             name,
			Purpose:          purpose,
			CreateTime:       kmsNow(),
			NextRotationTime: req.NextRotationTime,
			RotationPeriod:   req.RotationPeriod,
			ProtectionLevel:  protection,
			Algorithm:        algorithm,
			Labels:           req.Labels,
		}
		// ENCRYPT_DECRYPT keys get an initial primary version unless the
		// caller opts out.
		if purpose == kmsPurposeEncryptDecrypt && r.URL.Query().Get("skipInitialVersionCreation") != "true" {
			ver, err := kmsCreateVersion(name, "1", protection, algorithm)
			if err != nil {
				sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "could not generate key version: %v", err)
				return
			}
			stored.PrimaryVersionID = ver
		}
		kmsCryptoKeys.Put(name, stored)
		sim.WriteJSON(w, http.StatusOK, kmsAssembleCryptoKey(stored))
	})

	// ListCryptoKeys: GET .../keyRings/{keyRing}/cryptoKeys
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/keyRings/{keyRing}/cryptoKeys", func(w http.ResponseWriter, r *http.Request) {
		ringName := kmsKeyRingName(sim.PathParam(r, "project"), sim.PathParam(r, "location"), sim.PathParam(r, "keyRing"))
		prefix := ringName + "/cryptoKeys/"
		var all []kmsCryptoKey
		for _, k := range kmsCryptoKeys.List() {
			if strings.HasPrefix(k.Name, prefix) {
				all = append(all, kmsAssembleCryptoKey(k))
			}
		}
		sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
		page, next, ok := paginateList(w, r, all)
		if !ok {
			return
		}
		resp := map[string]any{"cryptoKeys": page, "totalSize": len(all)}
		if next != "" {
			resp["nextPageToken"] = next
		}
		sim.WriteJSON(w, http.StatusOK, resp)
	})

	// GetCryptoKey: GET .../cryptoKeys/{cryptoKey}
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/keyRings/{keyRing}/cryptoKeys/{cryptoKey}", func(w http.ResponseWriter, r *http.Request) {
		name := kmsCryptoKeyName(r)
		k, ok := kmsCryptoKeys.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "CryptoKey %s not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, kmsAssembleCryptoKey(k))
	})

	// UpdateCryptoKey: PATCH .../cryptoKeys/{cryptoKey}?updateMask=...
	srv.HandleFunc("PATCH /v1/projects/{project}/locations/{location}/keyRings/{keyRing}/cryptoKeys/{cryptoKey}", func(w http.ResponseWriter, r *http.Request) {
		name := kmsCryptoKeyName(r)
		k, ok := kmsCryptoKeys.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "CryptoKey %s not found", name)
			return
		}
		var req kmsCryptoKey
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		mask := r.URL.Query().Get("updateMask")
		for _, field := range strings.Split(mask, ",") {
			switch strings.TrimSpace(field) {
			case "rotationPeriod":
				k.RotationPeriod = req.RotationPeriod
			case "nextRotationTime":
				k.NextRotationTime = req.NextRotationTime
			case "labels":
				k.Labels = req.Labels
			}
		}
		kmsCryptoKeys.Put(name, k)
		sim.WriteJSON(w, http.StatusOK, kmsAssembleCryptoKey(k))
	})

	// Encrypt / Decrypt: POST .../cryptoKeys/{cryptoKeyAction}
	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/keyRings/{keyRing}/cryptoKeys/{cryptoKeyAction}", func(w http.ResponseWriter, r *http.Request) {
		ringName := kmsKeyRingName(sim.PathParam(r, "project"), sim.PathParam(r, "location"), sim.PathParam(r, "keyRing"))
		keyID, action, found := strings.Cut(sim.PathParam(r, "cryptoKeyAction"), ":")
		if !found {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown cryptoKey action %q", sim.PathParam(r, "cryptoKeyAction"))
			return
		}
		name := ringName + "/cryptoKeys/" + keyID
		switch action {
		case "encrypt":
			kmsHandleEncrypt(w, r, name)
		case "decrypt":
			kmsHandleDecrypt(w, r, name)
		default:
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown cryptoKey action %q", action)
		}
	})

	// ----- CryptoKeyVersions -----

	// ListCryptoKeyVersions: GET .../cryptoKeys/{cryptoKey}/cryptoKeyVersions
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/keyRings/{keyRing}/cryptoKeys/{cryptoKey}/cryptoKeyVersions", func(w http.ResponseWriter, r *http.Request) {
		keyName := kmsCryptoKeyName(r)
		if _, ok := kmsCryptoKeys.Get(keyName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "CryptoKey %s not found", keyName)
			return
		}
		prefix := keyName + "/cryptoKeyVersions/"
		var all []kmsCryptoKeyVersion
		for _, v := range kmsCryptoKeyVersions.List() {
			if strings.HasPrefix(v.Name, prefix) {
				all = append(all, v)
			}
		}
		sort.Slice(all, func(i, j int) bool { return kmsVersionLess(all[i].Name, all[j].Name) })
		page, next, ok := paginateList(w, r, all)
		if !ok {
			return
		}
		resp := map[string]any{"cryptoKeyVersions": page, "totalSize": len(all)}
		if next != "" {
			resp["nextPageToken"] = next
		}
		sim.WriteJSON(w, http.StatusOK, resp)
	})

	// GetCryptoKeyVersion: GET .../cryptoKeyVersions/{cryptoKeyVersion}
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/keyRings/{keyRing}/cryptoKeys/{cryptoKey}/cryptoKeyVersions/{cryptoKeyVersion}", func(w http.ResponseWriter, r *http.Request) {
		name := kmsCryptoKeyName(r) + "/cryptoKeyVersions/" + sim.PathParam(r, "cryptoKeyVersion")
		v, ok := kmsCryptoKeyVersions.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "CryptoKeyVersion %s not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, v)
	})

	// DestroyCryptoKeyVersion: POST .../cryptoKeyVersions/{cryptoKeyVersionAction}
	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/keyRings/{keyRing}/cryptoKeys/{cryptoKey}/cryptoKeyVersions/{cryptoKeyVersionAction}", func(w http.ResponseWriter, r *http.Request) {
		versionID, action, found := strings.Cut(sim.PathParam(r, "cryptoKeyVersionAction"), ":")
		if !found || action != "destroy" {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown cryptoKeyVersion action %q", sim.PathParam(r, "cryptoKeyVersionAction"))
			return
		}
		name := kmsCryptoKeyName(r) + "/cryptoKeyVersions/" + versionID
		v, ok := kmsCryptoKeyVersions.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "CryptoKeyVersion %s not found", name)
			return
		}
		if v.State == "DESTROY_SCHEDULED" || v.State == "DESTROYED" {
			sim.GCPErrorf(w, http.StatusBadRequest, "FAILED_PRECONDITION", "CryptoKeyVersion %s is already %s", name, v.State)
			return
		}
		v.State = "DESTROY_SCHEDULED"
		v.DestroyTime = time.Now().UTC().Add(kmsDestroyScheduledDelay).Format(time.RFC3339)
		kmsCryptoKeyVersions.Put(name, v)
		sim.WriteJSON(w, http.StatusOK, v)
	})
}

// ----- crypto handlers -----

func kmsHandleEncrypt(w http.ResponseWriter, r *http.Request, keyName string) {
	key, ok := kmsCryptoKeys.Get(keyName)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "CryptoKey %s not found", keyName)
		return
	}
	if key.Purpose != kmsPurposeEncryptDecrypt {
		sim.GCPErrorf(w, http.StatusBadRequest, "FAILED_PRECONDITION", "CryptoKey %s is not for ENCRYPT_DECRYPT", keyName)
		return
	}
	var req struct {
		Plaintext                         string `json:"plaintext"`
		PlaintextCrc32c                   *int64 `json:"plaintextCrc32c,string,omitempty"`
		AdditionalAuthenticatedData       string `json:"additionalAuthenticatedData"`
		AdditionalAuthenticatedDataCrc32c *int64 `json:"additionalAuthenticatedDataCrc32c,string,omitempty"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
		return
	}
	plaintext, err := kmsDecodeBytes(req.Plaintext)
	if err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "plaintext must be base64: %v", err)
		return
	}
	aad, err := kmsDecodeBytes(req.AdditionalAuthenticatedData)
	if err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "additionalAuthenticatedData must be base64: %v", err)
		return
	}
	verifiedPlaintext, ok := kmsVerifyCRC(w, plaintext, req.PlaintextCrc32c, "plaintext")
	if !ok {
		return
	}
	verifiedAAD, ok := kmsVerifyCRC(w, aad, req.AdditionalAuthenticatedDataCrc32c, "additionalAuthenticatedData")
	if !ok {
		return
	}

	versionID := key.PrimaryVersionID
	if versionID == "" {
		sim.GCPErrorf(w, http.StatusBadRequest, "FAILED_PRECONDITION", "CryptoKey %s has no primary version", keyName)
		return
	}
	versionName := keyName + "/cryptoKeyVersions/" + versionID
	version, ok := kmsCryptoKeyVersions.Get(versionName)
	if !ok || version.State != "ENABLED" {
		sim.GCPErrorf(w, http.StatusBadRequest, "FAILED_PRECONDITION", "primary version of %s is not enabled", keyName)
		return
	}
	material, ok := kmsKeyMaterial.Get(versionName)
	if !ok {
		sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "missing key material for %s", versionName)
		return
	}
	versionNum, _ := kmsVersionNumber(versionName)
	ciphertext, err := kmsEncryptBytes(material.Key, versionNum, plaintext, aad)
	if err != nil {
		sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "encryption failed: %v", err)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"name":                    versionName,
		"ciphertext":              base64.StdEncoding.EncodeToString(ciphertext),
		"ciphertextCrc32c":        fmt.Sprintf("%d", kmsCRC(ciphertext)),
		"verifiedPlaintextCrc32c": verifiedPlaintext,
		"verifiedAdditionalAuthenticatedDataCrc32c": verifiedAAD,
		"protectionLevel": key.ProtectionLevel,
	})
}

func kmsHandleDecrypt(w http.ResponseWriter, r *http.Request, keyName string) {
	if _, ok := kmsCryptoKeys.Get(keyName); !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "CryptoKey %s not found", keyName)
		return
	}
	var req struct {
		Ciphertext                        string `json:"ciphertext"`
		CiphertextCrc32c                  *int64 `json:"ciphertextCrc32c,string,omitempty"`
		AdditionalAuthenticatedData       string `json:"additionalAuthenticatedData"`
		AdditionalAuthenticatedDataCrc32c *int64 `json:"additionalAuthenticatedDataCrc32c,string,omitempty"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
		return
	}
	ciphertext, err := kmsDecodeBytes(req.Ciphertext)
	if err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "ciphertext must be base64: %v", err)
		return
	}
	aad, err := kmsDecodeBytes(req.AdditionalAuthenticatedData)
	if err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "additionalAuthenticatedData must be base64: %v", err)
		return
	}
	if _, ok := kmsVerifyCRC(w, ciphertext, req.CiphertextCrc32c, "ciphertext"); !ok {
		return
	}
	versionNum, blob, err := kmsParseCiphertext(ciphertext)
	if err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Decryption failed: the ciphertext is malformed")
		return
	}
	versionName := fmt.Sprintf("%s/cryptoKeyVersions/%d", keyName, versionNum)
	version, ok := kmsCryptoKeyVersions.Get(versionName)
	if !ok {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Decryption failed: the version used to encrypt does not exist")
		return
	}
	if version.State != "ENABLED" {
		sim.GCPErrorf(w, http.StatusBadRequest, "FAILED_PRECONDITION", "CryptoKeyVersion %s is not enabled (state %s)", versionName, version.State)
		return
	}
	material, ok := kmsKeyMaterial.Get(versionName)
	if !ok {
		sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "missing key material for %s", versionName)
		return
	}
	plaintext, err := kmsDecryptBytes(material.Key, blob, aad)
	if err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "Decryption failed: verify the ciphertext and AAD match what was used to encrypt")
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"plaintext":       base64.StdEncoding.EncodeToString(plaintext),
		"plaintextCrc32c": fmt.Sprintf("%d", kmsCRC(plaintext)),
		"protectionLevel": kmsDefaultProtectionLevel,
	})
}

// ----- helpers -----

func kmsNow() string { return time.Now().UTC().Format(time.RFC3339) }

// kmsDecodeBytes accepts both standard and URL-safe base64, padded or not.
// proto3 JSON bytes fields accept either alphabet, and `gcloud kms` emits
// URL-safe base64 (with `-`/`_`) for plaintext/ciphertext — which a plain
// StdEncoding decode rejects.
func kmsDecodeBytes(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return base64.StdEncoding.DecodeString(s)
}

func kmsCRC(b []byte) int64 { return int64(crc32.Checksum(b, kmsCRC32CTable)) }

// kmsVerifyCRC checks a client-supplied CRC32C against the data. Returns
// (verified, ok): verified is true when the client supplied a checksum
// that matched; ok is false (and an error already written) on mismatch.
func kmsVerifyCRC(w http.ResponseWriter, data []byte, supplied *int64, field string) (bool, bool) {
	if supplied == nil {
		return false, true
	}
	if *supplied != kmsCRC(data) {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT",
			"%s.crc32c mismatch: got %d, want %d", field, *supplied, kmsCRC(data))
		return false, false
	}
	return true, true
}

func kmsKeyRingName(project, location, id string) string {
	return fmt.Sprintf("projects/%s/locations/%s/keyRings/%s", project, location, id)
}

func kmsCryptoKeyName(r *http.Request) string {
	return kmsKeyRingName(sim.PathParam(r, "project"), sim.PathParam(r, "location"), sim.PathParam(r, "keyRing")) +
		"/cryptoKeys/" + sim.PathParam(r, "cryptoKey")
}

// kmsCreateVersion generates a new ENABLED version with fresh AES-256 key
// material and returns its numeric ID.
func kmsCreateVersion(keyName, versionID, protection, algorithm string) (string, error) {
	material := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, material); err != nil {
		return "", err
	}
	versionName := keyName + "/cryptoKeyVersions/" + versionID
	now := kmsNow()
	kmsCryptoKeyVersions.Put(versionName, kmsCryptoKeyVersion{
		Name:            versionName,
		State:           "ENABLED",
		ProtectionLevel: protection,
		Algorithm:       algorithm,
		CreateTime:      now,
		GenerateTime:    now,
	})
	kmsKeyMaterial.Put(versionName, kmsKeyMaterialRecord{Key: material})
	return versionID, nil
}

// kmsAssembleCryptoKey builds the wire CryptoKey, resolving the live
// primary version.
func kmsAssembleCryptoKey(k kmsStoredCryptoKey) kmsCryptoKey {
	out := kmsCryptoKey{
		Name:             k.Name,
		Purpose:          k.Purpose,
		CreateTime:       k.CreateTime,
		NextRotationTime: k.NextRotationTime,
		RotationPeriod:   k.RotationPeriod,
		Labels:           k.Labels,
		VersionTemplate: &kmsCryptoKeyVersionTemplate{
			ProtectionLevel: k.ProtectionLevel,
			Algorithm:       k.Algorithm,
		},
	}
	if k.PrimaryVersionID != "" {
		if v, ok := kmsCryptoKeyVersions.Get(k.Name + "/cryptoKeyVersions/" + k.PrimaryVersionID); ok {
			primary := v
			out.Primary = &primary
		}
	}
	return out
}

// kmsVersionNumber extracts the trailing numeric version ID from a
// version resource name.
func kmsVersionNumber(versionName string) (int, bool) {
	i := strings.LastIndex(versionName, "/cryptoKeyVersions/")
	if i < 0 {
		return 0, false
	}
	var n int
	if _, err := fmt.Sscanf(versionName[i+len("/cryptoKeyVersions/"):], "%d", &n); err != nil {
		return 0, false
	}
	return n, true
}

// kmsVersionLess orders version names by their numeric ID.
func kmsVersionLess(a, b string) bool {
	na, _ := kmsVersionNumber(a)
	nb, _ := kmsVersionNumber(b)
	return na < nb
}

// kmsEncryptBytes seals plaintext with AES-256-GCM and frames the result
// as version(4) || nonce || sealed so decrypt can pick the version.
func kmsEncryptBytes(material []byte, versionNum int, plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(material)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, aad)
	blob := make([]byte, 4+len(nonce)+len(sealed))
	binary.BigEndian.PutUint32(blob[:4], uint32(versionNum))
	copy(blob[4:], nonce)
	copy(blob[4+len(nonce):], sealed)
	return blob, nil
}

// kmsParseCiphertext splits the framed ciphertext into its version number
// and the nonce||sealed remainder.
func kmsParseCiphertext(ciphertext []byte) (int, []byte, error) {
	if len(ciphertext) < 4 {
		return 0, nil, fmt.Errorf("ciphertext too short")
	}
	return int(binary.BigEndian.Uint32(ciphertext[:4])), ciphertext[4:], nil
}

func kmsDecryptBytes(material, blob, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(material)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(blob) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, blob[:ns], blob[ns:], aad)
}
