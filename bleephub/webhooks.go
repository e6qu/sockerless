package bleephub

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // X-Hub-Signature legacy SHA1 alongside SHA256 — required for parity with real GH
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitStorage "github.com/go-git/go-git/v5/storage"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"gopkg.in/yaml.v3"
)

func computeHMACSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func computeHMACSignatureSHA1(secret string, payload []byte) string {
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(payload)
	return "sha1=" + hex.EncodeToString(mac.Sum(nil))
}

// emitWebhookEvent dispatches an event to matching webhooks (non-blocking).
// Org-owned repos additionally fan the event out to the owner org's hooks —
// real GitHub delivers every repo event on an org repository to matching
// organization webhooks too.
func (s *Server) emitWebhookEvent(repoKey, eventType, action string, payload interface{}) {
	hooks := s.store.ListHooks(repoKey)
	if ownerLogin, _, found := strings.Cut(repoKey, "/"); found {
		if s.store.GetOrg(ownerLogin) != nil {
			hooks = append(hooks, s.store.ListOrgHooks(ownerLogin)...)
		}
	}

	// Org-owned repos carry a top-level `organization` object on event
	// payloads (real GitHub adds it for every event on an org repo).
	// Attached centrally: every repo event funnels through here, and the
	// store lookup needs server access the payload builders don't have.
	if m, ok := payload.(map[string]interface{}); ok {
		if _, has := m["organization"]; !has {
			ownerLogin, _, found := strings.Cut(repoKey, "/")
			if found {
				if org := s.store.GetOrg(ownerLogin); org != nil {
					m["organization"] = orgWebhookPayload(org)
				}
			}
		}
	}

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

func hookMatchesEvent(hook *Webhook, eventType string) bool {
	for _, e := range hook.Events {
		if e == eventType || e == "*" {
			return true
		}
	}
	return false
}

// deliverWebhook sends an HTTP POST with retries (3 attempts, exponential backoff).
func (s *Server) deliverWebhook(hook *Webhook, event, action string, payloadBytes []byte) {
	guid := uuid.New().String()
	backoffs := []time.Duration{0, 1 * time.Second, 5 * time.Second}

	for attempt, backoff := range backoffs {
		if attempt > 0 {
			time.Sleep(backoff)
		}

		delivery := s.doDeliverAttempt(hook, event, action, guid, payloadBytes, attempt > 0)
		s.store.AddDelivery(delivery)
		if hook.RepoKey != "" {
			s.store.SetHookLastResponse(hook.RepoKey, hook.ID, deliveryLastResponse(delivery))
		} else if hook.OrgLogin != "" {
			s.store.SetOrgHookLastResponse(hook.OrgLogin, hook.ID, deliveryLastResponse(delivery))
		}

		if delivery.StatusCode >= 200 && delivery.StatusCode < 300 {
			return
		}
	}
}

// deliveryLastResponse maps a delivery attempt to the hook.last_response shape.
func deliveryLastResponse(d *WebhookDelivery) *HookLastResponse {
	msg := http.StatusText(d.StatusCode)
	if d.StatusCode >= 200 && d.StatusCode < 300 {
		msg = "OK"
	} else if d.StatusCode == 0 {
		msg = "failed to connect"
	}
	return &HookLastResponse{
		Code:    d.StatusCode,
		Status:  deliveryStatus(d.StatusCode),
		Message: msg,
	}
}

// doDeliverAttempt performs a single webhook delivery attempt.
func (s *Server) doDeliverAttempt(hook *Webhook, event, action, guid string, payloadBytes []byte, redelivery bool) *WebhookDelivery {
	start := time.Now()

	// content_type=form (GitHub's default) sends the JSON payload as the
	// value of a `payload` form field with an x-www-form-urlencoded body,
	// and signs THAT body — not the raw JSON. content_type=json sends the
	// JSON verbatim. The stored hook.ContentType picks which.
	contentType := "application/json"
	bodyBytes := payloadBytes
	if hook.ContentType == "form" {
		contentType = "application/x-www-form-urlencoded"
		bodyBytes = []byte(url.Values{"payload": {string(payloadBytes)}}.Encode())
	}

	reqHeaders := map[string]string{
		"Content-Type":      contentType,
		"User-Agent":        "GitHub-Hookshot/bleephub",
		"X-GitHub-Event":    event,
		"X-GitHub-Delivery": guid,
		"X-GitHub-Hook-ID":  strconv.Itoa(hook.ID),
	}
	if hook.Secret != "" {
		reqHeaders["X-Hub-Signature-256"] = computeHMACSignature(hook.Secret, bodyBytes)
		reqHeaders["X-Hub-Signature"] = computeHMACSignatureSHA1(hook.Secret, bodyBytes)
	}
	// Installation-target headers identify the resource that owns the hook.
	// App-bound hooks (HookID < 0) target the GitHub App ("integration");
	// org hooks target the organization's id; repository hooks the repo's.
	switch {
	case hook.MarketplaceSlug != "":
		// GitHub Marketplace webhooks are listing-scoped and do not advertise a
		// repository, organization, or GitHub App installation target.
	case hook.ID < 0:
		reqHeaders["X-GitHub-Hook-Installation-Target-Type"] = "integration"
		reqHeaders["X-GitHub-Hook-Installation-Target-ID"] = strconv.Itoa(-hook.ID)
	case hook.OrgLogin != "":
		reqHeaders["X-GitHub-Hook-Installation-Target-Type"] = "organization"
		if org := s.store.GetOrg(hook.OrgLogin); org != nil {
			reqHeaders["X-GitHub-Hook-Installation-Target-ID"] = strconv.Itoa(org.ID)
		}
	default:
		reqHeaders["X-GitHub-Hook-Installation-Target-Type"] = "repository"
		parts := splitRepoKeyParts(hook.RepoKey)
		if parts[1] != "" {
			if repo := s.store.GetRepo(parts[0], parts[1]); repo != nil {
				reqHeaders["X-GitHub-Hook-Installation-Target-ID"] = strconv.Itoa(repo.ID)
			}
		}
	}

	httpReq, err := http.NewRequest("POST", hook.URL, bytes.NewReader(bodyBytes))
	if err != nil {
		return &WebhookDelivery{
			HookID:      hook.ID,
			TargetURL:   hook.URL,
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

	client := &http.Client{Timeout: 10 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)}
	resp, err := client.Do(httpReq)
	elapsed := time.Since(start).Seconds()

	delivery := &WebhookDelivery{
		HookID:      hook.ID,
		TargetURL:   hook.URL,
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

// triggerWorkflowsForEvent triggers matching workflows from git storage
// for a concrete event occurrence. action carries the activity type
// ("opened", "synchronize", repository_dispatch's event_type, ...);
// payload is the webhook payload the event emitted — it becomes the
// run's github.event context and feeds the trigger filters (push
// before/after shas, PR base branch).
func (s *Server) triggerWorkflowsForEvent(repoKey, eventType, action, ref string, payload map[string]interface{}) {
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

	sha := resolveRefSha(stor, ref)
	if sha == "0000000000000000000000000000000000000000" {
		s.logger.Error().
			Str("repo", repoKey).
			Str("event", eventType).
			Str("ref", ref).
			Msg("workflow trigger rejected because the git ref did not resolve to a commit")
		return
	}
	ev := s.buildTriggerEvent(stor, eventType, action, ref, payload)

	for name, content := range workflowFiles {
		on, err := ParseWorkflowOn(content)
		if err != nil {
			s.logger.Warn().Err(err).Str("file", name).Msg("skip workflow with invalid on: definition")
			continue
		}
		if !workflowTriggersOn(on, ev) {
			continue
		}
		if s.workflowFileDisabled(repoKey, name) {
			s.logger.Info().Str("file", name).Str("trigger", eventType).Msg("workflow disabled — not triggered")
			continue
		}

		meta := &WorkflowEventMeta{
			EventName: eventType,
			Ref:       ref,
			Sha:       sha,
			Repo:      repoKey,
			Payload:   payload,
		}
		workflow, err := s.submitTriggeredWorkflow(name, content, meta)
		if err != nil {
			// Real GitHub creates a run with conclusion startup_failure
			// (no jobs) when a matched workflow can't start — the
			// failure must be visible on the runs API, not just a log.
			s.logger.Error().Err(err).Str("file", name).Msg("workflow failed at startup")
			s.createStartupFailureRun(name, content, meta)
			continue
		}

		s.logger.Info().
			Str("workflow_id", workflow.ID).
			Str("trigger", eventType).
			Str("action", action).
			Str("file", name).
			Msg("workflow triggered by event")
	}
}

// submitTriggeredWorkflow parses, expands, and submits one workflow file
// for an event. Event metadata must travel INTO submitWorkflow: it
// resolves the originating workflow file from RepoFullName at submit
// time (the run's workflow_id), and the workflow becomes visible to
// other goroutines the moment it is stored — patching fields afterwards
// would both mis-derive the file id and race those readers.
func (s *Server) submitTriggeredWorkflow(fileName string, content []byte, meta *WorkflowEventMeta) (*Workflow, error) {
	wfDef, err := ParseWorkflow(content)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", fileName, err)
	}

	expandedDef := expandMatrixJobs(wfDef)

	if expandedDef.Env == nil {
		expandedDef.Env = make(map[string]string)
	}
	expandedDef.Env["__defaultImage"] = ""

	serverURL := fmt.Sprintf("http://%s", s.addr)
	expandedDef.Env["__serverURL"] = serverURL

	return s.submitWorkflow(context.Background(), serverURL, expandedDef, "", meta)
}

// buildTriggerEvent assembles the filterable description of an event
// occurrence. For pushes the changed files come from the payload's
// before/after shas; for pull_request events the filterable ref is the
// BASE branch and the diff spans base...head.
func (s *Server) buildTriggerEvent(stor gitStorage.Storer, eventType, action, ref string, payload map[string]interface{}) triggerEvent {
	ev := triggerEvent{Type: eventType, Action: action, Ref: ref}
	switch eventType {
	case "push":
		before, _ := payload["before"].(string)
		after, _ := payload["after"].(string)
		ev.ChangedFiles, ev.ChangedFilesKnown = changedFilesBetween(stor, before, after)
	case "pull_request", "pull_request_target":
		pr, _ := payload["pull_request"].(map[string]interface{})
		if pr != nil {
			if base, _ := pr["base"].(map[string]interface{}); base != nil {
				if baseRef, _ := base["ref"].(string); baseRef != "" {
					ev.Ref = "refs/heads/" + baseRef
					baseSha, _ := base["sha"].(string)
					if baseSha == "" {
						baseSha = resolveBranchSha(stor, baseRef)
					}
					var headSha string
					if head, _ := pr["head"].(map[string]interface{}); head != nil {
						headSha, _ = head["sha"].(string)
						if headSha == "" {
							if headRef, _ := head["ref"].(string); headRef != "" {
								headSha = resolveBranchSha(stor, headRef)
							}
						}
					}
					ev.ChangedFiles, ev.ChangedFilesKnown = changedFilesBetween(stor, baseSha, headSha)
				}
			}
		}
	}
	return ev
}

// workflowFileDisabled reports whether the registered workflow file for
// (repo, filename) was manually disabled — disabled workflows never
// trigger, matching real GitHub.
func (s *Server) workflowFileDisabled(repoKey, filename string) bool {
	path := ".github/workflows/" + filename
	for _, f := range s.store.ListWorkflowFiles(repoKey) {
		if f.Path == path {
			return strings.HasPrefix(f.State, "disabled")
		}
	}
	return false
}

// resolveRefSha resolves the commit sha the triggering ref points at in git
// storage. Empty ref means HEAD. Non-empty refs must resolve exactly; event
// triggers must not silently substitute a different commit.
func resolveRefSha(stor gitStorage.Storer, ref string) string {
	const zeroSha = "0000000000000000000000000000000000000000"
	resolve := func(name plumbing.ReferenceName) (plumbing.Hash, bool) {
		r, err := stor.Reference(name)
		if err != nil {
			return plumbing.Hash{}, false
		}
		if r.Type() == plumbing.SymbolicReference {
			target, err := stor.Reference(r.Target())
			if err != nil {
				return plumbing.Hash{}, false
			}
			return target.Hash(), true
		}
		return r.Hash(), true
	}
	if ref != "" {
		if h, ok := resolve(plumbing.ReferenceName(ref)); ok && !h.IsZero() {
			return h.String()
		}
		return zeroSha
	}
	if h, ok := resolve(plumbing.HEAD); ok && !h.IsZero() {
		return h.String()
	}
	return zeroSha
}

// resolveGitHubRefInput resolves a GitHub API `ref` input. Public endpoints
// accept the same forms real GitHub accepts: full refs, branch names, tag
// names, and raw commit SHAs. The returned ref is normalized when a branch or
// tag name resolves; unresolved inputs stay fail-loud through the zero SHA.
func resolveGitHubRefInput(stor gitStorage.Storer, ref string) (string, string) {
	const zeroSha = "0000000000000000000000000000000000000000"
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ref, zeroSha
	}
	candidates := []string{ref}
	if !strings.HasPrefix(ref, "refs/") {
		candidates = append(candidates,
			plumbing.NewBranchReferenceName(ref).String(),
			plumbing.NewTagReferenceName(ref).String(),
		)
	}
	for _, candidate := range candidates {
		if sha := resolveRefSha(stor, candidate); sha != zeroSha {
			return candidate, sha
		}
	}
	if len(ref) == 40 {
		if _, err := hex.DecodeString(ref); err == nil {
			hash := plumbing.NewHash(ref)
			if _, err := stor.EncodedObject(plumbing.CommitObject, hash); err == nil {
				return ref, ref
			}
		}
	}
	return ref, zeroSha
}

func splitRepoKeyParts(repoKey string) [2]string {
	for i, c := range repoKey {
		if c == '/' {
			return [2]string{repoKey[:i], repoKey[i+1:]}
		}
	}
	return [2]string{repoKey, ""}
}

func listWorkflowFiles(stor gitStorage.Storer) map[string][]byte {
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

// createStartupFailureRun records a terminal, job-less run for a
// workflow that matched its trigger but could not start.
func (s *Server) createStartupFailureRun(fileName string, content []byte, meta *WorkflowEventMeta) {
	name := workflowNameFromYAML(content)
	if name == "" {
		name = strings.TrimSuffix(strings.TrimSuffix(fileName, ".yml"), ".yaml")
	}
	wf := &Workflow{
		ID:           uuid.New().String(),
		Name:         name,
		RunID:        s.store.ReserveRunID(),
		Jobs:         map[string]*WorkflowJob{},
		Status:       WorkflowStatusCompleted,
		Result:       ResultStartupFailure,
		CreatedAt:    time.Now(),
		EventName:    meta.EventName,
		Ref:          meta.Ref,
		Sha:          meta.Sha,
		RepoFullName: meta.Repo,
		EventPayload: meta.Payload,
	}
	wf.RunNumber = wf.RunID
	wf.WorkflowFileID, wf.WorkflowFilePath = s.resolveWorkflowFileForRun(wf)
	s.store.mu.Lock()
	s.store.Workflows[wf.ID] = wf
	s.store.persistWorkflowRecord(wf)
	s.store.mu.Unlock()
	s.queueActionsEvent(evRunCompleted, wf, nil)
}

// workflowNameFromYAML extracts just the workflow's name, tolerating
// definitions too broken for ParseWorkflow.
func workflowNameFromYAML(content []byte) string {
	var raw struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return ""
	}
	return raw.Name
}
