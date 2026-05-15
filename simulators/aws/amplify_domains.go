package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	sim "github.com/sockerless/simulator"
)

// AWS Amplify domain associations + backend environments. Same wire
// pattern as amplify.go (REST + JSON, versionless paths under /apps/).

// ---------- Domain types ----------

type AmplifyDomainAssociation struct {
	DomainAssociationArn             string              `json:"domainAssociationArn"`
	DomainName                       string              `json:"domainName"`
	EnableAutoSubDomain              bool                `json:"enableAutoSubDomain"`
	AutoSubDomainCreationPatterns    []string            `json:"autoSubDomainCreationPatterns,omitempty"`
	AutoSubDomainIamRole             string              `json:"autoSubDomainIAMRole,omitempty"`
	DomainStatus                     string              `json:"domainStatus"`
	UpdateStatus                     string              `json:"updateStatus"`
	StatusReason                     string              `json:"statusReason,omitempty"`
	Certificate                      *AmplifyCertificate `json:"certificate,omitempty"`
	CertificateVerificationDNSRecord string              `json:"certificateVerificationDNSRecord,omitempty"`
	SubDomains                       []AmplifySubDomain  `json:"subDomains"`
}

type AmplifyCertificate struct {
	Type                             string `json:"type"`
	CustomCertificateArn             string `json:"customCertificateArn,omitempty"`
	CertificateVerificationDNSRecord string `json:"certificateVerificationDNSRecord,omitempty"`
}

type AmplifySubDomain struct {
	SubDomainSetting AmplifySubDomainSetting `json:"subDomainSetting"`
	Verified         bool                    `json:"verified"`
	DnsRecord        string                  `json:"dnsRecord"`
}

type AmplifySubDomainSetting struct {
	Prefix     string `json:"prefix"`
	BranchName string `json:"branchName"`
}

type amplifyStoredDomain struct {
	Domain AmplifyDomainAssociation
	AppId  string
}

// ---------- BackendEnvironment types ----------

type AmplifyBackendEnvironment struct {
	BackendEnvironmentArn string  `json:"backendEnvironmentArn"`
	EnvironmentName       string  `json:"environmentName"`
	StackName             string  `json:"stackName,omitempty"`
	DeploymentArtifacts   string  `json:"deploymentArtifacts,omitempty"`
	CreateTime            float64 `json:"createTime"`
	UpdateTime            float64 `json:"updateTime"`
}

type amplifyStoredBackend struct {
	Env   AmplifyBackendEnvironment
	AppId string
}

// ---------- State ----------

var (
	amplifyDomains  sim.Store[amplifyStoredDomain]
	amplifyBackends sim.Store[amplifyStoredBackend]
)

func amplifyDomainARN(appID, domain string) string {
	return fmt.Sprintf("arn:aws:amplify:%s:%s:apps/%s/domains/%s", awsRegion(), awsAccountID(), appID, domain)
}
func amplifyBackendARN(appID, name string) string {
	return fmt.Sprintf("arn:aws:amplify:%s:%s:apps/%s/backendenvironments/%s", awsRegion(), awsAccountID(), appID, name)
}
func amplifyDomainKey(appID, name string) string { return appID + "/" + name }

// registerAmplifyDomains is invoked from registerAmplify in amplify.go.
func registerAmplifyDomains(srv *sim.Server) {
	amplifyDomains = sim.MakeStore[amplifyStoredDomain](srv.DB(), "amplify_domains")
	amplifyBackends = sim.MakeStore[amplifyStoredBackend](srv.DB(), "amplify_backends")

	mux := srv.Mux()
	// Domains
	mux.HandleFunc("POST /apps/{appId}/domains", handleAmplifyCreateDomain)
	mux.HandleFunc("GET /apps/{appId}/domains", handleAmplifyListDomains)
	mux.HandleFunc("GET /apps/{appId}/domains/{domainName}", handleAmplifyGetDomain)
	mux.HandleFunc("POST /apps/{appId}/domains/{domainName}", handleAmplifyUpdateDomain)
	mux.HandleFunc("DELETE /apps/{appId}/domains/{domainName}", handleAmplifyDeleteDomain)
	// BackendEnvironments
	mux.HandleFunc("POST /apps/{appId}/backendenvironments", handleAmplifyCreateBackend)
	mux.HandleFunc("GET /apps/{appId}/backendenvironments", handleAmplifyListBackends)
	mux.HandleFunc("GET /apps/{appId}/backendenvironments/{environmentName}", handleAmplifyGetBackend)
	mux.HandleFunc("DELETE /apps/{appId}/backendenvironments/{environmentName}", handleAmplifyDeleteBackend)
}

// ---------- Domain handlers ----------

type amplifyCreateDomainReq struct {
	DomainName                    string                    `json:"domainName"`
	EnableAutoSubDomain           *bool                     `json:"enableAutoSubDomain,omitempty"`
	SubDomainSettings             []AmplifySubDomainSetting `json:"subDomainSettings,omitempty"`
	AutoSubDomainCreationPatterns []string                  `json:"autoSubDomainCreationPatterns,omitempty"`
	AutoSubDomainIamRole          string                    `json:"autoSubDomainIAMRole,omitempty"`
	Certificate                   *AmplifyCertificate       `json:"certificateSettings,omitempty"`
}

func handleAmplifyCreateDomain(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	if _, ok := amplifyApps.Get(appID); !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	var req amplifyCreateDomainReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode: "+err.Error())
		return
	}
	if req.DomainName == "" {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "domainName is required")
		return
	}
	subs := make([]AmplifySubDomain, 0, len(req.SubDomainSettings))
	for _, s := range req.SubDomainSettings {
		subs = append(subs, AmplifySubDomain{
			SubDomainSetting: s,
			Verified:         true, // sim: eager verification
			DnsRecord:        s.Prefix + "." + req.DomainName + " CNAME " + amplifyAppID(appID) + ".cloudfront.net.",
		})
	}
	cert := req.Certificate
	if cert == nil {
		cert = &AmplifyCertificate{Type: "AMPLIFY_MANAGED"}
	}
	domain := AmplifyDomainAssociation{
		DomainAssociationArn:             amplifyDomainARN(appID, req.DomainName),
		DomainName:                       req.DomainName,
		EnableAutoSubDomain:              boolOr(req.EnableAutoSubDomain, false),
		AutoSubDomainCreationPatterns:    req.AutoSubDomainCreationPatterns,
		AutoSubDomainIamRole:             req.AutoSubDomainIamRole,
		DomainStatus:                     "AVAILABLE", // sim: eager — real AWS goes through PENDING_VERIFICATION → AVAILABLE
		UpdateStatus:                     "UPDATE_COMPLETE",
		Certificate:                      cert,
		CertificateVerificationDNSRecord: "_acm-challenge." + req.DomainName + ". CNAME _validation.acm-validations.aws.",
		SubDomains:                       subs,
	}
	amplifyDomains.Put(amplifyDomainKey(appID, req.DomainName), amplifyStoredDomain{Domain: domain, AppId: appID})
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyDomainAssociation{"domainAssociation": domain})
}

func amplifyAppID(appID string) string { return appID }

func handleAmplifyGetDomain(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	name := r.PathValue("domainName")
	stored, ok := amplifyDomains.Get(amplifyDomainKey(appID, name))
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "domain not found")
		return
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyDomainAssociation{"domainAssociation": stored.Domain})
}

func handleAmplifyUpdateDomain(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	name := r.PathValue("domainName")
	key := amplifyDomainKey(appID, name)
	stored, ok := amplifyDomains.Get(key)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "domain not found")
		return
	}
	var req amplifyCreateDomainReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode: "+err.Error())
		return
	}
	if req.EnableAutoSubDomain != nil {
		stored.Domain.EnableAutoSubDomain = *req.EnableAutoSubDomain
	}
	if req.AutoSubDomainCreationPatterns != nil {
		stored.Domain.AutoSubDomainCreationPatterns = req.AutoSubDomainCreationPatterns
	}
	if req.AutoSubDomainIamRole != "" {
		stored.Domain.AutoSubDomainIamRole = req.AutoSubDomainIamRole
	}
	if req.SubDomainSettings != nil {
		subs := make([]AmplifySubDomain, 0, len(req.SubDomainSettings))
		for _, s := range req.SubDomainSettings {
			subs = append(subs, AmplifySubDomain{
				SubDomainSetting: s,
				Verified:         true,
				DnsRecord:        s.Prefix + "." + stored.Domain.DomainName + " CNAME " + appID + ".cloudfront.net.",
			})
		}
		stored.Domain.SubDomains = subs
	}
	stored.Domain.UpdateStatus = "UPDATE_COMPLETE"
	amplifyDomains.Put(key, stored)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyDomainAssociation{"domainAssociation": stored.Domain})
}

func handleAmplifyDeleteDomain(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	name := r.PathValue("domainName")
	key := amplifyDomainKey(appID, name)
	stored, ok := amplifyDomains.Get(key)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "domain not found")
		return
	}
	amplifyDomains.Delete(key)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyDomainAssociation{"domainAssociation": stored.Domain})
}

func handleAmplifyListDomains(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	if _, ok := amplifyApps.Get(appID); !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	items := []AmplifyDomainAssociation{}
	for _, s := range amplifyDomains.List() {
		if s.AppId == appID {
			items = append(items, s.Domain)
		}
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]any{"domainAssociations": items})
}

// ---------- BackendEnvironment handlers ----------

type amplifyCreateBackendReq struct {
	EnvironmentName     string `json:"environmentName"`
	StackName           string `json:"stackName,omitempty"`
	DeploymentArtifacts string `json:"deploymentArtifacts,omitempty"`
}

func handleAmplifyCreateBackend(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	if _, ok := amplifyApps.Get(appID); !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	var req amplifyCreateBackendReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode: "+err.Error())
		return
	}
	if req.EnvironmentName == "" {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "environmentName is required")
		return
	}
	now := amplifyEpoch()
	env := AmplifyBackendEnvironment{
		BackendEnvironmentArn: amplifyBackendARN(appID, req.EnvironmentName),
		EnvironmentName:       req.EnvironmentName,
		StackName:             req.StackName,
		DeploymentArtifacts:   req.DeploymentArtifacts,
		CreateTime:            now,
		UpdateTime:            now,
	}
	amplifyBackends.Put(amplifyDomainKey(appID, req.EnvironmentName), amplifyStoredBackend{Env: env, AppId: appID})
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyBackendEnvironment{"backendEnvironment": env})
}

func handleAmplifyGetBackend(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	name := r.PathValue("environmentName")
	stored, ok := amplifyBackends.Get(amplifyDomainKey(appID, name))
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "backend environment not found")
		return
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyBackendEnvironment{"backendEnvironment": stored.Env})
}

func handleAmplifyDeleteBackend(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	name := r.PathValue("environmentName")
	key := amplifyDomainKey(appID, name)
	stored, ok := amplifyBackends.Get(key)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "backend environment not found")
		return
	}
	amplifyBackends.Delete(key)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyBackendEnvironment{"backendEnvironment": stored.Env})
}

func handleAmplifyListBackends(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	if _, ok := amplifyApps.Get(appID); !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	items := []AmplifyBackendEnvironment{}
	for _, s := range amplifyBackends.List() {
		if s.AppId == appID {
			items = append(items, s.Env)
		}
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]any{"backendEnvironments": items})
}
