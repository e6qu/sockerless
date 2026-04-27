package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	sim "github.com/sockerless/simulator"
)

// Secrets Manager — sockerless runner workflows commonly fetch DB
// credentials, API tokens, and service-account keys from this slice.
// Without it, every `aws-actions/configure-aws-credentials` follow-up
// or `aws secretsmanager get-secret-value` 404s. Wire format follows
// the JSON protocol with `X-Amz-Target: secretsmanager.<Action>`.

// SMSecret is a Secrets Manager secret. Real Secrets Manager stores
// per-version values keyed by VersionId + staging label; for runner
// use cases (read-most, single-current-version) the sim collapses to
// the AWSCURRENT version only.
type SMSecret struct {
	ARN              string  `json:"ARN"`
	Name             string  `json:"Name"`
	Description      string  `json:"Description,omitempty"`
	KmsKeyId         string  `json:"KmsKeyId,omitempty"`
	CreatedDate      float64 `json:"CreatedDate,omitempty"`
	LastChangedDate  float64 `json:"LastChangedDate,omitempty"`
	LastAccessedDate float64 `json:"LastAccessedDate,omitempty"`
	VersionId        string  `json:"VersionId"`
	SecretString     string  `json:"SecretString,omitempty"`
	SecretBinary     []byte  `json:"SecretBinary,omitempty"`
	Tags             []SMTag `json:"Tags,omitempty"`
}

// SMTag mirrors `aws.Tag`. Real Secrets Manager tags propagate to
// CloudTrail and AWS Config; the sim just round-trips them.
type SMTag struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

var smSecrets sim.Store[SMSecret]

func smArn(name string) string {
	// Real ARN format: arn:aws:secretsmanager:<region>:<account>:secret:<name>-<6-char-suffix>.
	// The suffix is a per-secret random string; we use a deterministic
	// 6-char slice so tests can match on prefix.
	return fmt.Sprintf("arn:aws:secretsmanager:%s:%s:secret:%s-%s",
		awsRegion(), awsAccountID(), name, generateUUID()[:6])
}

func registerSecretsManager(r *sim.AWSRouter, srv *sim.Server) {
	smSecrets = sim.MakeStore[SMSecret](srv.DB(), "sm_secrets")

	r.Register("secretsmanager.CreateSecret", handleSMCreateSecret)
	r.Register("secretsmanager.GetSecretValue", handleSMGetSecretValue)
	r.Register("secretsmanager.DescribeSecret", handleSMDescribeSecret)
	r.Register("secretsmanager.UpdateSecret", handleSMUpdateSecret)
	r.Register("secretsmanager.PutSecretValue", handleSMPutSecretValue)
	r.Register("secretsmanager.DeleteSecret", handleSMDeleteSecret)
	r.Register("secretsmanager.ListSecrets", handleSMListSecrets)
	r.Register("secretsmanager.TagResource", handleSMTagResource)
	r.Register("secretsmanager.UntagResource", handleSMUntagResource)
}

func handleSMCreateSecret(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string  `json:"Name"`
		Description  string  `json:"Description"`
		KmsKeyId     string  `json:"KmsKeyId"`
		SecretString string  `json:"SecretString"`
		SecretBinary []byte  `json:"SecretBinary"`
		Tags         []SMTag `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequestException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		sim.AWSError(w, "InvalidRequestException", "Name is required", http.StatusBadRequest)
		return
	}
	if _, exists := smSecrets.Get(req.Name); exists {
		sim.AWSErrorf(w, "ResourceExistsException", http.StatusBadRequest,
			"The operation failed because the secret %s already exists.", req.Name)
		return
	}

	now := float64(time.Now().Unix())
	secret := SMSecret{
		ARN:             smArn(req.Name),
		Name:            req.Name,
		Description:     req.Description,
		KmsKeyId:        req.KmsKeyId,
		CreatedDate:     now,
		LastChangedDate: now,
		VersionId:       generateUUID(),
		SecretString:    req.SecretString,
		SecretBinary:    req.SecretBinary,
		Tags:            req.Tags,
	}
	smSecrets.Put(req.Name, secret)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"ARN":       secret.ARN,
		"Name":      secret.Name,
		"VersionId": secret.VersionId,
	})
}

func handleSMGetSecretValue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SecretId string `json:"SecretId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequestException", "Invalid request body", http.StatusBadRequest)
		return
	}
	secret, ok := smSecrets.Get(req.SecretId)
	if !ok {
		// Real AWS accepts both the secret name and the full ARN as
		// SecretId. Fall back to scanning the store by ARN so callers
		// using the ARN form (terraform's secret data source, the SDK's
		// auto-resolved ARN) round-trip.
		secret, ok = lookupSecretByArn(req.SecretId)
	}
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Secrets Manager can't find the specified secret.")
		return
	}
	resp := map[string]any{
		"ARN":           secret.ARN,
		"Name":          secret.Name,
		"VersionId":     secret.VersionId,
		"CreatedDate":   secret.CreatedDate,
		"VersionStages": []string{"AWSCURRENT"},
	}
	if secret.SecretString != "" {
		resp["SecretString"] = secret.SecretString
	}
	if len(secret.SecretBinary) > 0 {
		resp["SecretBinary"] = base64.StdEncoding.EncodeToString(secret.SecretBinary)
	}

	// Real AWS records LastAccessedDate on read.
	smSecrets.Update(secret.Name, func(s *SMSecret) {
		s.LastAccessedDate = float64(time.Now().Unix())
	})

	sim.WriteJSON(w, http.StatusOK, resp)
}

func handleSMDescribeSecret(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SecretId string `json:"SecretId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequestException", "Invalid request body", http.StatusBadRequest)
		return
	}
	secret, ok := smSecrets.Get(req.SecretId)
	if !ok {
		secret, ok = lookupSecretByArn(req.SecretId)
	}
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Secrets Manager can't find the specified secret.")
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"ARN":              secret.ARN,
		"Name":             secret.Name,
		"Description":      secret.Description,
		"KmsKeyId":         secret.KmsKeyId,
		"CreatedDate":      secret.CreatedDate,
		"LastChangedDate":  secret.LastChangedDate,
		"LastAccessedDate": secret.LastAccessedDate,
		"VersionIdsToStages": map[string][]string{
			secret.VersionId: {"AWSCURRENT"},
		},
		"Tags": secret.Tags,
	})
}

func handleSMUpdateSecret(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SecretId     string `json:"SecretId"`
		Description  string `json:"Description"`
		KmsKeyId     string `json:"KmsKeyId"`
		SecretString string `json:"SecretString"`
		SecretBinary []byte `json:"SecretBinary"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequestException", "Invalid request body", http.StatusBadRequest)
		return
	}
	name, ok := resolveSecretName(req.SecretId)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Secrets Manager can't find the specified secret.")
		return
	}
	now := float64(time.Now().Unix())
	newVersion := generateUUID()
	smSecrets.Update(name, func(s *SMSecret) {
		if req.Description != "" {
			s.Description = req.Description
		}
		if req.KmsKeyId != "" {
			s.KmsKeyId = req.KmsKeyId
		}
		if req.SecretString != "" {
			s.SecretString = req.SecretString
			s.VersionId = newVersion
		}
		if len(req.SecretBinary) > 0 {
			s.SecretBinary = req.SecretBinary
			s.VersionId = newVersion
		}
		s.LastChangedDate = now
	})
	updated, _ := smSecrets.Get(name)
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"ARN":       updated.ARN,
		"Name":      updated.Name,
		"VersionId": updated.VersionId,
	})
}

func handleSMPutSecretValue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SecretId     string `json:"SecretId"`
		SecretString string `json:"SecretString"`
		SecretBinary []byte `json:"SecretBinary"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequestException", "Invalid request body", http.StatusBadRequest)
		return
	}
	name, ok := resolveSecretName(req.SecretId)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Secrets Manager can't find the specified secret.")
		return
	}
	now := float64(time.Now().Unix())
	newVersion := generateUUID()
	smSecrets.Update(name, func(s *SMSecret) {
		if req.SecretString != "" {
			s.SecretString = req.SecretString
		}
		if len(req.SecretBinary) > 0 {
			s.SecretBinary = req.SecretBinary
		}
		s.VersionId = newVersion
		s.LastChangedDate = now
	})
	updated, _ := smSecrets.Get(name)
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"ARN":           updated.ARN,
		"Name":          updated.Name,
		"VersionId":     updated.VersionId,
		"VersionStages": []string{"AWSCURRENT"},
	})
}

func handleSMDeleteSecret(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SecretId                   string `json:"SecretId"`
		ForceDeleteWithoutRecovery bool   `json:"ForceDeleteWithoutRecovery"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequestException", "Invalid request body", http.StatusBadRequest)
		return
	}
	name, ok := resolveSecretName(req.SecretId)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Secrets Manager can't find the specified secret.")
		return
	}
	secret, _ := smSecrets.Get(name)
	smSecrets.Delete(name)
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"ARN":          secret.ARN,
		"Name":         secret.Name,
		"DeletionDate": float64(time.Now().Unix()),
	})
}

func handleSMListSecrets(w http.ResponseWriter, r *http.Request) {
	all := smSecrets.List()
	if all == nil {
		all = []SMSecret{}
	}
	out := make([]map[string]any, 0, len(all))
	for _, s := range all {
		out = append(out, map[string]any{
			"ARN":              s.ARN,
			"Name":             s.Name,
			"Description":      s.Description,
			"KmsKeyId":         s.KmsKeyId,
			"CreatedDate":      s.CreatedDate,
			"LastChangedDate":  s.LastChangedDate,
			"LastAccessedDate": s.LastAccessedDate,
			"Tags":             s.Tags,
		})
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"SecretList": out})
}

func handleSMTagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SecretId string  `json:"SecretId"`
		Tags     []SMTag `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequestException", "Invalid request body", http.StatusBadRequest)
		return
	}
	name, ok := resolveSecretName(req.SecretId)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Secrets Manager can't find the specified secret.")
		return
	}
	smSecrets.Update(name, func(s *SMSecret) {
		// Real AWS overwrites by Key when tagging the same key twice.
		merged := make(map[string]string)
		for _, t := range s.Tags {
			merged[t.Key] = t.Value
		}
		for _, t := range req.Tags {
			merged[t.Key] = t.Value
		}
		s.Tags = nil
		for k, v := range merged {
			s.Tags = append(s.Tags, SMTag{Key: k, Value: v})
		}
	})
	w.WriteHeader(http.StatusOK)
}

func handleSMUntagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SecretId string   `json:"SecretId"`
		TagKeys  []string `json:"TagKeys"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidRequestException", "Invalid request body", http.StatusBadRequest)
		return
	}
	name, ok := resolveSecretName(req.SecretId)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"Secrets Manager can't find the specified secret.")
		return
	}
	keep := make(map[string]bool)
	for _, t := range req.TagKeys {
		keep[t] = true
	}
	smSecrets.Update(name, func(s *SMSecret) {
		var filtered []SMTag
		for _, t := range s.Tags {
			if !keep[t.Key] {
				filtered = append(filtered, t)
			}
		}
		s.Tags = filtered
	})
	w.WriteHeader(http.StatusOK)
}

// resolveSecretName accepts either a secret name or a full ARN and
// returns the canonical name used as the store key.
func resolveSecretName(idOrArn string) (string, bool) {
	if _, ok := smSecrets.Get(idOrArn); ok {
		return idOrArn, true
	}
	for _, s := range smSecrets.List() {
		if s.ARN == idOrArn {
			return s.Name, true
		}
	}
	return "", false
}

// lookupSecretByArn finds a secret by its ARN. Returns the secret and
// whether it was found.
func lookupSecretByArn(arn string) (SMSecret, bool) {
	for _, s := range smSecrets.List() {
		if s.ARN == arn {
			return s, true
		}
	}
	return SMSecret{}, false
}
