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

// AWS Amplify. Wire: REST + JSON, versionless paths (/apps, /apps/{id},
// etc.). Sim covers the apps + branches + webhooks + jobs surface here;
// domains + backendenvironments come in .
//
// Sim policy:
//   - Jobs synthesise SUCCEEDED eagerly (real Amplify runs a real build
// pipeline; sim doesn't have that). Per plan, out of scope.

// ---------- Types ----------

type AmplifyApp struct {
	AppArn                     string            `json:"appArn"`
	AppId                      string            `json:"appId"`
	Name                       string            `json:"name"`
	Description                string            `json:"description,omitempty"`
	Repository                 string            `json:"repository,omitempty"`
	Platform                   string            `json:"platform"`
	CreateTime                 float64           `json:"createTime"`
	UpdateTime                 float64           `json:"updateTime"`
	IamServiceRoleArn          string            `json:"iamServiceRoleArn,omitempty"`
	EnvironmentVariables       map[string]string `json:"environmentVariables,omitempty"`
	DefaultDomain              string            `json:"defaultDomain"`
	EnableBranchAutoBuild      bool              `json:"enableBranchAutoBuild"`
	EnableBranchAutoDeletion   bool              `json:"enableBranchAutoDeletion"`
	EnableBasicAuth            bool              `json:"enableBasicAuth"`
	BasicAuthCredentials       string            `json:"basicAuthCredentials,omitempty"`
	CustomRules                json.RawMessage   `json:"customRules,omitempty"`
	BuildSpec                  string            `json:"buildSpec,omitempty"`
	CustomHeaders              string            `json:"customHeaders,omitempty"`
	EnableAutoBranchCreation   bool              `json:"enableAutoBranchCreation"`
	AutoBranchCreationPatterns []string          `json:"autoBranchCreationPatterns,omitempty"`
	AutoBranchCreationConfig   json.RawMessage   `json:"autoBranchCreationConfig,omitempty"`
	Tags                       map[string]string `json:"tags,omitempty"`
	ProductionBranch           json.RawMessage   `json:"productionBranch,omitempty"`
	Repository_                string            `json:"-"`
}

type AmplifyBranch struct {
	BranchArn                  string            `json:"branchArn"`
	BranchName                 string            `json:"branchName"`
	Description                string            `json:"description,omitempty"`
	Tags                       map[string]string `json:"tags,omitempty"`
	Stage                      string            `json:"stage"`
	DisplayName                string            `json:"displayName,omitempty"`
	EnableNotification         bool              `json:"enableNotification"`
	CreateTime                 float64           `json:"createTime"`
	UpdateTime                 float64           `json:"updateTime"`
	EnvironmentVariables       map[string]string `json:"environmentVariables,omitempty"`
	EnableAutoBuild            bool              `json:"enableAutoBuild"`
	CustomDomains              []string          `json:"customDomains,omitempty"`
	Framework                  string            `json:"framework,omitempty"`
	ActiveJobId                string            `json:"activeJobId,omitempty"`
	TotalNumberOfJobs          string            `json:"totalNumberOfJobs"`
	EnableBasicAuth            bool              `json:"enableBasicAuth"`
	EnablePerformanceMode      bool              `json:"enablePerformanceMode"`
	ThumbnailUrl               string            `json:"thumbnailUrl,omitempty"`
	BasicAuthCredentials       string            `json:"basicAuthCredentials,omitempty"`
	BuildSpec                  string            `json:"buildSpec,omitempty"`
	TtL                        string            `json:"ttl"`
	AssociatedResources        []string          `json:"associatedResources,omitempty"`
	EnablePullRequestPreview   bool              `json:"enablePullRequestPreview"`
	PullRequestEnvironmentName string            `json:"pullRequestEnvironmentName,omitempty"`
	DestinationBranch          string            `json:"destinationBranch,omitempty"`
	SourceBranch               string            `json:"sourceBranch,omitempty"`
	BackendEnvironmentArn      string            `json:"backendEnvironmentArn,omitempty"`
}

type AmplifyWebhook struct {
	WebhookArn  string  `json:"webhookArn"`
	WebhookId   string  `json:"webhookId"`
	WebhookUrl  string  `json:"webhookUrl"`
	BranchName  string  `json:"branchName"`
	Description string  `json:"description,omitempty"`
	CreateTime  float64 `json:"createTime"`
	UpdateTime  float64 `json:"updateTime"`
	AppId       string  `json:"-"`
}

type AmplifyJobSummary struct {
	JobArn        string  `json:"jobArn"`
	JobId         string  `json:"jobId"`
	CommitId      string  `json:"commitId,omitempty"`
	CommitMessage string  `json:"commitMessage,omitempty"`
	CommitTime    float64 `json:"commitTime,omitempty"`
	StartTime     float64 `json:"startTime,omitempty"`
	Status        string  `json:"status"`
	EndTime       float64 `json:"endTime,omitempty"`
	JobType       string  `json:"jobType"`
}

type AmplifyJob struct {
	Summary AmplifyJobSummary `json:"summary"`
	Steps   []AmplifyJobStep  `json:"steps,omitempty"`
}

type AmplifyJobStep struct {
	StepName  string  `json:"stepName"`
	StartTime float64 `json:"startTime"`
	EndTime   float64 `json:"endTime"`
	Status    string  `json:"status"`
	LogUrl    string  `json:"logUrl,omitempty"`
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

var (
	amplifyApps     sim.Store[amplifyStoredApp]
	amplifyWebhooks sim.Store[amplifyStoredWebhook]
	amplifyJobs     sim.Store[amplifyStoredJob]
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

// ---------- Registration ----------

func registerAmplify(srv *sim.Server) {
	amplifyApps = sim.MakeStore[amplifyStoredApp](srv.DB(), "amplify_apps")
	amplifyWebhooks = sim.MakeStore[amplifyStoredWebhook](srv.DB(), "amplify_webhooks")
	amplifyJobs = sim.MakeStore[amplifyStoredJob](srv.DB(), "amplify_jobs")

	mux := srv.Mux()
	// Apps
	mux.HandleFunc("POST /apps", handleAmplifyCreateApp)
	mux.HandleFunc("GET /apps", handleAmplifyListApps)
	mux.HandleFunc("GET /apps/{appId}", handleAmplifyGetApp)
	mux.HandleFunc("POST /apps/{appId}", handleAmplifyUpdateApp)
	mux.HandleFunc("DELETE /apps/{appId}", handleAmplifyDeleteApp)
	// Branches
	mux.HandleFunc("POST /apps/{appId}/branches", handleAmplifyCreateBranch)
	mux.HandleFunc("GET /apps/{appId}/branches", handleAmplifyListBranches)
	mux.HandleFunc("GET /apps/{appId}/branches/{name}", handleAmplifyGetBranch)
	mux.HandleFunc("POST /apps/{appId}/branches/{name}", handleAmplifyUpdateBranch)
	mux.HandleFunc("DELETE /apps/{appId}/branches/{name}", handleAmplifyDeleteBranch)
	// Webhooks
	mux.HandleFunc("POST /apps/{appId}/webhooks", handleAmplifyCreateWebhook)
	mux.HandleFunc("GET /apps/{appId}/webhooks", handleAmplifyListWebhooks)
	mux.HandleFunc("GET /webhooks/{webhookId}", handleAmplifyGetWebhook)
	mux.HandleFunc("POST /webhooks/{webhookId}", handleAmplifyUpdateWebhook)
	mux.HandleFunc("DELETE /webhooks/{webhookId}", handleAmplifyDeleteWebhook)
	// Jobs / deployments
	mux.HandleFunc("POST /apps/{appId}/branches/{name}/jobs", handleAmplifyStartJob)
	mux.HandleFunc("GET /apps/{appId}/branches/{name}/jobs", handleAmplifyListJobs)
	mux.HandleFunc("GET /apps/{appId}/branches/{name}/jobs/{jobId}", handleAmplifyGetJob)
	mux.HandleFunc("DELETE /apps/{appId}/branches/{name}/jobs/{jobId}", handleAmplifyStopJob)
	// Deployments — note SDK routes:
	//   POST /apps/{appId}/branches/{name}/deployments       — CreateDeployment (returns upload URL)
	//   POST /apps/{appId}/branches/{name}/deployments/start — StartDeployment (kicks off the job)
	mux.HandleFunc("POST /apps/{appId}/branches/{name}/deployments", handleAmplifyCreateDeployment)
	mux.HandleFunc("POST /apps/{appId}/branches/{name}/deployments/start", handleAmplifyStartDeployment)
	// Tags
	mux.HandleFunc("GET /tags/{arn...}", handleAmplifyListTags)
	mux.HandleFunc("POST /tags/{arn...}", handleAmplifyTagResource)
	mux.HandleFunc("DELETE /tags/{arn...}", handleAmplifyUntagResource)

	// Domains + BackendEnvironments (amplify_domains.go)
	registerAmplifyDomains(srv)
}

// ---------- Apps ----------

type amplifyCreateAppReq struct {
	Name                       string            `json:"name"`
	Description                string            `json:"description,omitempty"`
	Repository                 string            `json:"repository,omitempty"`
	Platform                   string            `json:"platform,omitempty"`
	IamServiceRoleArn          string            `json:"iamServiceRoleArn,omitempty"`
	OauthToken                 string            `json:"oauthToken,omitempty"`
	AccessToken                string            `json:"accessToken,omitempty"`
	EnvironmentVariables       map[string]string `json:"environmentVariables,omitempty"`
	EnableBranchAutoBuild      *bool             `json:"enableBranchAutoBuild,omitempty"`
	EnableBranchAutoDeletion   *bool             `json:"enableBranchAutoDeletion,omitempty"`
	EnableBasicAuth            *bool             `json:"enableBasicAuth,omitempty"`
	BasicAuthCredentials       string            `json:"basicAuthCredentials,omitempty"`
	CustomRules                json.RawMessage   `json:"customRules,omitempty"`
	Tags                       map[string]string `json:"tags,omitempty"`
	BuildSpec                  string            `json:"buildSpec,omitempty"`
	CustomHeaders              string            `json:"customHeaders,omitempty"`
	EnableAutoBranchCreation   *bool             `json:"enableAutoBranchCreation,omitempty"`
	AutoBranchCreationPatterns []string          `json:"autoBranchCreationPatterns,omitempty"`
	AutoBranchCreationConfig   json.RawMessage   `json:"autoBranchCreationConfig,omitempty"`
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
	app := AmplifyApp{
		AppArn:                     amplifyAppARN(id),
		AppId:                      id,
		Name:                       req.Name,
		Description:                req.Description,
		Repository:                 req.Repository,
		Platform:                   platform,
		CreateTime:                 now,
		UpdateTime:                 now,
		IamServiceRoleArn:          req.IamServiceRoleArn,
		EnvironmentVariables:       req.EnvironmentVariables,
		DefaultDomain:              id + ".amplifyapp.com",
		EnableBranchAutoBuild:      boolOr(req.EnableBranchAutoBuild, true),
		EnableBranchAutoDeletion:   boolOr(req.EnableBranchAutoDeletion, false),
		EnableBasicAuth:            boolOr(req.EnableBasicAuth, false),
		BasicAuthCredentials:       req.BasicAuthCredentials,
		CustomRules:                req.CustomRules,
		BuildSpec:                  req.BuildSpec,
		CustomHeaders:              req.CustomHeaders,
		EnableAutoBranchCreation:   boolOr(req.EnableAutoBranchCreation, false),
		AutoBranchCreationPatterns: req.AutoBranchCreationPatterns,
		AutoBranchCreationConfig:   req.AutoBranchCreationConfig,
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
	// Cascade-delete: webhooks + jobs referencing this app go too.
	for _, wh := range amplifyWebhooks.List() {
		if wh.AppId == id {
			amplifyWebhooks.Delete(wh.Webhook.WebhookId)
		}
	}
	for _, jb := range amplifyJobs.List() {
		if jb.AppId == id {
			amplifyJobs.Delete(jb.Job.Summary.JobId)
		}
	}
	amplifyApps.Delete(id)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyApp{"app": stored.App})
}

type amplifyUpdateAppReq amplifyCreateAppReq

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
	if req.Name != "" {
		a.Name = req.Name
	}
	if req.Description != "" {
		a.Description = req.Description
	}
	if req.Repository != "" {
		a.Repository = req.Repository
	}
	if req.Platform != "" {
		a.Platform = req.Platform
	}
	if req.IamServiceRoleArn != "" {
		a.IamServiceRoleArn = req.IamServiceRoleArn
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
	if req.BasicAuthCredentials != "" {
		a.BasicAuthCredentials = req.BasicAuthCredentials
	}
	if len(req.CustomRules) > 0 {
		a.CustomRules = req.CustomRules
	}
	if req.BuildSpec != "" {
		a.BuildSpec = req.BuildSpec
	}
	if req.CustomHeaders != "" {
		a.CustomHeaders = req.CustomHeaders
	}
	a.UpdateTime = amplifyEpoch()
	amplifyApps.Put(id, stored)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyApp{"app": stored.App})
}

func handleAmplifyListApps(w http.ResponseWriter, r *http.Request) {
	apps := []AmplifyApp{}
	for _, s := range amplifyApps.List() {
		apps = append(apps, s.App)
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]any{"apps": apps})
}

// ---------- Branches ----------

type amplifyCreateBranchReq struct {
	BranchName                 string            `json:"branchName"`
	Description                string            `json:"description,omitempty"`
	Stage                      string            `json:"stage,omitempty"`
	Framework                  string            `json:"framework,omitempty"`
	EnableNotification         *bool             `json:"enableNotification,omitempty"`
	EnableAutoBuild            *bool             `json:"enableAutoBuild,omitempty"`
	EnvironmentVariables       map[string]string `json:"environmentVariables,omitempty"`
	BasicAuthCredentials       string            `json:"basicAuthCredentials,omitempty"`
	EnableBasicAuth            *bool             `json:"enableBasicAuth,omitempty"`
	EnablePerformanceMode      *bool             `json:"enablePerformanceMode,omitempty"`
	Tags                       map[string]string `json:"tags,omitempty"`
	BuildSpec                  string            `json:"buildSpec,omitempty"`
	Ttl                        string            `json:"ttl,omitempty"`
	DisplayName                string            `json:"displayName,omitempty"`
	EnablePullRequestPreview   *bool             `json:"enablePullRequestPreview,omitempty"`
	PullRequestEnvironmentName string            `json:"pullRequestEnvironmentName,omitempty"`
	BackendEnvironmentArn      string            `json:"backendEnvironmentArn,omitempty"`
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
		stage = "NONE"
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
	var req amplifyCreateBranchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode: "+err.Error())
		return
	}
	if req.Description != "" {
		br.Description = req.Description
	}
	if req.Stage != "" {
		br.Stage = req.Stage
	}
	if req.Framework != "" {
		br.Framework = req.Framework
	}
	if req.EnableNotification != nil {
		br.EnableNotification = *req.EnableNotification
	}
	if req.EnableAutoBuild != nil {
		br.EnableAutoBuild = *req.EnableAutoBuild
	}
	if req.EnvironmentVariables != nil {
		br.EnvironmentVariables = req.EnvironmentVariables
	}
	if req.EnableBasicAuth != nil {
		br.EnableBasicAuth = *req.EnableBasicAuth
	}
	if req.BuildSpec != "" {
		br.BuildSpec = req.BuildSpec
	}
	if req.Ttl != "" {
		br.TtL = req.Ttl
	}
	if req.DisplayName != "" {
		br.DisplayName = req.DisplayName
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
	branches := make([]AmplifyBranch, 0, len(stored.Branches))
	for _, br := range stored.Branches {
		branches = append(branches, br)
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]any{"branches": branches})
}

// ---------- Webhooks ----------

type amplifyCreateWebhookReq struct {
	BranchName  string `json:"branchName"`
	Description string `json:"description,omitempty"`
}

func handleAmplifyCreateWebhook(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	if _, ok := amplifyApps.Get(appID); !ok {
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
		BranchName:  req.BranchName,
		Description: req.Description,
		CreateTime:  now,
		UpdateTime:  now,
	}
	amplifyWebhooks.Put(id, amplifyStoredWebhook{Webhook: wh, AppId: appID})
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyWebhook{"webhook": wh})
}

func handleAmplifyGetWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("webhookId")
	stored, ok := amplifyWebhooks.Get(id)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "webhook not found")
		return
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyWebhook{"webhook": stored.Webhook})
}

func handleAmplifyUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("webhookId")
	stored, ok := amplifyWebhooks.Get(id)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "webhook not found")
		return
	}
	var req struct {
		BranchName  string `json:"branchName,omitempty"`
		Description string `json:"description,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		amplifyWriteError(w, http.StatusBadRequest, "BadRequestException", "could not decode: "+err.Error())
		return
	}
	if req.BranchName != "" {
		stored.Webhook.BranchName = req.BranchName
	}
	if req.Description != "" {
		stored.Webhook.Description = req.Description
	}
	stored.Webhook.UpdateTime = amplifyEpoch()
	amplifyWebhooks.Put(id, stored)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyWebhook{"webhook": stored.Webhook})
}

func handleAmplifyDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("webhookId")
	stored, ok := amplifyWebhooks.Get(id)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "webhook not found")
		return
	}
	amplifyWebhooks.Delete(id)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyWebhook{"webhook": stored.Webhook})
}

func handleAmplifyListWebhooks(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	if _, ok := amplifyApps.Get(appID); !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	items := []AmplifyWebhook{}
	for _, s := range amplifyWebhooks.List() {
		if s.AppId == appID {
			items = append(items, s.Webhook)
		}
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]any{"webhooks": items})
}

// ---------- Jobs ----------

type amplifyStartJobReq struct {
	JobType       string  `json:"jobType"` // RELEASE / RETRY / MANUAL / WEB_HOOK
	JobReason     string  `json:"jobReason,omitempty"`
	CommitId      string  `json:"commitId,omitempty"`
	CommitMessage string  `json:"commitMessage,omitempty"`
	CommitTime    float64 `json:"commitTime,omitempty"`
	JobId         string  `json:"jobId,omitempty"`
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
	// Per plan, sim synthesises SUCCEEDED eagerly. Real
	// Amplify runs a real npm build pipeline; sim doesn't.
	summary := AmplifyJobSummary{
		JobArn:        amplifyJobARN(appID, branch, jobID),
		JobId:         jobID,
		CommitId:      req.CommitId,
		CommitMessage: req.CommitMessage,
		CommitTime:    req.CommitTime,
		StartTime:     now,
		EndTime:       now + 1,
		Status:        "SUCCEED",
		JobType:       req.JobType,
	}
	job := AmplifyJob{
		Summary: summary,
		Steps: []AmplifyJobStep{
			{StepName: "PROVISION", StartTime: now, EndTime: now, Status: "SUCCEED"},
			{StepName: "BUILD", StartTime: now, EndTime: now + 1, Status: "SUCCEED"},
			{StepName: "DEPLOY", StartTime: now + 1, EndTime: now + 1, Status: "SUCCEED"},
		},
	}
	amplifyJobs.Put(jobID, amplifyStoredJob{Job: job, AppId: appID, BranchName: branch})
	// Bump branch active job + count.
	br := stored.Branches[branch]
	br.ActiveJobId = jobID
	br.TotalNumberOfJobs = fmt.Sprintf("%d", amplifyBranchJobCount(appID, branch)+1)
	stored.Branches[branch] = br
	amplifyApps.Put(appID, stored)
	amplifyWriteJSON(w, http.StatusOK, map[string]any{"jobSummary": summary})
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

func handleAmplifyGetJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	stored, ok := amplifyJobs.Get(jobID)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "job not found")
		return
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyJob{"job": stored.Job})
}

func handleAmplifyStopJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	stored, ok := amplifyJobs.Get(jobID)
	if !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "job not found")
		return
	}
	stored.Job.Summary.Status = "CANCELLED"
	stored.Job.Summary.EndTime = amplifyEpoch()
	amplifyJobs.Put(jobID, stored)
	amplifyWriteJSON(w, http.StatusOK, map[string]AmplifyJobSummary{"jobSummary": stored.Job.Summary})
}

func handleAmplifyListJobs(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	branch := r.PathValue("name")
	if _, ok := amplifyApps.Get(appID); !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	summaries := []AmplifyJobSummary{}
	for _, j := range amplifyJobs.List() {
		if j.AppId == appID && j.BranchName == branch {
			summaries = append(summaries, j.Job.Summary)
		}
	}
	amplifyWriteJSON(w, http.StatusOK, map[string]any{"jobSummaries": summaries})
}

// CreateDeployment is for manual zip-upload deployments. Sim accepts the
// upload-URL request and returns a synthesised S3-presigned URL.
func handleAmplifyCreateDeployment(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("appId")
	if _, ok := amplifyApps.Get(appID); !ok {
		amplifyWriteError(w, http.StatusNotFound, "NotFoundException", "app not found")
		return
	}
	jobID := amplifyJobID()
	amplifyWriteJSON(w, http.StatusOK, map[string]any{
		"jobId":          jobID,
		"fileUploadUrls": map[string]string{},
		"zipUploadUrl":   "https://amplify-sim.example.com/upload/" + jobID,
	})
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
	var req struct {
		JobId     string `json:"jobId,omitempty"`
		SourceUrl string `json:"sourceUrl,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	jobID := req.JobId
	if jobID == "" {
		jobID = amplifyJobID()
	}
	now := amplifyEpoch()
	summary := AmplifyJobSummary{
		JobArn: amplifyJobARN(appID, branch, jobID), JobId: jobID,
		StartTime: now, EndTime: now + 1,
		Status: "SUCCEED", JobType: "MANUAL",
	}
	amplifyJobs.Put(jobID, amplifyStoredJob{Job: AmplifyJob{Summary: summary}, AppId: appID, BranchName: branch})
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

// strings import kept ready for future filter logic.
var _ = strings.ToLower
