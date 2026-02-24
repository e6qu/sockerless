package bleephub

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// computeHMACSignature computes the HMAC-SHA256 signature for a webhook payload.
func computeHMACSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// emitWebhookEvent dispatches an event to all matching webhooks for a repo.
// Non-blocking: launches a goroutine per matching webhook.
func (s *Server) emitWebhookEvent(repoKey, eventType, action string, payload interface{}) {
	hooks := s.store.ListHooks(repoKey)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error().Err(err).Str("repo", repoKey).Msg("failed to marshal webhook payload")
		return
	}

	for _, hook := range hooks {
		if !hook.Active {
			continue
		}
		if !hookMatchesEvent(hook, eventType) {
			continue
		}
		h := hook // capture for goroutine
		go s.deliverWebhook(h, eventType, action, payloadBytes)
	}
}

// hookMatchesEvent checks if a webhook is subscribed to the given event type.
func hookMatchesEvent(hook *Webhook, eventType string) bool {
	for _, e := range hook.Events {
		if e == eventType || e == "*" {
			return true
		}
	}
	return false
}

// deliverWebhook sends an HTTP POST with headers and retries (3 attempts, exponential backoff).
func (s *Server) deliverWebhook(hook *Webhook, event, action string, payloadBytes []byte) {
	guid := uuid.New().String()
	backoffs := []time.Duration{0, 1 * time.Second, 5 * time.Second}

	for attempt, backoff := range backoffs {
		if attempt > 0 {
			time.Sleep(backoff)
		}

		delivery := s.doDeliverAttempt(hook, event, action, guid, payloadBytes, attempt > 0)
		s.store.AddDelivery(delivery)

		if delivery.StatusCode >= 200 && delivery.StatusCode < 300 {
			return
		}
	}
}

// doDeliverAttempt performs a single HTTP POST to the webhook URL and records the result.
func (s *Server) doDeliverAttempt(hook *Webhook, event, action, guid string, payloadBytes []byte, redelivery bool) *WebhookDelivery {
	start := time.Now()

	reqHeaders := map[string]string{
		"Content-Type":      "application/json",
		"User-Agent":        "GitHub-Hookshot/bleephub",
		"X-GitHub-Event":    event,
		"X-GitHub-Delivery": guid,
	}
	if hook.Secret != "" {
		reqHeaders["X-Hub-Signature-256"] = computeHMACSignature(hook.Secret, payloadBytes)
	}

	httpReq, err := http.NewRequest("POST", hook.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		return &WebhookDelivery{
			HookID:      hook.ID,
			GUID:        guid,
			Event:       event,
			Action:      action,
			StatusCode:  0,
			Duration:    time.Since(start).Seconds(),
			Request:     &DeliveryRequest{Headers: reqHeaders, Payload: json.RawMessage(payloadBytes)},
			Response:    &DeliveryResponse{StatusCode: 0, Body: err.Error()},
			Redelivery:  redelivery,
			DeliveredAt: time.Now(),
		}
	}

	for k, v := range reqHeaders {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	elapsed := time.Since(start).Seconds()

	delivery := &WebhookDelivery{
		HookID:      hook.ID,
		GUID:        guid,
		Event:       event,
		Action:      action,
		Redelivery:  redelivery,
		DeliveredAt: time.Now(),
		Duration:    elapsed,
		Request:     &DeliveryRequest{Headers: reqHeaders, Payload: json.RawMessage(payloadBytes)},
	}

	if err != nil {
		delivery.StatusCode = 0
		delivery.Response = &DeliveryResponse{StatusCode: 0, Body: err.Error()}
		return delivery
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	respHeaders := make(map[string]string)
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}

	delivery.StatusCode = resp.StatusCode
	delivery.Response = &DeliveryResponse{
		StatusCode: resp.StatusCode,
		Headers:    respHeaders,
		Body:       string(respBody),
	}

	return delivery
}

// triggerWorkflowsForEvent checks for matching workflow files in git storage
// and triggers them when a push or pull_request event fires.
func (s *Server) triggerWorkflowsForEvent(repoKey, eventType, ref string) {
	parts := splitRepoKeyParts(repoKey)
	if parts[0] == "" {
		return
	}

	stor := s.store.GetGitStorage(parts[0], parts[1])
	if stor == nil {
		return
	}

	workflowFiles := listWorkflowFiles(stor)
	if len(workflowFiles) == 0 {
		return
	}

	for name, content := range workflowFiles {
		wfDef, err := ParseWorkflow(content)
		if err != nil {
			s.logger.Debug().Err(err).Str("file", name).Msg("skip unparseable workflow")
			continue
		}

		if !workflowMatchesEvent(content, eventType) {
			continue
		}

		expandedDef := expandMatrixJobs(wfDef)

		if expandedDef.Env == nil {
			expandedDef.Env = make(map[string]string)
		}
		expandedDef.Env["__defaultImage"] = "alpine:latest"

		serverURL := fmt.Sprintf("http://%s", s.addr)
		expandedDef.Env["__serverURL"] = serverURL

		workflow, err := s.submitWorkflow(context.Background(), serverURL, expandedDef, "alpine:latest")
		if err != nil {
			s.logger.Error().Err(err).Str("file", name).Msg("failed to trigger workflow")
			continue
		}

		workflow.EventName = eventType
		workflow.Ref = ref
		workflow.RepoFullName = repoKey

		s.logger.Info().
			Str("workflow_id", workflow.ID).
			Str("trigger", eventType).
			Str("file", name).
			Msg("workflow triggered by event")
	}
}

// splitRepoKeyParts splits "owner/repo" into [owner, repo].
func splitRepoKeyParts(repoKey string) [2]string {
	for i, c := range repoKey {
		if c == '/' {
			return [2]string{repoKey[:i], repoKey[i+1:]}
		}
	}
	return [2]string{repoKey, ""}
}

// listWorkflowFiles reads .github/workflows/*.yml from git storage.
func listWorkflowFiles(stor *memory.Storage) map[string][]byte {
	// Find HEAD
	headRef, err := stor.Reference(plumbing.HEAD)
	if err != nil {
		return nil
	}

	var commitHash plumbing.Hash
	if headRef.Type() == plumbing.SymbolicReference {
		targetRef, err := stor.Reference(headRef.Target())
		if err != nil {
			return nil
		}
		commitHash = targetRef.Hash()
	} else {
		commitHash = headRef.Hash()
	}

	commit, err := object.GetCommit(stor, commitHash)
	if err != nil {
		return nil
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil
	}

	// Navigate to .github/workflows/
	ghEntry, err := tree.FindEntry(".github")
	if err != nil {
		return nil
	}
	ghTree, err := object.GetTree(stor, ghEntry.Hash)
	if err != nil {
		return nil
	}
	wfEntry, err := ghTree.FindEntry("workflows")
	if err != nil {
		return nil
	}
	wfTree, err := object.GetTree(stor, wfEntry.Hash)
	if err != nil {
		return nil
	}

	result := make(map[string][]byte)
	for _, entry := range wfTree.Entries {
		if !entry.Mode.IsFile() {
			continue
		}
		if !strings.HasSuffix(entry.Name, ".yml") && !strings.HasSuffix(entry.Name, ".yaml") {
			continue
		}
		blob, err := object.GetBlob(stor, entry.Hash)
		if err != nil {
			continue
		}
		reader, err := blob.Reader()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			continue
		}
		result[entry.Name] = content
	}
	return result
}

// workflowMatchesEvent checks if a workflow YAML has an "on" trigger matching the event.
func workflowMatchesEvent(yamlContent []byte, eventType string) bool {
	var raw struct {
		On interface{} `yaml:"on"`
	}
	if err := yaml.Unmarshal(yamlContent, &raw); err != nil {
		return false
	}
	if raw.On == nil {
		// "true" key from YAML — `on:` without value maps to boolean true
		return false
	}

	switch v := raw.On.(type) {
	case string:
		return v == eventType
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok && s == eventType {
				return true
			}
		}
	case map[string]interface{}:
		_, ok := v[eventType]
		return ok
	}
	return false
}
