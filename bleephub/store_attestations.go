package bleephub

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Artifact attestations. Uploaded Sigstore bundles are stored verbatim
// and round-tripped byte-for-byte; the subject digests and predicate
// type the list endpoints key on are extracted from the bundle's DSSE
// envelope payload (an in-toto statement), exactly the association real
// GitHub derives.

// Attestation is one uploaded artifact attestation.
type Attestation struct {
	ID             int             `json:"id"`
	RepoID         int             `json:"repo_id"`
	Bundle         json.RawMessage `json:"-"`
	StoragePath    string          `json:"storage_path,omitempty"`
	SubjectDigests []string        `json:"subject_digests"` // "algorithm:hex", lowercased
	PredicateType  string          `json:"predicate_type"`
	Initiator      string          `json:"initiator"` // login of the uploading user
	CreatedAt      time.Time       `json:"created_at"`
}

// parseSigstoreBundleSubjects decodes the DSSE envelope payload inside
// a Sigstore bundle and returns the in-toto statement's subject digests
// (as "algorithm:hex") and predicate type.
func parseSigstoreBundleSubjects(bundle json.RawMessage) (subjects []string, predicateType string, err error) {
	var b struct {
		DsseEnvelope struct {
			Payload string `json:"payload"`
		} `json:"dsseEnvelope"`
	}
	if err := json.Unmarshal(bundle, &b); err != nil {
		return nil, "", fmt.Errorf("bundle is not valid JSON: %w", err)
	}
	if b.DsseEnvelope.Payload == "" {
		return nil, "", fmt.Errorf("bundle has no dsseEnvelope.payload")
	}
	payload, err := base64.StdEncoding.DecodeString(b.DsseEnvelope.Payload)
	if err != nil {
		payload, err = base64.RawStdEncoding.DecodeString(b.DsseEnvelope.Payload)
		if err != nil {
			return nil, "", fmt.Errorf("dsseEnvelope.payload is not base64: %w", err)
		}
	}
	var stmt struct {
		PredicateType string `json:"predicateType"`
		Subject       []struct {
			Name   string            `json:"name"`
			Digest map[string]string `json:"digest"`
		} `json:"subject"`
	}
	if err := json.Unmarshal(payload, &stmt); err != nil {
		return nil, "", fmt.Errorf("dsseEnvelope.payload is not an in-toto statement: %w", err)
	}
	for _, sub := range stmt.Subject {
		for alg, hex := range sub.Digest {
			subjects = append(subjects, strings.ToLower(alg)+":"+strings.ToLower(hex))
		}
	}
	if len(subjects) == 0 {
		return nil, "", fmt.Errorf("in-toto statement has no subject digests")
	}
	sort.Strings(subjects)
	return subjects, stmt.PredicateType, nil
}

// attestationPredicateMatches applies the predicate_type filter. The
// filter accepts the shorthands GitHub documents (provenance, sbom,
// release) or a verbatim predicate type URI.
func attestationPredicateMatches(filter, predicateType string) bool {
	switch filter {
	case "":
		return true
	case "provenance":
		return strings.Contains(predicateType, "slsa.dev/provenance")
	case "sbom":
		return strings.Contains(predicateType, "spdx.dev") || strings.Contains(predicateType, "cyclonedx.org")
	case "release":
		return strings.Contains(predicateType, "in-toto.io/attestation/release")
	default:
		return filter == predicateType
	}
}

// CreateAttestation stores an uploaded bundle for a repository.
func (st *Store) CreateAttestation(repoID int, bundle json.RawMessage, subjects []string, predicateType, initiator string) (*Attestation, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	id := st.NextAttestationID
	storagePath := attestationBundleDataKey(id)
	if st.persist != nil && st.ObjectByteStore == nil {
		return nil, fmt.Errorf("persistent artifact attestation storage requires object byte store")
	}
	if st.ObjectByteStore != nil {
		if err := st.ObjectByteStore.Put(context.Background(), storagePath, bundle); err != nil {
			return nil, fmt.Errorf("write artifact attestation bundle %s: %w", storagePath, err)
		}
	}
	a := &Attestation{
		ID:             id,
		RepoID:         repoID,
		StoragePath:    storagePath,
		SubjectDigests: subjects,
		PredicateType:  predicateType,
		Initiator:      initiator,
		CreatedAt:      time.Now().UTC(),
	}
	if st.ObjectByteStore == nil {
		a.Bundle = append(json.RawMessage(nil), bundle...)
	}
	st.NextAttestationID++
	st.Attestations[a.ID] = a
	if st.persist != nil {
		st.persist.MustPut("attestations", strconv.Itoa(a.ID), a)
	}
	return a, nil
}

// GetAttestation returns an attestation by ID, or nil.
func (st *Store) GetAttestation(id int) *Attestation {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Attestations[id]
}

// hasSubjectDigest reports whether the attestation covers the digest.
func (a *Attestation) hasSubjectDigest(digest string) bool {
	digest = strings.ToLower(digest)
	for _, d := range a.SubjectDigests {
		if d == digest {
			return true
		}
	}
	return false
}

// ListAttestations returns the attestations across the given repos that
// cover subjectDigest (any digest when empty) and pass the
// predicate-type filter, sorted ascending by ID.
func (st *Store) ListAttestations(repoIDs map[int]bool, subjectDigest, predicateType string) []*Attestation {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]*Attestation, 0)
	for _, a := range st.Attestations {
		if !repoIDs[a.RepoID] {
			continue
		}
		if subjectDigest != "" && !a.hasSubjectDigest(subjectDigest) {
			continue
		}
		if !attestationPredicateMatches(predicateType, a.PredicateType) {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ReadAttestationBundle reads the Sigstore bundle bytes for an attestation.
func (st *Store) ReadAttestationBundle(ctx context.Context, a *Attestation) (json.RawMessage, error) {
	if a == nil {
		return nil, fmt.Errorf("attestation is nil")
	}
	if a.StoragePath != "" && st.ObjectByteStore != nil {
		data, err := st.ObjectByteStore.Get(ctx, a.StoragePath)
		if err != nil {
			return nil, fmt.Errorf("read artifact attestation bundle %s: %w", a.StoragePath, err)
		}
		return json.RawMessage(data), nil
	}
	if a.StoragePath != "" && a.Bundle == nil {
		return nil, fmt.Errorf("artifact attestation bundle %s requires object byte store", a.StoragePath)
	}
	return append(json.RawMessage(nil), a.Bundle...), nil
}

// DeleteAttestation removes an attestation by ID. Returns true if it existed.
func (st *Store) DeleteAttestation(id int) (bool, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	a, ok := st.Attestations[id]
	if !ok {
		return false, nil
	}
	if err := st.deleteAttestationBundleLocked(a); err != nil {
		return true, err
	}
	delete(st.Attestations, id)
	if st.persist != nil {
		st.persist.MustDelete("attestations", strconv.Itoa(id))
	}
	return true, nil
}

func (st *Store) deleteAttestationBundleLocked(a *Attestation) error {
	if a == nil || a.StoragePath == "" || st.ObjectByteStore == nil {
		return nil
	}
	if err := st.ObjectByteStore.Delete(context.Background(), a.StoragePath); err != nil {
		return fmt.Errorf("delete artifact attestation bundle %s: %w", a.StoragePath, err)
	}
	return nil
}

func (st *Store) deleteAttestationsForRepoLocked(repoID int) error {
	for id, a := range st.Attestations {
		if a.RepoID != repoID {
			continue
		}
		if err := st.deleteAttestationBundleLocked(a); err != nil {
			return err
		}
		delete(st.Attestations, id)
		if st.persist != nil {
			st.persist.MustDelete("attestations", strconv.Itoa(id))
		}
	}
	return nil
}

// RepoIDsOwnedBy returns the IDs of every repository whose owner
// segment matches login (an organization or user account name).
func (st *Store) RepoIDsOwnedBy(login string) map[int]bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := map[int]bool{}
	prefix := strings.ToLower(login) + "/"
	for id, repo := range st.Repos {
		if strings.HasPrefix(strings.ToLower(repo.FullName), prefix) {
			out[id] = true
		}
	}
	return out
}
