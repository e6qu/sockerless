package main

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

type CloudTrailTrail struct {
	Name           string
	S3BucketName   string
	S3KeyPrefix    string
	ARN            string
	HomeRegion     string
	Logging        bool
	CreatedAt      string
	LatestDelivery string
	Tags           []EC2Tag
	EventSelectors []map[string]any
}

type CloudTrailEvent struct {
	EventId     string
	EventName   string
	EventSource string
	EventTime   string
	Username    string
	Resources   []CloudTrailResource
}

type CloudTrailResource struct {
	ResourceName string
	ResourceType string
}

var (
	cloudTrailTrails sim.Store[CloudTrailTrail]
	cloudTrailEvents sim.Store[CloudTrailEvent]
)

type cloudTrailStatusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *cloudTrailStatusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *cloudTrailStatusRecorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (w *cloudTrailStatusRecorder) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func registerCloudTrail(r *sim.AWSRouter, srv *sim.Server) {
	cloudTrailTrails = sim.MakeStore[CloudTrailTrail](srv.DB(), "cloudtrail_trails")
	cloudTrailEvents = sim.MakeStore[CloudTrailEvent](srv.DB(), "cloudtrail_events")

	for _, op := range []string{
		"CreateTrail", "DescribeTrails", "GetTrail", "UpdateTrail", "GetTrailStatus",
		"StartLogging", "StopLogging", "LookupEvents", "DeleteTrail",
		"AddTags", "RemoveTags", "ListTags", "PutEventSelectors", "GetEventSelectors",
	} {
		handler := handleCloudTrail(op)
		r.Register("CloudTrail_20131101."+op, handler)
		r.Register("com.amazonaws.cloudtrail.v20131101.CloudTrail_20131101."+op, handler)
	}
}

func handleCloudTrail(op string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch op {
		case "CreateTrail":
			handleCloudTrailCreateTrail(w, r)
		case "DescribeTrails":
			handleCloudTrailDescribeTrails(w, r)
		case "GetTrail":
			handleCloudTrailGetTrail(w, r)
		case "UpdateTrail":
			handleCloudTrailUpdateTrail(w, r)
		case "GetTrailStatus":
			handleCloudTrailGetTrailStatus(w, r)
		case "StartLogging":
			handleCloudTrailStartLogging(w, r)
		case "StopLogging":
			handleCloudTrailStopLogging(w, r)
		case "LookupEvents":
			handleCloudTrailLookupEvents(w, r)
		case "DeleteTrail":
			handleCloudTrailDeleteTrail(w, r)
		case "AddTags":
			handleCloudTrailAddTags(w, r)
		case "RemoveTags":
			handleCloudTrailRemoveTags(w, r)
		case "ListTags":
			handleCloudTrailListTags(w, r)
		case "PutEventSelectors":
			handleCloudTrailPutEventSelectors(w, r)
		case "GetEventSelectors":
			handleCloudTrailGetEventSelectors(w, r)
		default:
			cloudTrailError(w, "UnsupportedOperationException", "unsupported CloudTrail operation", http.StatusBadRequest)
		}
	}
}

func handleCloudTrailCreateTrail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string
		S3BucketName string
		S3KeyPrefix  string
	}
	if !readAWSJSON(w, r, &req) {
		return
	}
	if req.Name == "" || req.S3BucketName == "" {
		cloudTrailError(w, "InvalidParameterException", "Name and S3BucketName are required", http.StatusBadRequest)
		return
	}
	if _, ok := s3Buckets_.Get(req.S3BucketName); !ok {
		cloudTrailError(w, "S3BucketDoesNotExistException", "S3 bucket does not exist", http.StatusBadRequest)
		return
	}
	trail := CloudTrailTrail{
		Name:         req.Name,
		S3BucketName: req.S3BucketName,
		S3KeyPrefix:  strings.Trim(req.S3KeyPrefix, "/"),
		ARN:          cloudTrailARN(req.Name),
		HomeRegion:   awsRegion(),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	cloudTrailTrails.Put(req.Name, trail)
	writeAWSJSON(w, http.StatusOK, map[string]any{
		"Name":         trail.Name,
		"S3BucketName": trail.S3BucketName,
		"S3KeyPrefix":  trail.S3KeyPrefix,
		"TrailARN":     trail.ARN,
	})
}

func handleCloudTrailDescribeTrails(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TrailNameList []string
	}
	_ = readAWSJSONAllowEmpty(r, &req)
	trails := make([]map[string]any, 0)
	if len(req.TrailNameList) > 0 {
		for _, name := range req.TrailNameList {
			if trail, ok := findCloudTrail(name); ok {
				trails = append(trails, cloudTrailSummary(trail))
			}
		}
	} else {
		for _, trail := range cloudTrailTrails.List() {
			trails = append(trails, cloudTrailSummary(trail))
		}
	}
	writeAWSJSON(w, http.StatusOK, map[string]any{"trailList": trails})
}

func handleCloudTrailGetTrail(w http.ResponseWriter, r *http.Request) {
	trail, ok := cloudTrailTrailFromJSON(w, r)
	if !ok {
		return
	}
	writeAWSJSON(w, http.StatusOK, map[string]any{"Trail": cloudTrailSummary(trail)})
}

func handleCloudTrailUpdateTrail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string
		S3BucketName string
		S3KeyPrefix  string
	}
	if !readAWSJSON(w, r, &req) {
		return
	}
	trail, ok := findCloudTrail(req.Name)
	if !ok {
		cloudTrailError(w, "TrailNotFoundException", "Trail not found", http.StatusNotFound)
		return
	}
	if req.S3BucketName != "" {
		if _, ok := s3Buckets_.Get(req.S3BucketName); !ok {
			cloudTrailError(w, "S3BucketDoesNotExistException", "S3 bucket does not exist", http.StatusBadRequest)
			return
		}
		trail.S3BucketName = req.S3BucketName
	}
	if req.S3KeyPrefix != "" {
		trail.S3KeyPrefix = strings.Trim(req.S3KeyPrefix, "/")
	}
	cloudTrailTrails.Put(trail.Name, trail)
	writeAWSJSON(w, http.StatusOK, cloudTrailSummary(trail))
}

func handleCloudTrailGetTrailStatus(w http.ResponseWriter, r *http.Request) {
	trail, ok := cloudTrailTrailFromJSON(w, r)
	if !ok {
		return
	}
	resp := map[string]any{"IsLogging": trail.Logging}
	if trail.LatestDelivery != "" {
		resp["LatestDeliveryTime"] = cloudTrailEpochSeconds(trail.LatestDelivery)
		resp["LatestDeliveryAttemptTime"] = trail.LatestDelivery
		resp["LatestDeliveryAttemptSucceeded"] = trail.LatestDelivery
	}
	writeAWSJSON(w, http.StatusOK, resp)
}

func handleCloudTrailStartLogging(w http.ResponseWriter, r *http.Request) {
	trail, ok := cloudTrailTrailFromJSON(w, r)
	if !ok {
		return
	}
	trail.Logging = true
	cloudTrailTrails.Put(trail.Name, trail)
	writeAWSJSON(w, http.StatusOK, map[string]any{})
}

func handleCloudTrailStopLogging(w http.ResponseWriter, r *http.Request) {
	trail, ok := cloudTrailTrailFromJSON(w, r)
	if !ok {
		return
	}
	trail.Logging = false
	cloudTrailTrails.Put(trail.Name, trail)
	writeAWSJSON(w, http.StatusOK, map[string]any{})
}

func handleCloudTrailLookupEvents(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LookupAttributes []struct {
			AttributeKey   string
			AttributeValue string
		}
		MaxResults int
	}
	_ = readAWSJSONAllowEmpty(r, &req)
	max := req.MaxResults
	if max <= 0 || max > 50 {
		max = 50
	}
	events := cloudTrailEvents.List()
	out := make([]map[string]any, 0, len(events))
	for i := len(events) - 1; i >= 0 && len(out) < max; i-- {
		ev := events[i]
		if !cloudTrailEventMatches(ev, req.LookupAttributes) {
			continue
		}
		out = append(out, cloudTrailEventJSON(ev))
	}
	writeAWSJSON(w, http.StatusOK, map[string]any{"Events": out})
}

func handleCloudTrailDeleteTrail(w http.ResponseWriter, r *http.Request) {
	trail, ok := cloudTrailTrailFromJSON(w, r)
	if !ok {
		return
	}
	cloudTrailTrails.Delete(trail.Name)
	writeAWSJSON(w, http.StatusOK, map[string]any{})
}

func handleCloudTrailAddTags(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceId string
		TagsList   []EC2Tag
	}
	if !readAWSJSON(w, r, &req) {
		return
	}
	trail, ok := findCloudTrail(req.ResourceId)
	if !ok {
		cloudTrailError(w, "TrailNotFoundException", "Trail not found", http.StatusNotFound)
		return
	}
	for _, tag := range req.TagsList {
		found := false
		for i := range trail.Tags {
			if trail.Tags[i].Key == tag.Key {
				trail.Tags[i].Value = tag.Value
				found = true
				break
			}
		}
		if !found {
			trail.Tags = append(trail.Tags, tag)
		}
	}
	cloudTrailTrails.Put(trail.Name, trail)
	writeAWSJSON(w, http.StatusOK, map[string]any{})
}

func handleCloudTrailRemoveTags(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceId string
		TagsList   []EC2Tag
	}
	if !readAWSJSON(w, r, &req) {
		return
	}
	trail, ok := findCloudTrail(req.ResourceId)
	if !ok {
		cloudTrailError(w, "TrailNotFoundException", "Trail not found", http.StatusNotFound)
		return
	}
	for _, remove := range req.TagsList {
		keep := trail.Tags[:0]
		for _, tag := range trail.Tags {
			if tag.Key != remove.Key {
				keep = append(keep, tag)
			}
		}
		trail.Tags = keep
	}
	cloudTrailTrails.Put(trail.Name, trail)
	writeAWSJSON(w, http.StatusOK, map[string]any{})
}

func handleCloudTrailListTags(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ResourceIdList []string
	}
	if !readAWSJSON(w, r, &req) {
		return
	}
	var out []map[string]any
	for _, resourceID := range req.ResourceIdList {
		trail, ok := findCloudTrail(resourceID)
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"ResourceId": trail.ARN,
			"TagsList":   trail.Tags,
		})
	}
	writeAWSJSON(w, http.StatusOK, map[string]any{"ResourceTagList": out})
}

func handleCloudTrailPutEventSelectors(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TrailName      string
		EventSelectors []map[string]any
	}
	if !readAWSJSON(w, r, &req) {
		return
	}
	trail, ok := findCloudTrail(req.TrailName)
	if !ok {
		cloudTrailError(w, "TrailNotFoundException", "Trail not found", http.StatusNotFound)
		return
	}
	trail.EventSelectors = req.EventSelectors
	cloudTrailTrails.Put(trail.Name, trail)
	writeAWSJSON(w, http.StatusOK, map[string]any{
		"TrailARN":       trail.ARN,
		"EventSelectors": trail.EventSelectors,
	})
}

func handleCloudTrailGetEventSelectors(w http.ResponseWriter, r *http.Request) {
	trail, ok := cloudTrailTrailFromJSON(w, r)
	if !ok {
		return
	}
	writeAWSJSON(w, http.StatusOK, map[string]any{
		"TrailARN":       trail.ARN,
		"EventSelectors": trail.EventSelectors,
	})
}

func cloudTrailTrailFromJSON(w http.ResponseWriter, r *http.Request) (CloudTrailTrail, bool) {
	var req struct {
		Name string
	}
	if !readAWSJSON(w, r, &req) {
		return CloudTrailTrail{}, false
	}
	trail, ok := findCloudTrail(req.Name)
	if !ok {
		cloudTrailError(w, "TrailNotFoundException", "Trail not found", http.StatusNotFound)
		return CloudTrailTrail{}, false
	}
	return trail, true
}

func findCloudTrail(nameOrARN string) (CloudTrailTrail, bool) {
	if trail, ok := cloudTrailTrails.Get(nameOrARN); ok {
		return trail, true
	}
	for _, trail := range cloudTrailTrails.List() {
		if trail.ARN == nameOrARN {
			return trail, true
		}
	}
	return CloudTrailTrail{}, false
}

func cloudTrailSummary(trail CloudTrailTrail) map[string]any {
	return map[string]any{
		"Name":         trail.Name,
		"S3BucketName": trail.S3BucketName,
		"S3KeyPrefix":  trail.S3KeyPrefix,
		"TrailARN":     trail.ARN,
		"HomeRegion":   trail.HomeRegion,
	}
}

func cloudTrailARN(name string) string {
	return fmt.Sprintf("arn:aws:cloudtrail:%s:%s:trail/%s", awsRegion(), awsAccountID(), name)
}

func cloudTrailRecordAPICall(r *http.Request, status int) {
	if cloudTrailEvents == nil || status >= 500 {
		return
	}
	eventName := awsRequestOperationName(r)
	if eventName == "" {
		return
	}
	event := CloudTrailEvent{
		EventId:     generateUUID(),
		EventName:   eventName,
		EventSource: awsEventSource(r),
		EventTime:   time.Now().UTC().Format(time.RFC3339),
		Username:    "sockerless",
	}
	cloudTrailEvents.Put(event.EventId, event)
	cloudTrailDeliverEvent(event)
}

func awsRequestOperationName(r *http.Request) string {
	if target := r.Header.Get("X-Amz-Target"); target != "" {
		if idx := strings.LastIndex(target, "."); idx >= 0 {
			return target[idx+1:]
		}
		return target
	}
	if err := r.ParseForm(); err == nil {
		return r.FormValue("Action")
	}
	return ""
}

func awsEventSource(r *http.Request) string {
	if target := r.Header.Get("X-Amz-Target"); target != "" {
		if strings.HasPrefix(target, "CloudTrail_") {
			return "cloudtrail.amazonaws.com"
		}
		if strings.HasPrefix(target, "AmazonEC2ContainerService") {
			return "ecs.amazonaws.com"
		}
		return "aws.amazonaws.com"
	}
	switch r.FormValue("Version") {
	case "2011-01-01":
		return "autoscaling.amazonaws.com"
	default:
		return "ec2.amazonaws.com"
	}
}

func cloudTrailDeliverEvent(event CloudTrailEvent) {
	for _, trail := range cloudTrailTrails.List() {
		if !trail.Logging {
			continue
		}
		if _, ok := s3Buckets_.Get(trail.S3BucketName); !ok {
			continue
		}
		body, err := cloudTrailLogBody(event)
		if err != nil {
			continue
		}
		key := cloudTrailObjectKey(trail, event)
		hash := md5.Sum(body)
		s3Objects.Put(s3ObjectKey(trail.S3BucketName, key), S3Object{
			Key:          s3ObjectKey(trail.S3BucketName, key),
			Data:         body,
			ContentType:  "application/json",
			ETag:         fmt.Sprintf("\"%x\"", hash),
			LastModified: time.Now().UTC(),
			Size:         int64(len(body)),
			Metadata:     map[string]string{"cloudtrail-event-id": event.EventId},
		})
		trail.LatestDelivery = event.EventTime
		cloudTrailTrails.Put(trail.Name, trail)
	}
}

func cloudTrailLogBody(event CloudTrailEvent) ([]byte, error) {
	payload := map[string]any{"Records": []map[string]any{{
		"eventVersion": "1.08",
		"userIdentity": map[string]any{"type": "IAMUser", "userName": event.Username},
		"eventTime":    event.EventTime,
		"eventSource":  event.EventSource,
		"eventName":    event.EventName,
		"awsRegion":    awsRegion(),
		"eventID":      event.EventId,
		"eventType":    "AwsApiCall",
	}}}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		_ = gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func cloudTrailObjectKey(trail CloudTrailTrail, event CloudTrailEvent) string {
	t, _ := time.Parse(time.RFC3339, event.EventTime)
	prefix := strings.Trim(trail.S3KeyPrefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	return fmt.Sprintf("%sAWSLogs/%s/CloudTrail/%s/%04d/%02d/%02d/%s_%s.json.gz",
		prefix, awsAccountID(), awsRegion(), t.Year(), t.Month(), t.Day(), trail.Name, event.EventId)
}

func cloudTrailEventMatches(ev CloudTrailEvent, attrs []struct {
	AttributeKey   string
	AttributeValue string
}) bool {
	for _, attr := range attrs {
		switch attr.AttributeKey {
		case "EventName":
			if ev.EventName != attr.AttributeValue {
				return false
			}
		case "EventSource":
			if ev.EventSource != attr.AttributeValue {
				return false
			}
		case "Username":
			if ev.Username != attr.AttributeValue {
				return false
			}
		}
	}
	return true
}

func cloudTrailEventJSON(ev CloudTrailEvent) map[string]any {
	return map[string]any{
		"EventId":     ev.EventId,
		"EventName":   ev.EventName,
		"EventSource": ev.EventSource,
		"EventTime":   cloudTrailEpochSeconds(ev.EventTime),
		"Username":    ev.Username,
		"Resources":   ev.Resources,
		"CloudTrailEvent": fmt.Sprintf(`{"eventSource":%q,"eventName":%q,"eventTime":%q,"eventID":%q}`,
			ev.EventSource, ev.EventName, ev.EventTime, ev.EventId),
	}
}

func cloudTrailEpochSeconds(value string) float64 {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0
	}
	return float64(t.UnixNano()) / float64(time.Second)
}

func readAWSJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		cloudTrailError(w, "InvalidRequestException", "invalid JSON request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func readAWSJSONAllowEmpty(r *http.Request, out any) error {
	if r.Body == nil {
		return nil
	}
	err := json.NewDecoder(r.Body).Decode(out)
	if err != nil && err.Error() == "EOF" {
		return nil
	}
	return err
}

func writeAWSJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func cloudTrailError(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.Header().Set("X-Amzn-Errortype", code)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"__type": code, "message": message})
}
