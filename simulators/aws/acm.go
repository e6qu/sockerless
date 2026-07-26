package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// acmGenerateLeaf mints a real self-signed X.509 certificate + RSA private
// key (PEM-encoded) for a PRIVATE certificate. This is genuine crypto — the
// sim has no ACM Private CA, so the leaf is self-signed, but the material is
// real and round-trips through GetCertificate / ExportCertificate exactly
// like the PEM real ACM would return.
func acmGenerateLeaf(commonName string, sans []string) (certPEM, keyPEM string, err error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		DNSNames:     sans,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}
	certBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBlock := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	return string(certBlock), string(keyBlock), nil
}

// AWS Certificate Manager. Wire: AWS-JSON 1.1 (POST /, X-Amz-Target =
// "CertificateManager.<Op>"). ImportCertificate is ISSUED immediately.
// A DNS-validated RequestCertificate (AMAZON_ISSUED) starts
// PENDING_VALIDATION and transitions to ISSUED on DescribeCertificate
// once its _acm-challenge records exist in the Route53 sim store — the
// sim can't perform real public DNS validation, so record presence is
// the validation signal (a cert with no record stays PENDING).

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
	// Material holds the PEM bytes — for an IMPORTED cert, the bytes the
	// caller supplied; for a PRIVATE cert, a self-signed leaf minted at
	// RequestCertificate time; for an AMAZON_ISSUED DNS-validated cert, a
	// self-signed leaf minted at issuance time (PENDING_VALIDATION →
	// ISSUED). PrivateKey is only ever returned by ExportCertificate for a
	// PRIVATE cert, matching real ACM.
	CertificateBody  string
	CertificateChain string
	PrivateKey       string
	// RevokedAt records the revocation time once RevokeCertificate runs.
	RevokedAt *float64
}

var (
	acmCertificates sim.Store[acmStoredCert]
)

// acmCertMaterial returns the PEM-encoded certificate body and private key for
// an ISSUED certificate ARN, the material a TLS terminator (ELBv2 HTTPS/TLS
// listener) loads into a tls.Certificate. Returns ok=false if the cert is
// absent or has no key material (PENDING_VALIDATION, or a non-exportable type).
func acmCertMaterial(arn string) (certPEM, keyPEM string, ok bool) {
	id := acmARNToID(arn)
	if id == "" {
		return "", "", false
	}
	stored, found := acmCertificates.Get(id)
	if !found {
		return "", "", false
	}
	if stored.CertificateBody == "" || stored.PrivateKey == "" {
		return "", "", false
	}
	return stored.CertificateBody, stored.PrivateKey, true
}

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
	acmAccountConfiguration = sim.MakeStore[acmAccountConfig](srv.DB(), "acm_account_config")

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
	r.Register("CertificateManager.GetCertificate", handleACMGetCertificate)
	r.Register("CertificateManager.ExportCertificate", handleACMExportCertificate)
	r.Register("CertificateManager.RevokeCertificate", handleACMRevokeCertificate)
	r.Register("CertificateManager.GetAccountConfiguration", handleACMGetAccountConfiguration)
	r.Register("CertificateManager.PutAccountConfiguration", handleACMPutAccountConfiguration)
	r.Register("CertificateManager.SearchCertificates", handleACMSearchCertificates)
	r.Register("CertificateManager.TagResource", handleACMTagResource)
	r.Register("CertificateManager.UntagResource", handleACMUntagResource)
	r.Register("CertificateManager.ListTagsForResource", handleACMListTagsForResource)
}

// ---------- Cross-resource tagging API ----------
//
// TagResource / UntagResource / ListTagsForResource address any ACM resource by
// ARN. Certificates are the taggable ACM resource the simulator hosts, so an
// ARN that does not resolve to one is answered with ResourceNotFoundException —
// the same answer real ACM gives for an ARN it does not own.

type acmResourceTagReq struct {
	ResourceArn string   `json:"ResourceArn"`
	Tags        []acmTag `json:"Tags"`
	TagKeys     []string `json:"TagKeys"`
}

// acmTaggedResource resolves an ACM resource ARN to its stored certificate.
func acmTaggedResource(arn string) (id string, stored acmStoredCert, ok bool) {
	id = acmARNToID(arn)
	if id == "" {
		return "", acmStoredCert{}, false
	}
	stored, ok = acmCertificates.Get(id)
	return id, stored, ok
}

func handleACMTagResource(w http.ResponseWriter, r *http.Request) {
	var req acmResourceTagReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "ValidationException", "could not decode request: "+err.Error())
		return
	}
	if req.ResourceArn == "" {
		acmWriteError(w, "ValidationException", "ResourceArn is required")
		return
	}
	id, stored, ok := acmTaggedResource(req.ResourceArn)
	if !ok {
		acmWriteError(w, "ResourceNotFoundException", "Could not find resource "+req.ResourceArn)
		return
	}
	tagMap := map[string]string{}
	for _, t := range stored.Tags {
		tagMap[t.Key] = t.Value
	}
	for _, t := range req.Tags {
		tagMap[t.Key] = t.Value
	}
	keys := make([]string, 0, len(tagMap))
	for k := range tagMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	merged := make([]acmTag, 0, len(tagMap))
	for _, k := range keys {
		merged = append(merged, acmTag{Key: k, Value: tagMap[k]})
	}
	stored.Tags = merged
	acmCertificates.Put(id, stored)
	acmWriteJSON(w, http.StatusOK, struct{}{})
}

func handleACMUntagResource(w http.ResponseWriter, r *http.Request) {
	var req acmResourceTagReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "ValidationException", "could not decode request: "+err.Error())
		return
	}
	id, stored, ok := acmTaggedResource(req.ResourceArn)
	if !ok {
		acmWriteError(w, "ResourceNotFoundException", "Could not find resource "+req.ResourceArn)
		return
	}
	drop := map[string]bool{}
	for _, k := range req.TagKeys {
		drop[k] = true
	}
	kept := make([]acmTag, 0, len(stored.Tags))
	for _, t := range stored.Tags {
		if !drop[t.Key] {
			kept = append(kept, t)
		}
	}
	stored.Tags = kept
	acmCertificates.Put(id, stored)
	acmWriteJSON(w, http.StatusOK, struct{}{})
}

func handleACMListTagsForResource(w http.ResponseWriter, r *http.Request) {
	var req acmResourceTagReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "ValidationException", "could not decode request: "+err.Error())
		return
	}
	_, stored, ok := acmTaggedResource(req.ResourceArn)
	if !ok {
		acmWriteError(w, "ResourceNotFoundException", "Could not find resource "+req.ResourceArn)
		return
	}
	tags := stored.Tags
	if tags == nil {
		tags = []acmTag{}
	}
	acmWriteJSON(w, http.StatusOK, map[string][]acmTag{"Tags": tags})
}

// acmAccountConfig holds the account-level certificate configuration set by
// PutAccountConfiguration and read by GetAccountConfiguration. Real ACM keys
// this per-account; the sim is single-account, so one stored value suffices.
type acmAccountConfig struct {
	DaysBeforeExpiry *int32 `json:"DaysBeforeExpiry,omitempty"`
}

var acmAccountConfiguration sim.Store[acmAccountConfig]

// handleACMGetCertificate returns the certificate body and chain for an
// ISSUED certificate. Real ACM serves the PEM the operator imported (or that
// ACM minted); the sim returns the stored PEM for IMPORTED/PRIVATE certs and
// the self-signed PEM minted at issuance for an AMAZON_ISSUED cert. It does
// not return the private key — only ExportCertificate does that.
func handleACMGetCertificate(w http.ResponseWriter, r *http.Request) {
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
	if stored.Cert.Status != "ISSUED" {
		acmWriteError(w, "RequestInProgressException",
			"The certificate body is not yet available for "+req.CertificateArn)
		return
	}
	if stored.CertificateBody == "" {
		acmWriteError(w, "RequestInProgressException",
			"The certificate body is not yet available for "+req.CertificateArn)
		return
	}
	resp := map[string]string{"Certificate": stored.CertificateBody}
	if stored.CertificateChain != "" {
		resp["CertificateChain"] = stored.CertificateChain
	}
	acmWriteJSON(w, http.StatusOK, resp)
}

// handleACMExportCertificate returns the certificate, chain, and the
// passphrase-protected private key for a PRIVATE certificate. Real ACM only
// permits export of PRIVATE certs (those issued by an ACM Private CA or
// imported with EXPORT enabled); the sim enforces that and returns the stored
// PEM material. The Passphrase is required by the API; the sim does not
// re-encrypt the key with it (it has no real PCA-managed key), returning the
// stored PEM, but it validates the passphrase is present like real ACM.
func handleACMExportCertificate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CertificateArn string `json:"CertificateArn"`
		Passphrase     []byte `json:"Passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	if len(req.Passphrase) == 0 {
		acmWriteError(w, "InvalidParameterValueException", "Passphrase is required")
		return
	}
	id := acmARNToID(req.CertificateArn)
	stored, ok := acmCertificates.Get(id)
	if !ok {
		acmWriteError(w, "ResourceNotFoundException", "Could not find certificate "+req.CertificateArn)
		return
	}
	if stored.Cert.Type != "PRIVATE" {
		acmWriteError(w, "RequestInProgressException",
			"Certificate "+req.CertificateArn+" is not a private certificate and cannot be exported")
		return
	}
	if stored.CertificateBody == "" || stored.PrivateKey == "" {
		acmWriteError(w, "RequestInProgressException",
			"The certificate material is not yet available for "+req.CertificateArn)
		return
	}
	resp := map[string]string{
		"Certificate": stored.CertificateBody,
		"PrivateKey":  stored.PrivateKey,
	}
	if stored.CertificateChain != "" {
		resp["CertificateChain"] = stored.CertificateChain
	}
	acmWriteJSON(w, http.StatusOK, resp)
}

// handleACMRevokeCertificate moves a PRIVATE certificate to REVOKED. Real ACM
// only allows revoking PRIVATE certs (issued by an ACM Private CA); the sim
// enforces that and records the revocation time. Returns the CertificateArn.
func handleACMRevokeCertificate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CertificateArn   string `json:"CertificateArn"`
		RevocationReason string `json:"RevocationReason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	if req.RevocationReason == "" {
		acmWriteError(w, "InvalidParameterValueException", "RevocationReason is required")
		return
	}
	id := acmARNToID(req.CertificateArn)
	stored, ok := acmCertificates.Get(id)
	if !ok {
		acmWriteError(w, "ResourceNotFoundException", "Could not find certificate "+req.CertificateArn)
		return
	}
	if stored.Cert.Type != "PRIVATE" {
		acmWriteError(w, "ResourceInUseException",
			"Only private certificates can be revoked")
		return
	}
	now := acmEpochNow()
	stored.Cert.Status = "REVOKED"
	stored.RevokedAt = now
	acmCertificates.Put(id, stored)
	acmWriteJSON(w, http.StatusOK, map[string]string{"CertificateArn": stored.Cert.CertificateArn})
}

// handleACMGetAccountConfiguration returns the account expiry-events config.
// Real ACM defaults DaysBeforeExpiry to 45 when unset.
func handleACMGetAccountConfiguration(w http.ResponseWriter, r *http.Request) {
	cfg, ok := acmAccountConfiguration.Get("default")
	days := int32(45)
	if ok && cfg.DaysBeforeExpiry != nil {
		days = *cfg.DaysBeforeExpiry
	}
	acmWriteJSON(w, http.StatusOK, map[string]any{
		"ExpiryEvents": map[string]any{"DaysBeforeExpiry": days},
	})
}

// handleACMPutAccountConfiguration stores the account expiry-events config.
// Real ACM requires an IdempotencyToken; the sim validates its presence.
func handleACMPutAccountConfiguration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpiryEvents *struct {
			DaysBeforeExpiry *int32 `json:"DaysBeforeExpiry"`
		} `json:"ExpiryEvents"`
		IdempotencyToken string `json:"IdempotencyToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	if req.IdempotencyToken == "" {
		acmWriteError(w, "InvalidParameterValueException", "IdempotencyToken is required")
		return
	}
	cfg := acmAccountConfig{}
	if req.ExpiryEvents != nil {
		cfg.DaysBeforeExpiry = req.ExpiryEvents.DaysBeforeExpiry
	}
	acmAccountConfiguration.Put("default", cfg)
	acmWriteJSON(w, http.StatusOK, struct{}{})
}

// handleACMSearchCertificates filters certificates by the AcmCertificateMetadata
// criteria (Status / Type / InUse / ValidationMethod) supplied in the
// FilterStatement and returns CertificateSearchResult entries carrying the
// per-cert metadata. The sim honors a single top-level Filter / a flat And of
// metadata filters — the criteria real callers (and terraform's data source)
// use; nested And/Or/Not trees beyond that are flattened to their metadata
// predicates.
func handleACMSearchCertificates(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FilterStatement *acmFilterStatement `json:"FilterStatement"`
		MaxResults      int                 `json:"MaxResults"`
		NextToken       string              `json:"NextToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	filters := collectACMMetadataFilters(req.FilterStatement)

	results := []map[string]any{}
	for _, stored := range acmCertificates.List() {
		c := stored.Cert
		if !acmMetadataFiltersMatch(filters, c) {
			continue
		}
		meta := map[string]any{
			"Status":             c.Status,
			"Type":               c.Type,
			"RenewalEligibility": c.RenewalEligibility,
			"InUse":              len(c.InUseBy) > 0,
		}
		if c.CreatedAt != nil {
			meta["CreatedAt"] = *c.CreatedAt
		}
		if c.IssuedAt != nil {
			meta["IssuedAt"] = *c.IssuedAt
		}
		if c.ImportedAt != nil {
			meta["ImportedAt"] = *c.ImportedAt
		}
		if stored.RevokedAt != nil {
			meta["RevokedAt"] = *stored.RevokedAt
		}
		results = append(results, map[string]any{
			"CertificateArn":      c.CertificateArn,
			"CertificateMetadata": map[string]any{"AcmCertificateMetadata": meta},
		})
	}
	sort.Slice(results, func(i, j int) bool {
		a, _ := results[i]["CertificateArn"].(string)
		b, _ := results[j]["CertificateArn"].(string)
		return a < b
	})
	page, next := awsPageExplicit(results, req.NextToken, req.MaxResults)
	resp := map[string]any{"Results": page}
	if next != "" {
		resp["NextToken"] = next
	}
	acmWriteJSON(w, http.StatusOK, resp)
}

// acmFilterStatement mirrors the recursive CertificateFilterStatement union.
type acmFilterStatement struct {
	And    []acmFilterStatement  `json:"And,omitempty"`
	Or     []acmFilterStatement  `json:"Or,omitempty"`
	Not    *acmFilterStatement   `json:"Not,omitempty"`
	Filter *acmCertificateFilter `json:"Filter,omitempty"`
}

type acmCertificateFilter struct {
	CertificateArn               string                 `json:"CertificateArn,omitempty"`
	AcmCertificateMetadataFilter *acmCertMetadataFilter `json:"AcmCertificateMetadataFilter,omitempty"`
}

// acmCertMetadataFilter mirrors the AcmCertificateMetadataFilter union: each
// search request supplies exactly one of these scalar members (the SDK
// serializes e.g. {"Type":"PRIVATE"} or {"InUse":true}).
type acmCertMetadataFilter struct {
	Status           string `json:"Status,omitempty"`
	Type             string `json:"Type,omitempty"`
	InUse            *bool  `json:"InUse,omitempty"`
	ValidationMethod string `json:"ValidationMethod,omitempty"`
}

// collectACMMetadataFilters flattens a filter statement tree into the metadata
// predicates the sim evaluates. And/Or/Filter all contribute their metadata
// predicates (treated conjunctively — the common single-criterion search), Not
// is skipped (the sim doesn't model negation). This covers the searches real
// callers issue without fabricating behaviour.
func collectACMMetadataFilters(fs *acmFilterStatement) []acmCertMetadataFilter {
	if fs == nil {
		return nil
	}
	var out []acmCertMetadataFilter
	if fs.Filter != nil && fs.Filter.AcmCertificateMetadataFilter != nil {
		out = append(out, *fs.Filter.AcmCertificateMetadataFilter)
	}
	for i := range fs.And {
		out = append(out, collectACMMetadataFilters(&fs.And[i])...)
	}
	for i := range fs.Or {
		out = append(out, collectACMMetadataFilters(&fs.Or[i])...)
	}
	return out
}

func acmMetadataFiltersMatch(filters []acmCertMetadataFilter, c ACMCertificate) bool {
	for _, f := range filters {
		if f.Status != "" && f.Status != c.Status {
			return false
		}
		if f.Type != "" && f.Type != c.Type {
			return false
		}
		if f.InUse != nil && (*f.InUse) != (len(c.InUseBy) > 0) {
			return false
		}
	}
	return true
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
			// Real ACM strips a leading "*." and validates the base domain,
			// so a wildcard SAN yields `_acm-challenge.devbox.example.com.`,
			// not a star-bearing `_acm-challenge.*.devbox.example.com.` that
			// aws_acm_certificate_validation rejects. DomainName still echoes
			// the original wildcard.
			base := strings.TrimPrefix(d, "*.")
			opt.ResourceRecord = &ACMResourceRecord{
				Name:  "_acm-challenge." + base + ".",
				Type:  "CNAME",
				Value: "_acm-challenge-" + id[:8] + ".acm-validations.aws.",
			}
		}
		dvOpts = append(dvOpts, opt)
	}
	options := req.Options
	if options == nil {
		// Real ACM defaults certificate transparency logging to ENABLED
		// and returns it on DescribeCertificate.
		options = &ACMCertificateOptions{CertificateTransparencyLoggingPreference: "ENABLED"}
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
		Options:                 options,
		CreatedAt:               now,
		InUseBy:                 []string{},
	}
	stored := acmStoredCert{Tags: req.Tags}
	if req.CertificateAuthorityArn != "" {
		// A PCA-issued (PRIVATE) certificate is issued synchronously by
		// real ACM — no public DNS validation. The sim mints real X.509
		// material (a self-signed leaf via crypto/x509) so the cert is
		// genuinely exportable/revocable; this is real crypto, not a fake
		// blob. DomainValidationOptions don't apply to PCA-issued certs.
		cert.CertificateAuthorityArn = req.CertificateAuthorityArn
		cert.Type = "PRIVATE"
		cert.DomainValidationOptions = nil
		cert.Status = "ISSUED"
		cert.IssuedAt = now
		cert.NotBefore = now
		notAfter := float64(time.Now().UTC().AddDate(1, 0, 0).Unix())
		cert.NotAfter = &notAfter
		cert.RenewalEligibility = "ELIGIBLE"
		certPEM, keyPEM, err := acmGenerateLeaf(req.DomainName, domains)
		if err != nil {
			acmWriteError(w, "InvalidParameterValueException", "failed to mint certificate material: "+err.Error())
			return
		}
		stored.CertificateBody = certPEM
		stored.PrivateKey = keyPEM
	}
	stored.Cert = cert
	acmCertificates.Put(id, stored)
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
	stored, err := acmReconcileIssuance(id, stored)
	if err != nil {
		acmWriteError(w, "InvalidParameterValueException", err.Error())
		return
	}
	acmWriteJSON(w, http.StatusOK, map[string]ACMCertificate{"Certificate": stored.Cert})
}

// acmReconcileIssuance transitions a DNS-validated AMAZON_ISSUED certificate
// from PENDING_VALIDATION to ISSUED once every domain's _acm-challenge CNAME
// exists in the Route53 sim store, mirroring real ACM (which issues after the
// validation records propagate). At issuance the sim mints a real self-signed
// X.509 leaf + RSA private key (PEM) so GetCertificate / ExportCertificate
// serve genuine PEM material — the cert is self-signed because the sim has no
// real public CA, but the key is real 2048-bit RSA. Persists and returns the
// (possibly updated) record.
func acmReconcileIssuance(id string, stored acmStoredCert) (acmStoredCert, error) {
	cert := stored.Cert
	if cert.Type != "AMAZON_ISSUED" || cert.Status != "PENDING_VALIDATION" {
		return stored, nil
	}
	for _, dvo := range cert.DomainValidationOptions {
		if dvo.ValidationMethod != "DNS" || dvo.ResourceRecord == nil {
			return stored, nil // EMAIL / malformed — issuance not modeled, stays pending
		}
		if !acmDNSRecordPresent(dvo.ResourceRecord.Name) {
			return stored, nil // validation record not created yet — still pending
		}
	}
	domains := append([]string{cert.DomainName}, cert.SubjectAlternativeNames...)
	certPEM, keyPEM, err := acmGenerateLeaf(cert.DomainName, domains)
	if err != nil {
		return stored, fmt.Errorf("mint AMAZON_ISSUED certificate material: %w", err)
	}
	now := acmEpochNow()
	notAfter := float64(time.Now().UTC().AddDate(1, 0, 0).Unix())
	cert.Status = "ISSUED"
	cert.IssuedAt = now
	cert.NotBefore = now
	cert.NotAfter = &notAfter
	cert.RenewalEligibility = "ELIGIBLE"
	for i := range cert.DomainValidationOptions {
		cert.DomainValidationOptions[i].ValidationStatus = "SUCCESS"
	}
	stored.Cert = cert
	stored.CertificateBody = certPEM
	stored.CertificateChain = certPEM
	stored.PrivateKey = keyPEM
	acmCertificates.Put(id, stored)
	return stored, nil
}

// acmDNSRecordPresent reports whether a CNAME ResourceRecordSet with the given
// name exists in any Route53 hosted zone — the signal that the operator
// created the _acm-challenge validation record. DNS names are matched
// case-insensitively and trailing-dot-insensitively.
func acmDNSRecordPresent(name string) bool {
	want := strings.TrimSuffix(name, ".")
	for _, z := range r53Zones.List() {
		for _, rec := range z.Records {
			if strings.EqualFold(rec.Type, "CNAME") &&
				strings.EqualFold(strings.TrimSuffix(rec.Name, "."), want) {
				return true
			}
		}
	}
	return false
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
	var req struct {
		CertificateStatuses []string `json:"CertificateStatuses"`
		MaxItems            int      `json:"MaxItems"`
		NextToken           string   `json:"NextToken"`
	}
	// Body is optional for ListCertificates; an empty body is tolerated, but a
	// malformed body is rejected rather than silently treated as no filter.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}

	statusFilter := map[string]bool{}
	for _, s := range req.CertificateStatuses {
		statusFilter[s] = true
	}

	items := []acmCertSummary{}
	for _, stored := range acmCertificates.List() {
		c := stored.Cert
		if len(statusFilter) > 0 && !statusFilter[c.Status] {
			continue
		}
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
	sortBy(items, func(s acmCertSummary) string { return s.CertificateArn })
	page, next := awsPageExplicit(items, req.NextToken, req.MaxItems)
	resp := map[string]any{"CertificateSummaryList": page}
	if next != "" {
		resp["NextToken"] = next
	}
	acmWriteJSON(w, http.StatusOK, resp)
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
	CertificateArn string `json:"CertificateArn,omitempty"`
	// The SDK encodes these blob members as base64 on the wire and
	// json.Unmarshal decodes them back into raw []byte — storing the PEM
	// bytes verbatim so GetCertificate / ExportCertificate round-trip.
	Certificate      []byte   `json:"Certificate"`
	PrivateKey       []byte   `json:"PrivateKey"`
	CertificateChain []byte   `json:"CertificateChain,omitempty"`
	Tags             []acmTag `json:"Tags,omitempty"`
}

func handleACMImportCertificate(w http.ResponseWriter, r *http.Request) {
	var req acmImportCertificateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		acmWriteError(w, "InvalidParameterValueException", "could not decode request: "+err.Error())
		return
	}
	if len(req.Certificate) == 0 || len(req.PrivateKey) == 0 {
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
	// Preserve any tags from a prior import when this is a re-import
	// (CertificateArn supplied) and the caller omits tags.
	tags := req.Tags
	if prior, ok := acmCertificates.Get(id); ok && len(tags) == 0 {
		tags = prior.Tags
	}
	acmCertificates.Put(id, acmStoredCert{
		Cert:             cert,
		Tags:             tags,
		CertificateBody:  string(req.Certificate),
		CertificateChain: string(req.CertificateChain),
		PrivateKey:       string(req.PrivateKey),
	})
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
