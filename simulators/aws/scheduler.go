package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// EventBridge Scheduler — terraform-provider-aws declares `aws_scheduler_schedule`
// (and `aws_scheduler_schedule_group`) for cron/rate-driven task invocation. This
// is a separate service from EventBridge rules/buses (eventbridge.go): it speaks
// the REST-JSON protocol over `scheduler.amazonaws.com` with path-addressed
// resources (`/schedules/{Name}`), not the X-Amz-Target JSON protocol. Names live
// in the path; the schedule group is a query parameter (`groupName`) or, for
// ListSchedules, `ScheduleGroup`.

// Schedule is a single scheduled invocation. Target and FlexibleTimeWindow are
// held as raw JSON so they round-trip byte-exact through Get/Update.
type Schedule struct {
	Name                       string          `json:"Name"`
	GroupName                  string          `json:"GroupName"`
	Arn                        string          `json:"Arn"`
	ScheduleExpression         string          `json:"ScheduleExpression"`
	ScheduleExpressionTimezone string          `json:"ScheduleExpressionTimezone,omitempty"`
	Description                string          `json:"Description,omitempty"`
	State                      string          `json:"State"`
	KmsKeyArn                  string          `json:"KmsKeyArn,omitempty"`
	StartDate                  *float64        `json:"StartDate,omitempty"`
	EndDate                    *float64        `json:"EndDate,omitempty"`
	ActionAfterCompletion      string          `json:"ActionAfterCompletion,omitempty"`
	Target                     json.RawMessage `json:"Target,omitempty"`
	FlexibleTimeWindow         json.RawMessage `json:"FlexibleTimeWindow,omitempty"`
	CreationDate               float64         `json:"CreationDate"`
	LastModificationDate       float64         `json:"LastModificationDate"`
}

// ScheduleGroup is a namespace for schedules. The "default" group always exists.
type ScheduleGroup struct {
	Name                 string  `json:"Name"`
	Arn                  string  `json:"Arn"`
	State                string  `json:"State"`
	CreationDate         float64 `json:"CreationDate"`
	LastModificationDate float64 `json:"LastModificationDate"`
}

var (
	schedules      sim.Store[Schedule]
	scheduleGroups sim.Store[ScheduleGroup]
	// schedulesMu guards reassignment of the package-level schedules store
	// (registration) against the once-per-second firing-loop goroutine that
	// reads it — they live in different goroutines when several sims are built
	// in one process (tests).
	schedulesMu sync.RWMutex
)

// schedulerStore returns the current schedules store under the read lock; the
// firing loop uses it so a concurrent re-registration can't race the read.
func schedulerStore() sim.Store[Schedule] {
	schedulesMu.RLock()
	defer schedulesMu.RUnlock()
	return schedules
}

func registerScheduler(srv *sim.Server) {
	schedulesMu.Lock()
	schedules = sim.MakeStore[Schedule](srv.DB(), "scheduler_schedules")
	scheduleGroups = sim.MakeStore[ScheduleGroup](srv.DB(), "scheduler_schedule_groups")
	schedulesMu.Unlock()

	srv.HandleFunc("POST /schedules/{Name}", schedulerRecorded("CreateSchedule", handleSchedulerCreateSchedule))
	srv.HandleFunc("GET /schedules/{Name}", schedulerRecorded("GetSchedule", handleSchedulerGetSchedule))
	srv.HandleFunc("PUT /schedules/{Name}", schedulerRecorded("UpdateSchedule", handleSchedulerUpdateSchedule))
	srv.HandleFunc("DELETE /schedules/{Name}", schedulerRecorded("DeleteSchedule", handleSchedulerDeleteSchedule))
	srv.HandleFunc("GET /schedules", schedulerRecorded("ListSchedules", handleSchedulerListSchedules))

	srv.HandleFunc("POST /schedule-groups/{Name}", schedulerRecorded("CreateScheduleGroup", handleSchedulerCreateScheduleGroup))
	srv.HandleFunc("GET /schedule-groups/{Name}", schedulerRecorded("GetScheduleGroup", handleSchedulerGetScheduleGroup))
	srv.HandleFunc("DELETE /schedule-groups/{Name}", schedulerRecorded("DeleteScheduleGroup", handleSchedulerDeleteScheduleGroup))
	srv.HandleFunc("GET /schedule-groups", schedulerRecorded("ListScheduleGroups", handleSchedulerListScheduleGroups))

	// Evaluate ScheduleExpressions and invoke due targets (ECS/Lambda/SQS/SNS).
	startSchedulerFiringLoop()
}

// schedulerRecorded wraps a Scheduler REST handler so its API call is recorded
// in CloudTrail. Scheduler is a REST/JSON service registered directly on the
// server mux (path-addressed, no X-Amz-Target), so it bypasses the central
// `POST /` recording middleware — every Scheduler operation must record itself
// so real CloudTrail-style lookup surfaces Scheduler API calls against
// scheduler.amazonaws.com.
func schedulerRecorded(eventName string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec := &cloudTrailStatusRecorder{ResponseWriter: w}
		h(rec, r)
		if rec.statusCode() >= 500 {
			return
		}
		var resources []CloudTrailResource
		if name := r.PathValue("Name"); name != "" {
			typ := "AWS::Scheduler::Schedule"
			if strings.Contains(r.URL.Path, "/schedule-groups/") {
				typ = "AWS::Scheduler::ScheduleGroup"
			}
			resources = []CloudTrailResource{{ResourceType: typ, ResourceName: name}}
		}
		cloudTrailRecord(CloudTrailEvent{
			EventName:   eventName,
			EventSource: "scheduler.amazonaws.com",
			AccessKeyId: cloudTrailAccessKeyID(r),
			ReadOnly:    cloudTrailReadOnly(eventName),
			Resources:   resources,
		})
	}
}

func schedulerScheduleARN(group, name string) string {
	return fmt.Sprintf("arn:aws:scheduler:%s:%s:schedule/%s/%s", awsRegion(), awsAccountID(), group, name)
}

func schedulerGroupARN(name string) string {
	return fmt.Sprintf("arn:aws:scheduler:%s:%s:schedule-group/%s", awsRegion(), awsAccountID(), name)
}

func scheduleKey(group, name string) string { return group + "/" + name }

// schedulerError writes a restJson1 typed error: the SDK reads the exception
// shape from the X-Amzn-Errortype header and the human message from the body.
func schedulerError(w http.ResponseWriter, errType string, status int, format string, args ...any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Amzn-Errortype", errType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"Message": fmt.Sprintf(format, args...)})
}

// scheduleGroupParam returns the group name from a query param, defaulting to
// "default" (the always-present group) when unset.
func scheduleGroupParam(r *http.Request, queryName string) string {
	if g := r.URL.Query().Get(queryName); g != "" {
		return g
	}
	return "default"
}

func handleSchedulerCreateSchedule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("Name")
	var req struct {
		GroupName                  string          `json:"GroupName"`
		ScheduleExpression         string          `json:"ScheduleExpression"`
		ScheduleExpressionTimezone string          `json:"ScheduleExpressionTimezone"`
		Description                string          `json:"Description"`
		State                      string          `json:"State"`
		KmsKeyArn                  string          `json:"KmsKeyArn"`
		StartDate                  *float64        `json:"StartDate"`
		EndDate                    *float64        `json:"EndDate"`
		ActionAfterCompletion      string          `json:"ActionAfterCompletion"`
		Target                     json.RawMessage `json:"Target"`
		FlexibleTimeWindow         json.RawMessage `json:"FlexibleTimeWindow"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		schedulerError(w, "ValidationException", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	group := req.GroupName
	if group == "" {
		group = "default"
	}
	if req.ScheduleExpression == "" {
		schedulerError(w, "ValidationException", http.StatusBadRequest, "ScheduleExpression is required")
		return
	}
	// A malformed cron(...) is rejected here (as real AWS does) rather than
	// accepted into a schedule that would silently never fire.
	if strings.HasPrefix(strings.TrimSpace(req.ScheduleExpression), "cron(") &&
		!schedulerCronValid(req.ScheduleExpression) {
		schedulerError(w, "ValidationException", http.StatusBadRequest,
			"Invalid Schedule Expression %q.", req.ScheduleExpression)
		return
	}
	if req.Target == nil {
		schedulerError(w, "ValidationException", http.StatusBadRequest, "Target is required")
		return
	}
	if req.FlexibleTimeWindow == nil {
		schedulerError(w, "ValidationException", http.StatusBadRequest, "FlexibleTimeWindow is required")
		return
	}
	key := scheduleKey(group, name)
	if _, exists := schedules.Get(key); exists {
		schedulerError(w, "ConflictException", http.StatusConflict,
			"Schedule %q already exists in group %q", name, group)
		return
	}
	state := req.State
	if state == "" {
		state = "ENABLED"
	}
	now := float64(time.Now().Unix())
	sched := Schedule{
		Name:                       name,
		GroupName:                  group,
		Arn:                        schedulerScheduleARN(group, name),
		ScheduleExpression:         req.ScheduleExpression,
		ScheduleExpressionTimezone: req.ScheduleExpressionTimezone,
		Description:                req.Description,
		State:                      state,
		KmsKeyArn:                  req.KmsKeyArn,
		StartDate:                  req.StartDate,
		EndDate:                    req.EndDate,
		ActionAfterCompletion:      req.ActionAfterCompletion,
		Target:                     req.Target,
		FlexibleTimeWindow:         req.FlexibleTimeWindow,
		CreationDate:               now,
		LastModificationDate:       now,
	}
	schedules.Put(key, sched)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"ScheduleArn": sched.Arn})
}

func handleSchedulerGetSchedule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("Name")
	group := scheduleGroupParam(r, "groupName")
	sched, ok := schedules.Get(scheduleKey(group, name))
	if !ok {
		schedulerError(w, "ResourceNotFoundException", http.StatusNotFound,
			"Schedule %q not found in group %q", name, group)
		return
	}
	sim.WriteJSON(w, http.StatusOK, scheduleToJSON(sched))
}

func scheduleToJSON(s Schedule) map[string]any {
	m := map[string]any{
		"Arn":                  s.Arn,
		"Name":                 s.Name,
		"GroupName":            s.GroupName,
		"ScheduleExpression":   s.ScheduleExpression,
		"State":                s.State,
		"CreationDate":         s.CreationDate,
		"LastModificationDate": s.LastModificationDate,
	}
	if s.ScheduleExpressionTimezone != "" {
		m["ScheduleExpressionTimezone"] = s.ScheduleExpressionTimezone
	}
	if s.Description != "" {
		m["Description"] = s.Description
	}
	if s.KmsKeyArn != "" {
		m["KmsKeyArn"] = s.KmsKeyArn
	}
	if s.StartDate != nil {
		m["StartDate"] = *s.StartDate
	}
	if s.EndDate != nil {
		m["EndDate"] = *s.EndDate
	}
	if s.ActionAfterCompletion != "" {
		m["ActionAfterCompletion"] = s.ActionAfterCompletion
	}
	if s.Target != nil {
		m["Target"] = s.Target
	}
	if s.FlexibleTimeWindow != nil {
		m["FlexibleTimeWindow"] = s.FlexibleTimeWindow
	}
	return m
}

func handleSchedulerUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("Name")
	var req struct {
		GroupName                  string          `json:"GroupName"`
		ScheduleExpression         string          `json:"ScheduleExpression"`
		ScheduleExpressionTimezone string          `json:"ScheduleExpressionTimezone"`
		Description                string          `json:"Description"`
		State                      string          `json:"State"`
		KmsKeyArn                  string          `json:"KmsKeyArn"`
		StartDate                  *float64        `json:"StartDate"`
		EndDate                    *float64        `json:"EndDate"`
		ActionAfterCompletion      string          `json:"ActionAfterCompletion"`
		Target                     json.RawMessage `json:"Target"`
		FlexibleTimeWindow         json.RawMessage `json:"FlexibleTimeWindow"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		schedulerError(w, "ValidationException", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	group := req.GroupName
	if group == "" {
		group = "default"
	}
	key := scheduleKey(group, name)
	existing, ok := schedules.Get(key)
	if !ok {
		schedulerError(w, "ResourceNotFoundException", http.StatusNotFound,
			"Schedule %q not found in group %q", name, group)
		return
	}
	// UpdateSchedule is a full replacement of the mutable fields.
	state := req.State
	if state == "" {
		state = "ENABLED"
	}
	existing.ScheduleExpression = req.ScheduleExpression
	existing.ScheduleExpressionTimezone = req.ScheduleExpressionTimezone
	existing.Description = req.Description
	existing.State = state
	existing.KmsKeyArn = req.KmsKeyArn
	existing.StartDate = req.StartDate
	existing.EndDate = req.EndDate
	existing.ActionAfterCompletion = req.ActionAfterCompletion
	existing.Target = req.Target
	existing.FlexibleTimeWindow = req.FlexibleTimeWindow
	existing.LastModificationDate = float64(time.Now().Unix())
	schedules.Put(key, existing)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"ScheduleArn": existing.Arn})
}

func handleSchedulerDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("Name")
	group := scheduleGroupParam(r, "groupName")
	key := scheduleKey(group, name)
	if _, ok := schedules.Get(key); !ok {
		schedulerError(w, "ResourceNotFoundException", http.StatusNotFound,
			"Schedule %q not found in group %q", name, group)
		return
	}
	schedules.Delete(key)
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleSchedulerListSchedules(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	groupFilter := q.Get("ScheduleGroup")
	namePrefix := q.Get("NamePrefix")
	stateFilter := q.Get("State")

	matched := schedules.Filter(func(s Schedule) bool {
		if groupFilter != "" && s.GroupName != groupFilter {
			return false
		}
		if namePrefix != "" && !strings.HasPrefix(s.Name, namePrefix) {
			return false
		}
		if stateFilter != "" && s.State != stateFilter {
			return false
		}
		return true
	})
	matched = sortBy(matched, func(s Schedule) string { return s.GroupName + "/" + s.Name })

	out := make([]map[string]any, 0, len(matched))
	for _, s := range matched {
		item := map[string]any{
			"Arn":                  s.Arn,
			"Name":                 s.Name,
			"GroupName":            s.GroupName,
			"State":                s.State,
			"CreationDate":         s.CreationDate,
			"LastModificationDate": s.LastModificationDate,
		}
		// ScheduleSummary carries a TargetSummary holding only the target ARN.
		if arn := targetARN(s.Target); arn != "" {
			item["Target"] = map[string]any{"Arn": arn}
		}
		out = append(out, item)
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Schedules": out})
}

// targetARN extracts the Arn from a stored Target JSON object, for the
// TargetSummary returned by ListSchedules.
func targetARN(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var t struct {
		Arn string `json:"Arn"`
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return ""
	}
	return t.Arn
}

func handleSchedulerCreateScheduleGroup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("Name")
	if _, exists := scheduleGroups.Get(name); exists || name == "default" {
		schedulerError(w, "ConflictException", http.StatusConflict,
			"Schedule group %q already exists", name)
		return
	}
	now := float64(time.Now().Unix())
	grp := ScheduleGroup{
		Name:                 name,
		Arn:                  schedulerGroupARN(name),
		State:                "ACTIVE",
		CreationDate:         now,
		LastModificationDate: now,
	}
	scheduleGroups.Put(name, grp)
	sim.WriteJSON(w, http.StatusOK, map[string]any{"ScheduleGroupArn": grp.Arn})
}

func handleSchedulerGetScheduleGroup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("Name")
	grp, ok := scheduleGroups.Get(name)
	if !ok {
		if name == "default" {
			// The default group is implicit — synthesize a stable record.
			grp = ScheduleGroup{Name: "default", Arn: schedulerGroupARN("default"), State: "ACTIVE"}
		} else {
			schedulerError(w, "ResourceNotFoundException", http.StatusNotFound,
				"Schedule group %q not found", name)
			return
		}
	}
	sim.WriteJSON(w, http.StatusOK, scheduleGroupToJSON(grp))
}

func scheduleGroupToJSON(g ScheduleGroup) map[string]any {
	m := map[string]any{
		"Name":  g.Name,
		"Arn":   g.Arn,
		"State": g.State,
	}
	if g.CreationDate != 0 {
		m["CreationDate"] = g.CreationDate
	}
	if g.LastModificationDate != 0 {
		m["LastModificationDate"] = g.LastModificationDate
	}
	return m
}

func handleSchedulerDeleteScheduleGroup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("Name")
	if name == "default" {
		schedulerError(w, "ValidationException", http.StatusBadRequest,
			"The default schedule group cannot be deleted")
		return
	}
	if _, ok := scheduleGroups.Get(name); !ok {
		schedulerError(w, "ResourceNotFoundException", http.StatusNotFound,
			"Schedule group %q not found", name)
		return
	}
	scheduleGroups.Delete(name)
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleSchedulerListScheduleGroups(w http.ResponseWriter, r *http.Request) {
	namePrefix := r.URL.Query().Get("NamePrefix")
	groups := scheduleGroups.List()
	groups = sortBy(groups, func(g ScheduleGroup) string { return g.Name })

	out := make([]map[string]any, 0, len(groups)+1)
	// The default group is always present.
	if namePrefix == "" || strings.HasPrefix("default", namePrefix) {
		out = append(out, map[string]any{
			"Name":  "default",
			"Arn":   schedulerGroupARN("default"),
			"State": "ACTIVE",
		})
	}
	for _, g := range groups {
		if namePrefix != "" && !strings.HasPrefix(g.Name, namePrefix) {
			continue
		}
		out = append(out, scheduleGroupToJSON(g))
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"ScheduleGroups": out})
}
