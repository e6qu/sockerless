package bleephub

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/crypto/nacl/box"
)

// ActionsVariable is a GitHub Actions configuration variable. The same
// shape serves all three scopes; Visibility/SelectedRepoIDs are populated
// only at the organization level (all|private|selected).
type ActionsVariable struct {
	Name            string    `json:"name"`
	Value           string    `json:"value"`
	Visibility      string    `json:"visibility,omitempty"`
	SelectedRepoIDs []int     `json:"selected_repository_ids,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// OrgSecret is an organization-level Actions secret: a Secret plus the
// org-only visibility scoping (all|private|selected + selected repos).
type OrgSecret struct {
	Secret
	Visibility      string `json:"visibility"`
	SelectedRepoIDs []int  `json:"selected_repository_ids,omitempty"`
}

// SecretsKeyPair is the X25519 keypair backing the Actions secrets
// sealed-box contract: clients GET the public key, libsodium-seal the
// secret value, and PUT {encrypted_value, key_id}; the server decrypts
// with the private key when injecting secrets into job messages. The
// pair is persisted so key_id stays stable across restarts (a client
// caching the public key must not go stale silently).
type SecretsKeyPair struct {
	KeyID      string `json:"key_id"`
	PublicKey  string `json:"public_key"`  // base64 32-byte X25519 public key
	PrivateKey string `json:"private_key"` // base64 32-byte X25519 private key
}

// TimelineRecord is the slice of the runner's Azure-DevOps-style timeline
// record bleephub consumes: enough to surface real per-step status, timing,
// and log association on the jobs API. Records arrive via
// PATCH /_apis/v1/Timeline/.../{timelineId}; Type is "Job" for the job
// record and "Task" for each step.
type TimelineRecord struct {
	ID         string          `json:"id"`
	ParentID   string          `json:"parentId"`
	Type       string          `json:"type"`
	Name       string          `json:"name"`
	RefName    string          `json:"refName"`
	Order      int             `json:"order"`
	State      string          `json:"state"`  // pending | inProgress | completed
	Result     string          `json:"result"` // succeeded | failed | skipped | canceled | abandoned
	StartTime  string          `json:"startTime"`
	FinishTime string          `json:"finishTime"`
	Log        *TimelineLogRef `json:"log"`
}

// TimelineLogRef points a timeline record at its uploaded log file.
type TimelineLogRef struct {
	ID int `json:"id"`
}

// ActionsKeyPair returns the server-wide sealed-box keypair, generating
// and persisting it on first use.
func (st *Store) ActionsKeyPair() (*SecretsKeyPair, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.actionsKeyPair != nil {
		return st.actionsKeyPair, nil
	}
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate actions secrets keypair: %w", err)
	}
	// GitHub key_ids are decimal strings; derive one deterministically from
	// the public key so the id and key can never disagree.
	kp := &SecretsKeyPair{
		KeyID:      strconv.FormatUint(binary.BigEndian.Uint64(pub[:8]), 10),
		PublicKey:  base64.StdEncoding.EncodeToString(pub[:]),
		PrivateKey: base64.StdEncoding.EncodeToString(priv[:]),
	}
	st.actionsKeyPair = kp
	if st.persist != nil {
		st.persist.MustPut("actions_crypto", "keypair", kp)
	}
	return kp, nil
}

// OpenSealedSecret decrypts a base64 libsodium sealed-box ciphertext
// (crypto_box_seal) produced against the server's Actions public key.
func (st *Store) OpenSealedSecret(encryptedValue string) (string, error) {
	kp, err := st.ActionsKeyPair()
	if err != nil {
		return "", err
	}
	ct, err := base64.StdEncoding.DecodeString(encryptedValue)
	if err != nil {
		return "", fmt.Errorf("encrypted_value is not valid base64: %w", err)
	}
	pubRaw, err := base64.StdEncoding.DecodeString(kp.PublicKey)
	if err != nil {
		return "", fmt.Errorf("stored public key corrupt: %w", err)
	}
	privRaw, err := base64.StdEncoding.DecodeString(kp.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("stored private key corrupt: %w", err)
	}
	var pub, priv [32]byte
	copy(pub[:], pubRaw)
	copy(priv[:], privRaw)
	plain, ok := box.OpenAnonymous(nil, ct, &pub, &priv)
	if !ok {
		return "", fmt.Errorf("sealed box does not open with the server's key (wrong key_id or corrupt ciphertext)")
	}
	return string(plain), nil
}

// SealSecretValue encrypts a plaintext against the server's own Actions
// public key — the client side of the sealed-box contract, used by tests
// and internal flows that exercise the real PUT shape.
func (st *Store) SealSecretValue(plaintext string) (encryptedValue, keyID string, err error) {
	kp, err := st.ActionsKeyPair()
	if err != nil {
		return "", "", err
	}
	pubRaw, err := base64.StdEncoding.DecodeString(kp.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("stored public key corrupt: %w", err)
	}
	var pub [32]byte
	copy(pub[:], pubRaw)
	ct, err := box.SealAnonymous(nil, []byte(plaintext), &pub, rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("seal secret value: %w", err)
	}
	return base64.StdEncoding.EncodeToString(ct), kp.KeyID, nil
}
