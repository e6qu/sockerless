package main

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
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
	EventId      string
	EventName    string
	EventSource  string
	EventTime    string
	Username     string
	AccessKeyId  string
	ReadOnly     bool
	InvokedBy    string
	Resources    []CloudTrailResource
	ErrorCode    string
	ErrorMessage string
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
	body   bytes.Buffer
}

func (w *cloudTrailStatusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *cloudTrailStatusRecorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	_, _ = w.body.Write(p)
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
		LookupAttributes []cloudTrailLookupAttribute
		MaxResults       int
	}
	_ = readAWSJSONAllowEmpty(r, &req)
	for _, attr := range req.LookupAttributes {
		if !cloudTrailValidLookupKeys[attr.AttributeKey] {
			cloudTrailError(w, "InvalidLookupAttributesException",
				"Invalid lookup attribute key: "+attr.AttributeKey, http.StatusBadRequest)
			return
		}
	}
	max := req.MaxResults
	if max <= 0 || max > 50 {
		max = 50
	}
	out := cloudTrailLookupEvents(cloudTrailEvents.List(), req.LookupAttributes, max)
	writeAWSJSON(w, http.StatusOK, map[string]any{"Events": out})
}

type cloudTrailLookupAttribute struct {
	AttributeKey   string
	AttributeValue string
}

func cloudTrailLookupEvents(events []CloudTrailEvent, attrs []cloudTrailLookupAttribute, max int) []map[string]any {
	if max <= 0 || max > 50 {
		max = 50
	}
	ordered := append([]CloudTrailEvent(nil), events...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339, ordered[i].EventTime)
		right, rightErr := time.Parse(time.RFC3339, ordered[j].EventTime)
		if leftErr == nil && rightErr == nil && !left.Equal(right) {
			return left.After(right)
		}
		if ordered[i].EventTime != ordered[j].EventTime {
			return ordered[i].EventTime > ordered[j].EventTime
		}
		return ordered[i].EventId > ordered[j].EventId
	})
	out := make([]map[string]any, 0, len(events))
	for _, ev := range ordered {
		if len(out) >= max {
			break
		}
		if !cloudTrailEventMatches(ev, attrs) {
			continue
		}
		out = append(out, cloudTrailEventJSON(ev))
	}
	return out
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

// cloudTrailRecordAPICall records a management event for an awsJson or
// query-protocol API call served by the central POST / router. reqBody is the
// buffered request payload (the handler has already consumed r.Body), used to
// extract the resources[] the call acted on for ResourceName/ResourceType
// lookups.
func cloudTrailRecordAPICall(srv *sim.Server, r *http.Request, reqBody []byte, status int, respHeaders http.Header, respBody []byte) {
	if cloudTrailEvents == nil || status >= 500 {
		return
	}
	eventName := awsRequestOperationName(r)
	if eventName == "" {
		return
	}
	source, ok := awsEventSource(r)
	if !ok {
		// A recognised operation whose service slice has no eventSource
		// mapping. Surface it loudly instead of recording a fabricated source
		// — the missing entry is a real gap in awsEventSourceByTargetPrefix /
		// awsEventSourceByQueryVersion that must be added.
		logger := srv.Logger()
		logger.Warn().
			Str("event", eventName).
			Str("x-amz-target", r.Header.Get("X-Amz-Target")).
			Str("version", r.FormValue("Version")).
			Msg("cloudtrail: no eventSource mapping for service slice; event not recorded")
		return
	}
	errorCode, errorMessage := cloudTrailErrorFields(status, respHeaders, respBody)
	cloudTrailRecord(CloudTrailEvent{
		EventName:    eventName,
		EventSource:  source,
		AccessKeyId:  cloudTrailAccessKeyID(r),
		ReadOnly:     cloudTrailReadOnly(eventName),
		Resources:    cloudTrailResources(source, eventName, reqBody, r),
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
	})
}

type cloudTrailRESTEventNameFunc func(*http.Request, []byte) string
type cloudTrailRESTResourceFunc func(*http.Request, []byte) []CloudTrailResource

func cloudTrailRecordedREST(eventName, source string, resources cloudTrailRESTResourceFunc, handler http.HandlerFunc) http.HandlerFunc {
	return cloudTrailRecordedRESTDynamic(func(*http.Request, []byte) string { return eventName }, source, resources, handler)
}

func cloudTrailRecordedRESTDynamic(eventName cloudTrailRESTEventNameFunc, source string, resources cloudTrailRESTResourceFunc, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		rec := &cloudTrailStatusRecorder{ResponseWriter: w}
		handler(rec, r)
		if cloudTrailEvents == nil || rec.statusCode() >= 500 {
			return
		}
		name := eventName(r, body)
		if name == "" {
			return
		}
		errorCode, errorMessage := cloudTrailErrorFields(rec.statusCode(), rec.Header(), rec.body.Bytes())
		ev := CloudTrailEvent{
			EventName:    name,
			EventSource:  source,
			AccessKeyId:  cloudTrailAccessKeyID(r),
			ReadOnly:     cloudTrailReadOnly(name),
			ErrorCode:    errorCode,
			ErrorMessage: errorMessage,
		}
		if resources != nil {
			ev.Resources = resources(r, body)
		}
		cloudTrailRecord(ev)
	}
}

func cloudTrailRESTResource(resourceType string, params ...string) cloudTrailRESTResourceFunc {
	return func(r *http.Request, _ []byte) []CloudTrailResource {
		for _, param := range params {
			if value := r.PathValue(param); value != "" {
				return []CloudTrailResource{{ResourceType: resourceType, ResourceName: cloudTrailShortName(value)}}
			}
		}
		return nil
	}
}

func cloudTrailErrorFields(status int, headers http.Header, body []byte) (string, string) {
	if status < 400 {
		return "", ""
	}
	code := cleanCloudTrailErrorCode(headers.Get("X-Amzn-Errortype"))
	var msg string
	if len(body) > 0 {
		var raw map[string]any
		if json.Unmarshal(body, &raw) == nil {
			if code == "" {
				code = cleanCloudTrailErrorCode(firstString(raw, "__type", "code", "Code"))
			}
			msg = firstString(raw, "message", "Message")
		}
		if code == "" || msg == "" {
			xmlCode, xmlMsg := cloudTrailXMLErrorFields(body)
			if code == "" {
				code = xmlCode
			}
			if msg == "" {
				msg = xmlMsg
			}
		}
	}
	return code, msg
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key].(string); ok {
			return value
		}
	}
	return ""
}

func cleanCloudTrailErrorCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	if i := strings.Index(code, ":"); i >= 0 {
		code = code[:i]
	}
	if i := strings.LastIndex(code, "#"); i >= 0 {
		code = code[i+1:]
	}
	return code
}

func cloudTrailXMLErrorFields(body []byte) (string, string) {
	var envelope struct {
		Error struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		} `xml:"Error"`
	}
	if err := xml.Unmarshal(body, &envelope); err == nil && (envelope.Error.Code != "" || envelope.Error.Message != "") {
		return envelope.Error.Code, envelope.Error.Message
	}
	var direct struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	if err := xml.Unmarshal(body, &direct); err == nil {
		return direct.Code, direct.Message
	}
	return "", ""
}

// cloudTrailRecord finalises and persists a management event: it stamps the
// event id/time and a default username, then delivers it to every logging
// trail. All recording paths (central API router, REST services like Scheduler,
// and service-invoked targets) funnel through here so every event carries a
// consistent shape.
func cloudTrailRecord(ev CloudTrailEvent) {
	if cloudTrailEvents == nil {
		return
	}
	ev.EventId = generateUUID()
	ev.EventTime = time.Now().UTC().Format(time.RFC3339)
	if ev.Username == "" && ev.InvokedBy == "" {
		ev.Username = "sockerless"
	}
	cloudTrailEvents.Put(ev.EventId, ev)
	cloudTrailDeliverEvent(ev)
}

// cloudTrailAccessKeyID extracts the IAM access key id from a SigV4
// Authorization header (`Credential=<AKID>/<date>/<region>/<service>/...`).
// Empty when the request is unsigned — real CloudTrail also omits AccessKeyId
// for anonymous or service-invoked calls.
func cloudTrailAccessKeyID(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	i := strings.Index(auth, "Credential=")
	if i < 0 {
		return ""
	}
	cred := auth[i+len("Credential="):]
	if j := strings.IndexAny(cred, "/,"); j >= 0 {
		return cred[:j]
	}
	return cred
}

// cloudTrailReadOnly classifies an operation as read-only by its verb, the
// convention AWS follows for its own per-operation readOnly flag (the value the
// ReadOnly lookup attribute filters on).
func cloudTrailReadOnly(eventName string) bool {
	for _, p := range []string{
		"Describe", "Get", "List", "Lookup", "BatchGet", "Search", "Scan",
		"Query", "View", "Estimate", "Filter", "Head", "Check", "Validate",
	} {
		if strings.HasPrefix(eventName, p) {
			return true
		}
	}
	return false
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

// awsEventSourceByTargetPrefix maps an awsJson `X-Amz-Target` service prefix to
// the CloudTrail `eventSource` value real AWS records for that service. The
// prefixes are the per-service namespaces the sim registers (see each
// service's Register call); the eventSource values are AWS's canonical
// `<service>.amazonaws.com` endpoints. Order-independent: every prefix is
// distinct after the point where sibling services diverge (e.g.
// AmazonEC2ContainerService vs AmazonEC2ContainerRegistry).
var awsEventSourceByTargetPrefix = []struct{ prefix, source string }{
	{"com.amazonaws.cloudtrail", "cloudtrail.amazonaws.com"},
	{"CloudTrail_", "cloudtrail.amazonaws.com"},
	{"AmazonEC2ContainerServiceV", "ecs.amazonaws.com"},
	{"AmazonEC2ContainerRegistry", "ecr.amazonaws.com"},
	{"AmazonSQS", "sqs.amazonaws.com"},
	{"AmazonSSM", "ssm.amazonaws.com"},
	{"AnyScaleFrontendService", "application-autoscaling.amazonaws.com"},
	{"AWSEvents", "events.amazonaws.com"},
	{"AWSGlue", "glue.amazonaws.com"},
	{"AWSStepFunctions", "states.amazonaws.com"},
	{"AWSWAF_", "wafv2.amazonaws.com"},
	{"CertificateManager", "acm.amazonaws.com"},
	{"CodeBuild_", "codebuild.amazonaws.com"},
	{"DynamoDB_", "dynamodb.amazonaws.com"},
	{"GraniteServiceVersion", "monitoring.amazonaws.com"},
	{"Kinesis_", "kinesis.amazonaws.com"},
	{"Logs_", "logs.amazonaws.com"},
	{"Route53AutoNaming", "servicediscovery.amazonaws.com"},
	{"TrentService", "kms.amazonaws.com"},
	{"secretsmanager", "secretsmanager.amazonaws.com"},
}

// awsEventSourceByQueryVersion maps a query-protocol service's request
// `Version` (sent by the SDK, unique per service) to its eventSource.
var awsEventSourceByQueryVersion = map[string]string{
	"2016-11-15": "ec2.amazonaws.com",
	"2011-01-01": "autoscaling.amazonaws.com",
	"2010-08-01": "monitoring.amazonaws.com",
	"2010-03-31": "sns.amazonaws.com",
	"2015-12-01": "elasticloadbalancing.amazonaws.com",
	"2014-10-31": "rds.amazonaws.com",
	"2010-05-08": "iam.amazonaws.com",
	"2011-06-15": "sts.amazonaws.com",
}

// awsEventSource resolves the CloudTrail eventSource for an awsJson or
// query-protocol request. It never guesses: an unmapped service returns
// ok=false so the caller can surface the gap (and skip the event) rather than
// record a fabricated source — a wrong source silently breaks LookupEvents
// filtering, which is the exact defect a generic default would reintroduce.
func awsEventSource(r *http.Request) (string, bool) {
	if target := r.Header.Get("X-Amz-Target"); target != "" {
		for _, m := range awsEventSourceByTargetPrefix {
			if strings.HasPrefix(target, m.prefix) {
				return m.source, true
			}
		}
		return "", false
	}
	if src, ok := awsEventSourceByQueryVersion[r.FormValue("Version")]; ok {
		return src, true
	}
	return "", false
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
	payload := map[string]any{"Records": []map[string]any{cloudTrailEventRecord(event)}}
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

// cloudTrailEventMatches implements LookupEvents filtering for all eight
// AttributeKey values the CloudTrail API defines (EventId, EventName, ReadOnly,
// Username, ResourceType, ResourceName, EventSource, AccessKeyId). Multiple
// attributes are ANDed, matching real CloudTrail. An unsupported key is
// rejected upstream by cloudTrailValidateLookupAttributes — it must never reach
// here, but if it did it returns false (never silently match-all, which is the
// exact defect of only handling three of the eight keys).
func cloudTrailEventMatches(ev CloudTrailEvent, attrs []cloudTrailLookupAttribute) bool {
	for _, attr := range attrs {
		switch attr.AttributeKey {
		case "EventId":
			if ev.EventId != attr.AttributeValue {
				return false
			}
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
		case "AccessKeyId":
			if ev.AccessKeyId != attr.AttributeValue {
				return false
			}
		case "ReadOnly":
			if (strings.EqualFold(attr.AttributeValue, "true")) != ev.ReadOnly {
				return false
			}
		case "ResourceName":
			if !cloudTrailHasResource(ev, func(r CloudTrailResource) bool {
				return r.ResourceName == attr.AttributeValue
			}) {
				return false
			}
		case "ResourceType":
			if !cloudTrailHasResource(ev, func(r CloudTrailResource) bool {
				return r.ResourceType == attr.AttributeValue
			}) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func cloudTrailHasResource(ev CloudTrailEvent, pred func(CloudTrailResource) bool) bool {
	for _, r := range ev.Resources {
		if pred(r) {
			return true
		}
	}
	return false
}

// cloudTrailValidLookupKeys is the set of AttributeKey values real CloudTrail
// accepts (per the LookupAttribute API reference). Any other key is rejected
// with InvalidLookupAttributesException rather than silently ignored.
var cloudTrailValidLookupKeys = map[string]bool{
	"EventId": true, "EventName": true, "ReadOnly": true, "Username": true,
	"ResourceType": true, "ResourceName": true, "EventSource": true, "AccessKeyId": true,
}

func cloudTrailEventJSON(ev CloudTrailEvent) map[string]any {
	resources := make([]map[string]any, 0, len(ev.Resources))
	for _, res := range ev.Resources {
		resources = append(resources, map[string]any{
			"ResourceType": res.ResourceType,
			"ResourceName": res.ResourceName,
		})
	}
	out := map[string]any{
		"EventId":     ev.EventId,
		"EventName":   ev.EventName,
		"EventSource": ev.EventSource,
		"EventTime":   cloudTrailEpochSeconds(ev.EventTime),
		"Username":    ev.Username,
		// The LookupEvents Event.ReadOnly response field is a string
		// ("true"/"false"), not a boolean (the boolean lives in the embedded
		// CloudTrailEvent record).
		"ReadOnly":        strconv.FormatBool(ev.ReadOnly),
		"Resources":       resources,
		"CloudTrailEvent": cloudTrailEventRecordJSON(ev),
	}
	if ev.AccessKeyId != "" {
		out["AccessKeyId"] = ev.AccessKeyId
	}
	return out
}

// cloudTrailEventRecordJSON renders the full event record AWS embeds as the
// `CloudTrailEvent` string field of a LookupEvents result.
func cloudTrailEventRecordJSON(ev CloudTrailEvent) string {
	raw, _ := json.Marshal(cloudTrailEventRecord(ev))
	return string(raw)
}

// cloudTrailEventRecord builds the canonical CloudTrail event-record object
// (the shape delivered to S3 and embedded in LookupEvents).
func cloudTrailEventRecord(ev CloudTrailEvent) map[string]any {
	identity := map[string]any{}
	if ev.InvokedBy != "" {
		identity["type"] = "AWSService"
		identity["invokedBy"] = ev.InvokedBy
	} else {
		identity["type"] = "IAMUser"
		identity["userName"] = ev.Username
		if ev.AccessKeyId != "" {
			identity["accessKeyId"] = ev.AccessKeyId
		}
	}
	resources := make([]map[string]any, 0, len(ev.Resources))
	for _, res := range ev.Resources {
		resources = append(resources, map[string]any{
			"resourceType": res.ResourceType,
			"resourceName": res.ResourceName,
		})
	}
	rec := map[string]any{
		"eventVersion": "1.08",
		"userIdentity": identity,
		"eventTime":    ev.EventTime,
		"eventSource":  ev.EventSource,
		"eventName":    ev.EventName,
		"awsRegion":    awsRegion(),
		"eventID":      ev.EventId,
		"eventType":    "AwsApiCall",
		"readOnly":     ev.ReadOnly,
		"resources":    resources,
	}
	if ev.InvokedBy != "" {
		rec["sourceIPAddress"] = ev.InvokedBy
	}
	// Real CloudTrail records failed API calls with errorCode/errorMessage at the
	// top level of the record (the call still happened — it just didn't succeed).
	if ev.ErrorCode != "" {
		rec["errorCode"] = ev.ErrorCode
	}
	if ev.ErrorMessage != "" {
		rec["errorMessage"] = ev.ErrorMessage
	}
	return rec
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
