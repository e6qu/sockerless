package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// CloudWatch Logs types

type CWLogGroup struct {
	LogGroupName    string `json:"logGroupName"`
	Arn             string `json:"arn"`
	CreationTime    int64  `json:"creationTime"`
	RetentionInDays int    `json:"retentionInDays,omitempty"`
	StoredBytes     int64  `json:"storedBytes"`
}

type CWLogStream struct {
	LogStreamName       string `json:"logStreamName"`
	LogGroupName        string `json:"-"`
	CreationTime        int64  `json:"creationTime"`
	FirstEventTimestamp int64  `json:"firstEventTimestamp,omitempty"`
	LastEventTimestamp  int64  `json:"lastEventTimestamp,omitempty"`
	LastIngestionTime   int64  `json:"lastIngestionTime,omitempty"`
	Arn                 string `json:"arn"`
	UploadSequenceToken string `json:"uploadSequenceToken"`
}

type CWLogEvent struct {
	Timestamp     int64  `json:"timestamp"`
	Message       string `json:"message"`
	IngestionTime int64  `json:"ingestionTime"`
}

// State stores
var (
	cwLogGroups  *sim.StateStore[CWLogGroup]
	cwLogStreams *sim.StateStore[CWLogStream]
	cwLogEvents  *sim.StateStore[[]CWLogEvent]
	cwSeqMu      sync.Mutex
	cwSeqCounter int64
)

func cwLogGroupArn(name string) string {
	return fmt.Sprintf("arn:aws:logs:us-east-1:123456789012:log-group:%s", name)
}

func cwLogStreamArn(group, stream string) string {
	return fmt.Sprintf("arn:aws:logs:us-east-1:123456789012:log-group:%s:log-stream:%s", group, stream)
}

func cwEventsKey(group, stream string) string {
	return group + ":" + stream
}

func registerCloudWatchLogs(r *sim.AWSRouter, srv *sim.Server) {
	cwLogGroups = sim.NewStateStore[CWLogGroup]()
	cwLogStreams = sim.NewStateStore[CWLogStream]()
	cwLogEvents = sim.NewStateStore[[]CWLogEvent]()
	cwSeqCounter = 1

	r.Register("Logs_20140328.CreateLogGroup", handleCWCreateLogGroup)
	r.Register("Logs_20140328.DescribeLogGroups", handleCWDescribeLogGroups)
	r.Register("Logs_20140328.DeleteLogGroup", handleCWDeleteLogGroup)
	r.Register("Logs_20140328.CreateLogStream", handleCWCreateLogStream)
	r.Register("Logs_20140328.DescribeLogStreams", handleCWDescribeLogStreams)
	r.Register("Logs_20140328.PutLogEvents", handleCWPutLogEvents)
	r.Register("Logs_20140328.GetLogEvents", handleCWGetLogEvents)
	r.Register("Logs_20140328.FilterLogEvents", handleCWFilterLogEvents)
	r.Register("Logs_20140328.PutRetentionPolicy", handleCWPutRetentionPolicy)
	r.Register("Logs_20140328.ListTagsForResource", handleCWListTagsForResource)
	r.Register("Logs_20140328.TagResource", handleCWTagResource)
}

func handleCWCreateLogGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LogGroupName    string `json:"logGroupName"`
		RetentionInDays int    `json:"retentionInDays"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.LogGroupName == "" {
		sim.AWSError(w, "InvalidParameterException", "logGroupName is required", http.StatusBadRequest)
		return
	}

	if _, exists := cwLogGroups.Get(req.LogGroupName); exists {
		sim.AWSErrorf(w, "ResourceAlreadyExistsException", http.StatusBadRequest,
			"The specified log group already exists: %s", req.LogGroupName)
		return
	}

	lg := CWLogGroup{
		LogGroupName:    req.LogGroupName,
		Arn:             cwLogGroupArn(req.LogGroupName),
		CreationTime:    time.Now().UnixMilli(),
		RetentionInDays: req.RetentionInDays,
	}
	cwLogGroups.Put(req.LogGroupName, lg)

	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCWDescribeLogGroups(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LogGroupNamePrefix string `json:"logGroupNamePrefix"`
		Limit              int    `json:"limit"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}

	var groups []CWLogGroup
	if req.LogGroupNamePrefix != "" {
		groups = cwLogGroups.Filter(func(lg CWLogGroup) bool {
			return strings.HasPrefix(lg.LogGroupName, req.LogGroupNamePrefix)
		})
	} else {
		groups = cwLogGroups.List()
	}
	if groups == nil {
		groups = []CWLogGroup{}
	}

	if req.Limit > 0 && len(groups) > req.Limit {
		groups = groups[:req.Limit]
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"logGroups": groups,
	})
}

func handleCWDeleteLogGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LogGroupName string `json:"logGroupName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.LogGroupName == "" {
		sim.AWSError(w, "InvalidParameterException", "logGroupName is required", http.StatusBadRequest)
		return
	}

	if !cwLogGroups.Delete(req.LogGroupName) {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"The specified log group does not exist: %s", req.LogGroupName)
		return
	}

	// Clean up streams and events for this group
	streams := cwLogStreams.Filter(func(s CWLogStream) bool {
		return s.LogGroupName == req.LogGroupName
	})
	for _, s := range streams {
		key := cwEventsKey(req.LogGroupName, s.LogStreamName)
		cwLogStreams.Delete(key)
		cwLogEvents.Delete(key)
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCWCreateLogStream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LogGroupName  string `json:"logGroupName"`
		LogStreamName string `json:"logStreamName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.LogGroupName == "" || req.LogStreamName == "" {
		sim.AWSError(w, "InvalidParameterException", "logGroupName and logStreamName are required", http.StatusBadRequest)
		return
	}

	if _, ok := cwLogGroups.Get(req.LogGroupName); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"The specified log group does not exist: %s", req.LogGroupName)
		return
	}

	key := cwEventsKey(req.LogGroupName, req.LogStreamName)
	if _, exists := cwLogStreams.Get(key); exists {
		sim.AWSErrorf(w, "ResourceAlreadyExistsException", http.StatusBadRequest,
			"The specified log stream already exists: %s", req.LogStreamName)
		return
	}

	cwSeqMu.Lock()
	cwSeqCounter++
	seq := cwSeqCounter
	cwSeqMu.Unlock()

	ls := CWLogStream{
		LogStreamName:       req.LogStreamName,
		LogGroupName:        req.LogGroupName,
		CreationTime:        time.Now().UnixMilli(),
		Arn:                 cwLogStreamArn(req.LogGroupName, req.LogStreamName),
		UploadSequenceToken: fmt.Sprintf("%016d", seq),
	}
	cwLogStreams.Put(key, ls)
	cwLogEvents.Put(key, []CWLogEvent{})

	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCWDescribeLogStreams(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LogGroupName        string `json:"logGroupName"`
		LogStreamNamePrefix string `json:"logStreamNamePrefix"`
		OrderBy             string `json:"orderBy"`
		Descending          *bool  `json:"descending"`
		Limit               int    `json:"limit"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.LogGroupName == "" {
		sim.AWSError(w, "InvalidParameterException", "logGroupName is required", http.StatusBadRequest)
		return
	}

	streams := cwLogStreams.Filter(func(s CWLogStream) bool {
		if s.LogGroupName != req.LogGroupName {
			return false
		}
		if req.LogStreamNamePrefix != "" {
			return strings.HasPrefix(s.LogStreamName, req.LogStreamNamePrefix)
		}
		return true
	})
	if streams == nil {
		streams = []CWLogStream{}
	}

	// Sort by OrderBy + Descending
	if req.OrderBy == "LastEventTime" {
		desc := req.Descending != nil && *req.Descending
		sort.Slice(streams, func(i, j int) bool {
			if desc {
				return streams[i].LastEventTimestamp > streams[j].LastEventTimestamp
			}
			return streams[i].LastEventTimestamp < streams[j].LastEventTimestamp
		})
	} else {
		// Default: sort by LogStreamName ascending
		desc := req.Descending != nil && *req.Descending
		sort.Slice(streams, func(i, j int) bool {
			if desc {
				return streams[i].LogStreamName > streams[j].LogStreamName
			}
			return streams[i].LogStreamName < streams[j].LogStreamName
		})
	}

	if req.Limit > 0 && len(streams) > req.Limit {
		streams = streams[:req.Limit]
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"logStreams": streams,
	})
}

func handleCWPutLogEvents(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LogGroupName  string `json:"logGroupName"`
		LogStreamName string `json:"logStreamName"`
		LogEvents     []struct {
			Timestamp int64  `json:"timestamp"`
			Message   string `json:"message"`
		} `json:"logEvents"`
		SequenceToken string `json:"sequenceToken"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.LogGroupName == "" || req.LogStreamName == "" {
		sim.AWSError(w, "InvalidParameterException", "logGroupName and logStreamName are required", http.StatusBadRequest)
		return
	}

	key := cwEventsKey(req.LogGroupName, req.LogStreamName)
	if _, ok := cwLogStreams.Get(key); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"The specified log stream does not exist: %s", req.LogStreamName)
		return
	}

	now := time.Now().UnixMilli()
	var newEvents []CWLogEvent
	for _, e := range req.LogEvents {
		newEvents = append(newEvents, CWLogEvent{
			Timestamp:     e.Timestamp,
			Message:       e.Message,
			IngestionTime: now,
		})
	}

	// Append events
	cwLogEvents.Update(key, func(events *[]CWLogEvent) {
		*events = append(*events, newEvents...)
	})

	// Update stream timestamps
	cwLogStreams.Update(key, func(s *CWLogStream) {
		s.LastIngestionTime = now
		if len(newEvents) > 0 {
			if s.FirstEventTimestamp == 0 {
				s.FirstEventTimestamp = newEvents[0].Timestamp
			}
			s.LastEventTimestamp = newEvents[len(newEvents)-1].Timestamp
		}
	})

	cwSeqMu.Lock()
	cwSeqCounter++
	seq := cwSeqCounter
	cwSeqMu.Unlock()

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"nextSequenceToken": fmt.Sprintf("%016d", seq),
	})
}

func handleCWGetLogEvents(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LogGroupName  string `json:"logGroupName"`
		LogStreamName string `json:"logStreamName"`
		StartTime     int64  `json:"startTime"`
		EndTime       int64  `json:"endTime"`
		Limit         int    `json:"limit"`
		StartFromHead *bool  `json:"startFromHead"`
		NextToken     string `json:"nextToken"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.LogGroupName == "" || req.LogStreamName == "" {
		sim.AWSError(w, "InvalidParameterException", "logGroupName and logStreamName are required", http.StatusBadRequest)
		return
	}

	key := cwEventsKey(req.LogGroupName, req.LogStreamName)
	events, ok := cwLogEvents.Get(key)
	if !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"The specified log stream does not exist: %s", req.LogStreamName)
		return
	}

	// Apply time range filter
	var filtered []CWLogEvent
	for _, e := range events {
		if req.StartTime > 0 && e.Timestamp < req.StartTime {
			continue
		}
		if req.EndTime > 0 && e.Timestamp > req.EndTime {
			continue
		}
		filtered = append(filtered, e)
	}

	// Parse offset from NextToken (format: "f/{offset}" or "b/{offset}")
	offset := 0
	if req.NextToken != "" {
		parts := strings.SplitN(req.NextToken, "/", 2)
		if len(parts) == 2 {
			if n, err := strconv.Atoi(parts[1]); err == nil && n >= 0 {
				offset = n
			}
		}
	}

	// Apply offset — skip events already consumed
	if offset > 0 && offset <= len(filtered) {
		filtered = filtered[offset:]
	} else if offset > len(filtered) {
		filtered = nil
	}

	// Apply limit after offset
	if req.Limit > 0 && len(filtered) > req.Limit {
		filtered = filtered[:req.Limit]
	}
	if filtered == nil {
		filtered = []CWLogEvent{}
	}

	// New forward token = offset + events returned
	newForwardOffset := offset + len(filtered)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"events":            filtered,
		"nextForwardToken":  fmt.Sprintf("f/%d", newForwardOffset),
		"nextBackwardToken": fmt.Sprintf("b/%d", offset),
	})
}

func handleCWFilterLogEvents(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LogGroupName   string   `json:"logGroupName"`
		LogStreamNames []string `json:"logStreamNames"`
		FilterPattern  string   `json:"filterPattern"`
		StartTime      int64    `json:"startTime"`
		EndTime        int64    `json:"endTime"`
		Limit          int      `json:"limit"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.LogGroupName == "" {
		sim.AWSError(w, "InvalidParameterException", "logGroupName is required", http.StatusBadRequest)
		return
	}

	// Find all streams for this group
	streams := cwLogStreams.Filter(func(s CWLogStream) bool {
		if s.LogGroupName != req.LogGroupName {
			return false
		}
		if len(req.LogStreamNames) > 0 {
			for _, name := range req.LogStreamNames {
				if s.LogStreamName == name {
					return true
				}
			}
			return false
		}
		return true
	})

	var results []map[string]any
	for _, stream := range streams {
		key := cwEventsKey(req.LogGroupName, stream.LogStreamName)
		events, ok := cwLogEvents.Get(key)
		if !ok {
			continue
		}

		for _, e := range events {
			if req.StartTime > 0 && e.Timestamp < req.StartTime {
				continue
			}
			if req.EndTime > 0 && e.Timestamp > req.EndTime {
				continue
			}
			if req.FilterPattern != "" && !strings.Contains(e.Message, req.FilterPattern) {
				continue
			}
			results = append(results, map[string]any{
				"logStreamName": stream.LogStreamName,
				"timestamp":     e.Timestamp,
				"message":       e.Message,
				"ingestionTime": e.IngestionTime,
				"eventId":       generateUUID(),
			})
		}
	}

	if req.Limit > 0 && len(results) > req.Limit {
		results = results[:req.Limit]
	}
	if results == nil {
		results = []map[string]any{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"events":             results,
		"searchedLogStreams": []any{},
	})
}

func handleCWPutRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LogGroupName    string `json:"logGroupName"`
		RetentionInDays int    `json:"retentionInDays"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidParameterException", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.LogGroupName == "" {
		sim.AWSError(w, "InvalidParameterException", "logGroupName is required", http.StatusBadRequest)
		return
	}

	if _, ok := cwLogGroups.Get(req.LogGroupName); !ok {
		sim.AWSErrorf(w, "ResourceNotFoundException", http.StatusBadRequest,
			"The specified log group does not exist: %s", req.LogGroupName)
		return
	}

	cwLogGroups.Update(req.LogGroupName, func(lg *CWLogGroup) {
		lg.RetentionInDays = req.RetentionInDays
	})

	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCWListTagsForResource(w http.ResponseWriter, r *http.Request) {
	// Terraform uses this to read tags for log groups
	_ = sim.ReadJSON(r, &struct{}{})
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"tags": map[string]string{},
	})
}

func handleCWTagResource(w http.ResponseWriter, r *http.Request) {
	// Accept and discard tag operations
	_ = sim.ReadJSON(r, &struct{}{})
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}
