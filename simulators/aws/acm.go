package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// AWS Certificate Manager. Wire: AWS-JSON 1.1 (POST /, X-Amz-Target =
// "CertificateManager.<Op>"). Sim eagerly transitions issued
// certificates to Status=ISSUED instead of synthesising the real
// AWS DNS-validation polling window (out of scope per Phase 159 plan).

// ---------- Types ----------

// AWS-JSON 1.1 encodes timestamps as Unix-epoch JSON numbers (seconds
// with optional fractional part), not RFC3339 strings. The SDK
// deserialiser fails with "expected TStamp to be a JSON Number, got
// string instead" if we send a string. Using float64 keeps it lossless.
type ACMCertificate struct {
	CertificateArn          string                      `json:"CertificateArn"`
	DomainName              string                      `json:"DomainName"`
	SubjectAlternativeNames []string                    `json:"SubjectAlternativeNames,omitempty"`
	DomainValidationOptions []ACMDomainValidationOption `json:"DomainValidationOptions,omitempty"`
	Status                  string                      `json:"Status"`
	IssuedAt                *float64                    `json:"IssuedAt,omitempty"`
	ImportedAt              *float64                    `json:"ImportedAt,omitempty"`
	NotBefore               *float64                    `json:"NotBefore,omitempty"`
	NotAfter                *float64                    `json:"NotAfter,omitempty"`
	KeyAlgorithm            string                      `json:"KeyAlgorithm,omitempty"`
	SignatureAlgorithm      string                      `json:"SignatureAlgorithm,omitempty"`
	InUseBy                 []string                    `json:"InUseBy"`
	Type                    string                      `json:"Type"`
	RenewalEligibility      string                      `json:"RenewalEligibility,omitempty"`
	Options                 *ACMCertificateOptions      `json:"Options,omitempty"`
	CreatedAt               *float64                    `json:"CreatedAt,omitempty"`
	CertificateAuthorityArn string                      `json:"CertificateAuthorityArn,omitempty"`
	Serial                  string                      `json:"Serial,omitempty"`
	Subject                 string                      `json:"Subject,omitempty"`
	Issuer                  string                      `json:"Issuer,omitempty"`
}

func acmEpochNow() *float64 {
	f := float64(time.Now().UTC().Unix())
	return &f
}

type ACMDomainValidationOption struct {
	DomainName       string             `json:"DomainName"`
	ValidationDomain string             `json:"ValidationDomain,omitempty"`
	ValidationMethod string             `json:"ValidationMethod,omitempty"`
	ValidationStatus string             `json:"ValidationStatus,omitempty"`
	ResourceRecord   *ACMResourceRecord `json:"ResourceRecord,omitempty"`
}

type ACMResourceRecord struct {
	Name  string `json:"Name"`
	Type  string `json:"Type"`
	Value string `json:"Value"`
}

type ACMCertificateOptions struct {
	CertificateTransparencyLoggingPreference string `json:"CertificateTransparencyLoggingPreference,omitempty"`
}

type acmTag struct {
	Key   string `json:"Key"`
	Value string `json:"Value,omitempty"`
}

type acmStoredCert struct {
	Cert ACMCertificate
	Tags []acmTag
}

var (
	acmCertificates sim.Store[acmStoredCert]
)

// acmCertARN constructs an ARN for the simulator's region. Real ACM
// pins us-east-1 only for CloudFront associations — that constraint
// is enforced on the CloudFront side (cloudfront.go) against the
// region embedded in the ARN, not here at certificate creation time.
func acmCertARN(id string) string {
	return fmt.Sprintf("arn:aws:acm:%s:%s:certificate/%s", awsRegion(), awsAccountID(), id)
}

func acmRandomID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	hex := hex.EncodeToString(buf)
	// AWS uses a UUID-like format with dashes
	return hex[0:8] + "-" + hex[8:12] + "-" + hex[12:16] + "-" + hex[16:20] + "-" + hex[20:32]
}

func acmARNToID(arn string) string {
	const prefix = "certificate/"
	i := strings.LastIndex(arn, prefix)
	if i < 0 {
		return ""
	}
	return arn[i+len(prefix):]
}

// ---------- Registration ----------

func registerACM(r *sim.AWSRouter, srv *sim.Server) {
	acmCertificates = sim.MakeStore[acmStoredCert](srv.DB(), "acm_certificates")

	r.Register("CertificateManager.RequestCertificate", handleACMRequestCertificate)
	r.Register("CertificateManager.DescribeCertificate", handleACMDescribeCertificate)
	r.Register("CertificateManager.DeleteCertificate", handleACMDeleteCertificate)
	r.Register("CertificateManager.ListCertificates", handleACMListCertificates)
	r.Register("CertificateManager.AddTagsToCertificate", handleACMAddTags)
	r.Register("CertificateManager.RemoveTagsFromCertificate", handleACMRemoveTags)
	r.Register("CertificateManager.ListTagsForCertificate", handleACMListTags)
	r.Register("CertificateManager.ImportCertificate", handleACMImportCertificate)
	r.Register("CertificateManager.UpdateCertificateOptions", handleACMUpdateOptions)
	r.Register("CertificateManager.ResendValidationEmail", handleACMResendValidationEmail)
	r.Register("CertificateManager.RenewCertificate", handleACMRenewCertificate)
}

// acmWriteJSON / acmWriteError — JSON-1.1 protocol wraps errors in
// {"__type": "Code", "message": "..."}; status is 400 for invalid /
// 200 + body for normal success. ACM only returns 400 on errors —
// real ACM does not use 404 for missing resources.
func acmWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func acmWriteError(w http.ResponseWriter, code, msg string) {
	acmWriteJSON(w, http.StatusBadRequest, map[string]string{
		"__type":  code,
		"message": msg,
	})
}

// ---------- Handlers ----------

type acmRequestCertificateReq struct {
	DomainName              string                      `json:"DomainName"`
	ValidationMethod        string                      `json:"ValidationMethod"`
	SubjectAlternativeNames []string                    `json:"SubjectAlternativeNames,omitempty"`
	IdempotencyToken        string                      `json:"IdempotencyToken,omitempty"`
	DomainValidationOptions []ACMDomainValidationOption `json:"DomainValidationOptions,omitempty"`
	Options                 *ACMCertificateOptions      `json:"Options,omitempty"`
	CertificateAuthorityArn string                      `json:"CertificateAuthorityArn,omitempty"`
	Tags                    []acmTag                    `json:"Tags,omitempty"`
	KeyAlgorithm            string                      `json:"KeyAlgorithm,omitempty"`
}

func handleACMRequestCertificate(w http.ResponseWriter, r *http.Request) {
	var req acmRequestCertificateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	if req.DomainName == "" {
		acmWriteError(w, "InvalidParameterValueException", "DomainName is required")
		return
	}
	method := req.ValidationMethod
	if method == "" {
		method = "EMAIL" // real ACM default
	}
	id := acmRandomID()
	now := acmEpochNow()
	domains := append([]string{req.DomainName}, req.SubjectAlternativeNames...)
	dvOpts := make([]ACMDomainValidationOption, 0, len(domains))
	for _, d := range domains {
		opt := ACMDomainValidationOption{
			DomainName:       d,
			ValidationDomain: d,
			ValidationMethod: method,
			ValidationStatus: "PENDING_VALIDATION",
		}
		if method == "DNS" {
			opt.ResourceRecord = &ACMResourceRecord{
				Name:  "_acm-challenge." + d + ".",
				Type:  "CNAME",
				Value: "_acm-challenge-" + id[:8] + ".acm-validations.aws.",
			}
		}
		dvOpts = append(dvOpts, opt)
	}
	cert := ACMCertificate{
		CertificateArn:          acmCertARN(id),
		DomainName:              req.DomainName,
		SubjectAlternativeNames: req.SubjectAlternativeNames,
		DomainValidationOptions: dvOpts,
		Status:                  "PENDING_VALIDATION",
		Type:                    "AMAZON_ISSUED",
		RenewalEligibility:      "INELIGIBLE",
		KeyAlgorithm:            firstNonEmpty(req.KeyAlgorithm, "RSA-2048"),
		SignatureAlgorithm:      "SHA256WITHRSA",
		Options:                 req.Options,
		CreatedAt:               now,
		InUseBy:                 []string{},
	}
	if req.CertificateAuthorityArn != "" {
		cert.CertificateAuthorityArn = req.CertificateAuthorityArn
		cert.Type = "PRIVATE"
	}
	acmCertificates.Put(id, acmStoredCert{Cert: cert, Tags: req.Tags})
	acmWriteJSON(w, http.StatusOK, map[string]string{"CertificateArn": cert.CertificateArn})
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

type acmCertARNReq struct {
	CertificateArn string `json:"CertificateArn"`
}

func handleACMDescribeCertificate(w http.ResponseWriter, r *http.Request) {
	var req acmCertARNReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	id := acmARNToID(req.CertificateArn)
	stored, ok := acmCertificates.Get(id)
	if !ok {
		acmWriteError(w, "ResourceNotFoundException", "Could not find certificate "+req.CertificateArn)
		return
	}
	acmWriteJSON(w, http.StatusOK, map[string]ACMCertificate{"Certificate": stored.Cert})
}

func handleACMDeleteCertificate(w http.ResponseWriter, r *http.Request) {
	var req acmCertARNReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	id := acmARNToID(req.CertificateArn)
	stored, ok := acmCertificates.Get(id)
	if !ok {
		acmWriteError(w, "ResourceNotFoundException", "Could not find certificate "+req.CertificateArn)
		return
	}
	if len(stored.Cert.InUseBy) > 0 {
		acmWriteError(w, "ResourceInUseException", "Certificate is in use and cannot be deleted")
		return
	}
	acmCertificates.Delete(id)
	acmWriteJSON(w, http.StatusOK, struct{}{})
}

type acmCertSummary struct {
	CertificateArn                  string   `json:"CertificateArn"`
	DomainName                      string   `json:"DomainName"`
	SubjectAlternativeNameSummaries []string `json:"SubjectAlternativeNameSummaries,omitempty"`
	Status                          string   `json:"Status"`
	Type                            string   `json:"Type"`
	KeyAlgorithm                    string   `json:"KeyAlgorithm,omitempty"`
	CreatedAt                       *float64 `json:"CreatedAt,omitempty"`
}

func handleACMListCertificates(w http.ResponseWriter, r *http.Request) {
	items := []acmCertSummary{}
	for _, stored := range acmCertificates.List() {
		c := stored.Cert
		items = append(items, acmCertSummary{
			CertificateArn:                  c.CertificateArn,
			DomainName:                      c.DomainName,
			SubjectAlternativeNameSummaries: c.SubjectAlternativeNames,
			Status:                          c.Status,
			Type:                            c.Type,
			KeyAlgorithm:                    c.KeyAlgorithm,
			CreatedAt:                       c.CreatedAt,
		})
	}
	acmWriteJSON(w, http.StatusOK, map[string]any{
		"CertificateSummaryList": items,
	})
}

type acmTagReq struct {
	CertificateArn string   `json:"CertificateArn"`
	Tags           []acmTag `json:"Tags"`
}

func handleACMAddTags(w http.ResponseWriter, r *http.Request) {
	var req acmTagReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	id := acmARNToID(req.CertificateArn)
	stored, ok := acmCertificates.Get(id)
	if !ok {
		acmWriteError(w, "ResourceNotFoundException", "Could not find certificate "+req.CertificateArn)
		return
	}
	tagMap := map[string]string{}
	for _, t := range stored.Tags {
		tagMap[t.Key] = t.Value
	}
	for _, t := range req.Tags {
		tagMap[t.Key] = t.Value
	}
	merged := make([]acmTag, 0, len(tagMap))
	for k, v := range tagMap {
		merged = append(merged, acmTag{Key: k, Value: v})
	}
	stored.Tags = merged
	acmCertificates.Put(id, stored)
	acmWriteJSON(w, http.StatusOK, struct{}{})
}

type acmRemoveTagsReq struct {
	CertificateArn string   `json:"CertificateArn"`
	Tags           []acmTag `json:"Tags"`
}

func handleACMRemoveTags(w http.ResponseWriter, r *http.Request) {
	var req acmRemoveTagsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	id := acmARNToID(req.CertificateArn)
	stored, ok := acmCertificates.Get(id)
	if !ok {
		acmWriteError(w, "ResourceNotFoundException", "Could not find certificate "+req.CertificateArn)
		return
	}
	drop := map[string]bool{}
	for _, t := range req.Tags {
		drop[t.Key] = true
	}
	kept := stored.Tags[:0]
	for _, t := range stored.Tags {
		if !drop[t.Key] {
			kept = append(kept, t)
		}
	}
	stored.Tags = kept
	acmCertificates.Put(id, stored)
	acmWriteJSON(w, http.StatusOK, struct{}{})
}

func handleACMListTags(w http.ResponseWriter, r *http.Request) {
	var req acmCertARNReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	id := acmARNToID(req.CertificateArn)
	stored, ok := acmCertificates.Get(id)
	if !ok {
		acmWriteError(w, "ResourceNotFoundException", "Could not find certificate "+req.CertificateArn)
		return
	}
	acmWriteJSON(w, http.StatusOK, map[string][]acmTag{"Tags": stored.Tags})
}

type acmImportCertificateReq struct {
	CertificateArn   string   `json:"CertificateArn,omitempty"`
	Certificate      string   `json:"Certificate"` // base64-encoded by SDK; sim stores opaque
	PrivateKey       string   `json:"PrivateKey"`
	CertificateChain string   `json:"CertificateChain,omitempty"`
	Tags             []acmTag `json:"Tags,omitempty"`
}

func handleACMImportCertificate(w http.ResponseWriter, r *http.Request) {
	var req acmImportCertificateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	if req.Certificate == "" || req.PrivateKey == "" {
		acmWriteError(w, "InvalidParameterValueException", "Certificate and PrivateKey are required")
		return
	}
	now := acmEpochNow()
	// If CertificateArn is provided, this is an update — replace in place.
	id := acmARNToID(req.CertificateArn)
	if id == "" {
		id = acmRandomID()
	}
	cert := ACMCertificate{
		CertificateArn:     acmCertARN(id),
		Status:             "ISSUED",
		Type:               "IMPORTED",
		ImportedAt:         now,
		CreatedAt:          now,
		IssuedAt:           now,
		KeyAlgorithm:       "RSA-2048",
		SignatureAlgorithm: "SHA256WITHRSA",
		InUseBy:            []string{},
		// Sim doesn't parse the PEM — DomainName is left empty unless the
		// caller updates it via a follow-up flow. Terraform-aws-provider
		// uses the embedded x509 cert to read DomainName; for now we leave
		// a synthesised placeholder so the SDK contract holds.
		DomainName: "imported-" + id[:8] + ".example.com",
	}
	acmCertificates.Put(id, acmStoredCert{Cert: cert, Tags: req.Tags})
	acmWriteJSON(w, http.StatusOK, map[string]string{"CertificateArn": cert.CertificateArn})
}

type acmUpdateOptionsReq struct {
	CertificateArn string                 `json:"CertificateArn"`
	Options        *ACMCertificateOptions `json:"Options"`
}

func handleACMUpdateOptions(w http.ResponseWriter, r *http.Request) {
	var req acmUpdateOptionsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	id := acmARNToID(req.CertificateArn)
	stored, ok := acmCertificates.Get(id)
	if !ok {
		acmWriteError(w, "ResourceNotFoundException", "Could not find certificate "+req.CertificateArn)
		return
	}
	stored.Cert.Options = req.Options
	acmCertificates.Put(id, stored)
	acmWriteJSON(w, http.StatusOK, struct{}{})
}

func handleACMResendValidationEmail(w http.ResponseWriter, r *http.Request) {
	// Stub — accepted but no-op (real ACM re-sends the validation email).
	acmWriteJSON(w, http.StatusOK, struct{}{})
}

func handleACMRenewCertificate(w http.ResponseWriter, r *http.Request) {
	var req acmCertARNReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	id := acmARNToID(req.CertificateArn)
	stored, ok := acmCertificates.Get(id)
	if !ok {
		acmWriteError(w, "ResourceNotFoundException", "Could not find certificate "+req.CertificateArn)
		return
	}
	// In real ACM, RenewCertificate is async — sim just refreshes IssuedAt.
	stored.Cert.IssuedAt = acmEpochNow()
	acmCertificates.Put(id, stored)
	acmWriteJSON(w, http.StatusOK, struct{}{})
}

// ---------- CloudFront cross-resource enforcement helper ----------

// acmCertExistsInRegion checks whether the given certificate ARN exists
// AND was issued in the named region. Returns (true, true) only if both
// hold. (false, false) for missing; (true, false) for region-mismatch.
// Used by cloudfront.go to enforce the us-east-1 pin on
// ViewerCertificate.ACMCertificateArn references.
func acmCertExistsInRegion(arn, requireRegion string) (exists bool, regionMatch bool) {
	id := acmARNToID(arn)
	if id == "" {
		return false, false
	}
	if _, ok := acmCertificates.Get(id); !ok {
		return false, false
	}
	// ARN form: arn:aws:acm:<region>:<account>:certificate/<id>
	parts := strings.Split(arn, ":")
	if len(parts) < 4 {
		return true, false
	}
	return true, parts[3] == requireRegion
}
