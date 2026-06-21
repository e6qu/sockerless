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
	r.Register("AWSStepFunctions.GetExecutionHistory", handleSFNGetExecutionHistory)
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
