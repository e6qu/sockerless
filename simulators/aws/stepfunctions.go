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

// AWS Step Functions — AWS JSON 1.0 protocol (X-Amz-Target: AWSStepFunctions.<Op>).
// Executions complete immediately with SUCCEEDED status (no actual execution engine).

type SFNStateMachine struct {
	StateMachineArn string   `json:"stateMachineArn"`
	Name            string   `json:"name"`
	Definition      string   `json:"definition"`
	RoleArn         string   `json:"roleArn"`
	Type            string   `json:"type"`
	Status          string   `json:"status"`
	CreationDate    float64  `json:"creationDate"`
	Tags            []SFNTag `json:"tags,omitempty"`
}

type SFNExecution struct {
	ExecutionArn    string   `json:"executionArn"`
	StateMachineArn string   `json:"stateMachineArn"`
	Name            string   `json:"name"`
	Status          string   `json:"status"`
	StartDate       float64  `json:"startDate"`
	StopDate        *float64 `json:"stopDate,omitempty"`
	Input           string   `json:"input,omitempty"`
	Output          string   `json:"output,omitempty"`
}

type SFNTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var (
	sfnStateMachines sim.Store[SFNStateMachine]
	sfnExecutions    sim.Store[SFNExecution]
	sfnMu            sync.Mutex
)

func registerStepFunctions(r *sim.AWSRouter, srv *sim.Server) {
	sfnStateMachines = sim.MakeStore[SFNStateMachine](srv.DB(), "sfn_state_machines")
	sfnExecutions = sim.MakeStore[SFNExecution](srv.DB(), "sfn_executions")

	r.Register("AWSStepFunctions.CreateStateMachine", handleSFNCreateStateMachine)
	r.Register("AWSStepFunctions.DescribeStateMachine", handleSFNDescribeStateMachine)
	r.Register("AWSStepFunctions.ListStateMachines", handleSFNListStateMachines)
	r.Register("AWSStepFunctions.DeleteStateMachine", handleSFNDeleteStateMachine)
	r.Register("AWSStepFunctions.UpdateStateMachine", handleSFNUpdateStateMachine)
	r.Register("AWSStepFunctions.TagResource", handleSFNTagResource)
	r.Register("AWSStepFunctions.UntagResource", handleSFNUntagResource)
	r.Register("AWSStepFunctions.ListTagsForResource", handleSFNListTagsForResource)
	r.Register("AWSStepFunctions.ValidateStateMachineDefinition", handleSFNValidateStateMachineDefinition)
	r.Register("AWSStepFunctions.StartExecution", handleSFNStartExecution)
	r.Register("AWSStepFunctions.DescribeExecution", handleSFNDescribeExecution)
	r.Register("AWSStepFunctions.ListExecutions", handleSFNListExecutions)
	r.Register("AWSStepFunctions.StopExecution", handleSFNStopExecution)
}

func sfnARN(resource string) string {
	return fmt.Sprintf("arn:aws:states:us-east-1:123456789012:%s", resource)
}

func sfnEpochNow() float64 {
	return float64(time.Now().UTC().Unix())
}

func sfnWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func sfnWriteError(w http.ResponseWriter, code string, msg string) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	w.Header().Set("X-Amzn-Errortype", code)
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"__type":  code,
		"message": msg,
	})
}

func handleSFNCreateStateMachine(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string   `json:"name"`
		Definition string   `json:"definition"`
		RoleArn    string   `json:"roleArn"`
		Type       string   `json:"type"`
		Tags       []SFNTag `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	if req.Name == "" {
		sfnWriteError(w, "InvalidName", "name is required")
		return
	}

	sfnMu.Lock()
	defer sfnMu.Unlock()

	if _, ok := sfnStateMachines.Get(req.Name); ok {
		sfnWriteError(w, "StateMachineAlreadyExists", "State machine already exists: "+req.Name)
		return
	}

	smType := req.Type
	if smType == "" {
		smType = "STANDARD"
	}
	arn := sfnARN("stateMachine:" + req.Name)
	sm := SFNStateMachine{
		StateMachineArn: arn,
		Name:            req.Name,
		Definition:      req.Definition,
		RoleArn:         req.RoleArn,
		Type:            smType,
		Status:          "ACTIVE",
		CreationDate:    sfnEpochNow(),
		Tags:            req.Tags,
	}
	sfnStateMachines.Put(req.Name, sm)
	sfnWriteJSON(w, http.StatusOK, map[string]any{
		"stateMachineArn": arn,
		"creationDate":    sm.CreationDate,
	})
}

func handleSFNDescribeStateMachine(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StateMachineArn string `json:"stateMachineArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}

	name := sfnNameFromARN(req.StateMachineArn)
	sm, ok := sfnStateMachines.Get(name)
	if !ok {
		sfnWriteError(w, "StateMachineDoesNotExist", "State machine does not exist: "+req.StateMachineArn)
		return
	}
	sfnWriteJSON(w, http.StatusOK, sm)
}

func handleSFNListStateMachines(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxResults *int   `json:"maxResults"`
		NextToken  string `json:"nextToken"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	all := sfnStateMachines.List()
	maxR := 0
	if req.MaxResults != nil {
		maxR = *req.MaxResults
	}
	page, nextTok := awsPage(all, req.NextToken, maxR, 100)

	items := make([]map[string]any, 0, len(page))
	for _, sm := range page {
		items = append(items, map[string]any{
			"stateMachineArn": sm.StateMachineArn,
			"name":            sm.Name,
			"type":            sm.Type,
			"creationDate":    sm.CreationDate,
		})
	}
	resp := map[string]any{"stateMachines": items}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	sfnWriteJSON(w, http.StatusOK, resp)
}

func handleSFNDeleteStateMachine(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StateMachineArn string `json:"stateMachineArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}

	sfnMu.Lock()
	defer sfnMu.Unlock()

	name := sfnNameFromARN(req.StateMachineArn)
	sfnStateMachines.Delete(name)
	sfnWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleSFNUpdateStateMachine(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StateMachineArn string `json:"stateMachineArn"`
		Definition      string `json:"definition"`
		RoleArn         string `json:"roleArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}

	sfnMu.Lock()
	defer sfnMu.Unlock()

	name := sfnNameFromARN(req.StateMachineArn)
	sm, ok := sfnStateMachines.Get(name)
	if !ok {
		sfnWriteError(w, "StateMachineDoesNotExist", "State machine does not exist: "+req.StateMachineArn)
		return
	}
	if req.Definition != "" {
		sm.Definition = req.Definition
	}
	if req.RoleArn != "" {
		sm.RoleArn = req.RoleArn
	}
	sfnStateMachines.Put(name, sm)
	sfnWriteJSON(w, http.StatusOK, map[string]any{
		"updateDate": sfnEpochNow(),
	})
}

func handleSFNTagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string   `json:"resourceArn"`
		Tags        []SFNTag `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}

	sfnMu.Lock()
	defer sfnMu.Unlock()

	name := sfnNameFromARN(req.ResourceArn)
	sm, ok := sfnStateMachines.Get(name)
	if !ok {
		sfnWriteError(w, "ResourceNotFound", "Resource not found: "+req.ResourceArn)
		return
	}
	tagMap := sfnTagsToMap(sm.Tags)
	for _, t := range req.Tags {
		tagMap[t.Key] = t.Value
	}
	sm.Tags = sfnMapToTags(tagMap)
	sfnStateMachines.Put(name, sm)
	sfnWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleSFNUntagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string   `json:"resourceArn"`
		TagKeys     []string `json:"tagKeys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}

	sfnMu.Lock()
	defer sfnMu.Unlock()

	name := sfnNameFromARN(req.ResourceArn)
	sm, ok := sfnStateMachines.Get(name)
	if !ok {
		sfnWriteError(w, "ResourceNotFound", "Resource not found: "+req.ResourceArn)
		return
	}
	tagMap := sfnTagsToMap(sm.Tags)
	for _, k := range req.TagKeys {
		delete(tagMap, k)
	}
	sm.Tags = sfnMapToTags(tagMap)
	sfnStateMachines.Put(name, sm)
	sfnWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleSFNListTagsForResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceArn string `json:"resourceArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}

	name := sfnNameFromARN(req.ResourceArn)
	sm, ok := sfnStateMachines.Get(name)
	if !ok {
		sfnWriteError(w, "ResourceNotFound", "Resource not found: "+req.ResourceArn)
		return
	}
	tags := sm.Tags
	if tags == nil {
		tags = []SFNTag{}
	}
	sfnWriteJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func handleSFNValidateStateMachineDefinition(w http.ResponseWriter, r *http.Request) {
	// TF provider calls this before CreateStateMachine. Always valid in sim.
	sfnWriteJSON(w, http.StatusOK, map[string]any{
		"result": "OK",
	})
}

func handleSFNStartExecution(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StateMachineArn string `json:"stateMachineArn"`
		Name            string `json:"name"`
		Input           string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}

	name := sfnNameFromARN(req.StateMachineArn)
	if _, ok := sfnStateMachines.Get(name); !ok {
		sfnWriteError(w, "StateMachineDoesNotExist", "State machine does not exist: "+req.StateMachineArn)
		return
	}

	execName := req.Name
	if execName == "" {
		execName = uuid.New().String()
	}
	execARN := sfnARN("execution:" + name + ":" + execName)

	sfnMu.Lock()
	defer sfnMu.Unlock()

	if _, ok := sfnExecutions.Get(execARN); ok {
		sfnWriteError(w, "ExecutionAlreadyExists", "Execution already exists: "+execARN)
		return
	}

	now := sfnEpochNow()
	stopDate := now
	input := req.Input
	if input == "" {
		input = "{}"
	}
	exec := SFNExecution{
		ExecutionArn:    execARN,
		StateMachineArn: req.StateMachineArn,
		Name:            execName,
		Status:          "SUCCEEDED",
		StartDate:       now,
		StopDate:        &stopDate,
		Input:           input,
		Output:          input,
	}
	sfnExecutions.Put(execARN, exec)
	sfnWriteJSON(w, http.StatusOK, map[string]any{
		"executionArn": execARN,
		"startDate":    now,
	})
}

func handleSFNDescribeExecution(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExecutionArn string `json:"executionArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}

	exec, ok := sfnExecutions.Get(req.ExecutionArn)
	if !ok {
		sfnWriteError(w, "ExecutionDoesNotExist", "Execution does not exist: "+req.ExecutionArn)
		return
	}
	sfnWriteJSON(w, http.StatusOK, exec)
}

func handleSFNListExecutions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StateMachineArn string `json:"stateMachineArn"`
		StatusFilter    string `json:"statusFilter"`
		MaxResults      *int   `json:"maxResults"`
		NextToken       string `json:"nextToken"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	all := sfnExecutions.List()
	var filtered []SFNExecution
	for _, e := range all {
		if e.StateMachineArn != req.StateMachineArn {
			continue
		}
		if req.StatusFilter != "" && e.Status != req.StatusFilter {
			continue
		}
		filtered = append(filtered, e)
	}

	maxR := 0
	if req.MaxResults != nil {
		maxR = *req.MaxResults
	}
	page, nextTok := awsPage(filtered, req.NextToken, maxR, 100)

	items := make([]map[string]any, 0, len(page))
	for _, e := range page {
		item := map[string]any{
			"executionArn":    e.ExecutionArn,
			"stateMachineArn": e.StateMachineArn,
			"name":            e.Name,
			"status":          e.Status,
			"startDate":       e.StartDate,
		}
		if e.StopDate != nil {
			item["stopDate"] = *e.StopDate
		}
		items = append(items, item)
	}
	resp := map[string]any{"executions": items}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	sfnWriteJSON(w, http.StatusOK, resp)
}

func handleSFNStopExecution(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExecutionArn string `json:"executionArn"`
		Error        string `json:"error"`
		Cause        string `json:"cause"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}

	sfnMu.Lock()
	defer sfnMu.Unlock()

	exec, ok := sfnExecutions.Get(req.ExecutionArn)
	if !ok {
		sfnWriteError(w, "ExecutionDoesNotExist", "Execution does not exist: "+req.ExecutionArn)
		return
	}
	now := sfnEpochNow()
	exec.Status = "ABORTED"
	exec.StopDate = &now
	sfnExecutions.Put(req.ExecutionArn, exec)
	sfnWriteJSON(w, http.StatusOK, map[string]any{"stopDate": now})
}

func sfnNameFromARN(arn string) string {
	// arn:aws:states:us-east-1:123456789012:stateMachine:name
	// arn:aws:states:us-east-1:123456789012:execution:name:execName
	parts := strings.Split(arn, ":")
	if len(parts) >= 7 {
		return parts[6]
	}
	return arn
}

func sfnTagsToMap(tags []SFNTag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[t.Key] = t.Value
	}
	return m
}

func sfnMapToTags(m map[string]string) []SFNTag {
	tags := make([]SFNTag, 0, len(m))
	for k, v := range m {
		tags = append(tags, SFNTag{Key: k, Value: v})
	}
	return tags
}
