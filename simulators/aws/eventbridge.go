package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
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

type EBTarget struct {
	ID        string         `json:"Id"`
	Arn       string         `json:"Arn"`
	RoleArn   string         `json:"RoleArn,omitempty"`
	Input     string         `json:"Input,omitempty"`
	InputPath string         `json:"InputPath,omitempty"`
	Extra     map[string]any `json:"-"`
}

type EBEventRecord struct {
	ID         string         `json:"id"`
	Source     string         `json:"source"`
	DetailType string         `json:"detail-type"`
	Detail     map[string]any `json:"detail,omitempty"`
	Time       string         `json:"time"`
	Resources  []string       `json:"resources,omitempty"`
}

var (
	ebRules   sim.Store[EBRule]
	ebTargets sim.Store[[]EBTarget]
	ebEvents  sim.Store[[]EBEventRecord]
)

func registerEventBridge(r *sim.AWSRouter, srv *sim.Server) {
	ebRules = sim.MakeStore[EBRule](srv.DB(), "eventbridge_rules")
	ebTargets = sim.MakeStore[[]EBTarget](srv.DB(), "eventbridge_targets")
	ebEvents = sim.MakeStore[[]EBEventRecord](srv.DB(), "eventbridge_events")

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
}

func ebRuleArn(name string) string {
	return fmt.Sprintf("arn:aws:events:%s:%s:rule/%s", awsRegion(), awsAccountID(), name)
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

func writeEBJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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
	state := req.State
	if state == "" {
		state = "ENABLED"
	}
	key := ebRuleKey(req.EventBusName, req.Name)
	rule := EBRule{
		Name:               req.Name,
		Arn:                ebRuleArn(req.Name),
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
	writeEBJSON(w, http.StatusOK, map[string]any{"Rules": rules})
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
	writeEBJSON(w, http.StatusOK, map[string]any{"Targets": targets})
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
	for _, entry := range req.Entries {
		eventID := generateUUID()
		detail := map[string]any{}
		if entry.Detail != "" {
			_ = json.Unmarshal([]byte(entry.Detail), &detail)
		}
		record := EBEventRecord{
			ID:         eventID,
			Source:     entry.Source,
			DetailType: entry.DetailType,
			Detail:     detail,
			Time:       time.Now().UTC().Format(time.RFC3339Nano),
			Resources:  entry.Resources,
		}
		bus := ebBusName(entry.EventBusName)
		events, _ := ebEvents.Get(bus)
		events = append(events, record)
		ebEvents.Put(bus, events)
		deliverEBEvent(bus, entry.Source, entry.DetailType, entry.Detail, eventID)
		entries = append(entries, map[string]string{"EventId": eventID})
	}
	writeEBJSON(w, http.StatusOK, map[string]any{"FailedEntryCount": 0, "Entries": entries})
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
	var pattern map[string][]string
	if err := json.Unmarshal([]byte(rule.EventPattern), &pattern); err != nil {
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

func handleEBTagResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceARN string                        `json:"ResourceARN"`
		Tags        []struct{ Key, Value string } `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "ValidationException", "Invalid request body", http.StatusBadRequest)
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
	for _, rule := range ebRules.List() {
		if rule.Arn != req.ResourceARN {
			continue
		}
		tags := make([]map[string]string, 0, len(rule.Tags))
		for k, v := range rule.Tags {
			tags = append(tags, map[string]string{"Key": k, "Value": v})
		}
		writeEBJSON(w, http.StatusOK, map[string]any{"Tags": tags})
		return
	}
	sim.AWSError(w, "ResourceNotFoundException", "Resource does not exist", http.StatusNotFound)
}

func ebRuleByARNUpdate(arn string, fn func(*EBRule)) bool {
	for _, rule := range ebRules.List() {
		if rule.Arn == arn {
			return ebRules.Update(ebRuleKey(rule.EventBusName, rule.Name), fn)
		}
	}
	return false
}
