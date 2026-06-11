package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sim "github.com/sockerless/simulator"
)

// AWS Step Functions — AWS JSON 1.0 protocol (X-Amz-Target: AWSStepFunctions.<Op>).
// Executions run a small ASL interpreter for the state types exercised by sockerless.

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
	sfnCancels       sync.Map
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
	r.Register("AWSStepFunctions.ListStateMachineVersions", handleSFNListStateMachineVersions)
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
	sfnWriteJSON(w, http.StatusOK, sfnStateMachineWire{sm})
}

// sfnStateMachineWire strips store-only members from state-machine
// responses: DescribeStateMachineOutput has no tags member — tags ride
// ListTagsForResource, which reads them from the store.
type sfnStateMachineWire struct {
	SFNStateMachine
}

func (s sfnStateMachineWire) MarshalJSON() ([]byte, error) {
	type alias SFNStateMachine
	clean := alias(s.SFNStateMachine)
	clean.Tags = nil
	return json.Marshal(clean)
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

func handleSFNListStateMachineVersions(w http.ResponseWriter, r *http.Request) {
	// TF provider calls this after CreateStateMachine. Sim doesn't track
	// versions separately — return empty list.
	sfnWriteJSON(w, http.StatusOK, map[string]any{"stateMachineVersions": []any{}})
}

func handleSFNValidateStateMachineDefinition(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Definition string `json:"definition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	if err := sfnValidateDefinition(req.Definition); err != nil {
		sfnWriteJSON(w, http.StatusOK, map[string]any{
			"result":      "FAIL",
			"diagnostics": []map[string]string{{"message": err.Error()}},
		})
		return
	}
	sfnWriteJSON(w, http.StatusOK, map[string]any{"result": "OK"})
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
	sm, ok := sfnStateMachines.Get(name)
	if !ok {
		sfnWriteError(w, "StateMachineDoesNotExist", "State machine does not exist: "+req.StateMachineArn)
		return
	}
	if err := sfnValidateDefinition(sm.Definition); err != nil {
		sfnWriteError(w, "InvalidDefinition", err.Error())
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
	input := req.Input
	if input == "" {
		input = "{}"
	}
	exec := SFNExecution{
		ExecutionArn:    execARN,
		StateMachineArn: req.StateMachineArn,
		Name:            execName,
		Status:          "RUNNING",
		StartDate:       now,
		Input:           input,
	}
	sfnExecutions.Put(execARN, exec)
	cancel := make(chan struct{})
	sfnCancels.Store(execARN, cancel)
	go sfnRunExecution(execARN, sm.Definition, input, cancel)
	sfnWriteJSON(w, http.StatusOK, map[string]any{
		"executionArn": execARN,
		"startDate":    now,
	})
}

type sfnDefinition struct {
	StartAt string              `json:"StartAt"`
	States  map[string]sfnState `json:"States"`
}

type sfnState struct {
	Type    string           `json:"Type"`
	Next    string           `json:"Next"`
	End     bool             `json:"End"`
	Result  *json.RawMessage `json:"Result"`
	Seconds *int             `json:"Seconds"`
	Error   string           `json:"Error"`
	Cause   string           `json:"Cause"`
}

var errSFNAborted = errors.New("execution aborted")

func sfnValidateDefinition(definition string) error {
	var def sfnDefinition
	if err := json.Unmarshal([]byte(definition), &def); err != nil {
		return fmt.Errorf("invalid ASL JSON: %w", err)
	}
	if def.StartAt == "" {
		return fmt.Errorf("StartAt is required")
	}
	if len(def.States) == 0 {
		return fmt.Errorf("states is required")
	}
	if _, ok := def.States[def.StartAt]; !ok {
		return fmt.Errorf("StartAt state %q does not exist", def.StartAt)
	}
	return nil
}

func sfnRunExecution(execARN, definition, input string, cancel <-chan struct{}) {
	defer sfnCancels.Delete(execARN)

	output, status, err := sfnExecute(definition, input, cancel)
	if errors.Is(err, errSFNAborted) {
		return
	}
	if err != nil {
		status = "FAILED"
		output = ""
	}
	sfnCompleteExecution(execARN, status, output)
}

func sfnExecute(definition, input string, cancel <-chan struct{}) (string, string, error) {
	var def sfnDefinition
	if err := json.Unmarshal([]byte(definition), &def); err != nil {
		return "", "FAILED", err
	}
	current := def.StartAt
	data := input
	for steps := 0; steps < 1000; steps++ {
		select {
		case <-cancel:
			return "", "ABORTED", errSFNAborted
		default:
		}
		state, ok := def.States[current]
		if !ok {
			return "", "FAILED", fmt.Errorf("state %q does not exist", current)
		}
		switch state.Type {
		case "Pass":
			if state.Result != nil {
				data = string(*state.Result)
			}
			if state.End {
				return data, "SUCCEEDED", nil
			}
			if state.Next == "" {
				return "", "FAILED", fmt.Errorf("pass state %q must declare End or Next", current)
			}
			current = state.Next
		case "Succeed":
			return data, "SUCCEEDED", nil
		case "Fail":
			msg := state.Cause
			if msg == "" {
				msg = state.Error
			}
			if msg == "" {
				msg = "Fail state reached"
			}
			return "", "FAILED", fmt.Errorf("%s", msg)
		case "Wait":
			if state.Seconds == nil || *state.Seconds < 0 {
				return "", "FAILED", fmt.Errorf("wait state %q requires non-negative Seconds", current)
			}
			timer := time.NewTimer(time.Duration(*state.Seconds) * time.Second)
			select {
			case <-cancel:
				timer.Stop()
				return "", "ABORTED", errSFNAborted
			case <-timer.C:
			}
			if state.End {
				return data, "SUCCEEDED", nil
			}
			if state.Next == "" {
				return "", "FAILED", fmt.Errorf("wait state %q must declare End or Next", current)
			}
			current = state.Next
		default:
			return "", "FAILED", fmt.Errorf("unsupported state type %q", state.Type)
		}
	}
	return "", "FAILED", fmt.Errorf("state transition limit exceeded")
}

func sfnCompleteExecution(execARN, status, output string) {
	sfnMu.Lock()
	defer sfnMu.Unlock()

	exec, ok := sfnExecutions.Get(execARN)
	if !ok || exec.Status != "RUNNING" {
		return
	}
	now := sfnEpochNow()
	exec.Status = status
	exec.StopDate = &now
	exec.Output = output
	sfnExecutions.Put(execARN, exec)
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
	if exec.Status != "RUNNING" {
		sfnWriteError(w, "ExecutionNotRunning", "Execution is not running: "+req.ExecutionArn)
		return
	}
	now := sfnEpochNow()
	exec.Status = "ABORTED"
	exec.StopDate = &now
	sfnExecutions.Put(req.ExecutionArn, exec)
	if cancelAny, ok := sfnCancels.LoadAndDelete(req.ExecutionArn); ok {
		close(cancelAny.(chan struct{}))
	}
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
