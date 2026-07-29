package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	sim "github.com/sockerless/simulator"
	"gopkg.in/yaml.v3"
)

// AWS CodeBuild — AWS JSON 1.1 protocol (X-Amz-Target: CodeBuild_20161006.<Op>).
// Builds execute project buildspec commands and record terminal state from exit status.

type CBProject struct {
	Name         string         `json:"name"`
	Arn          string         `json:"arn"`
	Description  string         `json:"description,omitempty"`
	Source       map[string]any `json:"source"`
	Artifacts    map[string]any `json:"artifacts"`
	Environment  map[string]any `json:"environment"`
	ServiceRole  string         `json:"serviceRole"`
	Created      float64        `json:"created"`
	LastModified float64        `json:"lastModified"`
	Tags         []CBTag        `json:"tags,omitempty"`
}

type CBBuild struct {
	ID          string         `json:"id"`
	Arn         string         `json:"arn"`
	ProjectName string         `json:"projectName"`
	BuildStatus string         `json:"buildStatus"`
	StartTime   float64        `json:"startTime"`
	EndTime     float64        `json:"endTime"`
	Phases      []CBPhase      `json:"phases"`
	Logs        map[string]any `json:"logs"`
	Environment map[string]any `json:"environment,omitempty"`
	ReportArns  []string       `json:"reportArns,omitempty"`
	// Seq is a sim-internal monotonic creation order used to sort ListBuilds
	// faithfully by start order; it's not part of the CodeBuild wire shape.
	Seq int64 `json:"-"`
	// ReportGroups holds the report-group names the buildspec references; the
	// build produces a Report per group on completion. Sim-internal, not wire.
	ReportGroups []string `json:"-"`
}

type CBPhase struct {
	PhaseType         string           `json:"phaseType"`
	PhaseStatus       string           `json:"phaseStatus"`
	StartTime         float64          `json:"startTime"`
	EndTime           float64          `json:"endTime"`
	DurationInSeconds float64          `json:"durationInSeconds"`
	Contexts          []CBPhaseContext `json:"contexts,omitempty"`
}

type CBPhaseContext struct {
	StatusCode string `json:"statusCode"`
	Message    string `json:"message"`
}

// cbLogsLocation is the LogsLocation the sim reports: builds run as local
// processes without a CloudWatch log sink, so log enablement is the real
// member cloudWatchLogs.status (not an invented boolean).
func cbLogsLocation() map[string]any {
	return map[string]any{
		"cloudWatchLogs": map[string]any{"status": "DISABLED"},
	}
}

type CBTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// CBReportGroup mirrors the CodeBuild ReportGroup shape. status is read-only
// and ACTIVE for a live group; type is TEST or CODE_COVERAGE.
type CBReportGroup struct {
	Arn          string         `json:"arn"`
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	ExportConfig map[string]any `json:"exportConfig,omitempty"`
	Created      float64        `json:"created"`
	LastModified float64        `json:"lastModified"`
	Status       string         `json:"status"`
	Tags         []CBTag        `json:"tags,omitempty"`
}

// CBReport mirrors the CodeBuild Report shape. A report is produced by a build
// whose buildspec references the report group; the sim creates it from the
// build's terminal state, never a synthetic placeholder.
type CBReport struct {
	Arn            string         `json:"arn"`
	Type           string         `json:"type"`
	Name           string         `json:"name"`
	ReportGroupArn string         `json:"reportGroupArn"`
	ExecutionId    string         `json:"executionId,omitempty"`
	Status         string         `json:"status"`
	Created        float64        `json:"created"`
	Expired        float64        `json:"expired,omitempty"`
	ExportConfig   map[string]any `json:"exportConfig,omitempty"`
	Truncated      bool           `json:"truncated"`
}

// CBSourceCredential mirrors the SourceCredentialsInfo shape. The token itself
// is never echoed back by the real API; only the ARN, authType, serverType,
// and (for SECRETS_MANAGER) the resource are readable.
type CBSourceCredential struct {
	Arn        string `json:"arn"`
	ServerType string `json:"serverType"`
	AuthType   string `json:"authType"`
	Resource   string `json:"resource,omitempty"`
}

var (
	cbProjects    sim.Store[CBProject]
	cbBuilds      sim.Store[CBBuild]
	cbReportGrps  sim.Store[CBReportGroup]
	cbReports     sim.Store[CBReport]
	cbSourceCreds sim.Store[CBSourceCredential]
	cbMu          sync.Mutex
)

func registerCodeBuild(r *sim.AWSRouter, srv *sim.Server) {
	cbProjects = sim.MakeStore[CBProject](srv.DB(), "codebuild_projects")
	cbBuilds = sim.MakeStore[CBBuild](srv.DB(), "codebuild_builds")
	cbReportGrps = sim.MakeStore[CBReportGroup](srv.DB(), "codebuild_report_groups")
	cbReports = sim.MakeStore[CBReport](srv.DB(), "codebuild_reports")
	cbSourceCreds = sim.MakeStore[CBSourceCredential](srv.DB(), "codebuild_source_credentials")

	r.Register("CodeBuild_20161006.CreateProject", handleCBCreateProject)
	r.Register("CodeBuild_20161006.BatchGetProjects", handleCBBatchGetProjects)
	r.Register("CodeBuild_20161006.ListProjects", handleCBListProjects)
	r.Register("CodeBuild_20161006.UpdateProject", handleCBUpdateProject)
	r.Register("CodeBuild_20161006.DeleteProject", handleCBDeleteProject)
	r.Register("CodeBuild_20161006.StartBuild", handleCBStartBuild)
	r.Register("CodeBuild_20161006.StopBuild", handleCBStopBuild)
	r.Register("CodeBuild_20161006.RetryBuild", handleCBRetryBuild)
	r.Register("CodeBuild_20161006.BatchGetBuilds", handleCBBatchGetBuilds)
	r.Register("CodeBuild_20161006.ListBuildsForProject", handleCBListBuildsForProject)
	r.Register("CodeBuild_20161006.ListBuilds", handleCBListBuilds)

	r.Register("CodeBuild_20161006.CreateReportGroup", handleCBCreateReportGroup)
	r.Register("CodeBuild_20161006.UpdateReportGroup", handleCBUpdateReportGroup)
	r.Register("CodeBuild_20161006.DeleteReportGroup", handleCBDeleteReportGroup)
	r.Register("CodeBuild_20161006.ListReportGroups", handleCBListReportGroups)
	r.Register("CodeBuild_20161006.BatchGetReportGroups", handleCBBatchGetReportGroups)
	r.Register("CodeBuild_20161006.ListReports", handleCBListReports)
	r.Register("CodeBuild_20161006.ListReportsForReportGroup", handleCBListReportsForReportGroup)
	r.Register("CodeBuild_20161006.BatchGetReports", handleCBBatchGetReports)

	r.Register("CodeBuild_20161006.ImportSourceCredentials", handleCBImportSourceCredentials)
	r.Register("CodeBuild_20161006.ListSourceCredentials", handleCBListSourceCredentials)
	r.Register("CodeBuild_20161006.DeleteSourceCredentials", handleCBDeleteSourceCredentials)
}

func cbARN(resource string) string {
	return fmt.Sprintf("arn:aws:codebuild:us-east-1:123456789012:%s", resource)
}

func cbEpochNow() float64 {
	return float64(time.Now().UTC().Unix())
}

// cbBuildSeq is a process-wide monotonic counter giving each build a strictly
// increasing creation order, so ListBuilds sorts faithfully by start order even
// when builds are created within the same wall-clock second.
var cbBuildSeq atomic.Int64

func cbNextBuildSeq() int64 { return cbBuildSeq.Add(1) }

func cbWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func cbWriteError(w http.ResponseWriter, code string, msg string) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.Header().Set("X-Amzn-Errortype", code)
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"__type":  code,
		"message": msg,
	})
}

func handleCBCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Source      map[string]any `json:"source"`
		Artifacts   map[string]any `json:"artifacts"`
		Environment map[string]any `json:"environment"`
		ServiceRole string         `json:"serviceRole"`
		Tags        []CBTag        `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.Name == "" {
		cbWriteError(w, "InvalidInputException", "name is required")
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	if _, ok := cbProjects.Get(req.Name); ok {
		cbWriteError(w, "ResourceAlreadyExistsException", "Project already exists: "+req.Name)
		return
	}

	now := cbEpochNow()
	if req.Source == nil {
		req.Source = map[string]any{"type": "NO_SOURCE"}
	}
	if req.Artifacts == nil {
		req.Artifacts = map[string]any{"type": "NO_ARTIFACTS"}
	}
	if req.Environment == nil {
		req.Environment = map[string]any{"type": "LINUX_CONTAINER", "image": "aws/codebuild/standard:7.0", "computeType": "BUILD_GENERAL1_SMALL"}
	}
	p := CBProject{
		Name:         req.Name,
		Arn:          cbARN("project/" + req.Name),
		Description:  req.Description,
		Source:       req.Source,
		Artifacts:    req.Artifacts,
		Environment:  req.Environment,
		ServiceRole:  req.ServiceRole,
		Created:      now,
		LastModified: now,
		Tags:         req.Tags,
	}
	cbProjects.Put(req.Name, p)
	cbWriteJSON(w, http.StatusOK, map[string]any{"project": p})
}

func handleCBBatchGetProjects(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	var found []CBProject
	var notFound []string
	for _, nameOrARN := range req.Names {
		p, ok := cbProjects.Get(nameOrARN)
		if !ok {
			p, ok = cbProjects.Get(cbNameFromARN(nameOrARN))
		}
		if ok {
			found = append(found, p)
		} else {
			notFound = append(notFound, nameOrARN)
		}
	}
	if found == nil {
		found = []CBProject{}
	}
	if notFound == nil {
		notFound = []string{}
	}
	cbWriteJSON(w, http.StatusOK, map[string]any{
		"projects":         found,
		"projectsNotFound": notFound,
	})
}

func handleCBListProjects(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SortBy    string `json:"sortBy"`
		SortOrder string `json:"sortOrder"`
		NextToken string `json:"nextToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	all := cbProjects.List()
	names := make([]string, 0, len(all))
	for _, p := range all {
		names = append(names, p.Name)
	}
	// ListProjects sorts by name; default order is ASCENDING.
	sort.Strings(names)
	if strings.EqualFold(req.SortOrder, "DESCENDING") {
		reverseStrings(names)
	}
	page, nextTok := awsPage(names, req.NextToken, 0, 100)
	resp := map[string]any{"projects": page}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	cbWriteJSON(w, http.StatusOK, resp)
}

func handleCBUpdateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Source      map[string]any `json:"source"`
		Artifacts   map[string]any `json:"artifacts"`
		Environment map[string]any `json:"environment"`
		ServiceRole string         `json:"serviceRole"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	p, ok := cbProjects.Get(req.Name)
	if !ok {
		cbWriteError(w, "ResourceNotFoundException", "Project not found: "+req.Name)
		return
	}
	if req.Description != "" {
		p.Description = req.Description
	}
	if req.Source != nil {
		p.Source = req.Source
	}
	if req.Artifacts != nil {
		p.Artifacts = req.Artifacts
	}
	if req.Environment != nil {
		p.Environment = req.Environment
	}
	if req.ServiceRole != "" {
		p.ServiceRole = req.ServiceRole
	}
	p.LastModified = cbEpochNow()
	cbProjects.Put(req.Name, p)
	cbWriteJSON(w, http.StatusOK, map[string]any{"project": p})
}

func handleCBDeleteProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	cbProjects.Delete(req.Name)
	cbWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCBStartBuild(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectName                  string           `json:"projectName"`
		BuildspecOverride            string           `json:"buildspecOverride"`
		EnvironmentVariablesOverride []map[string]any `json:"environmentVariablesOverride"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	p, ok := cbProjects.Get(req.ProjectName)
	if !ok {
		cbWriteError(w, "ResourceNotFoundException", "Project not found: "+req.ProjectName)
		return
	}
	commands, err := cbBuildCommands(p, req.BuildspecOverride)
	if err != nil {
		cbWriteError(w, "InvalidInputException", err.Error())
		return
	}
	reportGroups := cbBuildReportGroups(p, req.BuildspecOverride)

	cbMu.Lock()
	defer cbMu.Unlock()

	buildID := req.ProjectName + ":" + uuid.New().String()
	now := cbEpochNow()
	build := CBBuild{
		ID:           buildID,
		Arn:          cbARN("build/" + buildID),
		ProjectName:  req.ProjectName,
		BuildStatus:  "IN_PROGRESS",
		Seq:          cbNextBuildSeq(),
		StartTime:    now,
		EndTime:      now,
		Environment:  p.Environment,
		ReportGroups: reportGroups,
		Phases: []CBPhase{
			{PhaseType: "SUBMITTED", PhaseStatus: "SUCCEEDED", StartTime: now, EndTime: now, DurationInSeconds: 0},
			{PhaseType: "BUILD", PhaseStatus: "IN_PROGRESS", StartTime: now},
		},
		Logs: cbLogsLocation(),
	}
	cbBuilds.Put(buildID, build)
	go cbRunBuild(buildID, commands, cbEnvironment(p.Environment, req.EnvironmentVariablesOverride))
	cbWriteJSON(w, http.StatusOK, map[string]any{"build": build})
}

func handleCBStopBuild(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	build, ok := cbBuilds.Get(req.ID)
	if !ok {
		cbWriteError(w, "ResourceNotFoundException", "Build not found: "+req.ID)
		return
	}
	build = cbStopBuildByID(req.ID)
	cbWriteJSON(w, http.StatusOK, map[string]any{"build": build})
}

func cbStopBuildByID(buildID string) CBBuild {
	cbMu.Lock()
	defer cbMu.Unlock()
	build, ok := cbBuilds.Get(buildID)
	if !ok {
		return CBBuild{}
	}
	// StopBuild transitions a running build to STOPPED. A build that already
	// settled keeps its terminal status (real CodeBuild is idempotent here).
	if build.BuildStatus == "IN_PROGRESS" {
		now := cbEpochNow()
		build.BuildStatus = "STOPPED"
		build.EndTime = now
		build.Phases = []CBPhase{
			{PhaseType: "SUBMITTED", PhaseStatus: "SUCCEEDED", StartTime: build.StartTime, EndTime: build.StartTime, DurationInSeconds: 0},
			{PhaseType: "BUILD", PhaseStatus: "STOPPED", StartTime: build.StartTime, EndTime: now, DurationInSeconds: now - build.StartTime},
			{PhaseType: "COMPLETED", PhaseStatus: "STOPPED", StartTime: now, EndTime: now, DurationInSeconds: 0},
		}
		cbBuilds.Put(buildID, build)
	}
	return build
}

func handleCBRetryBuild(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	prior, ok := cbBuilds.Get(req.ID)
	if !ok {
		cbWriteError(w, "ResourceNotFoundException", "Build not found: "+req.ID)
		return
	}
	p, ok := cbProjects.Get(prior.ProjectName)
	if !ok {
		cbWriteError(w, "ResourceNotFoundException", "Project not found: "+prior.ProjectName)
		return
	}
	commands, err := cbBuildCommands(p, "")
	if err != nil {
		cbWriteError(w, "InvalidInputException", err.Error())
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	// RetryBuild starts a fresh build of the same project (a new build id),
	// mirroring real CodeBuild which produces a new build resource.
	buildID := prior.ProjectName + ":" + uuid.New().String()
	now := cbEpochNow()
	build := CBBuild{
		ID:          buildID,
		Arn:         cbARN("build/" + buildID),
		ProjectName: prior.ProjectName,
		BuildStatus: "IN_PROGRESS",
		Seq:         cbNextBuildSeq(),
		StartTime:   now,
		EndTime:     now,
		Environment: p.Environment,
		Phases: []CBPhase{
			{PhaseType: "SUBMITTED", PhaseStatus: "SUCCEEDED", StartTime: now, EndTime: now, DurationInSeconds: 0},
			{PhaseType: "BUILD", PhaseStatus: "IN_PROGRESS", StartTime: now},
		},
		Logs: cbLogsLocation(),
	}
	cbBuilds.Put(buildID, build)
	go cbRunBuild(buildID, commands, cbEnvironment(p.Environment, nil))
	cbWriteJSON(w, http.StatusOK, map[string]any{"build": build})
}

type cbBuildspec struct {
	Phases map[string]struct {
		Commands []string `yaml:"commands"`
	} `yaml:"phases"`
	// Reports maps a report-group name to its report config; a build with a
	// reports section produces a Report per group, exactly like real CodeBuild.
	Reports map[string]struct {
		Files []string `yaml:"files"`
	} `yaml:"reports"`
}

// cbBuildReportGroups returns the report-group names a project's buildspec
// references via its reports section (empty if none).
func cbBuildReportGroups(p CBProject, override string) []string {
	buildspec := override
	if buildspec == "" {
		buildspec = cbString(p.Source["buildspec"])
	}
	if buildspec == "" {
		return nil
	}
	var spec cbBuildspec
	if err := yaml.Unmarshal([]byte(buildspec), &spec); err != nil {
		return nil
	}
	groups := make([]string, 0, len(spec.Reports))
	for name := range spec.Reports {
		groups = append(groups, name)
	}
	sort.Strings(groups)
	return groups
}

func cbBuildCommands(p CBProject, override string) ([]string, error) {
	buildspec := override
	if buildspec == "" {
		buildspec = cbString(p.Source["buildspec"])
	}
	if buildspec == "" {
		return nil, fmt.Errorf("source.buildspec is required for NO_SOURCE builds")
	}
	var spec cbBuildspec
	if err := yaml.Unmarshal([]byte(buildspec), &spec); err != nil {
		return nil, fmt.Errorf("invalid buildspec: %w", err)
	}
	var commands []string
	for _, phase := range []string{"install", "pre_build", "build", "post_build"} {
		commands = append(commands, spec.Phases[phase].Commands...)
	}
	if len(commands) == 0 {
		return nil, fmt.Errorf("buildspec must contain at least one command")
	}
	return commands, nil
}

func cbRunBuild(buildID string, commands []string, env map[string]string) {
	exitCode, reason := cbRunCommands(commands, env)
	cbCompleteBuild(buildID, exitCode, reason)
}

func cbRunCommands(commands []string, env map[string]string) (int, string) {
	workDir, err := os.MkdirTemp("", "sockerless-codebuild-*")
	if err != nil {
		return -1, err.Error()
	}
	defer os.RemoveAll(workDir)

	exitCode := 0
	var reason string
	for _, command := range commands {
		handle := sim.StartProcess(sim.ProcessConfig{
			Command: []string{"/bin/sh", "-c", command},
			Dir:     filepath.Clean(workDir),
			Env:     env,
		}, sim.NoopSink{})
		result := handle.Wait()
		if result.Error != nil {
			exitCode = -1
			reason = result.Error.Error()
			break
		}
		if result.ExitCode != 0 {
			exitCode = result.ExitCode
			reason = fmt.Sprintf("Build command exited with status %d", result.ExitCode)
			break
		}
	}
	return exitCode, reason
}

func cbCompleteBuild(buildID string, exitCode int, reason string) {
	cbMu.Lock()
	defer cbMu.Unlock()

	build, ok := cbBuilds.Get(buildID)
	if !ok {
		return
	}
	now := cbEpochNow()
	status := "SUCCEEDED"
	if exitCode != 0 {
		status = "FAILED"
	}
	build.BuildStatus = status
	build.EndTime = now
	buildPhase := CBPhase{PhaseType: "BUILD", PhaseStatus: status, StartTime: build.StartTime, EndTime: now, DurationInSeconds: now - build.StartTime}
	if reason != "" {
		// Failure detail rides the phase's contexts (the real Build
		// shape's per-phase PhaseContext), not the LogsLocation.
		// COMMAND_EXECUTION_ERROR is real CodeBuild's status code for a
		// failed buildspec command.
		buildPhase.Contexts = []CBPhaseContext{{
			StatusCode: "COMMAND_EXECUTION_ERROR",
			Message:    reason,
		}}
	}
	build.Phases = []CBPhase{
		{PhaseType: "SUBMITTED", PhaseStatus: "SUCCEEDED", StartTime: build.StartTime, EndTime: build.StartTime, DurationInSeconds: 0},
		buildPhase,
		{PhaseType: "COMPLETED", PhaseStatus: status, StartTime: now, EndTime: now, DurationInSeconds: 0},
	}

	// A build whose buildspec references report groups produces one Report per
	// group, carrying the build's terminal status — the real CodeBuild flow.
	reportStatus := "SUCCEEDED"
	if status != "SUCCEEDED" {
		reportStatus = "FAILED"
	}
	for _, groupName := range build.ReportGroups {
		groupArn := cbARN("report-group/" + groupName)
		rg, ok := cbReportGrps.Get(groupArn)
		if !ok {
			continue
		}
		execID := strings.SplitN(build.ID, ":", 2)
		reportName := build.ID
		if len(execID) == 2 {
			reportName = execID[1]
		}
		reportArn := cbARN("report/" + groupName + ":" + reportName)
		report := CBReport{
			Arn:            reportArn,
			Type:           rg.Type,
			Name:           reportName,
			ReportGroupArn: groupArn,
			ExecutionId:    build.Arn,
			Status:         reportStatus,
			Created:        now,
			ExportConfig:   rg.ExportConfig,
			Truncated:      false,
		}
		cbReports.Put(reportArn, report)
		build.ReportArns = append(build.ReportArns, reportArn)
	}
	cbBuilds.Put(buildID, build)
}

func handleCBBatchGetBuilds(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	var found []CBBuild
	var notFound []string
	for _, id := range req.IDs {
		if b, ok := cbBuilds.Get(id); ok {
			found = append(found, b)
		} else {
			notFound = append(notFound, id)
		}
	}
	if found == nil {
		found = []CBBuild{}
	}
	if notFound == nil {
		notFound = []string{}
	}
	cbWriteJSON(w, http.StatusOK, map[string]any{
		"builds":         found,
		"buildsNotFound": notFound,
	})
}

func handleCBListBuildsForProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectName string `json:"projectName"`
		SortOrder   string `json:"sortOrder"`
		NextToken   string `json:"nextToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	all := cbBuilds.List()
	var builds []CBBuild
	for _, b := range all {
		if b.ProjectName == req.ProjectName {
			builds = append(builds, b)
		}
	}
	ids := cbSortBuildIDs(builds, req.SortOrder)
	page, nextTok := awsPage(ids, req.NextToken, 0, 100)
	resp := map[string]any{"ids": page}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	cbWriteJSON(w, http.StatusOK, resp)
}

func handleCBListBuilds(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SortOrder string `json:"sortOrder"`
		NextToken string `json:"nextToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	all := cbBuilds.List()
	ids := cbSortBuildIDs(all, req.SortOrder)
	page, nextTok := awsPage(ids, req.NextToken, 0, 100)
	resp := map[string]any{"ids": page}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	cbWriteJSON(w, http.StatusOK, resp)
}

// cbSortBuildIDs orders builds by start time and returns their IDs. AWS
// ListBuilds / ListBuildsForProject default to DESCENDING (most-recent first);
// sortOrder="ASCENDING" reverses it. Ties break on ID for a stable page cursor.
func cbSortBuildIDs(builds []CBBuild, sortOrder string) []string {
	ascending := strings.EqualFold(sortOrder, "ASCENDING")
	sort.Slice(builds, func(i, j int) bool {
		if ascending {
			return builds[i].Seq < builds[j].Seq
		}
		return builds[i].Seq > builds[j].Seq
	})
	ids := make([]string, 0, len(builds))
	for _, b := range builds {
		ids = append(ids, b.ID)
	}
	return ids
}

// --- Report groups ---

func handleCBCreateReportGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string         `json:"name"`
		Type         string         `json:"type"`
		ExportConfig map[string]any `json:"exportConfig"`
		Tags         []CBTag        `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.Name == "" {
		cbWriteError(w, "InvalidInputException", "name is required")
		return
	}
	if req.Type == "" {
		cbWriteError(w, "InvalidInputException", "type is required")
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	arn := cbARN("report-group/" + req.Name)
	if _, ok := cbReportGrps.Get(arn); ok {
		cbWriteError(w, "ResourceAlreadyExistsException", "Report group already exists: "+req.Name)
		return
	}
	now := cbEpochNow()
	rg := CBReportGroup{
		Arn:          arn,
		Name:         req.Name,
		Type:         req.Type,
		ExportConfig: req.ExportConfig,
		Created:      now,
		LastModified: now,
		Status:       "ACTIVE",
		Tags:         req.Tags,
	}
	cbReportGrps.Put(arn, rg)
	cbWriteJSON(w, http.StatusOK, map[string]any{"reportGroup": rg})
}

func handleCBUpdateReportGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Arn          string         `json:"arn"`
		ExportConfig map[string]any `json:"exportConfig"`
		Tags         []CBTag        `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	rg, ok := cbReportGrps.Get(req.Arn)
	if !ok {
		cbWriteError(w, "ResourceNotFoundException", "Report group not found: "+req.Arn)
		return
	}
	if req.ExportConfig != nil {
		rg.ExportConfig = req.ExportConfig
	}
	if req.Tags != nil {
		rg.Tags = req.Tags
	}
	rg.LastModified = cbEpochNow()
	cbReportGrps.Put(req.Arn, rg)
	cbWriteJSON(w, http.StatusOK, map[string]any{"reportGroup": rg})
}

func handleCBDeleteReportGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Arn           string `json:"arn"`
		DeleteReports bool   `json:"deleteReports"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	if req.DeleteReports {
		for _, rep := range cbReports.List() {
			if rep.ReportGroupArn == req.Arn {
				cbReports.Delete(rep.Arn)
			}
		}
	}
	cbReportGrps.Delete(req.Arn)
	cbWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCBListReportGroups(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SortOrder  string `json:"sortOrder"`
		MaxResults int    `json:"maxResults"`
		NextToken  string `json:"nextToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	all := cbReportGrps.List()
	arns := make([]string, 0, len(all))
	for _, rg := range all {
		arns = append(arns, rg.Arn)
	}
	sort.Strings(arns)
	if strings.EqualFold(req.SortOrder, "DESCENDING") {
		reverseStrings(arns)
	}
	page, nextTok := awsPage(arns, req.NextToken, req.MaxResults, 100)
	resp := map[string]any{"reportGroups": page}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	cbWriteJSON(w, http.StatusOK, resp)
}

func handleCBBatchGetReportGroups(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReportGroupArns []string `json:"reportGroupArns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	var found []CBReportGroup
	var notFound []string
	for _, arn := range req.ReportGroupArns {
		if rg, ok := cbReportGrps.Get(arn); ok {
			found = append(found, rg)
		} else {
			notFound = append(notFound, arn)
		}
	}
	if found == nil {
		found = []CBReportGroup{}
	}
	if notFound == nil {
		notFound = []string{}
	}
	cbWriteJSON(w, http.StatusOK, map[string]any{
		"reportGroups":         found,
		"reportGroupsNotFound": notFound,
	})
}

// --- Reports ---

func handleCBListReports(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SortOrder  string `json:"sortOrder"`
		MaxResults int    `json:"maxResults"`
		NextToken  string `json:"nextToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	cbWriteReportArnsPage(w, cbReports.List(), "", req.SortOrder, req.MaxResults, req.NextToken)
}

func handleCBListReportsForReportGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReportGroupArn string `json:"reportGroupArn"`
		SortOrder      string `json:"sortOrder"`
		MaxResults     int    `json:"maxResults"`
		NextToken      string `json:"nextToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.ReportGroupArn == "" {
		cbWriteError(w, "InvalidInputException", "reportGroupArn is required")
		return
	}
	cbWriteReportArnsPage(w, cbReports.List(), req.ReportGroupArn, req.SortOrder, req.MaxResults, req.NextToken)
}

// cbWriteReportArnsPage filters reports (optionally by group), sorts by creation
// order, and writes a paged {reports:[arns],nextToken}. ListReports defaults to
// DESCENDING (most-recent first), matching real CodeBuild.
func cbWriteReportArnsPage(w http.ResponseWriter, all []CBReport, groupArn, sortOrder string, maxResults int, nextToken string) {
	var reports []CBReport
	for _, rep := range all {
		if groupArn == "" || rep.ReportGroupArn == groupArn {
			reports = append(reports, rep)
		}
	}
	ascending := strings.EqualFold(sortOrder, "ASCENDING")
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].Created == reports[j].Created {
			if ascending {
				return reports[i].Arn < reports[j].Arn
			}
			return reports[i].Arn > reports[j].Arn
		}
		if ascending {
			return reports[i].Created < reports[j].Created
		}
		return reports[i].Created > reports[j].Created
	})
	arns := make([]string, 0, len(reports))
	for _, rep := range reports {
		arns = append(arns, rep.Arn)
	}
	page, nextTok := awsPage(arns, nextToken, maxResults, 100)
	resp := map[string]any{"reports": page}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	cbWriteJSON(w, http.StatusOK, resp)
}

func handleCBBatchGetReports(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReportArns []string `json:"reportArns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	var found []CBReport
	var notFound []string
	for _, arn := range req.ReportArns {
		if rep, ok := cbReports.Get(arn); ok {
			found = append(found, rep)
		} else {
			notFound = append(notFound, arn)
		}
	}
	if found == nil {
		found = []CBReport{}
	}
	if notFound == nil {
		notFound = []string{}
	}
	cbWriteJSON(w, http.StatusOK, map[string]any{
		"reports":         found,
		"reportsNotFound": notFound,
	})
}

// --- Source credentials ---

func handleCBImportSourceCredentials(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token           string `json:"token"`
		Username        string `json:"username"`
		ServerType      string `json:"serverType"`
		AuthType        string `json:"authType"`
		ShouldOverwrite *bool  `json:"shouldOverwrite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.ServerType == "" || req.AuthType == "" {
		cbWriteError(w, "InvalidInputException", "serverType and authType are required")
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	// Real CodeBuild keys a source credential by (serverType, authType) per
	// region/account, so the ARN is deterministic from the server type.
	arn := cbARN("token/" + strings.ToLower(req.ServerType))
	overwrite := req.ShouldOverwrite == nil || *req.ShouldOverwrite
	if _, ok := cbSourceCreds.Get(arn); ok && !overwrite {
		cbWriteError(w, "ResourceAlreadyExistsException", "Source credentials already exist for server type: "+req.ServerType)
		return
	}
	// For SECRETS_MANAGER auth, the token is a secret ARN echoed back as resource.
	resource := ""
	if strings.EqualFold(req.AuthType, "SECRETS_MANAGER") {
		resource = req.Token
	}
	cred := CBSourceCredential{
		Arn:        arn,
		ServerType: req.ServerType,
		AuthType:   req.AuthType,
		Resource:   resource,
	}
	cbSourceCreds.Put(arn, cred)
	cbWriteJSON(w, http.StatusOK, map[string]any{"arn": arn})
}

func handleCBListSourceCredentials(w http.ResponseWriter, r *http.Request) {
	all := cbSourceCreds.List()
	sort.Slice(all, func(i, j int) bool { return all[i].Arn < all[j].Arn })
	if all == nil {
		all = []CBSourceCredential{}
	}
	cbWriteJSON(w, http.StatusOK, map[string]any{"sourceCredentialsInfos": all})
}

func handleCBDeleteSourceCredentials(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Arn string `json:"arn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	if _, ok := cbSourceCreds.Get(req.Arn); !ok {
		cbWriteError(w, "ResourceNotFoundException", "Source credentials not found: "+req.Arn)
		return
	}
	cbSourceCreds.Delete(req.Arn)
	cbWriteJSON(w, http.StatusOK, map[string]any{"arn": req.Arn})
}

func reverseStrings(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func cbNameFromARN(arn string) string {
	// arn:aws:codebuild:us-east-1:123456789012:project/name
	parts := strings.Split(arn, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return arn
}

func cbString(v any) string {
	s, _ := v.(string)
	return s
}

func cbEnvironment(projectEnv map[string]any, overrides []map[string]any) map[string]string {
	env := map[string]string{
		"PATH": os.Getenv("PATH"),
		"HOME": os.Getenv("HOME"),
	}
	for k, v := range cbEnvironmentValues(projectEnv["environmentVariables"]) {
		env[k] = v
	}
	for _, item := range overrides {
		name := cbString(item["name"])
		if name == "" {
			name = cbString(item["Name"])
		}
		if name == "" {
			continue
		}
		value := cbString(item["value"])
		if value == "" {
			value = cbString(item["Value"])
		}
		env[name] = value
	}
	return env
}

func cbEnvironmentValues(v any) map[string]string {
	env := map[string]string{}
	values, ok := v.([]any)
	if !ok {
		return env
	}
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		name := cbString(item["name"])
		if name == "" {
			name = cbString(item["Name"])
		}
		if name == "" {
			continue
		}
		value := cbString(item["value"])
		if value == "" {
			value = cbString(item["Value"])
		}
		env[name] = value
	}
	return env
}
