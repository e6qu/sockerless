package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net/http"

	sim "github.com/sockerless/simulator"
)

// KMS symmetric encryption uses real AES-256-GCM. Each customer master key
// gets 32 bytes of random key material generated at creation time and stored
// in kmsKeyMaterial. The ciphertext blob is opaque to SDK callers but is a
// structured envelope so Decrypt can route to the right key and verify the
// authentication tag.
//
// Blob format v1:
//   magic    [3]byte  "SK1"
//   version  [1]byte  0x01
//   keyIdLen [2]byte  big-endian uint16
//   keyId    []byte
//   nonce    [12]byte AES-GCM nonce
//   ciphertext+tag []byte

const kmsBlobMagic = "SK1"
const kmsBlobVersion = byte(0x01)
const kmsKeyMaterialLen = 32

func registerKMSCrypto(r *sim.AWSRouter, srv *sim.Server) {
	registerKMSKeyMaterial(srv)
}

var kmsKeyMaterial sim.Store[[]byte]

func registerKMSKeyMaterial(srv *sim.Server) {
	kmsKeyMaterial = sim.MakeStore[[]byte](srv.DB(), "kms_key_material")
}

// kmsSigningAlgorithmsFor returns the signing algorithms real KMS advertises
// for an asymmetric signing key based on its KeySpec.
func kmsSigningAlgorithmsFor(spec string) []string {
	switch spec {
	case "RSA_2048", "RSA_3072", "RSA_4096":
		return []string{
			"RSASSA_PSS_SHA_256",
			"RSASSA_PSS_SHA_384",
			"RSASSA_PSS_SHA_512",
			"RSASSA_PKCS1_V1_5_SHA_256",
			"RSASSA_PKCS1_V1_5_SHA_384",
			"RSASSA_PKCS1_V1_5_SHA_512",
		}
	case "ECC_NIST_P256", "ECC_SECG_P256K1":
		return []string{"ECDSA_SHA_256"}
	case "ECC_NIST_P384":
		return []string{"ECDSA_SHA_384"}
	case "ECC_NIST_P521":
		return []string{"ECDSA_SHA_512"}
	}
	return nil
}

// kmsEncryptionAlgorithmsFor returns the encryption algorithms real KMS
// advertises for an asymmetric encryption key based on its KeySpec and usage.
func kmsEncryptionAlgorithmsFor(spec, usage string) []string {
	if usage != "ENCRYPT_DECRYPT" {
		return nil
	}
	switch spec {
	case "RSA_2048", "RSA_3072", "RSA_4096":
		return []string{"RSAES_OAEP_SHA_1", "RSAES_OAEP_SHA_256"}
	}
	return nil
}

// kmsKeyAgreementAlgorithmsFor returns the key-agreement algorithms real KMS
// advertises for an ECC key with KEY_AGREEMENT usage.
func kmsKeyAgreementAlgorithmsFor(spec, usage string) []string {
	if usage != "KEY_AGREEMENT" {
		return nil
	}
	switch spec {
	case "ECC_NIST_P256", "ECC_NIST_P384", "ECC_NIST_P521", "ECC_SECG_P256K1":
		return []string{"ECDH"}
	}
	return nil
}

// kmsMacAlgorithmsFor returns the MAC algorithms real KMS advertises for an
// HMAC key based on its KeySpec.
func kmsMacAlgorithmsFor(spec string) []string {
	switch spec {
	case "HMAC_224":
		return []string{"HMAC_SHA_224"}
	case "HMAC_256":
		return []string{"HMAC_SHA_256"}
	case "HMAC_384":
		return []string{"HMAC_SHA_384"}
	case "HMAC_512":
		return []string{"HMAC_SHA_512"}
	}
	return nil
}

// kmsGenerateKeyMaterial creates and persists 32 random bytes for a new CMK.
func kmsGenerateKeyMaterial(keyId string) ([]byte, error) {
	material := make([]byte, kmsKeyMaterialLen)
	if _, err := io.ReadFull(rand.Reader, material); err != nil {
		return nil, err
	}
	kmsKeyMaterial.Put(keyId, material)
	return material, nil
}

// kmsGetKeyMaterial returns the persisted AES key for a CMK.
func kmsGetKeyMaterial(keyId string) ([]byte, bool) {
	return kmsKeyMaterial.Get(keyId)
}

// kmsDeleteKeyMaterial removes the key material for a CMK.
func kmsDeleteKeyMaterial(keyId string) {
	kmsKeyMaterial.Delete(keyId)
}

// kmsEncryptBytes encrypts plaintext under the named CMK and returns the
// opaque ciphertext blob. Returns ok=false when the key has no material.
func kmsEncryptBytes(keyId string, plaintext []byte) ([]byte, bool) {
	material, ok := kmsGetKeyMaterial(keyId)
	if !ok {
		return nil, false
	}
	block, err := aes.NewCipher(material)
	if err != nil {
		return nil, false
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, false
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, false
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	keyIdBytes := []byte(keyId)
	if len(keyIdBytes) > 65535 {
		return nil, false
	}
	out := make([]byte, 0, 3+1+2+len(keyIdBytes)+len(nonce)+len(ciphertext))
	out = append(out, []byte(kmsBlobMagic)...)
	out = append(out, kmsBlobVersion)
	out = binary.BigEndian.AppendUint16(out, uint16(len(keyIdBytes)))
	out = append(out, keyIdBytes...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, true
}

// kmsDecryptBytes decrypts a blob produced by kmsEncryptBytes. It returns the
// source key id, plaintext, and ok. Authentication tag verification happens
// inside GCM.Open; a tampered blob returns ok=false.
func kmsDecryptBytes(blob []byte) (keyId string, plaintext []byte, ok bool) {
	if len(blob) < 3+1+2 {
		return "", nil, false
	}
	if string(blob[0:3]) != kmsBlobMagic {
		return "", nil, false
	}
	if blob[3] != kmsBlobVersion {
		return "", nil, false
	}
	keyIdLen := binary.BigEndian.Uint16(blob[4:6])
	off := 6
	if len(blob) < off+int(keyIdLen)+12 {
		return "", nil, false
	}
	keyId = string(blob[off : off+int(keyIdLen)])
	off += int(keyIdLen)

	material, exists := kmsGetKeyMaterial(keyId)
	if !exists {
		return "", nil, false
	}
	block, err := aes.NewCipher(material)
	if err != nil {
		return "", nil, false
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", nil, false
	}
	nonceSize := gcm.NonceSize()
	if len(blob) < off+nonceSize {
		return "", nil, false
	}
	nonce := blob[off : off+nonceSize]
	ciphertext := blob[off+nonceSize:]
	plaintext, err = gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", nil, false
	}
	return keyId, plaintext, true
}

// kmsIsUsable reports whether a CMK can currently perform cryptographic
// operations. Keys in Disabled or PendingDeletion states must reject crypto.
func kmsIsUsable(key KMSKey) bool {
	return key.KeyState == "Enabled"
}

// kmsCryptoDisabledError returns the service-specific error real KMS emits
// when a key is not in a valid state for the requested operation.
func kmsCryptoDisabledError(w http.ResponseWriter, op string) {
	sim.AWSErrorf(w, "DisabledException", http.StatusConflict,
		"%s is disabled.", op)
}

// kmsKeyPolicyArn returns the ARN used as the resource-policy key for a CMK.
func kmsKeyPolicyArn(keyId string) string {
	return kmsKeyArn(keyId)
}

// kmsPutKeyPolicy mirrors a KMS key policy into the central resource-policy
// store so the IAM enforcement gate evaluates it for crypto operations.
func kmsPutKeyPolicy(keyId, policyJSON string) {
	if policyJSON == "" {
		policyJSON = kmsDefaultKeyPolicyJSON()
	}
	iamPutResourcePolicy(kmsKeyPolicyArn(keyId), policyJSON)
}
