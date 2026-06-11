package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// EventBridge uses the AWS JSON protocol with X-Amz-Target:
// AWSEvents.<Operation>. The simulator implements the foundational
// rule/target/event slice used by SDK, CLI, and Terraform consumers.

type EBRule struct {
	Name               string            `json:"Name"`
	Arn                string            `json:"Arn"`
	EventBusName       string            `json:"EventBusName,omitempty"`
	EventPattern       string            `json:"EventPattern,omitempty"`
	ScheduleExpression string            `json:"ScheduleExpression,omitempty"`
	State              string            `json:"State"`
	Description        string            `json:"Description,omitempty"`
	RoleArn            string            `json:"RoleArn,omitempty"`
	Tags               map[string]string `json:"-"`
	CreatedAt          int64             `json:"-"`
}

type EBEventBus struct {
	Name             string            `json:"Name"`
	Arn              string            `json:"Arn"`
	Description      string            `json:"Description,omitempty"`
	Policy           string            `json:"Policy,omitempty"`
	CreationTime     int64             `json:"CreationTime,omitempty"`
	LastModifiedTime int64             `json:"LastModifiedTime,omitempty"`
	Tags             map[string]string `json:"-"`
}

type EBTarget struct {
	ID        string `json:"Id"`
	Arn       string `json:"Arn"`
	RoleArn   string `json:"RoleArn,omitempty"`
	Input     string `json:"Input,omitempty"`
	InputPath string `json:"InputPath,omitempty"`
	// Structured target parameters round-trip byte-exact: storing and
	// re-emitting the raw JSON preserves every sub-shape so ListTargetsByRule
	// returns what PutTargets received (terraform aws_cloudwatch_event_target
	// reads these back).
	EcsParameters        json.RawMessage `json:"EcsParameters,omitempty"`
	InputTransformer     json.RawMessage `json:"InputTransformer,omitempty"`
	RetryPolicy          json.RawMessage `json:"RetryPolicy,omitempty"`
	DeadLetterConfig     json.RawMessage `json:"DeadLetterConfig,omitempty"`
	SqsParameters        json.RawMessage `json:"SqsParameters,omitempty"`
	HttpParameters       json.RawMessage `json:"HttpParameters,omitempty"`
	BatchParameters      json.RawMessage `json:"BatchParameters,omitempty"`
	RunCommandParameters json.RawMessage `json:"RunCommandParameters,omitempty"`
	KinesisParameters    json.RawMessage `json:"KinesisParameters,omitempty"`
}

type EBEventRecord struct {
	ID         string   `json:"id"`
	Source     string   `json:"source"`
	DetailType string   `json:"detail-type"`
	Detail     string   `json:"detail,omitempty"`
	Time       int64    `json:"time"`
	Resources  []string `json:"resources,omitempty"`
}

type EBArchive struct {
	ArchiveName      string          `json:"ArchiveName"`
	ArchiveArn       string          `json:"ArchiveArn"`
	EventSourceArn   string          `json:"EventSourceArn"`
	Description      string          `json:"Description,omitempty"`
	EventPattern     string          `json:"EventPattern,omitempty"`
	RetentionDays    *int32          `json:"RetentionDays,omitempty"`
	State            string          `json:"State"`
	StateReason      string          `json:"StateReason,omitempty"`
	CreationTime     int64           `json:"CreationTime,omitempty"`
	EventCount       int64           `json:"EventCount"`
	SizeBytes        int64           `json:"SizeBytes"`
	ArchivedEvents   []EBEventRecord `json:"-"`
	KmsKeyIdentifier string          `json:"KmsKeyIdentifier,omitempty"`
}

type EBReplay struct {
	ReplayName              string         `json:"ReplayName"`
	ReplayArn               string         `json:"ReplayArn"`
	Description             string         `json:"Description,omitempty"`
	EventSourceArn          string         `json:"EventSourceArn"`
	EventStartTime          int64          `json:"EventStartTime,omitempty"`
	EventEndTime            int64          `json:"EventEndTime,omitempty"`
	EventLastReplayedTime   int64          `json:"EventLastReplayedTime,omitempty"`
	ReplayStartTime         int64          `json:"ReplayStartTime,omitempty"`
	ReplayEndTime           int64          `json:"ReplayEndTime,omitempty"`
	State                   string         `json:"State"`
	StateReason             string         `json:"StateReason,omitempty"`
	Destination             map[string]any `json:"-"`
	ReplayDestinationString string         `json:"-"`
}

var (
	ebBuses    sim.Store[EBEventBus]
	ebRules    sim.Store[EBRule]
	ebTargets  sim.Store[[]EBTarget]
	ebEvents   sim.Store[[]EBEventRecord]
	ebArchives sim.Store[EBArchive]
	ebReplays  sim.Store[EBReplay]
)

func registerEventBridge(r *sim.AWSRouter, srv *sim.Server) {
	ebBuses = sim.MakeStore[EBEventBus](srv.DB(), "eventbridge_buses")
	ebRules = sim.MakeStore[EBRule](srv.DB(), "eventbridge_rules")
	ebTargets = sim.MakeStore[[]EBTarget](srv.DB(), "eventbridge_targets")
	ebEvents = sim.MakeStore[[]EBEventRecord](srv.DB(), "eventbridge_events")
	ebArchives = sim.MakeStore[EBArchive](srv.DB(), "eventbridge_archives")
	ebReplays = sim.MakeStore[EBReplay](srv.DB(), "eventbridge_replays")

	r.Register("AWSEvents.CreateEventBus", handleEBCreateEventBus)
	r.Register("AWSEvents.DescribeEventBus", handleEBDescribeEventBus)
	r.Register("AWSEvents.ListEventBuses", handleEBListEventBuses)
	r.Register("AWSEvents.DeleteEventBus", handleEBDeleteEventBus)
	r.Register("AWSEvents.PutPermission", handleEBPutPermission)
	r.Register("AWSEvents.RemovePermission", handleEBRemovePermission)
	r.Register("AWSEvents.PutRule", handleEBPutRule)
	r.Register("AWSEvents.DescribeRule", handleEBDescribeRule)
	r.Register("AWSEvents.ListRules", handleEBListRules)
	r.Register("AWSEvents.DeleteRule", handleEBDeleteRule)
	r.Register("AWSEvents.EnableRule", handleEBEnableRule)
	r.Register("AWSEvents.DisableRule", handleEBDisableRule)
	r.Register("AWSEvents.PutTargets", handleEBPutTargets)
	r.Register("AWSEvents.ListTargetsByRule", handleEBListTargetsByRule)
	r.Register("AWSEvents.RemoveTargets", handleEBRemoveTargets)
	r.Register("AWSEvents.PutEvents", handleEBPutEvents)
	r.Register("AWSEvents.TagResource", handleEBTagResource)
	r.Register("AWSEvents.UntagResource", handleEBUntagResource)
	r.Register("AWSEvents.ListTagsForResource", handleEBListTagsForResource)
	r.Register("AWSEvents.CreateArchive", handleEBCreateArchive)
	r.Register("AWSEvents.DescribeArchive", handleEBDescribeArchive)
	r.Register("AWSEvents.ListArchives", handleEBListArchives)
	r.Register("AWSEvents.DeleteArchive", handleEBDeleteArchive)
	r.Register("AWSEvents.StartReplay", handleEBStartReplay)
	r.Register("AWSEvents.DescribeReplay", handleEBDescribeReplay)
	r.Register("AWSEvents.ListReplays", handleEBListReplays)
}

func ebRuleArn(name string) string {
	return fmt.Sprintf("arn:aws:events:%s:%s:rule/%s", awsRegion(), awsAccountID(), name)
}

func ebRuleArnForBus(bus, name string) string {
	if ebBusName(bus) == "default" {
		return ebRuleArn(name)
	}
	return fmt.Sprintf("arn:aws:events:%s:%s:rule/%s/%s", awsRegion(), awsAccountID(), ebBusName(bus), name)
}

func ebBusArn(name string) string {
	return fmt.Sprintf("arn:aws:events:%s:%s:event-bus/%s", awsRegion(), awsAccountID(), ebBusName(name))
}

func ebArchiveArn(name string) string {
	return fmt.Sprintf("arn:aws:events:%s:%s:archive/%s", awsRegion(), awsAccountID(), name)
}

func ebReplayArn(name string) string {
	return fmt.Sprintf("arn:aws:events:%s:%s:replay/%s", awsRegion(), awsAccountID(), name)
}

func ebBusName(name string) string {
	if name == "" {
		return "default"
	}
	return name
}

func ebRuleKey(bus, name string) string {
	return ebBusName(bus) + "/" + name
}

func ebDefaultBus() EBEventBus {
	now := time.Now().Unix()
	return EBEventBus{
		Name:             "default",
		Arn:              ebBusArn("default"),
		CreationTime:     now,
		LastModifiedTime: now,
	}
}

func ebGetBus(name string) (EBEventBus, bool) {
	busName := ebBusName(name)
	if busName == "default" {
		if bus, ok := ebBuses.Get("default"); ok {
			return bus, true
		}
		bus := ebDefaultBus()
		ebBuses.Put("default", bus)
		return bus, true
	}
	return ebBuses.Get(busName)
}

func ebPutBus(bus EBEventBus) {
	if bus.Name == "" {
		bus.Name = "default"
	}
	if bus.Arn == "" {
		bus.Arn = ebBusArn(bus.Name)
	}
	now := time.Now().Unix()
	if bus.CreationTime == 0 {
		bus.CreationTime = now
	}
	bus.LastModifiedTime = now
	ebBuses.Put(bus.Name, bus)
}

func writeEBJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func handleEBCreateEventBus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             string                        `json:"Name"`
		Description      string                        `json:"Description"`
		KmsKeyIdentifier string                        `json:"KmsKeyIdentifier"`
		Tags             []struct{ Key, Value string } `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Name == "default" {
		sim.AWSError(w, "ValidationException", "custom event bus name is required", http.StatusBadRequest)
		return
	}
	if _, ok := ebBuses.Get(req.Name); ok {
		sim.AWSError(w, "ResourceAlreadyExistsException", "Event bus already exists", http.StatusConflict)
		return
	}
	tags := map[string]string{}
	for _, tag := range req.Tags {
		tags[tag.Key] = tag.Value
	}
	bus := EBEventBus{
		Name:        req.Name,
		Arn:         ebBusArn(req.Name),
		Description: req.Description,
		Tags:        tags,
	}
	ebPutBus(bus)
	writeEBJSON(w, http.StatusOK, map[string]any{
		"EventBusArn":      bus.Arn,
		"Description":      req.Description,
		"KmsKeyIdentifier": req.KmsKeyIdentifier,
	})
}

func handleEBDescribeEventBus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"Name"`
	}
	_ = sim.ReadJSON(r, &req)
	bus, ok := ebGetBus(req.Name)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Event bus does not exist", http.StatusNotFound)
		return
	}
	writeEBJSON(w, http.StatusOK, bus)
}

func handleEBListEventBuses(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NamePrefix string `json:"NamePrefix"`
		Limit      int    `json:"Limit"`
		NextToken  string `json:"NextToken"`
	}
	_ = sim.ReadJSON(r, &req)
	_, _ = ebGetBus("default")
	buses := make([]EBEventBus, 0)
	for _, bus := range ebBuses.List() {
		if req.NamePrefix != "" && !strings.HasPrefix(bus.Name, req.NamePrefix) {
			continue
		}
		buses = append(buses, bus)
	}
	sort.Slice(buses, func(i, j int) bool { return buses[i].Name < buses[j].Name })
	page, next := awsPageExplicit(buses, req.NextToken, req.Limit)
	out := map[string]any{"EventBuses": page}
	if next != "" {
		out["NextToken"] = next
	}
	writeEBJSON(w, http.StatusOK, out)
}

func handleEBDeleteEventBus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"Name"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Name == "default" {
		sim.AWSError(w, "ValidationException", "default event bus cannot be deleted", http.StatusBadRequest)
		return
	}
	if !ebBuses.Delete(req.Name) {
		sim.AWSError(w, "ResourceNotFoundException", "Event bus does not exist", http.StatusNotFound)
		return
	}
	for _, rule := range ebRules.List() {
		if rule.EventBusName == req.Name {
			key := ebRuleKey(rule.EventBusName, rule.Name)
			ebRules.Delete(key)
			ebTargets.Delete(key)
		}
	}
	writeEBJSON(w, http.StatusOK, map[string]any{})
}

func handleEBPutPermission(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventBusName string         `json:"EventBusName"`
		StatementID  string         `json:"StatementId"`
		Action       string         `json:"Action"`
		Principal    string         `json:"Principal"`
		Policy       string         `json:"Policy"`
		Condition    map[string]any `json:"Condition"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	bus, ok := ebGetBus(req.EventBusName)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Event bus does not exist", http.StatusNotFound)
		return
	}
	if req.Policy != "" {
		bus.Policy = req.Policy
		ebPutBus(bus)
		writeEBJSON(w, http.StatusOK, map[string]any{})
		return
	}
	if req.StatementID == "" || req.Action == "" || req.Principal == "" {
		sim.AWSError(w, "ValidationException", "StatementId, Action, and Principal are required", http.StatusBadRequest)
		return
	}
	policy := ebPolicyDocument(bus.Policy)
	statements := ebPolicyStatements(policy)
	statement := map[string]any{
		"Sid":       req.StatementID,
		"Effect":    "Allow",
		"Principal": map[string]any{"AWS": req.Principal},
		"Action":    req.Action,
		"Resource":  bus.Arn,
	}
	if req.Condition != nil {
		statement["Condition"] = req.Condition
	}
	replaced := false
	for i, existing := range statements {
		if existing["Sid"] == req.StatementID {
			statements[i] = statement
			replaced = true
			break
		}
	}
	if !replaced {
		statements = append(statements, statement)
	}
	policy["Statement"] = statements
	body, _ := json.Marshal(policy)
	bus.Policy = string(body)
	ebPutBus(bus)
	writeEBJSON(w, http.StatusOK, map[string]any{})
}

func handleEBRemovePermission(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventBusName         string `json:"EventBusName"`
		StatementID          string `json:"StatementId"`
		RemoveAllPermissions bool   `json:"RemoveAllPermissions"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	bus, ok := ebGetBus(req.EventBusName)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Event bus does not exist", http.StatusNotFound)
		return
	}
	if req.RemoveAllPermissions {
		bus.Policy = ""
		ebPutBus(bus)
		writeEBJSON(w, http.StatusOK, map[string]any{})
		return
	}
	policy := ebPolicyDocument(bus.Policy)
	filtered := make([]map[string]any, 0)
	for _, statement := range ebPolicyStatements(policy) {
		if statement["Sid"] != req.StatementID {
			filtered = append(filtered, statement)
		}
	}
	if len(filtered) == 0 {
		bus.Policy = ""
	} else {
		policy["Statement"] = filtered
		body, _ := json.Marshal(policy)
		bus.Policy = string(body)
	}
	ebPutBus(bus)
	writeEBJSON(w, http.StatusOK, map[string]any{})
}

func ebPolicyDocument(raw string) map[string]any {
	policy := map[string]any{
		"Version":   "2012-10-17",
		"Statement": []map[string]any{},
	}
	if raw == "" {
		return policy
	}
	_ = json.Unmarshal([]byte(raw), &policy)
	if _, ok := policy["Statement"]; !ok {
		policy["Statement"] = []map[string]any{}
	}
	return policy
}

func ebPolicyStatements(policy map[string]any) []map[string]any {
	raw, ok := policy["Statement"]
	if !ok {
		return []map[string]any{}
	}
	switch v := raw.(type) {
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case map[string]any:
		return []map[string]any{v}
	default:
		return []map[string]any{}
	}
}

func handleEBPutRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name               string                        `json:"Name"`
		EventBusName       string                        `json:"EventBusName"`
		EventPattern       string                        `json:"EventPattern"`
		ScheduleExpression string                        `json:"ScheduleExpression"`
		State              string                        `json:"State"`
		Description        string                        `json:"Description"`
		RoleArn            string                        `json:"RoleArn"`
		Tags               []struct{ Key, Value string } `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		sim.AWSError(w, "ValidationException", "Name is required", http.StatusBadRequest)
		return
	}
	if _, ok := ebGetBus(req.EventBusName); !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Event bus does not exist", http.StatusNotFound)
		return
	}
	state := req.State
	if state == "" {
		state = "ENABLED"
	}
	key := ebRuleKey(req.EventBusName, req.Name)
	rule := EBRule{
		Name:               req.Name,
		Arn:                ebRuleArnForBus(req.EventBusName, req.Name),
		EventBusName:       ebBusName(req.EventBusName),
		EventPattern:       req.EventPattern,
		ScheduleExpression: req.ScheduleExpression,
		State:              state,
		Description:        req.Description,
		RoleArn:            req.RoleArn,
		CreatedAt:          time.Now().Unix(),
	}
	if existing, ok := ebRules.Get(key); ok {
		rule.Tags = existing.Tags
		rule.CreatedAt = existing.CreatedAt
	}
	if len(req.Tags) > 0 {
		rule.Tags = map[string]string{}
		for _, tag := range req.Tags {
			rule.Tags[tag.Key] = tag.Value
		}
	}
	ebRules.Put(key, rule)
	writeEBJSON(w, http.StatusOK, map[string]string{"RuleArn": rule.Arn})
}

func handleEBDescribeRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"Name"`
		EventBusName string `json:"EventBusName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	rule, ok := ebRules.Get(ebRuleKey(req.EventBusName, req.Name))
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Rule does not exist", http.StatusNotFound)
		return
	}
	writeEBJSON(w, http.StatusOK, rule)
}

func handleEBListRules(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NamePrefix   string `json:"NamePrefix"`
		EventBusName string `json:"EventBusName"`
		Limit        int    `json:"Limit"`
		NextToken    string `json:"NextToken"`
	}
	_ = sim.ReadJSON(r, &req)
	bus := ebBusName(req.EventBusName)
	rules := make([]EBRule, 0)
	for _, rule := range ebRules.List() {
		if rule.EventBusName != bus {
			continue
		}
		if req.NamePrefix != "" && !strings.HasPrefix(rule.Name, req.NamePrefix) {
			continue
		}
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
	page, next := awsPageExplicit(rules, req.NextToken, req.Limit)
	out := map[string]any{"Rules": page}
	if next != "" {
		out["NextToken"] = next
	}
	writeEBJSON(w, http.StatusOK, out)
}

func handleEBDeleteRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"Name"`
		EventBusName string `json:"EventBusName"`
		Force        bool   `json:"Force"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	key := ebRuleKey(req.EventBusName, req.Name)
	if _, ok := ebRules.Get(key); !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Rule does not exist", http.StatusNotFound)
		return
	}
	if targets, ok := ebTargets.Get(key); ok && len(targets) > 0 && !req.Force {
		sim.AWSError(w, "ConcurrentModificationException", "Rule has targets", http.StatusConflict)
		return
	}
	ebRules.Delete(key)
	ebTargets.Delete(key)
	writeEBJSON(w, http.StatusOK, map[string]any{})
}

func handleEBEnableRule(w http.ResponseWriter, r *http.Request)  { ebSetRuleState(w, r, "ENABLED") }
func handleEBDisableRule(w http.ResponseWriter, r *http.Request) { ebSetRuleState(w, r, "DISABLED") }

func ebSetRuleState(w http.ResponseWriter, r *http.Request, state string) {
	var req struct {
		Name         string `json:"Name"`
		EventBusName string `json:"EventBusName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	key := ebRuleKey(req.EventBusName, req.Name)
	if !ebRules.Update(key, func(rule *EBRule) { rule.State = state }) {
		sim.AWSError(w, "ResourceNotFoundException", "Rule does not exist", http.StatusNotFound)
		return
	}
	writeEBJSON(w, http.StatusOK, map[string]any{})
}

func handleEBPutTargets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rule         string     `json:"Rule"`
		EventBusName string     `json:"EventBusName"`
		Targets      []EBTarget `json:"Targets"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	key := ebRuleKey(req.EventBusName, req.Rule)
	if _, ok := ebRules.Get(key); !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Rule does not exist", http.StatusNotFound)
		return
	}
	existing, _ := ebTargets.Get(key)
	byID := map[string]EBTarget{}
	for _, target := range existing {
		byID[target.ID] = target
	}
	for _, target := range req.Targets {
		if target.ID == "" || target.Arn == "" {
			writeEBJSON(w, http.StatusOK, map[string]any{
				"FailedEntryCount": 1,
				"FailedEntries": []map[string]string{{
					"TargetId":     target.ID,
					"ErrorCode":    "ValidationException",
					"ErrorMessage": "Target Id and Arn are required",
				}},
			})
			return
		}
		byID[target.ID] = target
	}
	out := make([]EBTarget, 0, len(byID))
	for _, target := range byID {
		out = append(out, target)
	}
	ebTargets.Put(key, out)
	writeEBJSON(w, http.StatusOK, map[string]any{"FailedEntryCount": 0, "FailedEntries": []any{}})
}

func handleEBListTargetsByRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rule         string `json:"Rule"`
		EventBusName string `json:"EventBusName"`
		Limit        int    `json:"Limit"`
		NextToken    string `json:"NextToken"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	key := ebRuleKey(req.EventBusName, req.Rule)
	if _, ok := ebRules.Get(key); !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Rule does not exist", http.StatusNotFound)
		return
	}
	targets, _ := ebTargets.Get(key)
	if targets == nil {
		targets = []EBTarget{}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })
	page, next := awsPageExplicit(targets, req.NextToken, req.Limit)
	out := map[string]any{"Targets": page}
	if next != "" {
		out["NextToken"] = next
	}
	writeEBJSON(w, http.StatusOK, out)
}

func handleEBRemoveTargets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rule         string   `json:"Rule"`
		EventBusName string   `json:"EventBusName"`
		Ids          []string `json:"Ids"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	key := ebRuleKey(req.EventBusName, req.Rule)
	targets, _ := ebTargets.Get(key)
	remove := map[string]bool{}
	for _, id := range req.Ids {
		remove[id] = true
	}
	out := targets[:0]
	for _, target := range targets {
		if !remove[target.ID] {
			out = append(out, target)
		}
	}
	ebTargets.Put(key, out)
	writeEBJSON(w, http.StatusOK, map[string]any{"FailedEntryCount": 0, "FailedEntries": []any{}})
}

func handleEBPutEvents(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Entries []struct {
			Source       string   `json:"Source"`
			DetailType   string   `json:"DetailType"`
			Detail       string   `json:"Detail"`
			EventBusName string   `json:"EventBusName"`
			Resources    []string `json:"Resources"`
			Time         float64  `json:"Time"`
		} `json:"Entries"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	entries := make([]map[string]string, 0, len(req.Entries))
	failed := 0
	for _, entry := range req.Entries {
		eventID := generateUUID()
		now := time.Now().Unix()
		record := EBEventRecord{
			ID:         eventID,
			Source:     entry.Source,
			DetailType: entry.DetailType,
			Detail:     entry.Detail,
			Time:       now,
			Resources:  entry.Resources,
		}
		bus := ebBusName(entry.EventBusName)
		if _, ok := ebGetBus(bus); !ok {
			failed++
			entries = append(entries, map[string]string{
				"ErrorCode":    "ResourceNotFoundException",
				"ErrorMessage": "Event bus does not exist",
			})
			continue
		}
		events, _ := ebEvents.Get(bus)
		events = append(events, record)
		ebEvents.Put(bus, events)
		archiveEBEvent(bus, record)
		deliverEBEvent(bus, entry.Source, entry.DetailType, entry.Detail, eventID)
		entries = append(entries, map[string]string{"EventId": eventID})
	}
	writeEBJSON(w, http.StatusOK, map[string]any{"FailedEntryCount": failed, "Entries": entries})
}

func archiveEBEvent(bus string, record EBEventRecord) {
	sourceArn := ebBusArn(bus)
	for _, archive := range ebArchives.List() {
		if archive.EventSourceArn != sourceArn || archive.State != "ENABLED" {
			continue
		}
		if archive.EventPattern != "" && !ebEventPatternMatches(archive.EventPattern, record.Source, record.DetailType) {
			continue
		}
		archive.ArchivedEvents = append(archive.ArchivedEvents, record)
		archive.EventCount = int64(len(archive.ArchivedEvents))
		archive.SizeBytes += int64(len(record.Detail))
		ebArchives.Put(archive.ArchiveName, archive)
	}
}

func deliverEBEvent(bus, source, detailType, detail, eventID string) {
	for _, rule := range ebRules.List() {
		if rule.EventBusName != bus || rule.State == "DISABLED" {
			continue
		}
		if !ebRuleMatches(rule, source, detailType) {
			continue
		}
		targets, _ := ebTargets.Get(ebRuleKey(rule.EventBusName, rule.Name))
		for _, target := range targets {
			body := detail
			if target.Input != "" {
				body = target.Input
			}
			deliverEBTarget(target, body, source, detailType, eventID)
		}
	}
}

func ebRuleMatches(rule EBRule, source, detailType string) bool {
	if rule.EventPattern == "" {
		return true
	}
	return ebEventPatternMatches(rule.EventPattern, source, detailType)
}

func ebEventPatternMatches(patternJSON, source, detailType string) bool {
	var pattern map[string][]string
	if err := json.Unmarshal([]byte(patternJSON), &pattern); err != nil {
		return false
	}
	if allowed := pattern["source"]; len(allowed) > 0 && !stringIn(source, allowed) {
		return false
	}
	if allowed := pattern["detail-type"]; len(allowed) > 0 && !stringIn(detailType, allowed) {
		return false
	}
	return true
}

func stringIn(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func deliverEBTarget(target EBTarget, body, source, detailType, eventID string) {
	if strings.HasPrefix(target.Arn, "arn:aws:sqs:") {
		queue := snsTopicNameFromARN(target.Arn)
		hash := md5.Sum([]byte(body))
		sqsQueues.Update(queue, func(q *SQSQueue) {
			q.Messages = append(q.Messages, SQSMessage{
				MessageId:     eventID,
				Body:          body,
				MD5OfBody:     hex.EncodeToString(hash[:]),
				SentTimestamp: time.Now().Unix(),
			})
		})
		return
	}
	if strings.HasPrefix(target.Arn, "arn:aws:sns:") {
		name := snsTopicNameFromARN(target.Arn)
		if _, ok := snsTopics.Get(name); !ok {
			return
		}
		msgID := eventID
		for _, sub := range snsSubscriptions.List() {
			if sub.TopicARN != target.Arn || sub.Protocol != "sqs" {
				continue
			}
			queue := snsTopicNameFromARN(sub.Endpoint)
			envelope := fmt.Sprintf(
				`{"Type":"Notification","MessageId":%q,"TopicArn":%q,"Subject":%q,"Message":%q}`,
				msgID, target.Arn, detailType, body)
			hash := md5.Sum([]byte(envelope))
			sqsQueues.Update(queue, func(q *SQSQueue) {
				q.Messages = append(q.Messages, SQSMessage{
					MessageId:     generateUUID(),
					Body:          envelope,
					MD5OfBody:     hex.EncodeToString(hash[:]),
					SentTimestamp: time.Now().Unix(),
				})
			})
		}
	}
	_, _, _ = source, detailType, eventID
}

func handleEBCreateArchive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ArchiveName      string `json:"ArchiveName"`
		EventSourceArn   string `json:"EventSourceArn"`
		Description      string `json:"Description"`
		EventPattern     string `json:"EventPattern"`
		RetentionDays    *int32 `json:"RetentionDays"`
		KmsKeyIdentifier string `json:"KmsKeyIdentifier"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ArchiveName == "" || req.EventSourceArn == "" {
		sim.AWSError(w, "ValidationException", "ArchiveName and EventSourceArn are required", http.StatusBadRequest)
		return
	}
	if _, ok := ebArchives.Get(req.ArchiveName); ok {
		sim.AWSError(w, "ResourceAlreadyExistsException", "Archive already exists", http.StatusConflict)
		return
	}
	if _, ok := ebBusByARN(req.EventSourceArn); !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Event source bus does not exist", http.StatusNotFound)
		return
	}
	now := time.Now().Unix()
	archive := EBArchive{
		ArchiveName:      req.ArchiveName,
		ArchiveArn:       ebArchiveArn(req.ArchiveName),
		EventSourceArn:   req.EventSourceArn,
		Description:      req.Description,
		EventPattern:     req.EventPattern,
		RetentionDays:    req.RetentionDays,
		State:            "ENABLED",
		CreationTime:     now,
		KmsKeyIdentifier: req.KmsKeyIdentifier,
	}
	ebArchives.Put(archive.ArchiveName, archive)
	writeEBJSON(w, http.StatusOK, map[string]any{
		"ArchiveArn":   archive.ArchiveArn,
		"CreationTime": archive.CreationTime,
		"State":        archive.State,
	})
}

func handleEBDescribeArchive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ArchiveName string `json:"ArchiveName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	archive, ok := ebArchives.Get(req.ArchiveName)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Archive does not exist", http.StatusNotFound)
		return
	}
	writeEBJSON(w, http.StatusOK, archive)
}

func handleEBListArchives(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventSourceArn string `json:"EventSourceArn"`
		NamePrefix     string `json:"NamePrefix"`
		Limit          int    `json:"Limit"`
		NextToken      string `json:"NextToken"`
	}
	_ = sim.ReadJSON(r, &req)
	archives := make([]EBArchive, 0)
	for _, archive := range ebArchives.List() {
		if req.EventSourceArn != "" && archive.EventSourceArn != req.EventSourceArn {
			continue
		}
		if req.NamePrefix != "" && !strings.HasPrefix(archive.ArchiveName, req.NamePrefix) {
			continue
		}
		archives = append(archives, archive)
	}
	sort.Slice(archives, func(i, j int) bool { return archives[i].ArchiveName < archives[j].ArchiveName })
	page, next := awsPageExplicit(archives, req.NextToken, req.Limit)
	summaries := make([]map[string]any, 0, len(page))
	for _, archive := range page {
		summaries = append(summaries, ebArchiveSummary(archive))
	}
	out := map[string]any{"Archives": summaries}
	if next != "" {
		out["NextToken"] = next
	}
	writeEBJSON(w, http.StatusOK, out)
}

// ebArchiveSummary projects an archive onto the list-shape Archive
// members; ArchiveArn / Description / EventPattern / KmsKeyIdentifier
// are describe-only and must not appear in ListArchives entries.
func ebArchiveSummary(a EBArchive) map[string]any {
	out := map[string]any{
		"ArchiveName":    a.ArchiveName,
		"EventSourceArn": a.EventSourceArn,
		"State":          a.State,
		"EventCount":     a.EventCount,
		"SizeBytes":      a.SizeBytes,
	}
	if a.StateReason != "" {
		out["StateReason"] = a.StateReason
	}
	if a.RetentionDays != nil {
		out["RetentionDays"] = *a.RetentionDays
	}
	if a.CreationTime != 0 {
		out["CreationTime"] = a.CreationTime
	}
	return out
}

func handleEBDeleteArchive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ArchiveName string `json:"ArchiveName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if !ebArchives.Delete(req.ArchiveName) {
		sim.AWSError(w, "ResourceNotFoundException", "Archive does not exist", http.StatusNotFound)
		return
	}
	writeEBJSON(w, http.StatusOK, map[string]any{})
}

func handleEBStartReplay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReplayName     string          `json:"ReplayName"`
		Description    string          `json:"Description"`
		EventSourceArn string          `json:"EventSourceArn"`
		EventStartTime json.RawMessage `json:"EventStartTime"`
		EventEndTime   json.RawMessage `json:"EventEndTime"`
		Destination    map[string]any  `json:"Destination"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ReplayName == "" || req.EventSourceArn == "" || req.Destination == nil {
		sim.AWSError(w, "ValidationException", "ReplayName, EventSourceArn, and Destination are required", http.StatusBadRequest)
		return
	}
	archive, ok := ebArchiveByARN(req.EventSourceArn)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Archive does not exist", http.StatusNotFound)
		return
	}
	startTime, err := ebParseJSONTime(req.EventStartTime)
	if err != nil {
		sim.AWSError(w, "ValidationException", "EventStartTime is invalid", http.StatusBadRequest)
		return
	}
	endTime, err := ebParseJSONTime(req.EventEndTime)
	if err != nil {
		sim.AWSError(w, "ValidationException", "EventEndTime is invalid", http.StatusBadRequest)
		return
	}
	now := time.Now().Unix()
	replay := EBReplay{
		ReplayName:            req.ReplayName,
		ReplayArn:             ebReplayArn(req.ReplayName),
		Description:           req.Description,
		EventSourceArn:        req.EventSourceArn,
		EventStartTime:        startTime,
		EventEndTime:          endTime,
		ReplayStartTime:       now,
		ReplayEndTime:         now,
		EventLastReplayedTime: endTime,
		State:                 "COMPLETED",
		Destination:           req.Destination,
	}
	if arn, _ := req.Destination["Arn"].(string); arn != "" {
		replayArchivedEvents(archive, arn)
	}
	ebReplays.Put(replay.ReplayName, replay)
	writeEBJSON(w, http.StatusOK, map[string]any{
		"ReplayArn":       replay.ReplayArn,
		"ReplayStartTime": replay.ReplayStartTime,
		"State":           replay.State,
	})
}

func handleEBDescribeReplay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReplayName string `json:"ReplayName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	replay, ok := ebReplays.Get(req.ReplayName)
	if !ok {
		sim.AWSError(w, "ResourceNotFoundException", "Replay does not exist", http.StatusNotFound)
		return
	}
	writeEBJSON(w, http.StatusOK, replay)
}

func handleEBListReplays(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventSourceArn string `json:"EventSourceArn"`
		NamePrefix     string `json:"NamePrefix"`
		Limit          int    `json:"Limit"`
		NextToken      string `json:"NextToken"`
	}
	_ = sim.ReadJSON(r, &req)
	replays := make([]EBReplay, 0)
	for _, replay := range ebReplays.List() {
		if req.EventSourceArn != "" && replay.EventSourceArn != req.EventSourceArn {
			continue
		}
		if req.NamePrefix != "" && !strings.HasPrefix(replay.ReplayName, req.NamePrefix) {
			continue
		}
		replays = append(replays, replay)
	}
	sort.Slice(replays, func(i, j int) bool { return replays[i].ReplayName < replays[j].ReplayName })
	page, next := awsPageExplicit(replays, req.NextToken, req.Limit)
	summaries := make([]map[string]any, 0, len(page))
	for _, replay := range page {
		summaries = append(summaries, ebReplaySummary(replay))
	}
	out := map[string]any{"Replays": summaries}
	if next != "" {
		out["NextToken"] = next
	}
	writeEBJSON(w, http.StatusOK, out)
}

// ebReplaySummary projects a replay onto the list-shape Replay members;
// ReplayArn / Description are describe-only and must not appear in
// ListReplays entries.
func ebReplaySummary(rp EBReplay) map[string]any {
	out := map[string]any{
		"ReplayName":     rp.ReplayName,
		"EventSourceArn": rp.EventSourceArn,
		"State":          rp.State,
	}
	if rp.StateReason != "" {
		out["StateReason"] = rp.StateReason
	}
	for k, v := range map[string]int64{
		"EventStartTime":        rp.EventStartTime,
		"EventEndTime":          rp.EventEndTime,
		"EventLastReplayedTime": rp.EventLastReplayedTime,
		"ReplayStartTime":       rp.ReplayStartTime,
		"ReplayEndTime":         rp.ReplayEndTime,
	} {
		if v != 0 {
			out[k] = v
		}
	}
	return out
}

func ebParseJSONTime(raw json.RawMessage) (int64, error) {
	var seconds float64
	if err := json.Unmarshal(raw, &seconds); err == nil {
		return int64(seconds), nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, err
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return 0, err
	}
	return parsed.Unix(), nil
}

func ebBusByARN(arn string) (EBEventBus, bool) {
	for _, bus := range ebBuses.List() {
		if bus.Arn == arn {
			return bus, true
		}
	}
	if arn == ebBusArn("default") {
		return ebGetBus("default")
	}
	return EBEventBus{}, false
}

func ebArchiveByARN(arn string) (EBArchive, bool) {
	for _, archive := range ebArchives.List() {
		if archive.ArchiveArn == arn {
			return archive, true
		}
	}
	return EBArchive{}, false
}

func replayArchivedEvents(archive EBArchive, destinationBusArn string) {
	bus, ok := ebBusByARN(destinationBusArn)
	if !ok {
		return
	}
	for _, event := range archive.ArchivedEvents {
		deliverEBEvent(bus.Name, event.Source, event.DetailType, event.Detail, event.ID)
	}
}

func handleEBTagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceARN string                        `json:"ResourceARN"`
		Tags        []struct{ Key, Value string } `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if ebBusByARNUpdate(req.ResourceARN, func(bus *EBEventBus) {
		if bus.Tags == nil {
			bus.Tags = map[string]string{}
		}
		for _, tag := range req.Tags {
			bus.Tags[tag.Key] = tag.Value
		}
	}) {
		writeEBJSON(w, http.StatusOK, map[string]any{})
		return
	}
	if !ebRuleByARNUpdate(req.ResourceARN, func(rule *EBRule) {
		if rule.Tags == nil {
			rule.Tags = map[string]string{}
		}
		for _, tag := range req.Tags {
			rule.Tags[tag.Key] = tag.Value
		}
	}) {
		sim.AWSError(w, "ResourceNotFoundException", "Resource does not exist", http.StatusNotFound)
		return
	}
	writeEBJSON(w, http.StatusOK, map[string]any{})
}

func handleEBUntagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceARN string   `json:"ResourceARN"`
		TagKeys     []string `json:"TagKeys"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if ebBusByARNUpdate(req.ResourceARN, func(bus *EBEventBus) {
		for _, key := range req.TagKeys {
			delete(bus.Tags, key)
		}
	}) {
		writeEBJSON(w, http.StatusOK, map[string]any{})
		return
	}
	if !ebRuleByARNUpdate(req.ResourceARN, func(rule *EBRule) {
		for _, key := range req.TagKeys {
			delete(rule.Tags, key)
		}
	}) {
		sim.AWSError(w, "ResourceNotFoundException", "Resource does not exist", http.StatusNotFound)
		return
	}
	writeEBJSON(w, http.StatusOK, map[string]any{})
}

func handleEBListTagsForResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceARN string `json:"ResourceARN"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
		return
	}
	for _, bus := range ebBuses.List() {
		if bus.Arn != req.ResourceARN {
			continue
		}
		writeEBJSON(w, http.StatusOK, map[string]any{"Tags": ebTagsList(bus.Tags)})
		return
	}
	for _, rule := range ebRules.List() {
		if rule.Arn != req.ResourceARN {
			continue
		}
		writeEBJSON(w, http.StatusOK, map[string]any{"Tags": ebTagsList(rule.Tags)})
		return
	}
	sim.AWSError(w, "ResourceNotFoundException", "Resource does not exist", http.StatusNotFound)
}

func ebTagsList(tags map[string]string) []map[string]string {
	out := make([]map[string]string, 0, len(tags))
	for k, v := range tags {
		out = append(out, map[string]string{"Key": k, "Value": v})
	}
	return out
}

func ebRuleByARNUpdate(arn string, fn func(*EBRule)) bool {
	for _, rule := range ebRules.List() {
		if rule.Arn == arn {
			return ebRules.Update(ebRuleKey(rule.EventBusName, rule.Name), fn)
		}
	}
	return false
}

func ebBusByARNUpdate(arn string, fn func(*EBEventBus)) bool {
	for _, bus := range ebBuses.List() {
		if bus.Arn == arn {
			fn(&bus)
			ebPutBus(bus)
			return true
		}
	}
	return false
}
