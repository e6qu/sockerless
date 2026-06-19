package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// xmlEscape escapes &, <, >, ", ' for inclusion in awsQuery XML
// response bodies and attribute values.
func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// SNS — pub/sub topic + subscription + publish flow. The most common
// real-world use is SNS → SQS fan-out (one topic, N SQS subscribers
// receiving each published message). The sim implements that path
// in-process: a Publish to a topic enumerates Subscription entries
// pointing at SQS queues and pushes the message body into each
// queue's Messages slice. HTTP / HTTPS / email / SMS subscribers
// are recorded but not delivered (real HTTP/HTTPS
// subscription confirmation is out of scope for the first cut).
//
// Wire protocol: awsQuery (POST / + Action form param + XML envelope),
// same as SQS.

type SNSTopic struct {
	Name string
	ARN  string
	Tags map[string]string
	// Attributes are mutable settings — Policy, DisplayName,
	// DeliveryPolicy, KmsMasterKeyId, etc. — set via
	// SetTopicAttributes and surfaced by GetTopicAttributes alongside
	// the fixed read-only Owner / SubscriptionsConfirmed fields.
	Attributes map[string]string
}

type SNSSubscription struct {
	ARN       string
	TopicARN  string
	Protocol  string // "sqs", "http", "https", "email", "sms", "lambda"
	Endpoint  string // queue ARN for sqs, URL for http(s), email addr, etc.
	Confirmed bool
}

var (
	snsTopics        sim.Store[SNSTopic]
	snsSubscriptions sim.Store[SNSSubscription]
)

func snsTopicARN(name string) string {
	return fmt.Sprintf("arn:aws:sns:%s:%s:%s",
		awsRegion(), awsAccountID(), name)
}

func snsSubscriptionARN(topicName string) string {
	return fmt.Sprintf("arn:aws:sns:%s:%s:%s:%s",
		awsRegion(), awsAccountID(), topicName, generateUUID())
}

// snsAPIVersion is the canonical AWS SNS API version (Query
// Protocol). Used to disambiguate Action names from other awsQuery
// services in the AWSQueryRouter dispatch.
const snsAPIVersion = "2010-03-31"

func registerSNS(r *sim.AWSQueryRouter, srv *sim.Server) {
	snsTopics = sim.MakeStore[SNSTopic](srv.DB(), "sns_topics")
	snsSubscriptions = sim.MakeStore[SNSSubscription](srv.DB(), "sns_subscriptions")

	r.RegisterVersioned(snsAPIVersion, "CreateTopic", handleSNSCreateTopic)
	r.RegisterVersioned(snsAPIVersion, "DeleteTopic", handleSNSDeleteTopic)
	r.RegisterVersioned(snsAPIVersion, "ListTopics", handleSNSListTopics)
	r.RegisterVersioned(snsAPIVersion, "GetTopicAttributes", handleSNSGetTopicAttributes)
	r.RegisterVersioned(snsAPIVersion, "SetTopicAttributes", handleSNSSetTopicAttributes)
	r.RegisterVersioned(snsAPIVersion, "Subscribe", handleSNSSubscribe)
	r.RegisterVersioned(snsAPIVersion, "Unsubscribe", handleSNSUnsubscribe)
	r.RegisterVersioned(snsAPIVersion, "ListSubscriptions", handleSNSListSubscriptions)
	r.RegisterVersioned(snsAPIVersion, "ListSubscriptionsByTopic", handleSNSListSubscriptionsByTopic)
	r.RegisterVersioned(snsAPIVersion, "Publish", handleSNSPublish)
	r.RegisterVersioned(snsAPIVersion, "PublishBatch", handleSNSPublishBatch)
	r.RegisterVersioned(snsAPIVersion, "TagResource", handleSNSTagResource)
	r.RegisterVersioned(snsAPIVersion, "UntagResource", handleSNSUntagResource)
	r.RegisterVersioned(snsAPIVersion, "ListTagsForResource", handleSNSListTagsForResource)
}

func snsXMLResponse(w http.ResponseWriter, op string, body string, requestID string) {
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w,
		`<%sResponse xmlns="http://sns.amazonaws.com/doc/2010-03-31/">%s<ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata></%sResponse>`,
		op, body, requestID, op)
}

func snsErrorXML(w http.ResponseWriter, code, message string, status int, requestID string) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w,
		`<ErrorResponse xmlns="http://sns.amazonaws.com/doc/2010-03-31/"><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>%s</RequestId></ErrorResponse>`,
		code, message, requestID)
}

func handleSNSCreateTopic(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("Name")
	if name == "" {
		snsErrorXML(w, "InvalidParameter", "Name is required", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	// CreateTopic carries initial topic attributes as Attributes.entry.N.{key,value}
	// (the SNS query flattening of the Attributes map). FifoTopic, in
	// particular, must be set here so the .fifo-suffix coupling and the
	// per-message FIFO rules apply.
	attrs := snsCreateTopicAttributes(r)
	if msg, ok := snsFifoNameAttrMismatch(name, attrs); !ok {
		snsErrorXML(w, "InvalidParameter", msg, http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	arn := snsTopicARN(name)
	if _, ok := snsTopics.Get(name); !ok {
		snsTopics.Put(name, SNSTopic{Name: name, ARN: arn, Tags: make(map[string]string), Attributes: attrs})
	}
	body := fmt.Sprintf("<CreateTopicResult><TopicArn>%s</TopicArn></CreateTopicResult>", xmlEscape(arn))
	snsXMLResponse(w, "CreateTopic", body, sim.RequestID(r.Context()))
}

// snsCreateTopicAttributes pulls the initial Attributes map out of a
// CreateTopic query request (Attributes.entry.N.key / .value).
func snsCreateTopicAttributes(r *http.Request) map[string]string {
	out := map[string]string{}
	for i := 1; i <= 50; i++ {
		k := r.FormValue(fmt.Sprintf("Attributes.entry.%d.key", i))
		if k == "" {
			break
		}
		out[k] = r.FormValue(fmt.Sprintf("Attributes.entry.%d.value", i))
	}
	return out
}

// snsTopicIsFifo reports whether a topic is FIFO (FifoTopic=true).
func snsTopicIsFifo(t SNSTopic) bool {
	return strings.EqualFold(t.Attributes["FifoTopic"], "true")
}

// snsTopicContentBasedDedup reports whether the topic has
// ContentBasedDeduplication enabled.
func snsTopicContentBasedDedup(t SNSTopic) bool {
	return strings.EqualFold(t.Attributes["ContentBasedDeduplication"], "true")
}

// snsFifoNameAttrMismatch enforces the real-SNS coupling between the
// .fifo name suffix and FifoTopic=true: a topic named "<x>.fifo"
// requires FifoTopic=true and vice-versa. Returns (errMessage, ok=false)
// on mismatch.
func snsFifoNameAttrMismatch(name string, attrs map[string]string) (string, bool) {
	hasSuffix := strings.HasSuffix(name, ".fifo")
	fifoAttr := strings.EqualFold(attrs["FifoTopic"], "true")
	if fifoAttr && !hasSuffix {
		return "Fifo topic names must end with .fifo and be 1 to 256 characters long.", false
	}
	if hasSuffix && !fifoAttr {
		return "Topic names must be made up of only uppercase and lowercase ASCII letters, numbers, underscores, and hyphens, and must be between 1 and 256 characters long.", false
	}
	return "", true
}

func handleSNSDeleteTopic(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("TopicArn")
	name := snsTopicNameFromARN(arn)
	// snsTopics.Delete returning false is fine here — real SNS
	// DeleteTopic is idempotent and returns success even when the
	// topic doesn't exist. The cascade-clear below runs either way
	// so any dangling subscriptions also get cleared.
	snsTopics.Delete(name)
	// Cascade: drop subscriptions pointing at this topic.
	for _, sub := range snsSubscriptions.List() {
		if sub.TopicARN == arn {
			snsSubscriptions.Delete(sub.ARN)
		}
	}
	snsXMLResponse(w, "DeleteTopic", "", sim.RequestID(r.Context()))
}

func snsTopicNameFromARN(arn string) string {
	if i := strings.LastIndex(arn, ":"); i >= 0 {
		return arn[i+1:]
	}
	return arn
}

func handleSNSListTopics(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	token := r.FormValue("NextToken")
	all := snsTopics.List()
	sortBy(all, func(t SNSTopic) string { return t.ARN })
	page, next := awsPage(all, token, 0, 100)

	var b strings.Builder
	b.WriteString("<ListTopicsResult><Topics>")
	for _, t := range page {
		fmt.Fprintf(&b, "<member><TopicArn>%s</TopicArn></member>", xmlEscape(t.ARN))
	}
	b.WriteString("</Topics>")
	if next != "" {
		fmt.Fprintf(&b, "<NextToken>%s</NextToken>", xmlEscape(next))
	}
	b.WriteString("</ListTopicsResult>")
	snsXMLResponse(w, "ListTopics", b.String(), sim.RequestID(r.Context()))
}

func handleSNSGetTopicAttributes(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("TopicArn")
	name := snsTopicNameFromARN(arn)
	t, ok := snsTopics.Get(name)
	if !ok {
		snsErrorXML(w, "NotFound", "Topic does not exist", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	// Subscription counts are the load-bearing attribute for
	// real-world consumers (CloudWatch alarms on confirmed-vs-pending).
	confirmed := 0
	pending := 0
	for _, sub := range snsSubscriptions.List() {
		if sub.TopicARN == t.ARN {
			if sub.Confirmed {
				confirmed++
			} else {
				pending++
			}
		}
	}
	attrs := map[string]string{
		"TopicArn":               t.ARN,
		"DisplayName":            t.Name,
		"Owner":                  awsAccountID(),
		"SubscriptionsConfirmed": fmt.Sprintf("%d", confirmed),
		"SubscriptionsPending":   fmt.Sprintf("%d", pending),
		"SubscriptionsDeleted":   "0",
	}
	// Mutable attributes set via SetTopicAttributes override the
	// fixed defaults — real SNS does the same (e.g. a `DisplayName`
	// set on the topic replaces the auto-derived value).
	for k, v := range t.Attributes {
		attrs[k] = v
	}
	var b strings.Builder
	b.WriteString("<GetTopicAttributesResult><Attributes>")
	for k, v := range attrs {
		fmt.Fprintf(&b, "<entry><key>%s</key><value>%s</value></entry>",
			xmlEscape(k), xmlEscape(v))
	}
	b.WriteString("</Attributes></GetTopicAttributesResult>")
	snsXMLResponse(w, "GetTopicAttributes", b.String(), sim.RequestID(r.Context()))
}

// handleSNSSetTopicAttributes updates a single (AttributeName,
// AttributeValue) pair on a topic. terraform-provider-aws emits this
// repeatedly on aws_sns_topic for DeliveryPolicy / Policy / KmsMasterKeyId /
// etc.; pre-fix the sim returned InvalidAction and the apply failed.
func handleSNSSetTopicAttributes(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("TopicArn")
	name := snsTopicNameFromARN(arn)
	t, ok := snsTopics.Get(name)
	if !ok {
		snsErrorXML(w, "NotFound", "Topic does not exist", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	attrName := r.FormValue("AttributeName")
	attrValue := r.FormValue("AttributeValue")
	if attrName == "" {
		snsErrorXML(w, "InvalidParameter",
			"AttributeName is required",
			http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	if t.Attributes == nil {
		t.Attributes = map[string]string{}
	}
	if attrValue == "" {
		delete(t.Attributes, attrName)
	} else {
		t.Attributes[attrName] = attrValue
	}
	snsTopics.Put(name, t)
	snsXMLResponse(w, "SetTopicAttributes", "", sim.RequestID(r.Context()))
}

func handleSNSSubscribe(w http.ResponseWriter, r *http.Request) {
	topicARN := r.FormValue("TopicArn")
	protocol := r.FormValue("Protocol")
	endpoint := r.FormValue("Endpoint")
	if topicARN == "" || protocol == "" || endpoint == "" {
		snsErrorXML(w, "InvalidParameter",
			"TopicArn, Protocol, and Endpoint are required",
			http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	name := snsTopicNameFromARN(topicARN)
	if _, ok := snsTopics.Get(name); !ok {
		snsErrorXML(w, "NotFound", "Topic does not exist", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	sub := SNSSubscription{
		ARN:       snsSubscriptionARN(name),
		TopicARN:  topicARN,
		Protocol:  protocol,
		Endpoint:  endpoint,
		Confirmed: !snsProtocolRequiresConfirmation(protocol),
	}
	snsSubscriptions.Put(sub.ARN, sub)
	returnedARN := sub.ARN
	if !sub.Confirmed && !snsReturnSubscriptionARN(r) {
		returnedARN = "pending confirmation"
	}
	body := fmt.Sprintf("<SubscribeResult><SubscriptionArn>%s</SubscriptionArn></SubscribeResult>", xmlEscape(returnedARN))
	snsXMLResponse(w, "Subscribe", body, sim.RequestID(r.Context()))
}

func snsProtocolRequiresConfirmation(protocol string) bool {
	switch strings.ToLower(protocol) {
	case "sqs", "lambda", "application", "firehose":
		return false
	default:
		return true
	}
}

func snsReturnSubscriptionARN(r *http.Request) bool {
	return strings.EqualFold(r.FormValue("ReturnSubscriptionArn"), "true")
}

func handleSNSUnsubscribe(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("SubscriptionArn")
	snsSubscriptions.Delete(arn)
	snsXMLResponse(w, "Unsubscribe", "", sim.RequestID(r.Context()))
}

func handleSNSListSubscriptions(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString("<ListSubscriptionsResult><Subscriptions>")
	for _, sub := range snsSubscriptions.List() {
		fmt.Fprintf(&b,
			"<member><SubscriptionArn>%s</SubscriptionArn><TopicArn>%s</TopicArn><Protocol>%s</Protocol><Endpoint>%s</Endpoint><Owner>%s</Owner></member>",
			xmlEscape(sub.ARN), xmlEscape(sub.TopicARN), xmlEscape(sub.Protocol),
			xmlEscape(sub.Endpoint), awsAccountID())
	}
	b.WriteString("</Subscriptions></ListSubscriptionsResult>")
	snsXMLResponse(w, "ListSubscriptions", b.String(), sim.RequestID(r.Context()))
}

func handleSNSListSubscriptionsByTopic(w http.ResponseWriter, r *http.Request) {
	topicARN := r.FormValue("TopicArn")
	var b strings.Builder
	b.WriteString("<ListSubscriptionsByTopicResult><Subscriptions>")
	for _, sub := range snsSubscriptions.List() {
		if sub.TopicARN != topicARN {
			continue
		}
		fmt.Fprintf(&b,
			"<member><SubscriptionArn>%s</SubscriptionArn><TopicArn>%s</TopicArn><Protocol>%s</Protocol><Endpoint>%s</Endpoint><Owner>%s</Owner></member>",
			xmlEscape(sub.ARN), xmlEscape(sub.TopicARN), xmlEscape(sub.Protocol),
			xmlEscape(sub.Endpoint), awsAccountID())
	}
	b.WriteString("</Subscriptions></ListSubscriptionsByTopicResult>")
	snsXMLResponse(w, "ListSubscriptionsByTopic", b.String(), sim.RequestID(r.Context()))
}

// handleSNSPublish fans the message out to in-process SQS
// subscribers. HTTP / HTTPS / email / SMS / Lambda subscribers
// are recorded but not delivered — recording the URL is enough
// for integration tests of "did the subscription configuration
// get applied"; in-flight delivery is out of scope for the first
// cut.
// snsPublishEntry is the common publish payload shared by Publish and
// each PublishBatch entry.
type snsPublishEntry struct {
	Id                     string
	Message                string
	Subject                string
	MessageGroupId         string
	MessageDeduplicationId string
}

// snsValidatePublish applies the per-message validation real SNS
// performs: non-empty message, the 256 KiB limit, and — for a FIFO
// topic — the MessageGroupId and dedup requirements. Returns an
// (errCode, message) pair; empty code means valid.
func snsValidatePublish(t SNSTopic, e snsPublishEntry) (string, string) {
	if e.Message == "" {
		return "InvalidParameter", "Invalid parameter: Empty value for parameter Message"
	}
	if len(e.Message) > 262144 {
		return "InvalidParameter", "Invalid parameter: Message too long"
	}
	if snsTopicIsFifo(t) {
		if e.MessageGroupId == "" {
			return "InvalidParameter", "Invalid parameter: The MessageGroupId parameter is required for FIFO topics"
		}
		if !snsTopicContentBasedDedup(t) && e.MessageDeduplicationId == "" {
			return "InvalidParameter",
				"Invalid parameter: The topic should either have ContentBasedDeduplication enabled or MessageDeduplicationId provided explicitly"
		}
	}
	return "", ""
}

// snsFanout delivers one published message to the topic's SQS
// subscribers in-process, wrapping it in SNS's canonical Notification
// envelope so SQS consumers parse it like real SNS→SQS fan-out.
func snsFanout(topicARN, msgID, subject, message string) {
	for _, sub := range snsSubscriptions.List() {
		if sub.TopicARN != topicARN || sub.Protocol != "sqs" {
			continue
		}
		// Subscription Endpoint for an SQS subscriber is the queue ARN
		// (arn:aws:sqs:<region>:<account>:<queue-name>).
		queueName := snsTopicNameFromARN(sub.Endpoint)
		if _, ok := sqsQueues.Get(queueName); !ok {
			continue
		}
		envelope := fmt.Sprintf(
			`{"Type":"Notification","MessageId":%q,"TopicArn":%q,"Subject":%q,"Message":%q}`,
			msgID, topicARN, subject, message)
		envHash := md5.Sum([]byte(envelope))
		sqsQueues.Update(queueName, func(q *SQSQueue) {
			q.Messages = append(q.Messages, SQSMessage{
				MessageId:     generateUUID(),
				Body:          envelope,
				MD5OfBody:     hex.EncodeToString(envHash[:]),
				SentTimestamp: nowUnix(),
			})
		})
	}
}

func handleSNSPublish(w http.ResponseWriter, r *http.Request) {
	topicARN := r.FormValue("TopicArn")
	if topicARN == "" {
		snsErrorXML(w, "InvalidParameter",
			"TopicArn and Message are required",
			http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	name := snsTopicNameFromARN(topicARN)
	t, ok := snsTopics.Get(name)
	if !ok {
		snsErrorXML(w, "NotFound", "Topic does not exist", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	entry := snsPublishEntry{
		Message:                r.FormValue("Message"),
		Subject:                r.FormValue("Subject"),
		MessageGroupId:         r.FormValue("MessageGroupId"),
		MessageDeduplicationId: r.FormValue("MessageDeduplicationId"),
	}
	if code, msg := snsValidatePublish(t, entry); code != "" {
		snsErrorXML(w, code, msg, http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	msgID := generateUUID()
	snsFanout(topicARN, msgID, entry.Subject, entry.Message)

	body := fmt.Sprintf("<PublishResult><MessageId>%s</MessageId></PublishResult>", xmlEscape(msgID))
	snsXMLResponse(w, "Publish", body, sim.RequestID(r.Context()))
}

// handleSNSPublishBatch publishes up to 10 messages to a topic in a
// single call, reporting per-entry success/failure. Batch-level errors
// (empty list, >10 entries, duplicate Ids, total payload over 256 KiB)
// are top-level error responses; per-entry validation failures (e.g. a
// FIFO entry missing MessageGroupId) land in the <Failed> list with HTTP
// 200, matching the real SNS wire contract.
func handleSNSPublishBatch(w http.ResponseWriter, r *http.Request) {
	requestID := sim.RequestID(r.Context())
	topicARN := r.FormValue("TopicArn")
	if topicARN == "" {
		snsErrorXML(w, "InvalidParameter", "TopicArn is required", http.StatusBadRequest, requestID)
		return
	}
	name := snsTopicNameFromARN(topicARN)
	t, ok := snsTopics.Get(name)
	if !ok {
		snsErrorXML(w, "NotFound", "Topic does not exist", http.StatusNotFound, requestID)
		return
	}
	entries := snsPublishBatchEntries(r)
	if len(entries) == 0 {
		snsErrorXML(w, "EmptyBatchRequest",
			"The batch request doesn't contain any entries.", http.StatusBadRequest, requestID)
		return
	}
	if len(entries) > 10 {
		snsErrorXML(w, "TooManyEntriesInBatchRequest",
			"The batch request contains more entries than permissible.", http.StatusBadRequest, requestID)
		return
	}
	seen := map[string]bool{}
	total := 0
	for _, e := range entries {
		if seen[e.Id] {
			snsErrorXML(w, "BatchEntryIdsNotDistinct",
				"Two or more batch entries in the request have the same Id.", http.StatusBadRequest, requestID)
			return
		}
		seen[e.Id] = true
		total += len(e.Message)
	}
	if total > 262144 {
		snsErrorXML(w, "BatchRequestTooLong",
			"The length of all the batch messages put together is more than the limit.", http.StatusBadRequest, requestID)
		return
	}

	var b strings.Builder
	b.WriteString("<PublishBatchResult><Successful>")
	var failed strings.Builder
	for _, e := range entries {
		if code, msg := snsValidatePublish(t, e); code != "" {
			fmt.Fprintf(&failed,
				"<member><Id>%s</Id><Code>%s</Code><Message>%s</Message><SenderFault>true</SenderFault></member>",
				xmlEscape(e.Id), xmlEscape(code), xmlEscape(msg))
			continue
		}
		msgID := generateUUID()
		snsFanout(topicARN, msgID, e.Subject, e.Message)
		fmt.Fprintf(&b, "<member><Id>%s</Id><MessageId>%s</MessageId></member>",
			xmlEscape(e.Id), xmlEscape(msgID))
	}
	b.WriteString("</Successful><Failed>")
	b.WriteString(failed.String())
	b.WriteString("</Failed></PublishBatchResult>")
	snsXMLResponse(w, "PublishBatch", b.String(), requestID)
}

// snsPublishBatchEntries pulls the PublishBatchRequestEntries.member.N.*
// flattened query parameters into typed entries.
func snsPublishBatchEntries(r *http.Request) []snsPublishEntry {
	var out []snsPublishEntry
	// Parse past the 10-entry cap so an over-limit batch is detectable
	// (the handler rejects len > 10 with TooManyEntriesInBatchRequest).
	for i := 1; ; i++ {
		prefix := fmt.Sprintf("PublishBatchRequestEntries.member.%d.", i)
		id := r.FormValue(prefix + "Id")
		msg := r.FormValue(prefix + "Message")
		// An absent member breaks the contiguous member sequence.
		if id == "" && msg == "" &&
			r.FormValue(prefix+"MessageGroupId") == "" &&
			r.FormValue(prefix+"Subject") == "" {
			break
		}
		out = append(out, snsPublishEntry{
			Id:                     id,
			Message:                msg,
			Subject:                r.FormValue(prefix + "Subject"),
			MessageGroupId:         r.FormValue(prefix + "MessageGroupId"),
			MessageDeduplicationId: r.FormValue(prefix + "MessageDeduplicationId"),
		})
	}
	return out
}

func handleSNSTagResource(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceArn")
	name := snsTopicNameFromARN(arn)
	if _, ok := snsTopics.Get(name); !ok {
		snsErrorXML(w, "NotFound", "Topic does not exist", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	snsTopics.Update(name, func(t *SNSTopic) {
		if t.Tags == nil {
			t.Tags = make(map[string]string)
		}
		for i := 1; i <= 50; i++ {
			k := r.FormValue(fmt.Sprintf("Tags.member.%d.Key", i))
			v := r.FormValue(fmt.Sprintf("Tags.member.%d.Value", i))
			if k == "" {
				break
			}
			t.Tags[k] = v
		}
	})
	snsXMLResponse(w, "TagResource", "", sim.RequestID(r.Context()))
}

func handleSNSUntagResource(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceArn")
	name := snsTopicNameFromARN(arn)
	if _, ok := snsTopics.Get(name); !ok {
		snsErrorXML(w, "NotFound", "Topic does not exist", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	snsTopics.Update(name, func(t *SNSTopic) {
		for i := 1; i <= 50; i++ {
			k := r.FormValue(fmt.Sprintf("TagKeys.member.%d", i))
			if k == "" {
				break
			}
			delete(t.Tags, k)
		}
	})
	snsXMLResponse(w, "UntagResource", "", sim.RequestID(r.Context()))
}

func handleSNSListTagsForResource(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceArn")
	name := snsTopicNameFromARN(arn)
	t, ok := snsTopics.Get(name)
	if !ok {
		snsErrorXML(w, "NotFound", "Topic does not exist", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	var b strings.Builder
	b.WriteString("<ListTagsForResourceResult><Tags>")
	for k, v := range t.Tags {
		fmt.Fprintf(&b, "<member><Key>%s</Key><Value>%s</Value></member>",
			xmlEscape(k), xmlEscape(v))
	}
	b.WriteString("</Tags></ListTagsForResourceResult>")
	snsXMLResponse(w, "ListTagsForResource", b.String(), sim.RequestID(r.Context()))
}

// nowUnix returns the current Unix-second timestamp.
func nowUnix() int64 {
	return time.Now().Unix()
}
