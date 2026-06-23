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
	RedriveCount    int      `json:"redriveCount"`
	RedriveDate     *float64 `json:"redriveDate,omitempty"`
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
	r.Register("AWSStepFunctions.GetExecutionHistory", handleSFNGetExecutionHistory)
	r.Register("AWSStepFunctions.ListExecutions", handleSFNListExecutions)
	r.Register("AWSStepFunctions.StopExecution", handleSFNStopExecution)

	// Activities — named task-poll resources with an ARN, plus the
	// task-token lifecycle GetActivityTask/SendTask* manage.
	sfnActivities = sim.MakeStore[SFNActivity](srv.DB(), "sfn_activities")
	sfnActivityTasks = sim.MakeStore[SFNActivityTask](srv.DB(), "sfn_activity_tasks")
	r.Register("AWSStepFunctions.CreateActivity", handleSFNCreateActivity)
	r.Register("AWSStepFunctions.DeleteActivity", handleSFNDeleteActivity)
	r.Register("AWSStepFunctions.DescribeActivity", handleSFNDescribeActivity)
	r.Register("AWSStepFunctions.ListActivities", handleSFNListActivities)
	r.Register("AWSStepFunctions.GetActivityTask", handleSFNGetActivityTask)
	r.Register("AWSStepFunctions.SendTaskSuccess", handleSFNSendTaskSuccess)
	r.Register("AWSStepFunctions.SendTaskFailure", handleSFNSendTaskFailure)
	r.Register("AWSStepFunctions.SendTaskHeartbeat", handleSFNSendTaskHeartbeat)

	// State-machine versions (numbered immutable snapshots) + aliases
	// (named pointers with a routingConfiguration to versions).
	sfnVersions = sim.MakeStore[SFNStateMachineVersion](srv.DB(), "sfn_sm_versions")
	sfnAliases = sim.MakeStore[SFNStateMachineAlias](srv.DB(), "sfn_sm_aliases")
	r.Register("AWSStepFunctions.PublishStateMachineVersion", handleSFNPublishStateMachineVersion)
	r.Register("AWSStepFunctions.DeleteStateMachineVersion", handleSFNDeleteStateMachineVersion)
	r.Register("AWSStepFunctions.CreateStateMachineAlias", handleSFNCreateStateMachineAlias)
	r.Register("AWSStepFunctions.DeleteStateMachineAlias", handleSFNDeleteStateMachineAlias)
	r.Register("AWSStepFunctions.DescribeStateMachineAlias", handleSFNDescribeStateMachineAlias)
	r.Register("AWSStepFunctions.ListStateMachineAliases", handleSFNListStateMachineAliases)
	r.Register("AWSStepFunctions.UpdateStateMachineAlias", handleSFNUpdateStateMachineAlias)

	// Execution-scoped read/redrive + Map Run aggregation.
	r.Register("AWSStepFunctions.DescribeStateMachineForExecution", handleSFNDescribeStateMachineForExecution)
	r.Register("AWSStepFunctions.RedriveExecution", handleSFNRedriveExecution)
	sfnMapRuns = sim.MakeStore[SFNMapRun](srv.DB(), "sfn_map_runs")
	r.Register("AWSStepFunctions.DescribeMapRun", handleSFNDescribeMapRun)
	r.Register("AWSStepFunctions.ListMapRuns", handleSFNListMapRuns)
	r.Register("AWSStepFunctions.UpdateMapRun", handleSFNUpdateMapRun)

	// Synchronous state / state-machine evaluation.
	r.Register("AWSStepFunctions.TestState", handleSFNTestState)
	r.Register("AWSStepFunctions.StartSyncExecution", handleSFNStartSyncExecution)
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
	// Validate the ASL definition at create time (the validator AWS runs);
	// a malformed/empty definition is an InvalidDefinition error.
	if err := sfnValidateDefinition(req.Definition); err != nil {
		sfnWriteError(w, "InvalidDefinition", err.Error())
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
	// Task
	Resource string `json:"Resource"`
	// Choice
	Choices []sfnChoiceRule `json:"Choices"`
	Default string          `json:"Default"`
	// Parallel
	Branches []sfnDefinition `json:"Branches"`
	// Map
	Iterator      *sfnDefinition `json:"Iterator"`
	ItemProcessor *sfnDefinition `json:"ItemProcessor"`
	ItemsPath     string         `json:"ItemsPath"`
}

// sfnChoiceRule is one Choice rule: a data-test (optionally nested via
// And/Or/Not) plus the Next state to transition to when it matches.
type sfnChoiceRule struct {
	Variable           string          `json:"Variable"`
	StringEquals       *string         `json:"StringEquals"`
	NumericEquals      *float64        `json:"NumericEquals"`
	NumericGreaterThan *float64        `json:"NumericGreaterThan"`
	NumericLessThan    *float64        `json:"NumericLessThan"`
	BooleanEquals      *bool           `json:"BooleanEquals"`
	IsPresent          *bool           `json:"IsPresent"`
	And                []sfnChoiceRule `json:"And"`
	Or                 []sfnChoiceRule `json:"Or"`
	Not                *sfnChoiceRule  `json:"Not"`
	Next               string          `json:"Next"`
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
	return sfnRunDefDepth(def, input, cancel, 0)
}

// sfnMaxNestingDepth bounds Parallel/Map branch recursion so a pathologically
// nested definition can't overflow the goroutine stack and crash the process.
// AWS's own ASL nesting limit is far below this.
const sfnMaxNestingDepth = 200

// sfnRunDefDepth runs one (sub-)state-machine. Parallel/Map recurse through it
// for their branches/iterations, carrying a depth counter that bounds nesting.
func sfnRunDefDepth(def sfnDefinition, input string, cancel <-chan struct{}, depth int) (string, string, error) {
	if depth > sfnMaxNestingDepth {
		return "", "FAILED", fmt.Errorf("state machine nesting depth exceeded %d", sfnMaxNestingDepth)
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
		case "Task":
			out, err := sfnRunTask(state, data)
			if err != nil {
				return "", "FAILED", err
			}
			data = out
			if state.End {
				return data, "SUCCEEDED", nil
			}
			if state.Next == "" {
				return "", "FAILED", fmt.Errorf("task state %q must declare End or Next", current)
			}
			current = state.Next
		case "Choice":
			next := state.Default
			for _, rule := range state.Choices {
				if sfnEvalChoice(rule, data) {
					next = rule.Next
					break
				}
			}
			if next == "" {
				return "", "FAILED", fmt.Errorf("choice state %q matched no rule and has no Default", current)
			}
			current = next
		case "Parallel":
			results := make([]json.RawMessage, len(state.Branches))
			for i, branch := range state.Branches {
				out, status, err := sfnRunDefDepth(branch, data, cancel, depth+1)
				if err != nil || status != "SUCCEEDED" {
					if status == "ABORTED" {
						return "", "ABORTED", errSFNAborted
					}
					return "", "FAILED", fmt.Errorf("parallel branch %d failed: %v", i, err)
				}
				results[i] = json.RawMessage(sfnNormalizeJSON(out))
			}
			merged, _ := json.Marshal(results)
			data = string(merged)
			if state.End {
				return data, "SUCCEEDED", nil
			}
			if state.Next == "" {
				return "", "FAILED", fmt.Errorf("parallel state %q must declare End or Next", current)
			}
			current = state.Next
		case "Map":
			proc := state.ItemProcessor
			if proc == nil {
				proc = state.Iterator
			}
			if proc == nil {
				return "", "FAILED", fmt.Errorf("map state %q requires an ItemProcessor or Iterator", current)
			}
			items, ok := sfnJSONPathArray(data, state.ItemsPath)
			if !ok {
				return "", "FAILED", fmt.Errorf("map state %q: ItemsPath %q did not resolve to an array", current, state.ItemsPath)
			}
			results := make([]json.RawMessage, len(items))
			for i, item := range items {
				out, status, err := sfnRunDefDepth(*proc, string(item), cancel, depth+1)
				if err != nil || status != "SUCCEEDED" {
					if status == "ABORTED" {
						return "", "ABORTED", errSFNAborted
					}
					return "", "FAILED", fmt.Errorf("map iteration %d failed: %v", i, err)
				}
				results[i] = json.RawMessage(sfnNormalizeJSON(out))
			}
			merged, _ := json.Marshal(results)
			data = string(merged)
			if state.End {
				return data, "SUCCEEDED", nil
			}
			if state.Next == "" {
				return "", "FAILED", fmt.Errorf("map state %q must declare End or Next", current)
			}
			current = state.Next
		default:
			return "", "FAILED", fmt.Errorf("unsupported state type %q", state.Type)
		}
	}
	return "", "FAILED", fmt.Errorf("state transition limit exceeded")
}

// sfnRunTask dispatches a Task state to its Lambda resource (a direct
// `arn:aws:lambda:...:function:NAME` ARN), invoking it in-process exactly as a
// real Task would, and returns the function's response as the new state data.
func sfnRunTask(state sfnState, data string) (string, error) {
	name, ok := sfnLambdaNameFromResource(state.Resource)
	if !ok {
		return "", fmt.Errorf("unsupported Task resource %q (only lambda function ARNs are supported)", state.Resource)
	}
	fn, ok := lambdaFunctions.Get(name)
	if !ok {
		return "", fmt.Errorf("task resource lambda %q not found", name)
	}
	resp, unhandled, _ := invokeLambdaViaRuntimeAPI(fn, []byte(data))
	if unhandled {
		return "", fmt.Errorf("task lambda %q returned an error: %s", name, string(resp))
	}
	return string(resp), nil
}

func sfnLambdaNameFromResource(resource string) (string, bool) {
	if i := strings.Index(resource, ":function:"); i >= 0 {
		name := resource[i+len(":function:"):]
		if j := strings.IndexByte(name, ':'); j >= 0 { // strip a :version/:alias suffix
			name = name[:j]
		}
		return name, name != ""
	}
	return "", false
}

// sfnNormalizeJSON returns s if it is valid JSON, else a JSON string of s.
func sfnNormalizeJSON(s string) string {
	if json.Valid([]byte(s)) {
		return s
	}
	b, _ := json.Marshal(s)
	return string(b)
}

// sfnJSONValue resolves a Choice `Variable` reference-path (e.g. "$.a.b") against
// the state data.
func sfnJSONValue(data, path string) (any, bool) {
	var doc any
	if json.Unmarshal([]byte(data), &doc) != nil {
		return nil, false
	}
	path = strings.TrimPrefix(path, "$")
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return doc, true
	}
	cur := doc
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[seg]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func sfnJSONPathArray(data, path string) ([]json.RawMessage, bool) {
	src := data
	if path != "" {
		v, ok := sfnJSONValue(data, path)
		if !ok {
			return nil, false
		}
		b, _ := json.Marshal(v)
		src = string(b)
	}
	var arr []json.RawMessage
	if json.Unmarshal([]byte(src), &arr) != nil {
		return nil, false
	}
	return arr, true
}

func sfnEvalChoice(rule sfnChoiceRule, data string) bool {
	switch {
	case len(rule.And) > 0:
		for _, sub := range rule.And {
			if !sfnEvalChoice(sub, data) {
				return false
			}
		}
		return true
	case len(rule.Or) > 0:
		for _, sub := range rule.Or {
			if sfnEvalChoice(sub, data) {
				return true
			}
		}
		return false
	case rule.Not != nil:
		return !sfnEvalChoice(*rule.Not, data)
	}
	val, present := sfnJSONValue(data, rule.Variable)
	switch {
	case rule.IsPresent != nil:
		return present == *rule.IsPresent
	case rule.StringEquals != nil:
		s, ok := val.(string)
		return ok && s == *rule.StringEquals
	case rule.BooleanEquals != nil:
		b, ok := val.(bool)
		return ok && b == *rule.BooleanEquals
	case rule.NumericEquals != nil:
		n, ok := sfnAsFloat(val)
		return ok && n == *rule.NumericEquals
	case rule.NumericGreaterThan != nil:
		n, ok := sfnAsFloat(val)
		return ok && n > *rule.NumericGreaterThan
	case rule.NumericLessThan != nil:
		n, ok := sfnAsFloat(val)
		return ok && n < *rule.NumericLessThan
	}
	return false
}

func sfnAsFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
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

// handleSFNGetExecutionHistory returns the execution-level event history
// (ExecutionStarted + the terminal Succeeded/Failed/Aborted event).
func handleSFNGetExecutionHistory(w http.ResponseWriter, r *http.Request) {
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
	events := []map[string]any{{
		"timestamp": exec.StartDate,
		"type":      "ExecutionStarted",
		"id":        1,
		"executionStartedEventDetails": map[string]any{
			"input":   exec.Input,
			"roleArn": "",
		},
	}}
	if exec.Status != "RUNNING" && exec.StopDate != nil {
		ev := map[string]any{"timestamp": *exec.StopDate, "id": 2, "previousEventId": 1}
		switch exec.Status {
		case "SUCCEEDED":
			ev["type"] = "ExecutionSucceeded"
			ev["executionSucceededEventDetails"] = map[string]any{"output": exec.Output}
		case "ABORTED":
			ev["type"] = "ExecutionAborted"
			ev["executionAbortedEventDetails"] = map[string]any{}
		default:
			ev["type"] = "ExecutionFailed"
			ev["executionFailedEventDetails"] = map[string]any{}
		}
		events = append(events, ev)
	}
	sfnWriteJSON(w, http.StatusOK, map[string]any{"events": events})
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
		if cancel, ok := cancelAny.(chan struct{}); ok {
			close(cancel)
		}
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

// ── Activities ────────────────────────────────────────────────────────────
//
// An activity is a named task-poll resource: a worker polls GetActivityTask
// for work and reports back via SendTaskSuccess/SendTaskFailure/
// SendTaskHeartbeat. A Task state with `Resource: arn:...:activity:NAME`
// schedules a task token onto the activity's queue; the worker drains it.

type SFNActivity struct {
	ActivityArn  string  `json:"activityArn"`
	Name         string  `json:"name"`
	CreationDate float64 `json:"creationDate"`
}

// SFNActivityTask is one scheduled-but-not-yet-completed activity task,
// keyed by its opaque task token.
type SFNActivityTask struct {
	TaskToken   string  `json:"taskToken"`
	ActivityArn string  `json:"activityArn"`
	Input       string  `json:"input"`
	Status      string  `json:"status"` // SCHEDULED, RUNNING, SUCCEEDED, FAILED
	Output      string  `json:"output"`
	Error       string  `json:"error"`
	Cause       string  `json:"cause"`
	LastHB      float64 `json:"lastHeartbeat"`
}

var (
	sfnActivities    sim.Store[SFNActivity]
	sfnActivityTasks sim.Store[SFNActivityTask]
)

func handleSFNCreateActivity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string   `json:"name"`
		Tags []SFNTag `json:"tags"`
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

	arn := sfnARN("activity:" + req.Name)
	// CreateActivity is idempotent on (name) — re-creating returns the
	// existing ARN and original creation date.
	if existing, ok := sfnActivities.Get(req.Name); ok {
		sfnWriteJSON(w, http.StatusOK, map[string]any{
			"activityArn":  existing.ActivityArn,
			"creationDate": existing.CreationDate,
		})
		return
	}
	act := SFNActivity{
		ActivityArn:  arn,
		Name:         req.Name,
		CreationDate: sfnEpochNow(),
	}
	sfnActivities.Put(req.Name, act)
	sfnWriteJSON(w, http.StatusOK, map[string]any{
		"activityArn":  arn,
		"creationDate": act.CreationDate,
	})
}

func handleSFNDeleteActivity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ActivityArn string `json:"activityArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	sfnMu.Lock()
	defer sfnMu.Unlock()
	sfnActivities.Delete(sfnNameFromARN(req.ActivityArn))
	sfnWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleSFNDescribeActivity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ActivityArn string `json:"activityArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	act, ok := sfnActivities.Get(sfnNameFromARN(req.ActivityArn))
	if !ok {
		sfnWriteError(w, "ActivityDoesNotExist", "Activity does not exist: "+req.ActivityArn)
		return
	}
	sfnWriteJSON(w, http.StatusOK, map[string]any{
		"activityArn":  act.ActivityArn,
		"name":         act.Name,
		"creationDate": act.CreationDate,
	})
}

func handleSFNListActivities(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxResults *int   `json:"maxResults"`
		NextToken  string `json:"nextToken"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	all := sfnActivities.List()
	maxR := 0
	if req.MaxResults != nil {
		maxR = *req.MaxResults
	}
	page, nextTok := awsPage(all, req.NextToken, maxR, 100)
	items := make([]map[string]any, 0, len(page))
	for _, a := range page {
		items = append(items, map[string]any{
			"activityArn":  a.ActivityArn,
			"name":         a.Name,
			"creationDate": a.CreationDate,
		})
	}
	resp := map[string]any{"activities": items}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	sfnWriteJSON(w, http.StatusOK, resp)
}

func handleSFNGetActivityTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ActivityArn string `json:"activityArn"`
		WorkerName  string `json:"workerName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	if _, ok := sfnActivities.Get(sfnNameFromARN(req.ActivityArn)); !ok {
		sfnWriteError(w, "ActivityDoesNotExist", "Activity does not exist: "+req.ActivityArn)
		return
	}

	sfnMu.Lock()
	defer sfnMu.Unlock()

	// Dequeue the oldest SCHEDULED task for this activity, marking it RUNNING.
	for _, task := range sfnActivityTasks.List() {
		if task.ActivityArn == req.ActivityArn && task.Status == "SCHEDULED" {
			task.Status = "RUNNING"
			task.LastHB = sfnEpochNow()
			sfnActivityTasks.Put(task.TaskToken, task)
			sfnWriteJSON(w, http.StatusOK, map[string]any{
				"taskToken": task.TaskToken,
				"input":     task.Input,
			})
			return
		}
	}
	// No work available: the real API long-polls up to 60s then returns an
	// empty taskToken. We return immediately with an empty token (the
	// faithful no-work response shape).
	sfnWriteJSON(w, http.StatusOK, map[string]any{"taskToken": ""})
}

func handleSFNSendTaskSuccess(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskToken string `json:"taskToken"`
		Output    string `json:"output"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	sfnMu.Lock()
	defer sfnMu.Unlock()
	task, ok := sfnActivityTasks.Get(req.TaskToken)
	if !ok {
		sfnWriteError(w, "TaskDoesNotExist", "Task Token does not exist")
		return
	}
	if task.Status != "RUNNING" {
		sfnWriteError(w, "TaskTimedOut", "Task Timed Out")
		return
	}
	task.Status = "SUCCEEDED"
	task.Output = req.Output
	sfnActivityTasks.Put(req.TaskToken, task)
	sfnWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleSFNSendTaskFailure(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskToken string `json:"taskToken"`
		Error     string `json:"error"`
		Cause     string `json:"cause"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	sfnMu.Lock()
	defer sfnMu.Unlock()
	task, ok := sfnActivityTasks.Get(req.TaskToken)
	if !ok {
		sfnWriteError(w, "TaskDoesNotExist", "Task Token does not exist")
		return
	}
	task.Status = "FAILED"
	task.Error = req.Error
	task.Cause = req.Cause
	sfnActivityTasks.Put(req.TaskToken, task)
	sfnWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleSFNSendTaskHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskToken string `json:"taskToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	sfnMu.Lock()
	defer sfnMu.Unlock()
	task, ok := sfnActivityTasks.Get(req.TaskToken)
	if !ok {
		sfnWriteError(w, "TaskDoesNotExist", "Task Token does not exist")
		return
	}
	if task.Status != "RUNNING" {
		sfnWriteError(w, "TaskTimedOut", "Task Timed Out")
		return
	}
	task.LastHB = sfnEpochNow()
	sfnActivityTasks.Put(req.TaskToken, task)
	sfnWriteJSON(w, http.StatusOK, map[string]any{})
}

// ── State machine versions + aliases ──────────────────────────────────────
//
// PublishStateMachineVersion snapshots the current definition+role into a
// numbered, immutable version (arn:...:stateMachine:Name:N). An alias is a
// named pointer (arn:...:stateMachine:Name:aliasName) whose
// routingConfiguration weights traffic across one or two versions.

type SFNStateMachineVersion struct {
	StateMachineVersionArn string  `json:"stateMachineVersionArn"`
	StateMachineName       string  `json:"stateMachineName"`
	Version                int     `json:"version"`
	Definition             string  `json:"definition"`
	RoleArn                string  `json:"roleArn"`
	Description            string  `json:"description"`
	CreationDate           float64 `json:"creationDate"`
}

type SFNRoutingConfig struct {
	StateMachineVersionArn string `json:"stateMachineVersionArn"`
	Weight                 int    `json:"weight"`
}

type SFNStateMachineAlias struct {
	StateMachineAliasArn string             `json:"stateMachineAliasArn"`
	Name                 string             `json:"name"`
	Description          string             `json:"description"`
	RoutingConfiguration []SFNRoutingConfig `json:"routingConfiguration"`
	CreationDate         float64            `json:"creationDate"`
	UpdateDate           float64            `json:"updateDate"`
}

var (
	sfnVersions sim.Store[SFNStateMachineVersion]
	sfnAliases  sim.Store[SFNStateMachineAlias]
)

// sfnNextVersionNumber returns 1 + the highest existing version number for a
// state machine.
func sfnNextVersionNumber(smName string) int {
	max := 0
	for _, v := range sfnVersions.List() {
		if v.StateMachineName == smName && v.Version > max {
			max = v.Version
		}
	}
	return max + 1
}

func handleSFNPublishStateMachineVersion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StateMachineArn string `json:"stateMachineArn"`
		Description     string `json:"description"`
		RevisionId      string `json:"revisionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	sfnMu.Lock()
	defer sfnMu.Unlock()

	smName := sfnNameFromARN(req.StateMachineArn)
	sm, ok := sfnStateMachines.Get(smName)
	if !ok {
		sfnWriteError(w, "StateMachineDoesNotExist", "State machine does not exist: "+req.StateMachineArn)
		return
	}
	n := sfnNextVersionNumber(smName)
	arn := fmt.Sprintf("%s:%d", sm.StateMachineArn, n)
	ver := SFNStateMachineVersion{
		StateMachineVersionArn: arn,
		StateMachineName:       smName,
		Version:                n,
		Definition:             sm.Definition,
		RoleArn:                sm.RoleArn,
		Description:            req.Description,
		CreationDate:           sfnEpochNow(),
	}
	sfnVersions.Put(arn, ver)
	sfnWriteJSON(w, http.StatusOK, map[string]any{
		"stateMachineVersionArn": arn,
		"creationDate":           ver.CreationDate,
	})
}

func handleSFNDeleteStateMachineVersion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StateMachineVersionArn string `json:"stateMachineVersionArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	sfnMu.Lock()
	defer sfnMu.Unlock()
	sfnVersions.Delete(req.StateMachineVersionArn)
	sfnWriteJSON(w, http.StatusOK, map[string]any{})
}

// sfnAliasKey is the store key for an alias: the state-machine name joined
// with the alias name, so aliases are unique per state machine.
func sfnAliasKey(smName, aliasName string) string {
	return smName + "/" + aliasName
}

// sfnSMNameFromVersionArn pulls the state-machine name out of a version ARN
// (arn:...:stateMachine:Name:N).
func sfnSMNameFromVersionArn(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) >= 7 {
		return parts[6]
	}
	return arn
}

func handleSFNCreateStateMachineAlias(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name                 string             `json:"name"`
		Description          string             `json:"description"`
		RoutingConfiguration []SFNRoutingConfig `json:"routingConfiguration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	if req.Name == "" {
		sfnWriteError(w, "InvalidName", "name is required")
		return
	}
	if len(req.RoutingConfiguration) == 0 {
		sfnWriteError(w, "ValidationException", "routingConfiguration is required")
		return
	}

	sfnMu.Lock()
	defer sfnMu.Unlock()

	// The alias belongs to the state machine that owns the version(s) it
	// routes to.
	smName := sfnSMNameFromVersionArn(req.RoutingConfiguration[0].StateMachineVersionArn)
	if _, ok := sfnStateMachines.Get(smName); !ok {
		sfnWriteError(w, "StateMachineDoesNotExist", "State machine does not exist for version: "+req.RoutingConfiguration[0].StateMachineVersionArn)
		return
	}
	aliasArn := fmt.Sprintf("%s:%s", sfnARN("stateMachine:"+smName), req.Name)
	now := sfnEpochNow()
	alias := SFNStateMachineAlias{
		StateMachineAliasArn: aliasArn,
		Name:                 req.Name,
		Description:          req.Description,
		RoutingConfiguration: req.RoutingConfiguration,
		CreationDate:         now,
		UpdateDate:           now,
	}
	sfnAliases.Put(sfnAliasKey(smName, req.Name), alias)
	sfnWriteJSON(w, http.StatusOK, map[string]any{
		"stateMachineAliasArn": aliasArn,
		"creationDate":         now,
	})
}

// sfnAliasByArn resolves an alias by its full ARN
// (arn:...:stateMachine:Name:aliasName).
func sfnAliasByArn(arn string) (SFNStateMachineAlias, bool) {
	for _, a := range sfnAliases.List() {
		if a.StateMachineAliasArn == arn {
			return a, true
		}
	}
	return SFNStateMachineAlias{}, false
}

func handleSFNDescribeStateMachineAlias(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StateMachineAliasArn string `json:"stateMachineAliasArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	alias, ok := sfnAliasByArn(req.StateMachineAliasArn)
	if !ok {
		sfnWriteError(w, "ResourceNotFound", "State machine alias does not exist: "+req.StateMachineAliasArn)
		return
	}
	sfnWriteJSON(w, http.StatusOK, map[string]any{
		"stateMachineAliasArn": alias.StateMachineAliasArn,
		"name":                 alias.Name,
		"description":          alias.Description,
		"routingConfiguration": alias.RoutingConfiguration,
		"creationDate":         alias.CreationDate,
		"updateDate":           alias.UpdateDate,
	})
}

func handleSFNListStateMachineAliases(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StateMachineArn string `json:"stateMachineArn"`
		MaxResults      *int   `json:"maxResults"`
		NextToken       string `json:"nextToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	smName := sfnNameFromARN(req.StateMachineArn)
	if _, ok := sfnStateMachines.Get(smName); !ok {
		sfnWriteError(w, "StateMachineDoesNotExist", "State machine does not exist: "+req.StateMachineArn)
		return
	}
	var owned []SFNStateMachineAlias
	smArn := sfnARN("stateMachine:" + smName)
	for _, a := range sfnAliases.List() {
		if strings.HasPrefix(a.StateMachineAliasArn, smArn+":") {
			owned = append(owned, a)
		}
	}
	maxR := 0
	if req.MaxResults != nil {
		maxR = *req.MaxResults
	}
	page, nextTok := awsPage(owned, req.NextToken, maxR, 100)
	items := make([]map[string]any, 0, len(page))
	for _, a := range page {
		items = append(items, map[string]any{
			"stateMachineAliasArn": a.StateMachineAliasArn,
			"creationDate":         a.CreationDate,
		})
	}
	resp := map[string]any{"stateMachineAliases": items}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	sfnWriteJSON(w, http.StatusOK, resp)
}

func handleSFNUpdateStateMachineAlias(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StateMachineAliasArn string             `json:"stateMachineAliasArn"`
		Description          *string            `json:"description"`
		RoutingConfiguration []SFNRoutingConfig `json:"routingConfiguration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	sfnMu.Lock()
	defer sfnMu.Unlock()

	alias, ok := sfnAliasByArn(req.StateMachineAliasArn)
	if !ok {
		sfnWriteError(w, "ResourceNotFound", "State machine alias does not exist: "+req.StateMachineAliasArn)
		return
	}
	if req.Description != nil {
		alias.Description = *req.Description
	}
	if len(req.RoutingConfiguration) > 0 {
		alias.RoutingConfiguration = req.RoutingConfiguration
	}
	alias.UpdateDate = sfnEpochNow()
	smName := sfnSMNameFromVersionArn(alias.RoutingConfiguration[0].StateMachineVersionArn)
	sfnAliases.Put(sfnAliasKey(smName, alias.Name), alias)
	sfnWriteJSON(w, http.StatusOK, map[string]any{"updateDate": alias.UpdateDate})
}

func handleSFNDeleteStateMachineAlias(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StateMachineAliasArn string `json:"stateMachineAliasArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	sfnMu.Lock()
	defer sfnMu.Unlock()
	if alias, ok := sfnAliasByArn(req.StateMachineAliasArn); ok {
		smName := sfnSMNameFromVersionArn(alias.RoutingConfiguration[0].StateMachineVersionArn)
		sfnAliases.Delete(sfnAliasKey(smName, alias.Name))
	}
	sfnWriteJSON(w, http.StatusOK, map[string]any{})
}

// ── DescribeStateMachineForExecution + RedriveExecution ───────────────────

func handleSFNDescribeStateMachineForExecution(w http.ResponseWriter, r *http.Request) {
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
	sm, ok := sfnStateMachines.Get(sfnNameFromARN(exec.StateMachineArn))
	if !ok {
		sfnWriteError(w, "StateMachineDoesNotExist", "State machine does not exist: "+exec.StateMachineArn)
		return
	}
	sfnWriteJSON(w, http.StatusOK, map[string]any{
		"stateMachineArn": sm.StateMachineArn,
		"name":            sm.Name,
		"definition":      sm.Definition,
		"roleArn":         sm.RoleArn,
		"updateDate":      sm.CreationDate,
	})
}

func handleSFNRedriveExecution(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExecutionArn string `json:"executionArn"`
		ClientToken  string `json:"clientToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}

	sfnMu.Lock()
	exec, ok := sfnExecutions.Get(req.ExecutionArn)
	if !ok {
		sfnMu.Unlock()
		sfnWriteError(w, "ExecutionDoesNotExist", "Execution does not exist: "+req.ExecutionArn)
		return
	}
	// Redrive only applies to a terminal, non-successful execution
	// (FAILED/ABORTED/TIMED_OUT). A RUNNING or SUCCEEDED execution is not
	// redrivable.
	if exec.Status == "RUNNING" || exec.Status == "SUCCEEDED" {
		sfnMu.Unlock()
		sfnWriteError(w, "ExecutionNotRedrivable", "Execution is not redrivable: "+req.ExecutionArn)
		return
	}
	sm, ok := sfnStateMachines.Get(sfnNameFromARN(exec.StateMachineArn))
	if !ok {
		sfnMu.Unlock()
		sfnWriteError(w, "StateMachineDoesNotExist", "State machine does not exist: "+exec.StateMachineArn)
		return
	}
	now := sfnEpochNow()
	exec.Status = "RUNNING"
	exec.StopDate = nil
	exec.Output = ""
	exec.RedriveCount++
	exec.RedriveDate = &now
	sfnExecutions.Put(req.ExecutionArn, exec)
	input := exec.Input
	cancel := make(chan struct{})
	sfnCancels.Store(req.ExecutionArn, cancel)
	sfnMu.Unlock()

	go sfnRunExecution(req.ExecutionArn, sm.Definition, input, cancel)
	sfnWriteJSON(w, http.StatusOK, map[string]any{"redriveDate": now})
}

// ── Map Runs ──────────────────────────────────────────────────────────────
//
// A Map Run aggregates the child workflow executions a Distributed Map state
// launches. It is keyed by its mapRunArn and tied to the parent execution.

type SFNMapRun struct {
	MapRunArn                  string   `json:"mapRunArn"`
	ExecutionArn               string   `json:"executionArn"`
	StateMachineArn            string   `json:"stateMachineArn"`
	Status                     string   `json:"status"`
	StartDate                  float64  `json:"startDate"`
	StopDate                   *float64 `json:"stopDate"`
	MaxConcurrency             int      `json:"maxConcurrency"`
	ToleratedFailurePercentage float64  `json:"toleratedFailurePercentage"`
	ToleratedFailureCount      int      `json:"toleratedFailureCount"`
	Total                      int      `json:"total"`
	Succeeded                  int      `json:"succeeded"`
	Failed                     int      `json:"failed"`
	RedriveCount               int      `json:"redriveCount"`
}

var sfnMapRuns sim.Store[SFNMapRun]

func handleSFNDescribeMapRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MapRunArn string `json:"mapRunArn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	mr, ok := sfnMapRuns.Get(req.MapRunArn)
	if !ok {
		sfnWriteError(w, "ResourceNotFound", "Map Run does not exist: "+req.MapRunArn)
		return
	}
	resp := map[string]any{
		"mapRunArn":                  mr.MapRunArn,
		"executionArn":               mr.ExecutionArn,
		"status":                     mr.Status,
		"startDate":                  mr.StartDate,
		"maxConcurrency":             mr.MaxConcurrency,
		"toleratedFailurePercentage": mr.ToleratedFailurePercentage,
		"toleratedFailureCount":      mr.ToleratedFailureCount,
		"itemCounts": map[string]any{
			"pending":        0,
			"running":        0,
			"succeeded":      mr.Succeeded,
			"failed":         mr.Failed,
			"timedOut":       0,
			"aborted":        0,
			"total":          mr.Total,
			"resultsWritten": mr.Succeeded,
		},
		"executionCounts": map[string]any{
			"pending":        0,
			"running":        0,
			"succeeded":      mr.Succeeded,
			"failed":         mr.Failed,
			"timedOut":       0,
			"aborted":        0,
			"total":          mr.Total,
			"resultsWritten": mr.Succeeded,
		},
		"redriveCount": mr.RedriveCount,
	}
	if mr.StopDate != nil {
		resp["stopDate"] = *mr.StopDate
	}
	sfnWriteJSON(w, http.StatusOK, resp)
}

func handleSFNListMapRuns(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExecutionArn string `json:"executionArn"`
		MaxResults   *int   `json:"maxResults"`
		NextToken    string `json:"nextToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	var owned []SFNMapRun
	for _, mr := range sfnMapRuns.List() {
		if mr.ExecutionArn == req.ExecutionArn {
			owned = append(owned, mr)
		}
	}
	maxR := 0
	if req.MaxResults != nil {
		maxR = *req.MaxResults
	}
	page, nextTok := awsPage(owned, req.NextToken, maxR, 100)
	items := make([]map[string]any, 0, len(page))
	for _, mr := range page {
		item := map[string]any{
			"executionArn":    mr.ExecutionArn,
			"mapRunArn":       mr.MapRunArn,
			"stateMachineArn": mr.StateMachineArn,
			"startDate":       mr.StartDate,
		}
		if mr.StopDate != nil {
			item["stopDate"] = *mr.StopDate
		}
		items = append(items, item)
	}
	resp := map[string]any{"mapRuns": items}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	sfnWriteJSON(w, http.StatusOK, resp)
}

func handleSFNUpdateMapRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MapRunArn                  string   `json:"mapRunArn"`
		MaxConcurrency             *int     `json:"maxConcurrency"`
		ToleratedFailurePercentage *float64 `json:"toleratedFailurePercentage"`
		ToleratedFailureCount      *int     `json:"toleratedFailureCount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	sfnMu.Lock()
	defer sfnMu.Unlock()
	mr, ok := sfnMapRuns.Get(req.MapRunArn)
	if !ok {
		sfnWriteError(w, "ResourceNotFound", "Map Run does not exist: "+req.MapRunArn)
		return
	}
	if req.MaxConcurrency != nil {
		mr.MaxConcurrency = *req.MaxConcurrency
	}
	if req.ToleratedFailurePercentage != nil {
		mr.ToleratedFailurePercentage = *req.ToleratedFailurePercentage
	}
	if req.ToleratedFailureCount != nil {
		mr.ToleratedFailureCount = *req.ToleratedFailureCount
	}
	sfnMapRuns.Put(req.MapRunArn, mr)
	sfnWriteJSON(w, http.StatusOK, map[string]any{})
}

// ── TestState + StartSyncExecution ────────────────────────────────────────
//
// TestState runs a single state synchronously; StartSyncExecution runs the
// whole state machine synchronously and returns the terminal result. Both
// reuse the same ASL interpreter that backs StartExecution.

func handleSFNTestState(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Definition string `json:"definition"`
		Input      string `json:"input"`
		StateName  string `json:"stateName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	if req.Definition == "" {
		sfnWriteError(w, "ValidationException", "definition is required")
		return
	}
	// TestState's definition is a single ASL *state* object (not a full
	// state machine). Wrap it in a one-state machine and run it through the
	// interpreter so the result is a real evaluation, not a fabricated one.
	var state sfnState
	if err := json.Unmarshal([]byte(req.Definition), &state); err != nil {
		sfnWriteError(w, "InvalidDefinition", "invalid state definition: "+err.Error())
		return
	}
	stateName := req.StateName
	if stateName == "" {
		stateName = "TestState"
	}
	// Force the single state to be terminal so the interpreter returns its
	// result rather than chasing a Next that isn't in the wrapper.
	nextState := state.Next
	state.Next = ""
	state.End = true
	wrapped := sfnDefinition{
		StartAt: stateName,
		States:  map[string]sfnState{stateName: state},
	}
	input := req.Input
	if input == "" {
		input = "{}"
	}
	wrappedJSON, _ := json.Marshal(wrapped)
	output, status, err := sfnExecute(string(wrappedJSON), input, nil)

	resp := map[string]any{
		"inspectionData": map[string]any{"input": input},
	}
	if err != nil || status == "FAILED" {
		resp["status"] = "FAILED"
		resp["error"] = "States.Runtime"
		if err != nil {
			resp["cause"] = err.Error()
		}
	} else {
		resp["status"] = "SUCCEEDED"
		resp["output"] = output
		if nextState != "" {
			resp["nextState"] = nextState
		}
		if id, ok := resp["inspectionData"].(map[string]any); ok {
			id["result"] = output
		}
	}
	sfnWriteJSON(w, http.StatusOK, resp)
}

func handleSFNStartSyncExecution(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StateMachineArn string `json:"stateMachineArn"`
		Name            string `json:"name"`
		Input           string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sfnWriteError(w, "InvalidRequest", "invalid JSON")
		return
	}
	smName := sfnNameFromARN(req.StateMachineArn)
	sm, ok := sfnStateMachines.Get(smName)
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
	execARN := sfnARN("express:" + smName + ":" + execName + ":" + uuid.New().String())
	input := req.Input
	if input == "" {
		input = "{}"
	}
	startDate := sfnEpochNow()
	output, status, err := sfnExecute(sm.Definition, input, nil)
	stopDate := sfnEpochNow()

	resp := map[string]any{
		"executionArn":    execARN,
		"stateMachineArn": req.StateMachineArn,
		"name":            execName,
		"startDate":       startDate,
		"stopDate":        stopDate,
		"input":           input,
		"inputDetails":    map[string]any{"included": true},
		"outputDetails":   map[string]any{"included": true},
		"billingDetails": map[string]any{
			"billedMemoryUsedInMB":         64,
			"billedDurationInMilliseconds": 100,
		},
	}
	if err != nil || status == "FAILED" {
		resp["status"] = "FAILED"
		resp["error"] = "States.Runtime"
		if err != nil {
			resp["cause"] = err.Error()
		}
	} else {
		resp["status"] = "SUCCEEDED"
		resp["output"] = output
	}
	sfnWriteJSON(w, http.StatusOK, resp)
}
