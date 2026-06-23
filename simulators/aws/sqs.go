package main

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// SQS — used by runner workflows that need a managed message queue
// for fan-out / fan-in patterns + by application-event pipelines
// fronted by SNS → SQS.
//
// Wire protocol note: AWS migrated SQS from awsQuery to awsJson1_0
// in late 2023 (aws-sdk-go-v2 service/sqs v1.28+). All current SDK
// callers dispatch via `X-Amz-Target: AmazonSQS.<Action>` against
// POST /, with JSON request + JSON response bodies. The handlers
// below assume that protocol — older awsQuery callers would need
// a separate registration path (out of scope for the first cut).

// SQSQueue is a managed queue. Real SQS distinguishes Standard
// (at-least-once, best-effort ordering) from FIFO (exactly-once,
// strict ordering by MessageGroupId). The sim's queue is a Standard
// in-memory FIFO list with visibility-timeout-based at-most-once-
// per-handle semantics — close enough for integration tests of
// produce-consume-ack flows.
type SQSQueue struct {
	Name              string
	URL               string
	ARN               string
	CreatedTimestamp  int64
	VisibilityTimeout int // seconds; mirrored into Attributes["VisibilityTimeout"]
	Tags              map[string]string
	Messages          []SQSMessage
	// Attributes stores every operator-supplied queue attribute
	// from CreateQueue / SetQueueAttributes — DelaySeconds,
	// MessageRetentionPeriod, MaximumMessageSize, RedrivePolicy,
	// KmsMasterKeyId, FifoQueue, ContentBasedDeduplication, etc.
	// GetQueueAttributes echoes these alongside the system-emitted
	// values (QueueArn, CreatedTimestamp, message counts).
	Attributes map[string]string
}

// SQSMessage is one message currently buffered in a queue.
// VisibleAt is the Unix-second timestamp at which this message
// becomes visible to ReceiveMessage callers again — set forward
// on receive by the queue's VisibilityTimeout, set to 0 again on
// DeleteMessage (which removes the message entirely).
type SQSMessage struct {
	MessageId               string
	Body                    string
	MD5OfBody               string
	ReceiptHandle           string
	SentTimestamp           int64
	VisibleAt               int64
	ApproximateReceiveCount int
	FirstReceivedAt         int64
	MessageAttributes       map[string]SQSMessageAttribute
	MD5OfMessageAttributes  string
}

type SQSMessageAttribute struct {
	DataType    string `json:"DataType"`
	StringValue string `json:"StringValue,omitempty"`
	BinaryValue []byte `json:"BinaryValue,omitempty"`
}

// sqsMessageAttributeMD5 computes the MD5 digest of a message-attribute set the
// way real SQS does (the aws-sdk-go-v2 client validates it on receive), per
// https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-message-metadata.html:
// attributes sorted by name, each encoded as length-prefixed name + data-type,
// a transport byte (1 string/number, 2 binary), then the length-prefixed value.
func sqsMessageAttributeMD5(attrs map[string]SQSMessageAttribute) string {
	if len(attrs) == 0 {
		return ""
	}
	names := make([]string, 0, len(attrs))
	for n := range attrs {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	encStr := func(s string) {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(s)))
		buf.Write(l[:])
		buf.WriteString(s)
	}
	encBytes := func(b []byte) {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(b)))
		buf.Write(l[:])
		buf.Write(b)
	}
	for _, n := range names {
		a := attrs[n]
		encStr(n)
		encStr(a.DataType)
		if strings.HasPrefix(a.DataType, "Binary") {
			buf.WriteByte(2)
			encBytes(a.BinaryValue)
		} else {
			buf.WriteByte(1)
			encStr(a.StringValue)
		}
	}
	sum := md5.Sum(buf.Bytes())
	return hex.EncodeToString(sum[:])
}

// sqsSelectMessageAttributeSubset returns the stored attributes restricted to
// the requested names ("All" / ".*" select everything), the way ReceiveMessage
// scopes them. Returns nil when none match. The typed subset lets the caller
// both render it and recompute MD5OfMessageAttributes over exactly the returned
// set.
func sqsSelectMessageAttributeSubset(attrs map[string]SQSMessageAttribute, requested []string) map[string]SQSMessageAttribute {
	if len(attrs) == 0 || len(requested) == 0 {
		return nil
	}
	all := false
	want := map[string]bool{}
	for _, n := range requested {
		if n == "All" || n == ".*" {
			all = true
		}
		want[n] = true
	}
	out := map[string]SQSMessageAttribute{}
	for name, a := range attrs {
		if all || want[name] {
			out[name] = a
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// sqsRenderMessageAttributes renders an attribute set into the ReceiveMessage
// JSON response shape.
func sqsRenderMessageAttributes(attrs map[string]SQSMessageAttribute) map[string]any {
	out := map[string]any{}
	for name, a := range attrs {
		entry := map[string]any{"DataType": a.DataType}
		if strings.HasPrefix(a.DataType, "Binary") {
			entry["BinaryValue"] = a.BinaryValue
		} else {
			entry["StringValue"] = a.StringValue
		}
		out[name] = entry
	}
	return out
}

var sqsQueues sim.Store[SQSQueue]

// sqsQueueURL builds the canonical queue URL real SQS emits.
// external: real-AWS canonical `sqs.<region>.amazonaws.com` host;
// the aws-sdk-go-v2 SQS client treats the queue URL as an opaque
// key (used as the QueueUrl input to every subsequent operation)
// rather than dereferencing it directly, so this works for SDK
// callers configured with a sim endpoint.
func sqsQueueURL(name string) string {
	return fmt.Sprintf("https://sqs.%s.amazonaws.com/%s/%s",
		awsRegion(), awsAccountID(), name)
}

func sqsQueueARN(name string) string {
	return fmt.Sprintf("arn:aws:sqs:%s:%s:%s",
		awsRegion(), awsAccountID(), name)
}

// sqsEnqueueByARN delivers a plain-body message into the queue named by its ARN,
// reporting whether the queue exists. The sim's internal event delivery (SNS /
// EventBridge → SQS) calls this after the delivery has been authorized against
// the queue's resource policy.
func sqsEnqueueByARN(queueARN, body string) bool {
	name := queueARN
	if i := strings.LastIndex(queueARN, ":"); i >= 0 {
		name = queueARN[i+1:]
	}
	if _, ok := sqsQueues.Get(name); !ok {
		return false
	}
	sqsEnqueue(name, sqsSendEntry{MessageBody: body})
	return true
}

func registerSQS(r *sim.AWSRouter, srv *sim.Server) {
	// Message-level ops are CloudTrail DATA events (excluded from LookupEvents);
	// queue-management ops are management events.
	cloudTrailDeclareDataEvents("sqs.amazonaws.com",
		"SendMessage", "SendMessageBatch", "ReceiveMessage", "DeleteMessage",
		"DeleteMessageBatch", "ChangeMessageVisibility", "ChangeMessageVisibilityBatch")
	sqsQueues = sim.MakeStore[SQSQueue](srv.DB(), "sqs_queues")

	r.Register("AmazonSQS.CreateQueue", handleSQSCreateQueue)
	r.Register("AmazonSQS.DeleteQueue", handleSQSDeleteQueue)
	r.Register("AmazonSQS.GetQueueUrl", handleSQSGetQueueURL)
	r.Register("AmazonSQS.ListQueues", handleSQSListQueues)
	r.Register("AmazonSQS.GetQueueAttributes", handleSQSGetQueueAttributes)
	r.Register("AmazonSQS.SetQueueAttributes", handleSQSSetQueueAttributes)
	r.Register("AmazonSQS.SendMessage", handleSQSSendMessage)
	r.Register("AmazonSQS.SendMessageBatch", handleSQSSendMessageBatch)
	r.Register("AmazonSQS.ReceiveMessage", handleSQSReceiveMessage)
	r.Register("AmazonSQS.DeleteMessage", handleSQSDeleteMessage)
	r.Register("AmazonSQS.TagQueue", handleSQSTagQueue)
	r.Register("AmazonSQS.UntagQueue", handleSQSUntagQueue)
	r.Register("AmazonSQS.ListQueueTags", handleSQSListQueueTags)
	r.Register("AmazonSQS.PurgeQueue", handleSQSPurgeQueue)
}

// queueNameFromURL extracts the queue name from a queue URL or
// from the bare name. Real SQS callers pass the URL on
// every op after CreateQueue.
func queueNameFromURL(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

func sqsErrorJSON(w http.ResponseWriter, code, message string, status int) {
	sim.AWSError(w, code, message, status)
}

func sqsQueueDoesNotExist(w http.ResponseWriter) {
	w.Header().Set("x-amzn-query-error", "AWS.SimpleQueueService.NonExistentQueue;Sender")
	sqsErrorJSON(w, "QueueDoesNotExist", "The specified queue does not exist.", http.StatusBadRequest)
}

// sqsNumericAttributes are the queue attributes whose values must be
// integer-valued. Real SQS rejects a non-numeric value for any of these
// with InvalidParameterValue at CreateQueue / SetQueueAttributes time.
var sqsNumericAttributes = []string{
	"VisibilityTimeout",
	"DelaySeconds",
	"MessageRetentionPeriod",
	"MaximumMessageSize",
	"ReceiveMessageWaitTimeSeconds",
}

// sqsValidateNumericAttributes returns the first numeric attribute whose
// value is present but not parseable as an integer, mirroring real SQS's
// InvalidParameterValue rejection. ok is false when a value is invalid.
func sqsValidateNumericAttributes(attrs map[string]string) (msg string, ok bool) {
	for _, k := range sqsNumericAttributes {
		v, present := attrs[k]
		if !present {
			continue
		}
		if _, err := strconv.Atoi(v); err != nil {
			return fmt.Sprintf("Invalid value for the parameter %s.", k), false
		}
	}
	return "", true
}

func handleSQSCreateQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueName  string            `json:"QueueName"`
		Attributes map[string]string `json:"Attributes"`
		Tags       map[string]string `json:"tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sqsErrorJSON(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	if req.QueueName == "" {
		sqsErrorJSON(w, "MissingParameter", "QueueName is required", http.StatusBadRequest)
		return
	}
	if msg, ok := sqsFifoNameAttrMismatch(req.QueueName, req.Attributes); !ok {
		sqsErrorJSON(w, "InvalidParameterValue", msg, http.StatusBadRequest)
		return
	}
	if msg, ok := sqsValidateNumericAttributes(req.Attributes); !ok {
		sqsErrorJSON(w, "InvalidParameterValue", msg, http.StatusBadRequest)
		return
	}
	if existing, ok := sqsQueues.Get(req.QueueName); ok {
		// Real SQS is idempotent only on an identical-attribute Create; the same
		// name with different attribute values is a QueueNameExists error.
		if sqsAttributesEqual(existing.Attributes, req.Attributes) {
			sim.WriteJSON(w, http.StatusOK, map[string]string{"QueueUrl": existing.URL})
			return
		}
		sqsErrorJSON(w, "QueueNameExists",
			"A queue already exists with the same name and a different value for attribute(s).",
			http.StatusBadRequest)
		return
	}
	q := SQSQueue{
		Name:              req.QueueName,
		URL:               sqsQueueURL(req.QueueName),
		ARN:               sqsQueueARN(req.QueueName),
		CreatedTimestamp:  time.Now().Unix(),
		VisibilityTimeout: 30,
		Tags:              map[string]string{},
		Attributes:        map[string]string{},
	}
	// Persist every operator-supplied attribute. VisibilityTimeout
	// is mirrored to the typed field for hot-path use by Receive;
	// every attribute is also retained in the Attributes map so
	// GetQueueAttributes echoes them all back.
	for k, v := range req.Attributes {
		q.Attributes[k] = v
		if k == "VisibilityTimeout" {
			if n, err := strconv.Atoi(v); err == nil {
				q.VisibilityTimeout = n
			}
		}
	}
	for k, v := range req.Tags {
		q.Tags[k] = v
	}
	sqsQueues.Put(req.QueueName, q)
	sim.WriteJSON(w, http.StatusOK, map[string]string{"QueueUrl": q.URL})
}

func handleSQSDeleteQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl string `json:"QueueUrl"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sqsErrorJSON(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	if !sqsQueues.Delete(queueNameFromURL(req.QueueUrl)) {
		sqsQueueDoesNotExist(w)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

// handleSQSPurgeQueue removes every message currently held in the
// queue without deleting the queue itself. Real SQS allows one
// PurgeQueue per minute; the sim doesn't enforce that throttle
// because the surface that depends on it (testing-time purge to
// reset a queue between runs) explicitly works around it. The
// queue's Attributes / Tags are preserved across the purge.
func handleSQSPurgeQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl string `json:"QueueUrl"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sqsErrorJSON(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	name := queueNameFromURL(req.QueueUrl)
	q, ok := sqsQueues.Get(name)
	if !ok {
		sqsQueueDoesNotExist(w)
		return
	}
	q.Messages = nil
	sqsQueues.Put(name, q)
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleSQSGetQueueURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueName string `json:"QueueName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sqsErrorJSON(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	q, ok := sqsQueues.Get(req.QueueName)
	if !ok {
		sqsQueueDoesNotExist(w)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]string{"QueueUrl": q.URL})
}

func handleSQSListQueues(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueNamePrefix string `json:"QueueNamePrefix"`
		NextToken       string `json:"NextToken"`
		MaxResults      int    `json:"MaxResults"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	all := sqsQueues.List()
	sortBy(all, func(q SQSQueue) string { return q.URL })
	var filtered []SQSQueue
	for _, q := range all {
		if req.QueueNamePrefix != "" && !strings.HasPrefix(q.Name, req.QueueNamePrefix) {
			continue
		}
		filtered = append(filtered, q)
	}
	page, next := awsPage(filtered, req.NextToken, req.MaxResults, 1000)
	urls := make([]string, 0, len(page))
	for _, q := range page {
		urls = append(urls, q.URL)
	}
	out := map[string]any{"QueueUrls": urls}
	if next != "" {
		out["NextToken"] = next
	}
	sim.WriteJSON(w, http.StatusOK, out)
}

func handleSQSGetQueueAttributes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl       string   `json:"QueueUrl"`
		AttributeNames []string `json:"AttributeNames"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sqsErrorJSON(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	q, ok := sqsQueues.Get(queueNameFromURL(req.QueueUrl))
	if !ok {
		sqsQueueDoesNotExist(w)
		return
	}
	wanted := map[string]bool{}
	for _, n := range req.AttributeNames {
		wanted[n] = true
	}
	all := len(wanted) == 0 || wanted["All"]

	now := time.Now().Unix()
	visibleCount, invisibleCount := 0, 0
	for _, m := range q.Messages {
		if m.VisibleAt <= now {
			visibleCount++
		} else {
			invisibleCount++
		}
	}

	// Start with system-emitted attributes; layer in operator-set
	// attributes (DelaySeconds, MessageRetentionPeriod, etc.) on
	// top so an explicit operator value wins over any default.
	// VisibilityTimeout is sourced from the typed field so the
	// CreateQueue → SetQueueAttributes hot path stays consistent.
	allAttrs := map[string]string{
		"QueueArn":                              q.ARN,
		"VisibilityTimeout":                     strconv.Itoa(q.VisibilityTimeout),
		"CreatedTimestamp":                      strconv.FormatInt(q.CreatedTimestamp, 10),
		"ApproximateNumberOfMessages":           strconv.Itoa(visibleCount),
		"ApproximateNumberOfMessagesNotVisible": strconv.Itoa(invisibleCount),
	}
	for k, v := range q.Attributes {
		allAttrs[k] = v
	}
	// VisibilityTimeout in the Attributes map may diverge from the
	// typed field if SetQueueAttributes only updated the map; the
	// typed field is the source of truth for the seen-on-receive
	// behavior, so it wins.
	allAttrs["VisibilityTimeout"] = strconv.Itoa(q.VisibilityTimeout)
	out := map[string]string{}
	for k, v := range allAttrs {
		if all || wanted[k] {
			out[k] = v
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Attributes": out})
}

func handleSQSSetQueueAttributes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl   string            `json:"QueueUrl"`
		Attributes map[string]string `json:"Attributes"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sqsErrorJSON(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	name := queueNameFromURL(req.QueueUrl)
	if _, ok := sqsQueues.Get(name); !ok {
		sqsQueueDoesNotExist(w)
		return
	}
	if msg, ok := sqsValidateNumericAttributes(req.Attributes); !ok {
		sqsErrorJSON(w, "InvalidParameterValue", msg, http.StatusBadRequest)
		return
	}
	var queueARN string
	sqsQueues.Update(name, func(q *SQSQueue) {
		if q.Attributes == nil {
			q.Attributes = map[string]string{}
		}
		for k, v := range req.Attributes {
			q.Attributes[k] = v
			if k == "VisibilityTimeout" {
				if n, err := strconv.Atoi(v); err == nil {
					q.VisibilityTimeout = n
				}
			}
		}
		queueARN = q.ARN
	})
	// Mirror the queue policy into the central resource-policy store so the IAM
	// enforcement gate can resolve it by the queue ARN.
	if policy, ok := req.Attributes["Policy"]; ok {
		if policy == "" {
			iamDeleteResourcePolicy(queueARN)
		} else {
			iamPutResourcePolicy(queueARN, policy)
		}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

// sqsSendEntry is the common send payload shared by SendMessage and
// each SendMessageBatch entry. The batch entry carries an Id; the
// single-send path leaves it empty.
type sqsSendEntry struct {
	Id                     string                         `json:"Id"`
	MessageBody            string                         `json:"MessageBody"`
	MessageAttributes      map[string]SQSMessageAttribute `json:"MessageAttributes"`
	MessageGroupId         string                         `json:"MessageGroupId"`
	MessageDeduplicationId string                         `json:"MessageDeduplicationId"`
}

// sqsValidateSend applies the per-message validation real SQS performs
// before enqueue: non-empty body, the 256 KiB body limit, and — for a
// FIFO queue — the MessageGroupId and dedup requirements. It returns an
// (errCode, message) pair on failure; empty code means the entry is valid.
func sqsValidateSend(q SQSQueue, e sqsSendEntry) (string, string) {
	if e.MessageBody == "" {
		return "MissingParameter", "MessageBody is required"
	}
	if len(e.MessageBody) > sqsMaxMessageBytes {
		return "InvalidParameterValue",
			"One or more parameters are invalid. Reason: Message must be shorter than 262144 bytes."
	}
	if sqsQueueIsFifo(q) {
		if e.MessageGroupId == "" {
			return "MissingParameter", "The request must contain the parameter MessageGroupId."
		}
		if !sqsContentBasedDedup(q) && e.MessageDeduplicationId == "" {
			return "InvalidParameterValue",
				"The queue should either have ContentBasedDeduplication enabled or MessageDeduplicationId provided explicitly"
		}
	}
	return "", ""
}

// sqsEnqueue appends a validated entry to the queue and returns the
// SDK-shaped result fields (MessageId, MD5OfMessageBody, and — when
// attributes are present — MD5OfMessageAttributes).
func sqsEnqueue(name string, e sqsSendEntry) (msgID, md5OfBody, md5OfAttrs string) {
	msgID = generateUUID()
	hash := md5.Sum([]byte(e.MessageBody))
	md5OfBody = hex.EncodeToString(hash[:])
	md5OfAttrs = sqsMessageAttributeMD5(e.MessageAttributes)
	now := time.Now().Unix()
	sqsQueues.Update(name, func(q *SQSQueue) {
		q.Messages = append(q.Messages, SQSMessage{
			MessageId:              msgID,
			Body:                   e.MessageBody,
			MD5OfBody:              md5OfBody,
			SentTimestamp:          now,
			MessageAttributes:      e.MessageAttributes,
			MD5OfMessageAttributes: md5OfAttrs,
		})
	})
	return msgID, md5OfBody, md5OfAttrs
}

// sqsEnqueueBody appends a raw message body to a queue with the MD5 and
// timestamp a real delivery sets, the same store mutation a SendMessage
// performs. It is the in-process enqueue path SNS→SQS fan-out uses to
// deliver a Notification envelope; a subsequent ReceiveMessage returns it.
func sqsEnqueueBody(queueName, body string) {
	hash := md5.Sum([]byte(body))
	sqsQueues.Update(queueName, func(q *SQSQueue) {
		q.Messages = append(q.Messages, SQSMessage{
			MessageId:     generateUUID(),
			Body:          body,
			MD5OfBody:     hex.EncodeToString(hash[:]),
			SentTimestamp: time.Now().Unix(),
		})
	})
}

func handleSQSSendMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl               string                         `json:"QueueUrl"`
		MessageBody            string                         `json:"MessageBody"`
		MessageAttributes      map[string]SQSMessageAttribute `json:"MessageAttributes"`
		MessageGroupId         string                         `json:"MessageGroupId"`
		MessageDeduplicationId string                         `json:"MessageDeduplicationId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sqsErrorJSON(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	name := queueNameFromURL(req.QueueUrl)
	q, ok := sqsQueues.Get(name)
	if !ok {
		sqsQueueDoesNotExist(w)
		return
	}
	entry := sqsSendEntry{
		MessageBody:            req.MessageBody,
		MessageAttributes:      req.MessageAttributes,
		MessageGroupId:         req.MessageGroupId,
		MessageDeduplicationId: req.MessageDeduplicationId,
	}
	if code, msg := sqsValidateSend(q, entry); code != "" {
		sqsErrorJSON(w, code, msg, http.StatusBadRequest)
		return
	}
	msgID, md5OfBody, md5OfAttrs := sqsEnqueue(name, entry)
	resp := map[string]string{
		"MessageId":        msgID,
		"MD5OfMessageBody": md5OfBody,
	}
	if md5OfAttrs != "" {
		resp["MD5OfMessageAttributes"] = md5OfAttrs
	}
	sim.WriteJSON(w, http.StatusOK, resp)
}

// handleSQSSendMessageBatch sends up to 10 messages in a single call,
// reporting per-entry success/failure the way real SQS does. Batch-level
// failures (empty list, >10 entries, duplicate Ids, total payload over
// the 256 KiB limit) are returned as top-level errors; per-entry
// validation failures (e.g. a FIFO entry missing MessageGroupId) land in
// the Failed array with HTTP 200, matching the real wire contract.
func handleSQSSendMessageBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl string         `json:"QueueUrl"`
		Entries  []sqsSendEntry `json:"Entries"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sqsErrorJSON(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	name := queueNameFromURL(req.QueueUrl)
	q, ok := sqsQueues.Get(name)
	if !ok {
		sqsQueueDoesNotExist(w)
		return
	}
	if len(req.Entries) == 0 {
		sqsErrorJSON(w, "AWS.SimpleQueueService.EmptyBatchRequest",
			"There should be at least one SendMessageBatchRequestEntry in the request.",
			http.StatusBadRequest)
		return
	}
	if len(req.Entries) > 10 {
		sqsErrorJSON(w, "AWS.SimpleQueueService.TooManyEntriesInBatchRequest",
			"Maximum number of entries per request are 10. You have sent "+strconv.Itoa(len(req.Entries))+".",
			http.StatusBadRequest)
		return
	}
	seen := map[string]bool{}
	total := 0
	for _, e := range req.Entries {
		if seen[e.Id] {
			sqsErrorJSON(w, "AWS.SimpleQueueService.BatchEntryIdsNotDistinct",
				"Id "+e.Id+" repeated.", http.StatusBadRequest)
			return
		}
		seen[e.Id] = true
		total += len(e.MessageBody)
	}
	if total > sqsMaxMessageBytes {
		sqsErrorJSON(w, "AWS.SimpleQueueService.BatchRequestTooLong",
			"Batch requests cannot be longer than 262144 bytes. You have sent "+strconv.Itoa(total)+" bytes.",
			http.StatusBadRequest)
		return
	}

	successful := make([]map[string]string, 0, len(req.Entries))
	failed := make([]map[string]any, 0)
	for _, e := range req.Entries {
		if code, msg := sqsValidateSend(q, e); code != "" {
			failed = append(failed, map[string]any{
				"Id":          e.Id,
				"Code":        code,
				"Message":     msg,
				"SenderFault": true,
			})
			continue
		}
		msgID, md5OfBody, md5OfAttrs := sqsEnqueue(name, e)
		entry := map[string]string{
			"Id":               e.Id,
			"MessageId":        msgID,
			"MD5OfMessageBody": md5OfBody,
		}
		if md5OfAttrs != "" {
			entry["MD5OfMessageAttributes"] = md5OfAttrs
		}
		successful = append(successful, entry)
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Successful": successful,
		"Failed":     failed,
	})
}

func handleSQSReceiveMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl              string   `json:"QueueUrl"`
		MaxNumberOfMessages   int      `json:"MaxNumberOfMessages"`
		VisibilityTimeout     *int     `json:"VisibilityTimeout"`
		MessageAttributeNames []string `json:"MessageAttributeNames"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sqsErrorJSON(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	name := queueNameFromURL(req.QueueUrl)
	q, ok := sqsQueues.Get(name)
	if !ok {
		sqsQueueDoesNotExist(w)
		return
	}
	maxN := req.MaxNumberOfMessages
	if maxN == 0 {
		maxN = 1
	} else if maxN < 1 || maxN > 10 {
		sqsErrorJSON(w, "InvalidParameterValue",
			fmt.Sprintf("Value %d for parameter MaxNumberOfMessages is invalid. Reason: must be between 1 and 10.", maxN),
			http.StatusBadRequest)
		return
	}
	visTimeout := q.VisibilityTimeout
	if req.VisibilityTimeout != nil {
		visTimeout = *req.VisibilityTimeout
	}

	now := time.Now().Unix()
	var picked []SQSMessage

	sqsQueues.Update(name, func(qq *SQSQueue) {
		for i := range qq.Messages {
			if len(picked) >= maxN {
				break
			}
			if qq.Messages[i].VisibleAt > now {
				continue
			}
			qq.Messages[i].ReceiptHandle = generateUUID()
			qq.Messages[i].VisibleAt = now + int64(visTimeout)
			qq.Messages[i].ApproximateReceiveCount++
			if qq.Messages[i].FirstReceivedAt == 0 {
				qq.Messages[i].FirstReceivedAt = now
			}
			picked = append(picked, qq.Messages[i])
		}
	})

	out := make([]map[string]any, 0, len(picked))
	for _, m := range picked {
		msg := map[string]any{
			"MessageId":     m.MessageId,
			"ReceiptHandle": m.ReceiptHandle,
			"MD5OfBody":     m.MD5OfBody,
			"Body":          m.Body,
			"Attributes": map[string]string{
				"ApproximateReceiveCount":          strconv.Itoa(m.ApproximateReceiveCount),
				"SentTimestamp":                    strconv.FormatInt(m.SentTimestamp, 10),
				"ApproximateFirstReceiveTimestamp": strconv.FormatInt(m.FirstReceivedAt, 10),
			},
		}
		if subset := sqsSelectMessageAttributeSubset(m.MessageAttributes, req.MessageAttributeNames); subset != nil {
			msg["MessageAttributes"] = sqsRenderMessageAttributes(subset)
			// The MD5 must cover exactly the returned subset: aws-sdk-go-v2's
			// ValidateMessageChecksums recomputes MD5OfMessageAttributes over the
			// received attributes and rejects the message on mismatch, so a
			// partial selection cannot reuse the stored full-set digest.
			msg["MD5OfMessageAttributes"] = sqsMessageAttributeMD5(subset)
		}
		out = append(out, msg)
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Messages": out})
}

func handleSQSDeleteMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl      string `json:"QueueUrl"`
		ReceiptHandle string `json:"ReceiptHandle"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sqsErrorJSON(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	name := queueNameFromURL(req.QueueUrl)
	if _, ok := sqsQueues.Get(name); !ok {
		sqsQueueDoesNotExist(w)
		return
	}
	sqsQueues.Update(name, func(qq *SQSQueue) {
		out := qq.Messages[:0]
		for _, m := range qq.Messages {
			if m.ReceiptHandle == req.ReceiptHandle {
				continue
			}
			out = append(out, m)
		}
		qq.Messages = out
	})
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleSQSTagQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl string            `json:"QueueUrl"`
		Tags     map[string]string `json:"Tags"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sqsErrorJSON(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	name := queueNameFromURL(req.QueueUrl)
	if _, ok := sqsQueues.Get(name); !ok {
		sqsQueueDoesNotExist(w)
		return
	}
	sqsQueues.Update(name, func(q *SQSQueue) {
		if q.Tags == nil {
			q.Tags = map[string]string{}
		}
		for k, v := range req.Tags {
			q.Tags[k] = v
		}
	})
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleSQSUntagQueue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl string   `json:"QueueUrl"`
		TagKeys  []string `json:"TagKeys"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sqsErrorJSON(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	name := queueNameFromURL(req.QueueUrl)
	if _, ok := sqsQueues.Get(name); !ok {
		sqsQueueDoesNotExist(w)
		return
	}
	sqsQueues.Update(name, func(q *SQSQueue) {
		for _, k := range req.TagKeys {
			delete(q.Tags, k)
		}
	})
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleSQSListQueueTags(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl string `json:"QueueUrl"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sqsErrorJSON(w, "MalformedInputException", err.Error(), http.StatusBadRequest)
		return
	}
	q, ok := sqsQueues.Get(queueNameFromURL(req.QueueUrl))
	if !ok {
		sqsQueueDoesNotExist(w)
		return
	}
	tags := q.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"Tags": tags})
}

// sqsMaxMessageBytes is the SQS message-body size limit (256 KiB).
const sqsMaxMessageBytes = 262144

// sqsQueueIsFifo reports whether the queue is a FIFO queue, per its
// FifoQueue attribute (real SQS keys FIFO behavior off this attribute,
// which CreateQueue requires to agree with the .fifo name suffix).
func sqsQueueIsFifo(q SQSQueue) bool {
	return strings.EqualFold(q.Attributes["FifoQueue"], "true")
}

// sqsContentBasedDedup reports whether the queue has
// ContentBasedDeduplication enabled — when set, a FIFO send may omit
// MessageDeduplicationId (SQS derives the dedup id from the body).
func sqsContentBasedDedup(q SQSQueue) bool {
	return strings.EqualFold(q.Attributes["ContentBasedDeduplication"], "true")
}

// sqsFifoNameAttrMismatch enforces the real-SQS coupling between the
// .fifo name suffix and the FifoQueue=true attribute: a queue named
// "<x>.fifo" requires FifoQueue=true, and FifoQueue=true requires the
// .fifo suffix. It returns (errMessage, ok=false) on a mismatch.
func sqsFifoNameAttrMismatch(name string, attrs map[string]string) (string, bool) {
	hasSuffix := strings.HasSuffix(name, ".fifo")
	fifoAttr := strings.EqualFold(attrs["FifoQueue"], "true")
	if fifoAttr && !hasSuffix {
		return "The name of a FIFO queue can only include alphanumeric characters, hyphens, or underscores, must end with the .fifo suffix and be 1 to 80 in length", false
	}
	if hasSuffix && !fifoAttr {
		// Real SQS rejects the '.' in a non-FIFO queue name: a dot is
		// only valid as part of the trailing .fifo suffix, which is
		// itself only valid when FifoQueue=true.
		return "Can only include alphanumeric characters, hyphens, or underscores. 1 to 80 in length", false
	}
	return "", true
}

// sqsAttributesEqual reports whether two attribute maps are equal (CreateQueue
// idempotency: same name + same attributes → OK; differing → QueueNameExists).
func sqsAttributesEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
