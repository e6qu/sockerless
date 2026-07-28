package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

var (
	snsSigningKey      *rsa.PrivateKey
	snsSigningCertPEM  []byte
	snsSigningCertName string
)

func registerSNSHTTPDelivery(srv *sim.Server) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("generate Amazon SNS signing key: %v", err))
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: "sns." + awsRegion() + ".amazonaws.com"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"sns." + awsRegion() + ".amazonaws.com"},
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		panic(fmt.Sprintf("create Amazon SNS signing certificate: %v", err))
	}
	sum := sha256.Sum256(raw)
	snsSigningKey = key
	snsSigningCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw})
	snsSigningCertName = hex.EncodeToString(sum[:])

	srv.HandleFunc("GET /SimpleNotificationService-"+snsSigningCertName+".pem", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-pem-file")
		_, _ = w.Write(snsSigningCertPEM)
	})
}

func snsRequestOrigin(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	return scheme + "://" + r.Host
}

func snsControlURL(sub SNSSubscription, action string, values url.Values) string {
	values.Set("Action", action)
	values.Set("Version", snsAPIVersion)
	return strings.TrimRight(sub.ControlPlaneOrigin, "/") + "/?" + values.Encode()
}

func snsSigningCertificateURL(sub SNSSubscription) string {
	return strings.TrimRight(sub.ControlPlaneOrigin, "/") +
		"/SimpleNotificationService-" + snsSigningCertName + ".pem"
}

func snsDeliverHTTPConfirmation(sub SNSSubscription) {
	envelope := snsConfirmationEnvelope(sub)
	messageID := snsEnvelopeString(envelope, "MessageId")
	snsPostHTTP(sub, "SubscriptionConfirmation", envelope, messageID, "")
}

func snsDeliverHTTPNotification(sub SNSSubscription, messageID, subject, message string, attributes map[string]SQSMessageAttribute) {
	if strings.EqualFold(sub.Attributes["RawMessageDelivery"], "true") {
		snsPostHTTP(sub, "Notification", nil, messageID, message)
		return
	}
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	unsubscribeURL := snsControlURL(sub, "Unsubscribe", url.Values{
		"SubscriptionArn": {sub.ARN},
	})
	envelope := map[string]any{
		"Type":             "Notification",
		"MessageId":        messageID,
		"TopicArn":         sub.TopicARN,
		"Message":          message,
		"Timestamp":        timestamp,
		"SignatureVersion": "1",
		"SigningCertURL":   snsSigningCertificateURL(sub),
		"UnsubscribeURL":   unsubscribeURL,
	}
	if subject != "" {
		envelope["Subject"] = subject
	}
	if messageAttributes := snsMessageAttributesEnvelope(attributes); messageAttributes != nil {
		envelope["MessageAttributes"] = messageAttributes
	}
	envelope["Signature"] = snsSignEnvelope(envelope)
	snsPostHTTP(sub, "Notification", envelope, messageID, "")
}

func snsEnvelopeString(envelope map[string]any, field string) string {
	value, _ := envelope[field].(string)
	return value
}

func snsSignEnvelope(envelope map[string]any) string {
	var fields []string
	switch snsEnvelopeString(envelope, "Type") {
	case "Notification":
		fields = []string{"Message", "MessageId"}
		if snsEnvelopeString(envelope, "Subject") != "" {
			fields = append(fields, "Subject")
		}
		fields = append(fields, "Timestamp", "TopicArn", "Type")
	default:
		fields = []string{"Message", "MessageId", "SubscribeURL", "Timestamp", "Token", "TopicArn", "Type"}
	}
	var canonical strings.Builder
	for _, field := range fields {
		canonical.WriteString(field)
		canonical.WriteByte('\n')
		canonical.WriteString(snsEnvelopeString(envelope, field))
		canonical.WriteByte('\n')
	}
	digest := sha1.Sum([]byte(canonical.String()))
	signature, err := rsa.SignPKCS1v15(rand.Reader, snsSigningKey, crypto.SHA1, digest[:])
	if err != nil {
		panic(fmt.Sprintf("sign Amazon SNS message: %v", err))
	}
	return base64.StdEncoding.EncodeToString(signature)
}

func snsPostHTTP(sub SNSSubscription, messageType string, envelope map[string]any, messageID, raw string) {
	var body []byte
	if envelope != nil {
		body, _ = json.Marshal(envelope)
	} else {
		body = []byte(raw)
	}
	req, err := http.NewRequest(http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		cwEvalLogger.Error().Err(err).Str("endpoint", sub.Endpoint).Msg("Amazon SNS HTTP delivery request failed")
		return
	}
	req.Header.Set("Content-Type", "text/plain; charset=UTF-8")
	req.Header.Set("x-amz-sns-message-type", messageType)
	if envelope != nil {
		req.Header.Set("x-amz-sns-message-id", snsEnvelopeString(envelope, "MessageId"))
		req.Header.Set("x-amz-sns-topic-arn", snsEnvelopeString(envelope, "TopicArn"))
	} else {
		req.Header.Set("x-amz-sns-message-id", messageID)
		req.Header.Set("x-amz-sns-topic-arn", sub.TopicARN)
		req.Header.Set("x-amz-sns-subscription-arn", sub.ARN)
		req.Header.Set("x-amz-sns-rawdelivery", "true")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		cwEvalLogger.Error().Err(err).Str("endpoint", sub.Endpoint).Msg("Amazon SNS HTTP delivery failed")
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		cwEvalLogger.Error().Int("status", resp.StatusCode).Str("endpoint", sub.Endpoint).Msg("Amazon SNS HTTP delivery was rejected")
	}
}
