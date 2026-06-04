package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sim "github.com/sockerless/simulator"
)

// AWS CodeBuild — AWS JSON 1.1 protocol (X-Amz-Target: CodeBuild_20161006.<Op>).
// Builds complete immediately with SUCCEEDED status.

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
	ID                           string           `json:"id"`
	Arn                          string           `json:"arn"`
	ProjectName                  string           `json:"projectName"`
	BuildStatus                  string           `json:"buildStatus"`
	StartTime                    float64          `json:"startTime"`
	EndTime                      float64          `json:"endTime"`
	Phases                       []CBPhase        `json:"phases"`
	Logs                         map[string]any   `json:"logs"`
	Environment                  map[string]any   `json:"environment,omitempty"`
	EnvironmentVariablesOverride []map[string]any `json:"environmentVariablesOverride,omitempty"`
}

type CBPhase struct {
	PhaseType         string  `json:"phaseType"`
	PhaseStatus       string  `json:"phaseStatus"`
	StartTime         float64 `json:"startTime"`
	EndTime           float64 `json:"endTime"`
	DurationInSeconds float64 `json:"durationInSeconds"`
}

type CBTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var (
	cbProjects sim.Store[CBProject]
	cbBuilds   sim.Store[CBBuild]
	cbMu       sync.Mutex
)

func registerCodeBuild(r *sim.AWSRouter, srv *sim.Server) {
	cbProjects = sim.MakeStore[CBProject](srv.DB(), "codebuild_projects")
	cbBuilds = sim.MakeStore[CBBuild](srv.DB(), "codebuild_builds")

	r.Register("CodeBuild_20161006.CreateProject", handleCBCreateProject)
	r.Register("CodeBuild_20161006.BatchGetProjects", handleCBBatchGetProjects)
	r.Register("CodeBuild_20161006.ListProjects", handleCBListProjects)
	r.Register("CodeBuild_20161006.UpdateProject", handleCBUpdateProject)
	r.Register("CodeBuild_20161006.DeleteProject", handleCBDeleteProject)
	r.Register("CodeBuild_20161006.StartBuild", handleCBStartBuild)
	r.Register("CodeBuild_20161006.BatchGetBuilds", handleCBBatchGetBuilds)
	r.Register("CodeBuild_20161006.ListBuildsForProject", handleCBListBuildsForProject)
	r.Register("CodeBuild_20161006.ListBuilds", handleCBListBuilds)
	r.Register("CodeBuild_20161006.TagResource", handleCBTagResource)
	r.Register("CodeBuild_20161006.UntagResource", handleCBUntagResource)
	r.Register("CodeBuild_20161006.ListTagsForResource", handleCBListTagsForResource)
}

func cbARN(resource string) string {
	return fmt.Sprintf("arn:aws:codebuild:us-east-1:123456789012:%s", resource)
}

func cbEpochNow() float64 {
	return float64(time.Now().UTC().Unix())
}

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
	for _, name := range req.Names {
		if p, ok := cbProjects.Get(name); ok {
			found = append(found, p)
		} else {
			notFound = append(notFound, name)
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
	_ = json.NewDecoder(r.Body).Decode(&req)

	all := cbProjects.List()
	names := make([]string, 0, len(all))
	for _, p := range all {
		names = append(names, p.Name)
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

	cbMu.Lock()
	defer cbMu.Unlock()

	buildID := req.ProjectName + ":" + uuid.New().String()
	now := cbEpochNow()
	build := CBBuild{
		ID:          buildID,
		Arn:         cbARN("build/" + buildID),
		ProjectName: req.ProjectName,
		BuildStatus: "SUCCEEDED",
		StartTime:   now,
		EndTime:     now,
		Environment: p.Environment,
		Phases: []CBPhase{
			{PhaseType: "SUBMITTED", PhaseStatus: "SUCCEEDED", StartTime: now, EndTime: now, DurationInSeconds: 0},
			{PhaseType: "INSTALL", PhaseStatus: "SUCCEEDED", StartTime: now, EndTime: now, DurationInSeconds: 0},
			{PhaseType: "BUILD", PhaseStatus: "SUCCEEDED", StartTime: now, EndTime: now, DurationInSeconds: 0},
			{PhaseType: "COMPLETED", PhaseStatus: "SUCCEEDED", StartTime: now, EndTime: now, DurationInSeconds: 0},
		},
		Logs:                         map[string]any{"enabled": false},
		EnvironmentVariablesOverride: req.EnvironmentVariablesOverride,
	}
	cbBuilds.Put(buildID, build)
	cbWriteJSON(w, http.StatusOK, map[string]any{"build": build})
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
	var ids []string
	for _, b := range all {
		if b.ProjectName == req.ProjectName {
			ids = append(ids, b.ID)
		}
	}
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
	_ = json.NewDecoder(r.Body).Decode(&req)

	all := cbBuilds.List()
	ids := make([]string, 0, len(all))
	for _, b := range all {
		ids = append(ids, b.ID)
	}
	page, nextTok := awsPage(ids, req.NextToken, 0, 100)
	resp := map[string]any{"ids": page}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	cbWriteJSON(w, http.StatusOK, resp)
}

func handleCBTagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string  `json:"resourceArn"`
		Tags        []CBTag `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	name := cbNameFromARN(req.ResourceArn)
	p, ok := cbProjects.Get(name)
	if !ok {
		cbWriteError(w, "ResourceNotFoundException", "Resource not found: "+req.ResourceArn)
		return
	}
	tagMap := cbTagsToMap(p.Tags)
	for _, t := range req.Tags {
		tagMap[t.Key] = t.Value
	}
	p.Tags = cbMapToTags(tagMap)
	cbProjects.Put(name, p)
	cbWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCBUntagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string   `json:"resourceArn"`
		TagKeys     []string `json:"tagKeys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	name := cbNameFromARN(req.ResourceArn)
	p, ok := cbProjects.Get(name)
	if !ok {
		cbWriteError(w, "ResourceNotFoundException", "Resource not found: "+req.ResourceArn)
		return
	}
	tagMap := cbTagsToMap(p.Tags)
	for _, k := range req.TagKeys {
		delete(tagMap, k)
	}
	p.Tags = cbMapToTags(tagMap)
	cbProjects.Put(name, p)
	cbWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCBListTagsForResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string `json:"resourceArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	name := cbNameFromARN(req.ResourceArn)
	p, ok := cbProjects.Get(name)
	if !ok {
		cbWriteError(w, "ResourceNotFoundException", "Resource not found: "+req.ResourceArn)
		return
	}
	tags := p.Tags
	if tags == nil {
		tags = []CBTag{}
	}
	cbWriteJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func cbNameFromARN(arn string) string {
	// arn:aws:codebuild:us-east-1:123456789012:project/name
	parts := strings.Split(arn, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return arn
}

func cbTagsToMap(tags []CBTag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[t.Key] = t.Value
	}
	return m
}

func cbMapToTags(m map[string]string) []CBTag {
	tags := make([]CBTag, 0, len(m))
	for k, v := range m {
		tags = append(tags, CBTag{Key: k, Value: v})
	}
	return tags
}
