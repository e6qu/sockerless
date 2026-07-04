package bleephub

import (
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
	Bundle         json.RawMessage `json:"bundle"`
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
func (st *Store) CreateAttestation(repoID int, bundle json.RawMessage, subjects []string, predicateType, initiator string) *Attestation {
	st.mu.Lock()
	defer st.mu.Unlock()
	a := &Attestation{
		ID:             st.NextAttestationID,
		RepoID:         repoID,
		Bundle:         bundle,
		SubjectDigests: subjects,
		PredicateType:  predicateType,
		Initiator:      initiator,
		CreatedAt:      time.Now().UTC(),
	}
	st.NextAttestationID++
	st.Attestations[a.ID] = a
	if st.persist != nil {
		st.persist.MustPut("attestations", strconv.Itoa(a.ID), a)
	}
	return a
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

// DeleteAttestation removes an attestation by ID. Returns true if it existed.
func (st *Store) DeleteAttestation(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, ok := st.Attestations[id]; !ok {
		return false
	}
	delete(st.Attestations, id)
	if st.persist != nil {
		st.persist.MustDelete("attestations", strconv.Itoa(id))
	}
	return true
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
