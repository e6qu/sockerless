package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
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

func registerSQS(r *sim.AWSRouter, srv *sim.Server) {
	sqsQueues = sim.MakeStore[SQSQueue](srv.DB(), "sqs_queues")

	r.Register("AmazonSQS.CreateQueue", handleSQSCreateQueue)
	r.Register("AmazonSQS.DeleteQueue", handleSQSDeleteQueue)
	r.Register("AmazonSQS.GetQueueUrl", handleSQSGetQueueURL)
	r.Register("AmazonSQS.ListQueues", handleSQSListQueues)
	r.Register("AmazonSQS.GetQueueAttributes", handleSQSGetQueueAttributes)
	r.Register("AmazonSQS.SetQueueAttributes", handleSQSSetQueueAttributes)
	r.Register("AmazonSQS.SendMessage", handleSQSSendMessage)
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
	if existing, ok := sqsQueues.Get(req.QueueName); ok {
		// Real SQS is idempotent on identical-attribute Create.
		sim.WriteJSON(w, http.StatusOK, map[string]string{"QueueUrl": existing.URL})
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
	})
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleSQSSendMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl    string `json:"QueueUrl"`
		MessageBody string `json:"MessageBody"`
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
	if req.MessageBody == "" {
		sqsErrorJSON(w, "MissingParameter", "MessageBody is required", http.StatusBadRequest)
		return
	}
	msgID := generateUUID()
	hash := md5.Sum([]byte(req.MessageBody))
	md5OfBody := hex.EncodeToString(hash[:])
	now := time.Now().Unix()
	sqsQueues.Update(name, func(q *SQSQueue) {
		q.Messages = append(q.Messages, SQSMessage{
			MessageId:     msgID,
			Body:          req.MessageBody,
			MD5OfBody:     md5OfBody,
			SentTimestamp: now,
		})
	})
	sim.WriteJSON(w, http.StatusOK, map[string]string{
		"MessageId":        msgID,
		"MD5OfMessageBody": md5OfBody,
	})
}

func handleSQSReceiveMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QueueUrl            string `json:"QueueUrl"`
		MaxNumberOfMessages int    `json:"MaxNumberOfMessages"`
		VisibilityTimeout   *int   `json:"VisibilityTimeout"`
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
			picked = append(picked, qq.Messages[i])
		}
	})

	out := make([]map[string]any, 0, len(picked))
	for _, m := range picked {
		out = append(out, map[string]any{
			"MessageId":     m.MessageId,
			"ReceiptHandle": m.ReceiptHandle,
			"MD5OfBody":     m.MD5OfBody,
			"Body":          m.Body,
			"Attributes": map[string]string{
				"ApproximateReceiveCount": strconv.Itoa(m.ApproximateReceiveCount),
				"SentTimestamp":           strconv.FormatInt(m.SentTimestamp, 10),
			},
		})
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
