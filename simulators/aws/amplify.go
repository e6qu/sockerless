package main

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// AWS Amplify. Wire: REST + JSON, versionless paths (/apps, /apps/{id},
// etc.). Sim covers the apps + branches + webhooks + jobs + deployments
// surface here; domains + backendenvironments live in amplify_domains.go.
//
// Sim policy:
//   - RELEASE/RETRY/WEB_HOOK jobs run a REAL build (amplify_build.go) when
//     the app has a clonable HTTP(S) git repository and a buildSpec (branch
//     wins over app). Otherwise — the common control-plane-test case — jobs
//     pass through the observable PENDING → RUNNING → SUCCEED states on a
//     short synthetic timer (same shape as the ECS task PROVISIONING →
//     PENDING → RUNNING flip) so clients see the documented state machine
//     and StopJob has a real cancellation window. MANUAL deployment jobs
//     (StartDeployment) never build: they deploy the uploaded content as-is.
//   - Deployed branch content is served by the hosting data plane
//     (amplify_dataplane.go) on the app's default-domain, cloudfront.net,
//     and verified custom-domain hosts.

// ---------- Types ----------

// AmplifyJobStatus is the JobStatus enum subset the sim assigns. The full
// model enum also carries CREATED/PROVISIONING/CANCELLING, which the sim's
// pipelines never enter.
type AmplifyJobStatus string

const (
	AmplifyJobStatusPending   AmplifyJobStatus = "PENDING"
	AmplifyJobStatusRunning   AmplifyJobStatus = "RUNNING"
	AmplifyJobStatusSucceed   AmplifyJobStatus = "SUCCEED"
	AmplifyJobStatusFailed    AmplifyJobStatus = "FAILED"
	AmplifyJobStatusCancelled AmplifyJobStatus = "CANCELLED"
)

// Terminal reports whether the job can no longer be stopped or advanced.
func (s AmplifyJobStatus) Terminal() bool {
	return s == AmplifyJobStatusSucceed || s == AmplifyJobStatusCancelled || s == AmplifyJobStatusFailed
}

// AmplifyBranchStage is the branch Stage enum. Real Amplify additionally
// reads back "NONE" for branches created without a stage, even though the
// model enum omits it.
type AmplifyBranchStage string

const (
	AmplifyStageProduction AmplifyBranchStage = "PRODUCTION"
	AmplifyStageNone       AmplifyBranchStage = "NONE"
)

// AmplifySourceUrlType is the SourceUrlType enum (ZIP | BUCKET_PREFIX).
type AmplifySourceUrlType string

const (
	AmplifySourceUrlZip          AmplifySourceUrlType = "ZIP"
	AmplifySourceUrlBucketPrefix AmplifySourceUrlType = "BUCKET_PREFIX"
)

type AmplifyCustomRule struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Status    string `json:"status,omitempty"`
	Condition string `json:"condition,omitempty"`
}

type AmplifyProductionBranch struct {
	LastDeployTime float64          `json:"lastDeployTime,omitempty"`
	Status         AmplifyJobStatus `json:"status,omitempty"`
	ThumbnailUrl   string           `json:"thumbnailUrl,omitempty"` // external: real-AWS hosted thumbnail — sim doesn't serve screenshots, member stays absent
	BranchName     string           `json:"branchName,omitempty"`
}

type AmplifyCacheConfig struct {
	Type string `json:"type"`
}

type AmplifyJobConfig struct {
	BuildComputeType string `json:"buildComputeType"`
}

type AmplifyBackend struct {
	StackArn string `json:"stackArn,omitempty"`
}

type AmplifyApp struct {
	AppArn                     string                   `json:"appArn"`
	AppId                      string                   `json:"appId"`
	Name                       string                   `json:"name"`
	Description                string                   `json:"description,omitempty"`
	Repository                 string                   `json:"repository,omitempty"`
	RepositoryCloneMethod      string                   `json:"repositoryCloneMethod,omitempty"`
	Platform                   string                   `json:"platform"`
	CreateTime                 float64                  `json:"createTime"`
	UpdateTime                 float64                  `json:"updateTime"`
	ComputeRoleArn             string                   `json:"computeRoleArn,omitempty"`
	IamServiceRoleArn          string                   `json:"iamServiceRoleArn,omitempty"`
	EnvironmentVariables       map[string]string        `json:"environmentVariables,omitempty"`
	DefaultDomain              string                   `json:"defaultDomain"`
	EnableBranchAutoBuild      bool                     `json:"enableBranchAutoBuild"`
	EnableBranchAutoDeletion   bool                     `json:"enableBranchAutoDeletion"`
	EnableBasicAuth            bool                     `json:"enableBasicAuth"`
	BasicAuthCredentials       string                   `json:"basicAuthCredentials,omitempty"`
	CustomRules                []AmplifyCustomRule      `json:"customRules,omitempty"`
	ProductionBranch           *AmplifyProductionBranch `json:"productionBranch,omitempty"`
	BuildSpec                  string                   `json:"buildSpec,omitempty"`
	CustomHeaders              string                   `json:"customHeaders,omitempty"`
	EnableAutoBranchCreation   bool                     `json:"enableAutoBranchCreation"`
	AutoBranchCreationPatterns []string                 `json:"autoBranchCreationPatterns,omitempty"`
	AutoBranchCreationConfig   json.RawMessage          `json:"autoBranchCreationConfig,omitempty"`
	CacheConfig                *AmplifyCacheConfig      `json:"cacheConfig,omitempty"`
	JobConfig                  *AmplifyJobConfig        `json:"jobConfig,omitempty"`
	WebhookCreateTime          float64                  `json:"webhookCreateTime,omitempty"`
	Tags                       map[string]string        `json:"tags,omitempty"`
}

type AmplifyBranch struct {
	BranchArn                  string             `json:"branchArn"`
	BranchName                 string             `json:"branchName"`
	Description                string             `json:"description,omitempty"`
	Tags                       map[string]string  `json:"tags,omitempty"`
	Stage                      AmplifyBranchStage `json:"stage"`
	DisplayName                string             `json:"displayName,omitempty"`
	EnableNotification         bool               `json:"enableNotification"`
	CreateTime                 float64            `json:"createTime"`
	UpdateTime                 float64            `json:"updateTime"`
	EnvironmentVariables       map[string]string  `json:"environmentVariables,omitempty"`
	EnableAutoBuild            bool               `json:"enableAutoBuild"`
	EnableSkewProtection       bool               `json:"enableSkewProtection"`
	CustomDomains              []string           `json:"customDomains,omitempty"`
	Framework                  string             `json:"framework,omitempty"`
	ActiveJobId                string             `json:"activeJobId,omitempty"`
	TotalNumberOfJobs          string             `json:"totalNumberOfJobs"`
	EnableBasicAuth            bool               `json:"enableBasicAuth"`
	EnablePerformanceMode      bool               `json:"enablePerformanceMode"`
	ThumbnailUrl               string             `json:"thumbnailUrl,omitempty"` // external: real-AWS hosted thumbnail of the deployed Amplify app — sim doesn't serve screenshots
	BasicAuthCredentials       string             `json:"basicAuthCredentials,omitempty"`
	BuildSpec                  string             `json:"buildSpec,omitempty"`
	TtL                        string             `json:"ttl"`
	AssociatedResources        []string           `json:"associatedResources,omitempty"`
	EnablePullRequestPreview   bool               `json:"enablePullRequestPreview"`
	PullRequestEnvironmentName string             `json:"pullRequestEnvironmentName,omitempty"`
	DestinationBranch          string             `json:"destinationBranch,omitempty"`
	SourceBranch               string             `json:"sourceBranch,omitempty"`
	BackendEnvironmentArn      string             `json:"backendEnvironmentArn,omitempty"`
	Backend                    *AmplifyBackend    `json:"backend,omitempty"`
	ComputeRoleArn             string             `json:"computeRoleArn,omitempty"`
}

// AmplifyWebhook represents an Amplify webhook endpoint configured
// for a branch's deploy-on-push automation.
//
// WebhookUrl is an EXTERNAL URL pointing at real AWS
// (`https://webhooks.amplify.<region>.amazonaws.com/prod/webhooks?id=...&token=...`).
// Real Amplify accepts POSTs to that URL and triggers a deploy; the
// sim emits the canonical-shape URL so terraform-provider-aws +
// SDK consumers parsing the envelope see what they expect, but the
// sim itself does not service POSTs to webhooks.amplify.<region>.
// Marked as external per the `sim-emitted-url-roundtrip` skill's
// "document external" branch.
type AmplifyWebhook struct {
	WebhookArn  string  `json:"webhookArn"`
	WebhookId   string  `json:"webhookId"`
	WebhookUrl  string  `json:"webhookUrl"` // external: real AWS webhooks.amplify.<region>.amazonaws.com endpoint
	AppId       string  `json:"appId"`
	BranchName  string  `json:"branchName"`
	Description string  `json:"description,omitempty"`
	CreateTime  float64 `json:"createTime"`
	UpdateTime  float64 `json:"updateTime"`
}

type AmplifyJobSummary struct {
	JobArn        string               `json:"jobArn"`
	JobId         string               `json:"jobId"`
	CommitId      string               `json:"commitId,omitempty"`
	CommitMessage string               `json:"commitMessage,omitempty"`
	CommitTime    float64              `json:"commitTime,omitempty"`
	StartTime     float64              `json:"startTime,omitempty"`
	Status        AmplifyJobStatus     `json:"status"`
	EndTime       float64              `json:"endTime,omitempty"`
	JobType       string               `json:"jobType"`
	SourceUrl     string               `json:"sourceUrl,omitempty"`
	SourceUrlType AmplifySourceUrlType `json:"sourceUrlType,omitempty"`
}

type AmplifyJob struct {
	Summary AmplifyJobSummary `json:"summary"`
	Steps   []AmplifyJobStep  `json:"steps,omitempty"`
}

type AmplifyJobStep struct {
	StepName  string           `json:"stepName"`
	StartTime float64          `json:"startTime"`
	EndTime   float64          `json:"endTime,omitempty"`
	Status    AmplifyJobStatus `json:"status"`
	LogUrl    string           `json:"logUrl,omitempty"`
}

type AmplifyArtifact struct {
	ArtifactFileName string `json:"artifactFileName"`
	ArtifactId       string `json:"artifactId"`
}

type amplifyStoredApp struct {
	App      AmplifyApp
	Branches map[string]AmplifyBranch
}

type amplifyStoredWebhook struct {
	Webhook AmplifyWebhook
	AppId   string
}

type amplifyStoredJob struct {
	Job        AmplifyJob
	AppId      string
	BranchName string
}

type amplifyStoredArtifact struct {
	Artifact   AmplifyArtifact
	AppId      string
	BranchName string
	JobId      string
	Key        string
	URL        string
}

// amplifyStoredDeployment is a pending zip/file-map deployment created by
// CreateDeployment and consumed by StartDeployment. The keys point into the
// sim's own S3 bucket, where the presigned upload URLs land the bytes.
type amplifyStoredDeployment struct {
	JobId      string
	AppId      string
	BranchName string
	ZipKey     string
	FileKeys   map[string]string
	CreateTime float64
}

var (
	amplifyApps        sim.Store[amplifyStoredApp]
	amplifyWebhooks    sim.Store[amplifyStoredWebhook]
	amplifyJobs        sim.Store[amplifyStoredJob]
	amplifyArtifacts   sim.Store[amplifyStoredArtifact]
	amplifyDeployments sim.Store[amplifyStoredDeployment]
)

// ---------- Helpers ----------

func amplifyRandomID() string {
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	return "d" + hex.EncodeToString(buf)
}

func amplifyJobID() string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func amplifyAppARN(id string) string {
	return fmt.Sprintf("arn:aws:amplify:%s:%s:apps/%s", awsRegion(), awsAccountID(), id)
}
func amplifyBranchARN(appID, branch string) string {
	return fmt.Sprintf("arn:aws:amplify:%s:%s:apps/%s/branches/%s", awsRegion(), awsAccountID(), appID, branch)
}
func amplifyWebhookARN(id string) string {
	return fmt.Sprintf("arn:aws:amplify:%s:%s:webhooks/%s", awsRegion(), awsAccountID(), id)
}
func amplifyJobARN(appID, branch, jobID string) string {
	return fmt.Sprintf("arn:aws:amplify:%s:%s:apps/%s/branches/%s/jobs/%s", awsRegion(), awsAccountID(), appID, branch, jobID)
}

func amplifyArtifactID(jobID string) string {
	return jobID + "-" + amplifyJobID()
}

func amplifyEpoch() float64 { return float64(time.Now().UTC().Unix()) }

func amplifyWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func amplifyWriteError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Amzn-Errortype", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"__type":  code,
		"message": msg,
	})
}

// amplifyPageQuery parses the restJson1 ?nextToken=&maxResults= pagination
// query params every Amplify List* operation carries. On a malformed value
// it writes a BadRequestException and reports !ok.
func amplifyPageQuery(w http.ResponseWriter, r *http.Request) (token string, maxResults int, ok bool) {
	token = r.URL.Query().Get("nextToken")
	if token != "" {
		if offset, err := strconv.Atoi(token); err != nil || offset < 0 {
			amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "invalid nextToken")
			return "", 0, false
		}
	}
	if raw := r.URL.Query().Get("maxResults"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "invalid maxResults")
			return "", 0, false
		}
		maxResults = parsed
	}
	return token, maxResults, true
}

// amplifyWriteListPage pages a sorted slice with awsPageExplicit (paginate
// only on an explicit positive maxResults) and writes the {key: page[,
// nextToken]} envelope.
func amplifyWriteListPage[T any](w http.ResponseWriter, key string, items []T, token string, maxResults int) {
	page, next := awsPageExplicit(items, token, maxResults)
	out := map[string]any{key: page}
	if next != "" {
		out["nextToken"] = next
	}
	amplifyWriteJSON(w, http.StatusOK, out)
}

// amplifyRepositoryCloneMethod derives the read-only repositoryCloneMethod
// member the way real Amplify documents it: TOKEN for GitHub, SIGV4 for
// CodeCommit, SSH for GitLab and Bitbucket. Unrecognized hosts (and apps
// without a repository) carry no clone method.
func amplifyRepositoryCloneMethod(repository string) string {
	repo := strings.ToLower(repository)
	switch {
	case repo == "":
		return ""
	case strings.Contains(repo, "github.com"):
		return "TOKEN"
	case strings.Contains(repo, "codecommit"):
		return "SIGV4"
	case strings.Contains(repo, "gitlab"), strings.Contains(repo, "bitbucket"):
		return "SSH"
	}
	return ""
}

// ---------- Registration ----------

func registerAmplify(srv *sim.Server) {
	amplifyApps = sim.MakeStore[amplifyStoredApp](srv.DB(), "amplify_apps")
	amplifyWebhooks = sim.MakeStore[amplifyStoredWebhook](srv.DB(), "amplify_webhooks")
	amplifyJobs = sim.MakeStore[amplifyStoredJob](srv.DB(), "amplify_jobs")
	amplifyArtifacts = sim.MakeStore[amplifyStoredArtifact](srv.DB(), "amplify_artifacts")
	amplifyDeployments = sim.MakeStore[amplifyStoredDeployment](srv.DB(), "amplify_deployments")

	mux := srv
	appResource := cloudTrailRESTResource("AWS::Amplify::App", "appId", "arn")
	branchResource := cloudTrailRESTResource("AWS::Amplify::Branch", "name", "arn")
	webhookResource := cloudTrailRESTResource("AWS::Amplify::Webhook", "webhookId")
	jobResource := cloudTrailRESTResource("AWS::Amplify::Job", "jobId")
	// Apps
	mux.HandleFunc("POST /apps", cloudTrailRecordedREST("CreateApp", "amplify.amazonaws.com", nil, handleAmplifyCreateApp))
	mux.HandleFunc("GET /apps", cloudTrailRecordedREST("ListApps", "amplify.amazonaws.com", nil, handleAmplifyListApps))
	mux.HandleFunc("GET /apps/{appId}", cloudTrailRecordedREST("GetApp", "amplify.amazonaws.com", appResource, handleAmplifyGetApp))
	mux.HandleFunc("POST /apps/{appId}", cloudTrailRecordedREST("UpdateApp", "amplify.amazonaws.com", appResource, handleAmplifyUpdateApp))
	mux.HandleFunc("DELETE /apps/{appId}", cloudTrailRecordedREST("DeleteApp", "amplify.amazonaws.com", appResource, handleAmplifyDeleteApp))
	// Branches
	mux.HandleFunc("POST /apps/{appId}/branches", cloudTrailRecordedREST("CreateBranch", "amplify.amazonaws.com", appResource, handleAmplifyCreateBranch))
	mux.HandleFunc("GET /apps/{appId}/branches", cloudTrailRecordedREST("ListBranches", "amplify.amazonaws.com", appResource, handleAmplifyListBranches))
	mux.HandleFunc("GET /apps/{appId}/branches/{name}", cloudTrailRecordedREST("GetBranch", "amplify.amazonaws.com", branchResource, handleAmplifyGetBranch))
	mux.HandleFunc("POST /apps/{appId}/branches/{name}", cloudTrailRecordedREST("UpdateBranch", "amplify.amazonaws.com", branchResource, handleAmplifyUpdateBranch))
	mux.HandleFunc("DELETE /apps/{appId}/branches/{name}", cloudTrailRecordedREST("DeleteBranch", "amplify.amazonaws.com", branchResource, handleAmplifyDeleteBranch))
	// Webhooks
	mux.HandleFunc("POST /apps/{appId}/webhooks", cloudTrailRecordedREST("CreateWebhook", "amplify.amazonaws.com", appResource, handleAmplifyCreateWebhook))
	mux.HandleFunc("GET /apps/{appId}/webhooks", cloudTrailRecordedREST("ListWebhooks", "amplify.amazonaws.com", appResource, handleAmplifyListWebhooks))
	mux.HandleFunc("GET /webhooks/{webhookId}", cloudTrailRecordedREST("GetWebhook", "amplify.amazonaws.com", webhookResource, handleAmplifyGetWebhook))
	mux.HandleFunc("POST /webhooks/{webhookId}", cloudTrailRecordedREST("UpdateWebhook", "amplify.amazonaws.com", webhookResource, handleAmplifyUpdateWebhook))
	mux.HandleFunc("DELETE /webhooks/{webhookId}", cloudTrailRecordedREST("DeleteWebhook", "amplify.amazonaws.com", webhookResource, handleAmplifyDeleteWebhook))
	// Jobs / deployments
	mux.HandleFunc("POST /apps/{appId}/branches/{name}/jobs", cloudTrailRecordedREST("StartJob", "amplify.amazonaws.com", branchResource, handleAmplifyStartJob))
	mux.HandleFunc("GET /apps/{appId}/branches/{name}/jobs", cloudTrailRecordedREST("ListJobs", "amplify.amazonaws.com", branchResource, handleAmplifyListJobs))
	mux.HandleFunc("GET /apps/{appId}/branches/{name}/jobs/{jobId}", cloudTrailRecordedREST("GetJob", "amplify.amazonaws.com", jobResource, handleAmplifyGetJob))
	mux.HandleFunc("DELETE /apps/{appId}/branches/{name}/jobs/{jobId}/stop", cloudTrailRecordedREST("StopJob", "amplify.amazonaws.com", jobResource, handleAmplifyStopJob))
	mux.HandleFunc("DELETE /apps/{appId}/branches/{name}/jobs/{jobId}", cloudTrailRecordedREST("DeleteJob", "amplify.amazonaws.com", jobResource, handleAmplifyDeleteJob))
	mux.HandleFunc("GET /apps/{appId}/branches/{name}/jobs/{jobId}/artifacts", cloudTrailRecordedREST("ListArtifacts", "amplify.amazonaws.com", jobResource, handleAmplifyListArtifacts))
	mux.HandleFunc("GET /artifacts/{artifactId}", cloudTrailRecordedREST("GetArtifactUrl", "amplify.amazonaws.com", cloudTrailRESTResource("AWS::Amplify::Artifact", "artifactId"), handleAmplifyGetArtifactURL))
	mux.HandleFunc("POST /apps/{appId}/accesslogs", cloudTrailRecordedREST("GenerateAccessLogs", "amplify.amazonaws.com", appResource, handleAmplifyGenerateAccessLogs))
	// Deployments — note SDK routes:
	//   POST /apps/{appId}/branches/{name}/deployments       — CreateDeployment (returns upload URL)
	//   POST /apps/{appId}/branches/{name}/deployments/start — StartDeployment (kicks off the job)
	mux.HandleFunc("POST /apps/{appId}/branches/{name}/deployments", cloudTrailRecordedREST("CreateDeployment", "amplify.amazonaws.com", branchResource, handleAmplifyCreateDeployment))
	mux.HandleFunc("POST /apps/{appId}/branches/{name}/deployments/start", cloudTrailRecordedREST("StartDeployment", "amplify.amazonaws.com", branchResource, handleAmplifyStartDeployment))
	// Tags
	mux.HandleFunc("GET /tags/{arn...}", cloudTrailRecordedREST("ListTagsForResource", "amplify.amazonaws.com", appResource, handleAmplifyListTags))
	mux.HandleFunc("POST /tags/{arn...}", cloudTrailRecordedREST("TagResource", "amplify.amazonaws.com", appResource, handleAmplifyTagResource))
	mux.HandleFunc("DELETE /tags/{arn...}", cloudTrailRecordedREST("UntagResource", "amplify.amazonaws.com", appResource, handleAmplifyUntagResource))

	// Domains + BackendEnvironments (amplify_domains.go)
	registerAmplifyDomains(srv)
}

// ---------- Apps ----------

type amplifyCreateAppReq struct {
	Name                       string              `json:"name"`
	Description                string              `json:"description,omitempty"`
	Repository                 string              `json:"repository,omitempty"`
	Platform                   string              `json:"platform,omitempty"`
	ComputeRoleArn             string              `json:"computeRoleArn,omitempty"`
	IamServiceRoleArn          string              `json:"iamServiceRoleArn,omitempty"`
	OauthToken                 string              `json:"oauthToken,omitempty"`
	AccessToken                string              `json:"accessToken,omitempty"`
	EnvironmentVariables       map[string]string   `json:"environmentVariables,omitempty"`
	EnableBranchAutoBuild      *bool               `json:"enableBranchAutoBuild,omitempty"`
	EnableBranchAutoDeletion   *bool               `json:"enableBranchAutoDeletion,omitempty"`
	EnableBasicAuth            *bool               `json:"enableBasicAuth,omitempty"`
	BasicAuthCredentials       string              `json:"basicAuthCredentials,omitempty"`
	CustomRules                []AmplifyCustomRule `json:"customRules,omitempty"`
	Tags                       map[string]string   `json:"tags,omitempty"`
	BuildSpec                  string              `json:"buildSpec,omitempty"`
	CustomHeaders              string              `json:"customHeaders,omitempty"`
	EnableAutoBranchCreation   *bool               `json:"enableAutoBranchCreation,omitempty"`
	AutoBranchCreationPatterns []string            `json:"autoBranchCreationPatterns,omitempty"`
	AutoBranchCreationConfig   json.RawMessage     `json:"autoBranchCreationConfig,omitempty"`
	JobConfig                  *AmplifyJobConfig   `json:"jobConfig,omitempty"`
	CacheConfig                *AmplifyCacheConfig `json:"cacheConfig,omitempty"`
}

func handleAmplifyCreateApp(w http.ResponseWriter, r *http.Request) {
	var req amplifyCreateAppReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode: "+err.Error())
		return
	}
	if req.Name == "" {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "name is required")
		return
	}
	id := amplifyRandomID()
	now := amplifyEpoch()
	platform := req.Platform
	if platform == "" {
		platform = "WEB"
	}
	cacheConfig := req.CacheConfig
	if cacheConfig == nil {
		// Documented default: "If you don't specify the cache configuration
		// type, Amplify uses the default AMPLIFY_MANAGED setting."
		cacheConfig = &AmplifyCacheConfig{Type: "AMPLIFY_MANAGED"}
	}
	jobConfig := req.JobConfig
	if jobConfig == nil {
		// Documented default: "If you don't specify a value, Amplify uses the
		// STANDARD_8GB default."
		jobConfig = &AmplifyJobConfig{BuildComputeType: "STANDARD_8GB"}
	}
	app := AmplifyApp{
		AppArn:                amplifyAppARN(id),
		AppId:                 id,
		Name:                  req.Name,
		Description:           req.Description,
		Repository:            req.Repository,
		RepositoryCloneMethod: amplifyRepositoryCloneMethod(req.Repository),
		Platform:              platform,
		CreateTime:            now,
		UpdateTime:            now,
		ComputeRoleArn:        req.ComputeRoleArn,
		IamServiceRoleArn:     req.IamServiceRoleArn,
		EnvironmentVariables:  req.EnvironmentVariables,
		DefaultDomain:         id + ".amplifyapp.com",
		// Unset reads back false (terraform-provider-aws only sends the
		// member when true and trusts the read-back for idempotency).
		EnableBranchAutoBuild:      boolOr(req.EnableBranchAutoBuild, false),
		EnableBranchAutoDeletion:   boolOr(req.EnableBranchAutoDeletion, false),
		EnableBasicAuth:            boolOr(req.EnableBasicAuth, false),
		BasicAuthCredentials:       req.BasicAuthCredentials,
		CustomRules:                req.CustomRules,
		BuildSpec:                  req.BuildSpec,
		CustomHeaders:              req.CustomHeaders,
		EnableAutoBranchCreation:   boolOr(req.EnableAutoBranchCreation, false),
		AutoBranchCreationPatterns: req.AutoBranchCreationPatterns,
		AutoBranchCreationConfig:   req.AutoBranchCreationConfig,
		CacheConfig:                cacheConfig,
		JobConfig:                  jobConfig,
		Tags:                       req.Tags,
	}
	amplifyApps.Put(id, amplifyStoredApp{App: app, Branches: map[string]AmplifyBranch{}})
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyApp{"app": app})
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func handleAmplifyGetApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("appId")
	stored, ok := amplifyApps.Get(id)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyApp{"app": stored.App})
}

func handleAmplifyDeleteApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("appId")
	stored, ok := amplifyApps.Get(id)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	// Cascade-delete every sub-resource keyed to this app: webhooks, jobs (+
	// artifacts), pending deployments, domain associations, and backend
	// environments. A later app reusing the ID must start clean.
	for _, wh := range amplifyWebhooks.List() {
		if wh.AppId == id {
			amplifyWebhooks.Delete(wh.Webhook.WebhookId)
		}
	}
	for _, jb := range amplifyJobs.List() {
		if jb.AppId == id {
			amplifyDeleteArtifactsForJob(jb.Job.Summary.JobId)
			amplifyJobs.Delete(jb.Job.Summary.JobId)
		}
	}
	for _, dep := range amplifyDeployments.List() {
		if dep.AppId == id {
			amplifyDeleteDeployment(dep)
		}
	}
	for branchName := range stored.Branches {
		amplifyStopCompute(id, branchName)
		amplifyInvalidateHostingCache(id, branchName)
	}
	for _, dom := range amplifyDomains.List() {
		if dom.AppId == id {
			amplifyDomains.Delete(amplifyDomainKey(id, dom.Domain.DomainName))
		}
	}
	for _, be := range amplifyBackends.List() {
		if be.AppId == id {
			amplifyBackends.Delete(amplifyDomainKey(id, be.Env.EnvironmentName))
		}
	}
	amplifyApps.Delete(id)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyApp{"app": stored.App})
}

// amplifyUpdateAppReq mirrors UpdateAppRequest with presence-tracking
// members: a member is applied only when the client sent it, so explicit
// empty values clear the stored value instead of being ignored.
type amplifyUpdateAppReq struct {
	Name                       *string              `json:"name"`
	Description                *string              `json:"description"`
	Platform                   *string              `json:"platform"`
	ComputeRoleArn             *string              `json:"computeRoleArn"`
	IamServiceRoleArn          *string              `json:"iamServiceRoleArn"`
	EnvironmentVariables       map[string]string    `json:"environmentVariables"`
	EnableBranchAutoBuild      *bool                `json:"enableBranchAutoBuild"`
	EnableBranchAutoDeletion   *bool                `json:"enableBranchAutoDeletion"`
	EnableBasicAuth            *bool                `json:"enableBasicAuth"`
	BasicAuthCredentials       *string              `json:"basicAuthCredentials"`
	CustomRules                *[]AmplifyCustomRule `json:"customRules"`
	BuildSpec                  *string              `json:"buildSpec"`
	CustomHeaders              *string              `json:"customHeaders"`
	EnableAutoBranchCreation   *bool                `json:"enableAutoBranchCreation"`
	AutoBranchCreationPatterns *[]string            `json:"autoBranchCreationPatterns"`
	AutoBranchCreationConfig   json.RawMessage      `json:"autoBranchCreationConfig"`
	Repository                 *string              `json:"repository"`
	OauthToken                 *string              `json:"oauthToken"`
	AccessToken                *string              `json:"accessToken"`
	JobConfig                  *AmplifyJobConfig    `json:"jobConfig"`
	CacheConfig                *AmplifyCacheConfig  `json:"cacheConfig"`
}

func handleAmplifyUpdateApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("appId")
	stored, ok := amplifyApps.Get(id)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	var req amplifyUpdateAppReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode: "+err.Error())
		return
	}
	a := &stored.App
	if req.Name != nil {
		if *req.Name == "" {
			amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "name cannot be empty")
			return
		}
		a.Name = *req.Name
	}
	if req.Description != nil {
		a.Description = *req.Description
	}
	if req.Repository != nil {
		a.Repository = *req.Repository
		a.RepositoryCloneMethod = amplifyRepositoryCloneMethod(*req.Repository)
	}
	if req.Platform != nil && *req.Platform != "" {
		a.Platform = *req.Platform
	}
	if req.ComputeRoleArn != nil {
		a.ComputeRoleArn = *req.ComputeRoleArn
	}
	if req.IamServiceRoleArn != nil {
		a.IamServiceRoleArn = *req.IamServiceRoleArn
	}
	if req.EnvironmentVariables != nil {
		a.EnvironmentVariables = req.EnvironmentVariables
	}
	if req.EnableBranchAutoBuild != nil {
		a.EnableBranchAutoBuild = *req.EnableBranchAutoBuild
	}
	if req.EnableBranchAutoDeletion != nil {
		a.EnableBranchAutoDeletion = *req.EnableBranchAutoDeletion
	}
	if req.EnableBasicAuth != nil {
		a.EnableBasicAuth = *req.EnableBasicAuth
	}
	if req.BasicAuthCredentials != nil {
		a.BasicAuthCredentials = *req.BasicAuthCredentials
	}
	if req.CustomRules != nil {
		a.CustomRules = *req.CustomRules
	}
	if req.BuildSpec != nil {
		a.BuildSpec = *req.BuildSpec
	}
	if req.CustomHeaders != nil {
		a.CustomHeaders = *req.CustomHeaders
	}
	if req.EnableAutoBranchCreation != nil {
		a.EnableAutoBranchCreation = *req.EnableAutoBranchCreation
	}
	if req.AutoBranchCreationPatterns != nil {
		a.AutoBranchCreationPatterns = *req.AutoBranchCreationPatterns
	}
	if len(req.AutoBranchCreationConfig) > 0 {
		if string(req.AutoBranchCreationConfig) == "null" {
			a.AutoBranchCreationConfig = nil
		} else {
			a.AutoBranchCreationConfig = req.AutoBranchCreationConfig
		}
	}
	if req.JobConfig != nil {
		a.JobConfig = req.JobConfig
	}
	if req.CacheConfig != nil {
		a.CacheConfig = req.CacheConfig
	}
	// oauthToken/accessToken are write-only credentials: accepted, never
	// stored or echoed (the App shape has no such members).
	a.UpdateTime = amplifyEpoch()
	amplifyApps.Put(id, stored)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyApp{"app": stored.App})
}

func handleAmplifyListApps(w http.ResponseWriter, r *http.Request) {
	token, maxResults, ok := amplifyPageQuery(w, r)
	if !ok {
		return
	}
	apps := []AmplifyApp{}
	for _, s := range amplifyApps.List() {
		apps = append(apps, s.App)
	}
	sortBy(apps, func(a AmplifyApp) string { return a.AppId })
	amplifyWriteListPage(w, "apps", apps, token, maxResults)
}

// ---------- Branches ----------

type amplifyCreateBranchReq struct {
	BranchName                 string             `json:"branchName"`
	Description                string             `json:"description,omitempty"`
	Stage                      AmplifyBranchStage `json:"stage,omitempty"`
	Framework                  string             `json:"framework,omitempty"`
	EnableNotification         *bool              `json:"enableNotification,omitempty"`
	EnableAutoBuild            *bool              `json:"enableAutoBuild,omitempty"`
	EnableSkewProtection       *bool              `json:"enableSkewProtection,omitempty"`
	EnvironmentVariables       map[string]string  `json:"environmentVariables,omitempty"`
	BasicAuthCredentials       string             `json:"basicAuthCredentials,omitempty"`
	EnableBasicAuth            *bool              `json:"enableBasicAuth,omitempty"`
	EnablePerformanceMode      *bool              `json:"enablePerformanceMode,omitempty"`
	Tags                       map[string]string  `json:"tags,omitempty"`
	BuildSpec                  string             `json:"buildSpec,omitempty"`
	Ttl                        string             `json:"ttl,omitempty"`
	DisplayName                string             `json:"displayName,omitempty"`
	EnablePullRequestPreview   *bool              `json:"enablePullRequestPreview,omitempty"`
	PullRequestEnvironmentName string             `json:"pullRequestEnvironmentName,omitempty"`
	BackendEnvironmentArn      string             `json:"backendEnvironmentArn,omitempty"`
	Backend                    *AmplifyBackend    `json:"backend,omitempty"`
	ComputeRoleArn             string             `json:"computeRoleArn,omitempty"`
}

func handleAmplifyCreateBranch(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	stored, ok := amplifyApps.Get(appID)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	var req amplifyCreateBranchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode: "+err.Error())
		return
	}
	if req.BranchName == "" {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "branchName is required")
		return
	}
	if _, exists := stored.Branches[req.BranchName]; exists {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "branch already exists")
		return
	}
	now := amplifyEpoch()
	stage := req.Stage
	if stage == "" {
		stage = AmplifyStageNone
	}
	br := AmplifyBranch{
		BranchArn:                  amplifyBranchARN(appID, req.BranchName),
		BranchName:                 req.BranchName,
		Description:                req.Description,
		Stage:                      stage,
		DisplayName:                req.DisplayName,
		Framework:                  req.Framework,
		EnableNotification:         boolOr(req.EnableNotification, false),
		EnableAutoBuild:            boolOr(req.EnableAutoBuild, true),
		EnableSkewProtection:       boolOr(req.EnableSkewProtection, false),
		EnvironmentVariables:       req.EnvironmentVariables,
		BasicAuthCredentials:       req.BasicAuthCredentials,
		EnableBasicAuth:            boolOr(req.EnableBasicAuth, false),
		EnablePerformanceMode:      boolOr(req.EnablePerformanceMode, false),
		Tags:                       req.Tags,
		BuildSpec:                  req.BuildSpec,
		TtL:                        req.Ttl,
		EnablePullRequestPreview:   boolOr(req.EnablePullRequestPreview, false),
		PullRequestEnvironmentName: req.PullRequestEnvironmentName,
		BackendEnvironmentArn:      req.BackendEnvironmentArn,
		Backend:                    req.Backend,
		ComputeRoleArn:             req.ComputeRoleArn,
		CreateTime:                 now,
		UpdateTime:                 now,
		TotalNumberOfJobs:          "0",
		CustomDomains:              []string{},
		AssociatedResources:        []string{},
	}
	if br.TtL == "" {
		br.TtL = "5"
	}
	stored.Branches[req.BranchName] = br
	amplifyApps.Put(appID, stored)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyBranch{"branch": br})
}

func handleAmplifyGetBranch(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	name := r.PathValue("name")
	stored, ok := amplifyApps.Get(appID)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	br, ok := stored.Branches[name]
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "branch not found")
		return
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyBranch{"branch": br})
}

// amplifyUpdateBranchReq mirrors UpdateBranchRequest with presence-tracking
// members (see amplifyUpdateAppReq).
type amplifyUpdateBranchReq struct {
	Description                *string             `json:"description"`
	Framework                  *string             `json:"framework"`
	Stage                      *AmplifyBranchStage `json:"stage"`
	EnableNotification         *bool               `json:"enableNotification"`
	EnableAutoBuild            *bool               `json:"enableAutoBuild"`
	EnableSkewProtection       *bool               `json:"enableSkewProtection"`
	EnvironmentVariables       map[string]string   `json:"environmentVariables"`
	BasicAuthCredentials       *string             `json:"basicAuthCredentials"`
	EnableBasicAuth            *bool               `json:"enableBasicAuth"`
	EnablePerformanceMode      *bool               `json:"enablePerformanceMode"`
	BuildSpec                  *string             `json:"buildSpec"`
	Ttl                        *string             `json:"ttl"`
	DisplayName                *string             `json:"displayName"`
	EnablePullRequestPreview   *bool               `json:"enablePullRequestPreview"`
	PullRequestEnvironmentName *string             `json:"pullRequestEnvironmentName"`
	BackendEnvironmentArn      *string             `json:"backendEnvironmentArn"`
	Backend                    *AmplifyBackend     `json:"backend"`
	ComputeRoleArn             *string             `json:"computeRoleArn"`
}

func handleAmplifyUpdateBranch(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	name := r.PathValue("name")
	stored, ok := amplifyApps.Get(appID)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	br, ok := stored.Branches[name]
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "branch not found")
		return
	}
	var req amplifyUpdateBranchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode: "+err.Error())
		return
	}
	if req.Description != nil {
		br.Description = *req.Description
	}
	if req.Framework != nil {
		br.Framework = *req.Framework
	}
	if req.Stage != nil && *req.Stage != "" {
		br.Stage = *req.Stage
	}
	if req.EnableNotification != nil {
		br.EnableNotification = *req.EnableNotification
	}
	if req.EnableAutoBuild != nil {
		br.EnableAutoBuild = *req.EnableAutoBuild
	}
	if req.EnableSkewProtection != nil {
		br.EnableSkewProtection = *req.EnableSkewProtection
	}
	if req.EnvironmentVariables != nil {
		br.EnvironmentVariables = req.EnvironmentVariables
	}
	if req.BasicAuthCredentials != nil {
		br.BasicAuthCredentials = *req.BasicAuthCredentials
	}
	if req.EnableBasicAuth != nil {
		br.EnableBasicAuth = *req.EnableBasicAuth
	}
	if req.EnablePerformanceMode != nil {
		br.EnablePerformanceMode = *req.EnablePerformanceMode
	}
	if req.BuildSpec != nil {
		br.BuildSpec = *req.BuildSpec
	}
	if req.Ttl != nil && *req.Ttl != "" {
		br.TtL = *req.Ttl
	}
	if req.DisplayName != nil {
		br.DisplayName = *req.DisplayName
	}
	if req.EnablePullRequestPreview != nil {
		br.EnablePullRequestPreview = *req.EnablePullRequestPreview
	}
	if req.PullRequestEnvironmentName != nil {
		br.PullRequestEnvironmentName = *req.PullRequestEnvironmentName
	}
	if req.BackendEnvironmentArn != nil {
		br.BackendEnvironmentArn = *req.BackendEnvironmentArn
	}
	if req.Backend != nil {
		br.Backend = req.Backend
	}
	if req.ComputeRoleArn != nil {
		br.ComputeRoleArn = *req.ComputeRoleArn
	}
	br.UpdateTime = amplifyEpoch()
	stored.Branches[name] = br
	amplifyApps.Put(appID, stored)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyBranch{"branch": br})
}

func handleAmplifyDeleteBranch(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	name := r.PathValue("name")
	stored, ok := amplifyApps.Get(appID)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	br, ok := stored.Branches[name]
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "branch not found")
		return
	}
	for _, jb := range amplifyJobs.List() {
		if jb.AppId == appID && jb.BranchName == name {
			amplifyDeleteArtifactsForJob(jb.Job.Summary.JobId)
			amplifyJobs.Delete(jb.Job.Summary.JobId)
		}
	}
	for _, dep := range amplifyDeployments.List() {
		if dep.AppId == appID && dep.BranchName == name {
			amplifyDeleteDeployment(dep)
		}
	}
	amplifyStopCompute(appID, name)
	amplifyInvalidateHostingCache(appID, name)
	delete(stored.Branches, name)
	amplifyApps.Put(appID, stored)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyBranch{"branch": br})
}

func handleAmplifyListBranches(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	stored, ok := amplifyApps.Get(appID)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	token, maxResults, ok := amplifyPageQuery(w, r)
	if !ok {
		return
	}
	branches := make([]AmplifyBranch, 0, len(stored.Branches))
	for _, br := range stored.Branches {
		branches = append(branches, br)
	}
	sortBy(branches, func(b AmplifyBranch) string { return b.BranchName })
	amplifyWriteListPage(w, "branches", branches, token, maxResults)
}

// ---------- Webhooks ----------

type amplifyCreateWebhookReq struct {
	BranchName  string `json:"branchName"`
	Description string `json:"description,omitempty"`
}

// amplifyWebhookWire is the response view of a stored webhook. The store
// row's AppId is the authoritative linkage — persisted rows are not
// guaranteed to carry appId inside the embedded wire struct — so the wire
// view hydrates it on every read.
func amplifyWebhookWire(stored amplifyStoredWebhook) AmplifyWebhook {
	wh := stored.Webhook
	wh.AppId = stored.AppId
	return wh
}

func handleAmplifyCreateWebhook(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	app, ok := amplifyApps.Get(appID)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	var req amplifyCreateWebhookReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode: "+err.Error())
		return
	}
	if req.BranchName == "" {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "branchName is required")
		return
	}
	id := amplifyRandomID()
	now := amplifyEpoch()
	wh := AmplifyWebhook{
		WebhookArn:  amplifyWebhookARN(id),
		WebhookId:   id,
		WebhookUrl:  "https://webhooks.amplify." + awsRegion() + ".amazonaws.com/prod/webhooks?id=" + id + "&token=" + amplifyRandomID(),
		AppId:       appID,
		BranchName:  req.BranchName,
		Description: req.Description,
		CreateTime:  now,
		UpdateTime:  now,
	}
	amplifyWebhooks.Put(id, amplifyStoredWebhook{Webhook: wh, AppId: appID})
	if app.App.WebhookCreateTime == 0 {
		// webhookCreateTime: "A timestamp of when Amplify created the webhook
		// in your Git repository" — real data exists once the app has one.
		app.App.WebhookCreateTime = now
		amplifyApps.Put(appID, app)
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyWebhook{"webhook": wh})
}

func handleAmplifyGetWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("webhookId")
	stored, ok := amplifyWebhooks.Get(id)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "webhook not found")
		return
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyWebhook{"webhook": amplifyWebhookWire(stored)})
}

func handleAmplifyUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("webhookId")
	stored, ok := amplifyWebhooks.Get(id)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "webhook not found")
		return
	}
	var req struct {
		BranchName  *string `json:"branchName"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode: "+err.Error())
		return
	}
	if req.BranchName != nil && *req.BranchName != "" {
		stored.Webhook.BranchName = *req.BranchName
	}
	if req.Description != nil {
		stored.Webhook.Description = *req.Description
	}
	stored.Webhook.UpdateTime = amplifyEpoch()
	amplifyWebhooks.Put(id, stored)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyWebhook{"webhook": amplifyWebhookWire(stored)})
}

func handleAmplifyDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("webhookId")
	stored, ok := amplifyWebhooks.Get(id)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "webhook not found")
		return
	}
	amplifyWebhooks.Delete(id)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyWebhook{"webhook": amplifyWebhookWire(stored)})
}

func handleAmplifyListWebhooks(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	if _, ok := amplifyApps.Get(appID); !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	token, maxResults, ok := amplifyPageQuery(w, r)
	if !ok {
		return
	}
	items := []AmplifyWebhook{}
	for _, s := range amplifyWebhooks.List() {
		if s.AppId == appID {
			items = append(items, amplifyWebhookWire(s))
		}
	}
	sortBy(items, func(wh AmplifyWebhook) string { return wh.WebhookId })
	amplifyWriteListPage(w, "webhooks", items, token, maxResults)
}

// ---------- Jobs ----------

// Synthetic job pipeline timing: PENDING → (running delay) → RUNNING →
// (succeed delay) → SUCCEED. Short enough that polling clients settle
// quickly, long enough that a StopJob issued right after StartJob — even
// through a freshly spawned `aws` CLI process on a contended host — lands
// inside the cancellation window.
const (
	amplifyJobRunningDelay = 200 * time.Millisecond
	amplifyJobSucceedDelay = 1300 * time.Millisecond
)

type amplifyStartJobReq struct {
	JobType       string  `json:"jobType"` // RELEASE / RETRY / MANUAL / WEB_HOOK
	JobReason     string  `json:"jobReason,omitempty"`
	CommitId      string  `json:"commitId,omitempty"`
	CommitMessage string  `json:"commitMessage,omitempty"`
	CommitTime    float64 `json:"commitTime,omitempty"`
	JobId         string  `json:"jobId,omitempty"`
}

func amplifyJobSteps(start float64, status AmplifyJobStatus) []AmplifyJobStep {
	return []AmplifyJobStep{
		{StepName: "PROVISION", StartTime: start, Status: status},
		{StepName: "BUILD", StartTime: start, Status: status},
		{StepName: "DEPLOY", StartTime: start, Status: status},
	}
}

func handleAmplifyStartJob(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	branch := r.PathValue("name")
	stored, ok := amplifyApps.Get(appID)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	if _, ok := stored.Branches[branch]; !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "branch not found")
		return
	}
	var req amplifyStartJobReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode: "+err.Error())
		return
	}
	if req.JobType == "" {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "jobType is required")
		return
	}
	now := amplifyEpoch()
	jobID := amplifyJobID()
	if req.JobId != "" {
		jobID = req.JobId
	}
	summary := AmplifyJobSummary{
		JobArn:        amplifyJobARN(appID, branch, jobID),
		JobId:         jobID,
		CommitId:      req.CommitId,
		CommitMessage: req.CommitMessage,
		CommitTime:    req.CommitTime,
		StartTime:     now,
		Status:        AmplifyJobStatusPending,
		JobType:       req.JobType,
	}
	job := AmplifyJob{Summary: summary, Steps: amplifyJobSteps(now, AmplifyJobStatusPending)}
	amplifyJobs.Put(jobID, amplifyStoredJob{Job: job, AppId: appID, BranchName: branch})
	amplifyTrackJobStart(appID, branch, jobID)
	// Only the build-shaped job types run a real build; a MANUAL StartJob
	// deploys without building, like StartDeployment.
	buildJobType := req.JobType == "RELEASE" || req.JobType == "RETRY" || req.JobType == "WEB_HOOK"
	br := stored.Branches[branch]
	if spec, repo, ok := amplifyRealBuildPlan(stored.App, br); ok && buildJobType {
		amplifyScheduleRealBuild(appID, branch, jobID, amplifyURLBase(r), repo, spec,
			amplifyBuildEnv(stored.App, br, jobID), req.CommitId)
	} else {
		amplifyScheduleJobRun(appID, branch, jobID, amplifyURLBase(r), nil, "e2e-test-artifacts.zip")
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyJobSummary{"jobSummary": summary})
}

// amplifyTrackJobStart bumps the branch's active job + count.
func amplifyTrackJobStart(appID, branch, jobID string) {
	stored, ok := amplifyApps.Get(appID)
	if !ok {
		return
	}
	br, ok := stored.Branches[branch]
	if !ok {
		return
	}
	br.ActiveJobId = jobID
	br.TotalNumberOfJobs = strconv.Itoa(amplifyBranchJobCount(appID, branch))
	stored.Branches[branch] = br
	amplifyApps.Put(appID, stored)
}

// amplifyUploadedArtifact is one client-uploaded deployment object the
// finished job exposes as its build artifact.
type amplifyUploadedArtifact struct {
	FileName string
	Key      string
}

// amplifyScheduleJobRun drives the synthetic PENDING → RUNNING → SUCCEED
// transition. At SUCCEED it materializes the job artifacts — the uploaded
// deployment objects when the client PUT real bytes, otherwise the
// sim-shaped placeholder — and records the production-branch deploy.
func amplifyScheduleJobRun(appID, branch, jobID, urlBase string, uploads []amplifyUploadedArtifact, syntheticName string) {
	go func() {
		time.Sleep(amplifyJobRunningDelay)
		amplifyAdvanceJob(jobID, AmplifyJobStatusPending, AmplifyJobStatusRunning)
		time.Sleep(amplifyJobSucceedDelay)
		if !amplifyAdvanceJob(jobID, AmplifyJobStatusRunning, AmplifyJobStatusSucceed) {
			return
		}
		if len(uploads) == 0 {
			key := "artifacts/" + appID + "/" + branch + "/" + jobID + "/" + syntheticName
			artifactID := amplifyArtifactID(jobID)
			amplifyPutS3Object(key, "application/zip", []byte("amplify artifact "+artifactID+"\n"))
			amplifyRegisterJobArtifact(urlBase, appID, branch, jobID, artifactID, syntheticName, key)
		} else {
			for _, up := range uploads {
				amplifyRegisterJobArtifact(urlBase, appID, branch, jobID, amplifyArtifactID(jobID), up.FileName, up.Key)
			}
		}
		amplifyMarkProductionDeploy(appID, branch, jobID)
	}()
}

// amplifyAdvanceJob moves a job from one status to the next, refusing to
// clobber a job that left the expected state (stopped or deleted meanwhile).
func amplifyAdvanceJob(jobID string, from, to AmplifyJobStatus) bool {
	advanced := false
	amplifyJobs.Update(jobID, func(j *amplifyStoredJob) {
		if j.Job.Summary.Status != from {
			return
		}
		j.Job.Summary.Status = to
		now := amplifyEpoch()
		if to.Terminal() {
			j.Job.Summary.EndTime = now
		}
		for i := range j.Job.Steps {
			j.Job.Steps[i].Status = to
			if to.Terminal() {
				j.Job.Steps[i].EndTime = now
			}
		}
		advanced = true
	})
	return advanced
}

// amplifyMarkProductionDeploy records the app's productionBranch after a
// successful job on a PRODUCTION-stage branch, the way real Amplify
// populates it. thumbnailUrl stays absent (external, sim has no
// screenshots).
func amplifyMarkProductionDeploy(appID, branch, jobID string) {
	job, ok := amplifyJobs.Get(jobID)
	if !ok || job.Job.Summary.Status != AmplifyJobStatusSucceed {
		return
	}
	stored, ok := amplifyApps.Get(appID)
	if !ok {
		return
	}
	br, ok := stored.Branches[branch]
	if !ok || br.Stage != AmplifyStageProduction {
		return
	}
	stored.App.ProductionBranch = &AmplifyProductionBranch{
		BranchName:     branch,
		LastDeployTime: job.Job.Summary.EndTime,
		Status:         job.Job.Summary.Status,
	}
	amplifyApps.Put(appID, stored)
}

func amplifyBranchJobCount(appID, branch string) int {
	n := 0
	for _, j := range amplifyJobs.List() {
		if j.AppId == appID && j.BranchName == branch {
			n++
		}
	}
	return n
}

func amplifyRegisterJobArtifact(urlBase, appID, branch, jobID, artifactID, fileName, key string) {
	amplifyArtifacts.Put(artifactID, amplifyStoredArtifact{
		Artifact: AmplifyArtifact{
			ArtifactId:       artifactID,
			ArtifactFileName: fileName,
		},
		AppId:      appID,
		BranchName: branch,
		JobId:      jobID,
		Key:        key,
		URL:        amplifyPresignedS3URLBase(urlBase, key),
	})
}

func amplifyDeleteArtifactsForJob(jobID string) {
	for _, artifact := range amplifyArtifacts.List() {
		if artifact.JobId == jobID {
			s3Objects.Delete(s3ObjectKey(amplifyArtifactBucketName(), artifact.Key))
			amplifyArtifacts.Delete(artifact.Artifact.ArtifactId)
		}
	}
}

// amplifyDeleteDeployment removes a pending deployment row and any bytes the
// client uploaded against its presigned URLs.
func amplifyDeleteDeployment(dep amplifyStoredDeployment) {
	bucket := amplifyArtifactBucketName()
	s3Objects.Delete(s3ObjectKey(bucket, dep.ZipKey))
	for _, key := range dep.FileKeys {
		s3Objects.Delete(s3ObjectKey(bucket, key))
	}
	amplifyDeployments.Delete(dep.JobId)
}

func amplifyArtifactBucketName() string {
	bucket := "amplify-sim-artifacts-" + awsRegion() + "-" + awsAccountID()
	if _, ok := s3Buckets_.Get(bucket); !ok {
		s3Buckets_.Put(bucket, S3Bucket{
			Name:         bucket,
			CreationDate: time.Now().UTC().Format(time.RFC3339),
		})
	}
	return bucket
}

func amplifyPutS3Object(key, contentType string, data []byte) {
	bucket := amplifyArtifactBucketName()
	hash := md5.Sum(data)
	s3Objects.Put(s3ObjectKey(bucket, key), S3Object{
		Key:          s3ObjectKey(bucket, key),
		Data:         data,
		ContentType:  contentType,
		ETag:         fmt.Sprintf("\"%x\"", hash),
		LastModified: time.Now().UTC(),
		Size:         int64(len(data)),
		Metadata:     map[string]string{"amplify": "true"},
	})
}

func amplifyURLBase(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	return scheme + "://" + r.Host
}

// amplifyPresignedS3URLBase mints a presigned-shape URL into the sim's own
// S3 data plane; the same URL services both the GET (artifact download) and
// PUT (deployment upload) sides because the sim's S3 object routes don't
// gate on the signature query params.
func amplifyPresignedS3URLBase(urlBase, key string) string {
	return fmt.Sprintf("%s/%s/%s?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Expires=3600",
		urlBase, amplifyArtifactBucketName(), key)
}

func amplifyPresignedS3URL(r *http.Request, key string) string {
	return amplifyPresignedS3URLBase(amplifyURLBase(r), key)
}

func amplifyJobForRequest(r *http.Request) (amplifyStoredJob, bool) {
	appID := r.PathValue("appId")
	branch := r.PathValue("name")
	jobID := r.PathValue("jobId")
	stored, ok := amplifyJobs.Get(jobID)
	if !ok || stored.AppId != appID || stored.BranchName != branch {
		return amplifyStoredJob{}, false
	}
	return stored, true
}

func amplifyUpdateBranchAfterJobChange(appID, branch, changedJobID string) {
	stored, ok := amplifyApps.Get(appID)
	if !ok {
		return
	}
	br, ok := stored.Branches[branch]
	if !ok {
		return
	}
	br.TotalNumberOfJobs = strconv.Itoa(amplifyBranchJobCount(appID, branch))
	if br.ActiveJobId == changedJobID {
		br.ActiveJobId = ""
	}
	stored.Branches[branch] = br
	amplifyApps.Put(appID, stored)
}

func handleAmplifyGetJob(w http.ResponseWriter, r *http.Request) {
	stored, ok := amplifyJobForRequest(r)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "job not found")
		return
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyJob{"job": stored.Job})
}

func handleAmplifyStopJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	stored, ok := amplifyJobForRequest(r)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "job not found")
		return
	}
	if status := stored.Job.Summary.Status; status.Terminal() {
		// Real Amplify only stops jobs that are in progress; a finished job
		// is rejected with a BadRequestException.
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException",
			fmt.Sprintf("Job with id %s is already in status %s and cannot be stopped", jobID, status))
		return
	}
	// Inline-settle: real Amplify passes through CANCELLING before
	// CANCELLED; the sim has no build worker to wind down, so the stop
	// lands terminal immediately.
	now := amplifyEpoch()
	stored.Job.Summary.Status = AmplifyJobStatusCancelled
	stored.Job.Summary.EndTime = now
	for i := range stored.Job.Steps {
		if !stored.Job.Steps[i].Status.Terminal() {
			stored.Job.Steps[i].Status = AmplifyJobStatusCancelled
			stored.Job.Steps[i].EndTime = now
		}
	}
	amplifyJobs.Put(jobID, stored)
	// A real build in flight has a container to wind down.
	amplifyCancelRunningBuild(jobID)
	amplifyUpdateBranchAfterJobChange(stored.AppId, stored.BranchName, jobID)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyJobSummary{"jobSummary": stored.Job.Summary})
}

func handleAmplifyDeleteJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	stored, ok := amplifyJobForRequest(r)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "job not found")
		return
	}
	amplifyDeleteArtifactsForJob(jobID)
	amplifyJobs.Delete(jobID)
	amplifyUpdateBranchAfterJobChange(stored.AppId, stored.BranchName, jobID)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyJobSummary{"jobSummary": stored.Job.Summary})
}

func handleAmplifyListJobs(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	branch := r.PathValue("name")
	if _, ok := amplifyApps.Get(appID); !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	token, maxResults, ok := amplifyPageQuery(w, r)
	if !ok {
		return
	}
	summaries := []AmplifyJobSummary{}
	for _, j := range amplifyJobs.List() {
		if j.AppId == appID && j.BranchName == branch {
			summaries = append(summaries, j.Job.Summary)
		}
	}
	// Real ListJobs returns the newest job first.
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].StartTime != summaries[j].StartTime {
			return summaries[i].StartTime > summaries[j].StartTime
		}
		return summaries[i].JobId < summaries[j].JobId
	})
	amplifyWriteListPage(w, "jobSummaries", summaries, token, maxResults)
}

func handleAmplifyListArtifacts(w http.ResponseWriter, r *http.Request) {
	storedJob, ok := amplifyJobForRequest(r)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "job not found")
		return
	}
	token, maxResults, ok := amplifyPageQuery(w, r)
	if !ok {
		return
	}
	artifacts := []AmplifyArtifact{}
	for _, stored := range amplifyArtifacts.List() {
		if stored.AppId == storedJob.AppId && stored.BranchName == storedJob.BranchName && stored.JobId == storedJob.Job.Summary.JobId {
			artifacts = append(artifacts, stored.Artifact)
		}
	}
	sortBy(artifacts, func(a AmplifyArtifact) string { return a.ArtifactId })
	amplifyWriteListPage(w, "artifacts", artifacts, token, maxResults)
}

func handleAmplifyGetArtifactURL(w http.ResponseWriter, r *http.Request) {
	artifactID := r.PathValue("artifactId")
	stored, ok := amplifyArtifacts.Get(artifactID)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "artifact not found")
		return
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]string{
		"artifactId":  stored.Artifact.ArtifactId,
		"artifactUrl": stored.URL,
	})
}

type amplifyGenerateAccessLogsReq struct {
	DomainName string  `json:"domainName"`
	StartTime  float64 `json:"startTime,omitempty"`
	EndTime    float64 `json:"endTime,omitempty"`
}

func handleAmplifyGenerateAccessLogs(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	stored, ok := amplifyApps.Get(appID)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	var req amplifyGenerateAccessLogsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode body: "+err.Error())
		return
	}
	if req.DomainName == "" {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "domainName is required")
		return
	}
	if !amplifyAppOwnsDomain(stored, req.DomainName) {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "domain not found")
		return
	}
	key := "accesslogs/" + appID + "/" + req.DomainName + "/" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10) + ".log"
	amplifyPutS3Object(key, "text/plain", []byte("date time x-edge-location sc-bytes c-ip cs-method cs-host cs-uri-stem sc-status\n"))
	amplifyWriteJSON(w, http.StatusOK, map[string]string{
		"logUrl": amplifyPresignedS3URL(r, key),
	})
}

func amplifyAppOwnsDomain(stored amplifyStoredApp, domain string) bool {
	if stored.App.DefaultDomain == domain {
		return true
	}
	for _, domainAssoc := range amplifyDomains.List() {
		if domainAssoc.AppId == stored.App.AppId && domainAssoc.Domain.DomainName == domain {
			return true
		}
	}
	return false
}

// ---------- Deployments ----------

type amplifyCreateDeploymentReq struct {
	FileMap map[string]string `json:"fileMap,omitempty"`
}

// CreateDeployment mints presigned upload URLs into the sim's own S3 data
// plane and parks a pending-deployment row that StartDeployment consumes.
// Per the model, CreateDeployment carries no NotFoundException — unknown
// app/branch surfaces as BadRequestException.
func handleAmplifyCreateDeployment(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	branch := r.PathValue("name")
	stored, ok := amplifyApps.Get(appID)
	if !ok {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "app not found: "+appID)
		return
	}
	if _, ok := stored.Branches[branch]; !ok {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "branch not found: "+branch)
		return
	}
	var req amplifyCreateDeploymentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode body: "+err.Error())
		return
	}
	jobID := amplifyJobID()
	keyBase := "deployments/" + appID + "/" + branch + "/" + jobID
	dep := amplifyStoredDeployment{
		JobId:      jobID,
		AppId:      appID,
		BranchName: branch,
		ZipKey:     keyBase + "/archive.zip",
		FileKeys:   map[string]string{},
		CreateTime: amplifyEpoch(),
	}
	fileUploadUrls := map[string]string{}
	for name := range req.FileMap {
		key := keyBase + "/files/" + name
		dep.FileKeys[name] = key
		fileUploadUrls[name] = amplifyPresignedS3URL(r, key)
	}
	amplifyDeployments.Put(jobID, dep)
	amplifyWriteJSON(w, http.StatusOK, map[string]any{
		"jobId":          jobID,
		"fileUploadUrls": fileUploadUrls,
		"zipUploadUrl":   amplifyPresignedS3URL(r, dep.ZipKey),
	})
}

type amplifyStartDeploymentReq struct {
	JobId         string               `json:"jobId,omitempty"`
	SourceUrl     string               `json:"sourceUrl,omitempty"`
	SourceUrlType AmplifySourceUrlType `json:"sourceUrlType,omitempty"`
}

func handleAmplifyStartDeployment(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	branch := r.PathValue("name")
	stored, ok := amplifyApps.Get(appID)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	if _, ok := stored.Branches[branch]; !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "branch not found")
		return
	}
	var req amplifyStartDeploymentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode body: "+err.Error())
		return
	}
	if req.JobId == "" && req.SourceUrl == "" {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException",
			"Must specify jobId or sourceUrl for StartDeploymentRequest")
		return
	}
	if req.JobId != "" && req.SourceUrl != "" {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException",
			"Cannot specify both jobId and sourceUrl for StartDeploymentRequest")
		return
	}
	now := amplifyEpoch()
	summary := AmplifyJobSummary{
		StartTime: now,
		Status:    AmplifyJobStatusPending,
		JobType:   "MANUAL",
	}
	var uploads []amplifyUploadedArtifact
	if req.JobId != "" {
		dep, ok := amplifyDeployments.Get(req.JobId)
		if !ok || dep.AppId != appID || dep.BranchName != branch {
			amplifyWriteError(w, http.StatusNotFound, "NotFoundException",
				"no deployment found for jobId "+req.JobId)
			return
		}
		summary.JobId = dep.JobId
		bucket := amplifyArtifactBucketName()
		if _, ok := s3Objects.Get(s3ObjectKey(bucket, dep.ZipKey)); ok {
			uploads = append(uploads, amplifyUploadedArtifact{FileName: "archive.zip", Key: dep.ZipKey})
		}
		for name, key := range dep.FileKeys {
			if _, ok := s3Objects.Get(s3ObjectKey(bucket, key)); ok {
				uploads = append(uploads, amplifyUploadedArtifact{FileName: name, Key: key})
			}
		}
		sort.Slice(uploads, func(i, j int) bool { return uploads[i].FileName < uploads[j].FileName })
		// Consumed: the uploaded objects now belong to the job's artifacts.
		amplifyDeployments.Delete(dep.JobId)
	} else {
		sourceURLType := req.SourceUrlType
		if sourceURLType == "" {
			// Documented default: "If no value is specified, the default is ZIP."
			sourceURLType = AmplifySourceUrlZip
		}
		if sourceURLType != AmplifySourceUrlZip && sourceURLType != AmplifySourceUrlBucketPrefix {
			amplifyWriteError(w, http.StatusBadRequest, "BadRequestException",
				"sourceUrlType must be ZIP or BUCKET_PREFIX")
			return
		}
		if !strings.HasPrefix(req.SourceUrl, "s3://") && !strings.HasPrefix(req.SourceUrl, "https://") && !strings.HasPrefix(req.SourceUrl, "http://") {
			amplifyWriteError(w, http.StatusBadRequest, "BadRequestException",
				"sourceUrl must start with s3://, https:// or http://")
			return
		}
		summary.JobId = amplifyJobID()
		summary.SourceUrl = req.SourceUrl
		summary.SourceUrlType = sourceURLType
	}
	summary.JobArn = amplifyJobARN(appID, branch, summary.JobId)
	job := AmplifyJob{Summary: summary, Steps: amplifyJobSteps(now, AmplifyJobStatusPending)}
	amplifyJobs.Put(summary.JobId, amplifyStoredJob{Job: job, AppId: appID, BranchName: branch})
	amplifyTrackJobStart(appID, branch, summary.JobId)
	amplifyScheduleJobRun(appID, branch, summary.JobId, amplifyURLBase(r), uploads, "deployment-artifacts.zip")
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyJobSummary{"jobSummary": summary})
}

// ---------- Tagging ----------

// Tag URLs use the resource ARN as a wildcard tail. We just trim the prefix.

func amplifyTagARN(r *http.Request) string {
	arn := r.PathValue("arn")
	return arn
}

func handleAmplifyListTags(w http.ResponseWriter, r *http.Request) {
	arn := amplifyTagARN(r)
	tags, _ := amplifyTagsForARN(arn)
	if tags == nil {
		tags = map[string]string{}
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func handleAmplifyTagResource(w http.ResponseWriter, r *http.Request) {
	arn := amplifyTagARN(r)
	var req struct {
		Tags map[string]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode: "+err.Error())
		return
	}
	if !amplifySetTagsForARN(arn, req.Tags, false) {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "resource not found")
		return
	}
	amplifyWriteJSON(w, http.StatusOK, struct{}{})
}

func handleAmplifyUntagResource(w http.ResponseWriter, r *http.Request) {
	arn := amplifyTagARN(r)
	// AWS CLI sends ?tagKeys=Key1&tagKeys=Key2
	keys := r.URL.Query()["tagKeys"]
	if !amplifyRemoveTagsForARN(arn, keys) {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "resource not found")
		return
	}
	amplifyWriteJSON(w, http.StatusOK, struct{}{})
}

func amplifyTagsForARN(arn string) (map[string]string, bool) {
	for _, s := range amplifyApps.List() {
		if s.App.AppArn == arn {
			return s.App.Tags, true
		}
		for _, br := range s.Branches {
			if br.BranchArn == arn {
				return br.Tags, true
			}
		}
	}
	return nil, false
}

func amplifySetTagsForARN(arn string, tags map[string]string, replace bool) bool {
	for _, s := range amplifyApps.List() {
		if s.App.AppArn == arn {
			if replace || s.App.Tags == nil {
				s.App.Tags = map[string]string{}
			}
			for k, v := range tags {
				s.App.Tags[k] = v
			}
			amplifyApps.Put(s.App.AppId, s)
			return true
		}
		for k, br := range s.Branches {
			if br.BranchArn == arn {
				if replace || br.Tags == nil {
					br.Tags = map[string]string{}
				}
				for tk, tv := range tags {
					br.Tags[tk] = tv
				}
				s.Branches[k] = br
				amplifyApps.Put(s.App.AppId, s)
				return true
			}
		}
	}
	return false
}

func amplifyRemoveTagsForARN(arn string, keys []string) bool {
	for _, s := range amplifyApps.List() {
		if s.App.AppArn == arn {
			for _, k := range keys {
				delete(s.App.Tags, k)
			}
			amplifyApps.Put(s.App.AppId, s)
			return true
		}
		for k, br := range s.Branches {
			if br.BranchArn == arn {
				for _, key := range keys {
					delete(br.Tags, key)
				}
				s.Branches[k] = br
				amplifyApps.Put(s.App.AppId, s)
				return true
			}
		}
	}
	return false
}
