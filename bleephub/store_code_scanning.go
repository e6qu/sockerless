package bleephub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CodeScanningAlertInstance is one occurrence of a code-scanning alert.
type CodeScanningAlertInstance struct {
	Ref         string `json:"ref"`
	AnalysisKey string `json:"analysis_key"`
	Category    string `json:"category"`
	State       string `json:"state"`
	CommitSHA   string `json:"commit_sha"`
	Path        string `json:"path"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	StartColumn int    `json:"start_column"`
	EndColumn   int    `json:"end_column"`
	Message     string `json:"message"`
}

// CodeScanningAlert is a repo-scoped code-scanning alert produced by SARIF
// uploads or the internal seed endpoint.
type CodeScanningAlert struct {
	ID               int                         `json:"id"`
	NodeID           string                      `json:"node_id"`
	Number           int                         `json:"number"`
	RepoKey          string                      `json:"repo_key"`
	RuleID           string                      `json:"rule_id"`
	RuleSeverity     string                      `json:"rule_severity"`
	RuleDescription  string                      `json:"rule_description"`
	ToolName         string                      `json:"tool_name"`
	State            string                      `json:"state"`
	DismissedReason  string                      `json:"dismissed_reason"`
	DismissedComment string                      `json:"dismissed_comment"`
	DismissedAt      *time.Time                  `json:"dismissed_at"`
	FixedAt          *time.Time                  `json:"fixed_at"`
	HTMLURL          string                      `json:"html_url"`
	URL              string                      `json:"url"`
	InstancesURL     string                      `json:"instances_url"`
	Instances        []CodeScanningAlertInstance `json:"instances"`
	CreatedAt        time.Time                   `json:"created_at"`
	UpdatedAt        time.Time                   `json:"updated_at"`
}

// CodeScanningAnalysis is a single code-scanning analysis run for a repo.
type CodeScanningAnalysis struct {
	ID            int       `json:"id"`
	NodeID        string    `json:"node_id"`
	RepoKey       string    `json:"repo_key"`
	Ref           string    `json:"ref"`
	CommitSHA     string    `json:"commit_sha"`
	AnalysisKey   string    `json:"analysis_key"`
	Category      string    `json:"category"`
	ToolName      string    `json:"tool_name"`
	ResultsCount  int       `json:"results_count"`
	RulesCount    int       `json:"rules_count"`
	SARIFUploadID string    `json:"sarif_upload_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	HTMLURL       string    `json:"html_url"`
	URL           string    `json:"url"`
}

// SARIFUpload tracks a SARIF upload request. Real GitHub is asynchronous;
// bleephub processes synchronously and stores the upload as complete.
type SARIFUpload struct {
	ID        string    `json:"id"`
	RepoKey   string    `json:"repo_key"`
	Status    string    `json:"status"`
	Errors    []string  `json:"errors"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateCodeScanningAlert seeds a code-scanning alert directly. This is the
// internal/admin path used by tests when constructing SARIF is unnecessary.
func (st *Store) CreateCodeScanningAlert(repoKey, ruleID, severity, description, toolName, state string, instances []CodeScanningAlertInstance) *CodeScanningAlert {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.CodeScanningAlertsByRepo[repoKey] == nil {
		st.CodeScanningAlertsByRepo[repoKey] = make(map[int]*CodeScanningAlert)
	}
	if st.CodeScanningNextNumber[repoKey] == 0 {
		st.CodeScanningNextNumber[repoKey] = 1
	}

	now := time.Now().UTC()
	if state == "" {
		state = "open"
	}

	number := st.CodeScanningNextNumber[repoKey]
	st.CodeScanningNextNumber[repoKey] = number + 1

	alert := &CodeScanningAlert{
		ID:              st.NextCodeScanningAlertID,
		NodeID:          fmt.Sprintf("CSWA%d", st.NextCodeScanningAlertID),
		Number:          number,
		RepoKey:         repoKey,
		RuleID:          ruleID,
		RuleSeverity:    severity,
		RuleDescription: description,
		ToolName:        toolName,
		State:           state,
		Instances:       instances,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	st.NextCodeScanningAlertID++

	st.CodeScanningAlerts[alert.ID] = alert
	st.CodeScanningAlertsByRepo[repoKey][number] = alert
	st.persistCodeScanningAlert(alert)
	return alert
}

// GetCodeScanningAlert returns an alert by repo + alert number.
func (st *Store) GetCodeScanningAlert(repoKey string, number int) *CodeScanningAlert {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.CodeScanningAlertsByRepo[repoKey][number]
}

// ListCodeScanningAlerts returns repo alerts filtered/sorted per GitHub's list
// endpoint.
func (st *Store) ListCodeScanningAlerts(repoKey, state, severity, toolName, rule, sortField, direction string) []*CodeScanningAlert {
	st.mu.RLock()
	defer st.mu.RUnlock()

	byRepo := st.CodeScanningAlertsByRepo[repoKey]
	out := make([]*CodeScanningAlert, 0, len(byRepo))
	for _, a := range byRepo {
		if state != "" && a.State != state {
			continue
		}
		if severity != "" && a.RuleSeverity != severity {
			continue
		}
		if toolName != "" && a.ToolName != toolName {
			continue
		}
		if rule != "" && a.RuleID != rule {
			continue
		}
		out = append(out, a)
	}

	if sortField == "" {
		sortField = "created"
	}
	if direction == "" {
		direction = "desc"
	}

	sort.SliceStable(out, func(i, j int) bool {
		var less bool
		switch sortField {
		case "updated":
			less = out[i].UpdatedAt.Before(out[j].UpdatedAt)
		default:
			less = out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		if direction == "asc" {
			return less
		}
		return !less
	})
	return out
}

// UpdateCodeScanningAlert applies a state/dismissed_reason transition to a
// single alert. Valid transitions mirror real GitHub: open → dismissed,
// open → fixed, dismissed → open.
func (st *Store) UpdateCodeScanningAlert(a *CodeScanningAlert, state, dismissedReason, dismissedComment string) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if err := validateCodeScanningTransition(a.State, state, dismissedReason); err != nil {
		return err
	}

	now := time.Now().UTC()
	switch state {
	case "dismissed":
		a.State = "dismissed"
		a.DismissedReason = dismissedReason
		a.DismissedComment = dismissedComment
		a.DismissedAt = &now
		a.FixedAt = nil
	case "fixed":
		a.State = "fixed"
		a.FixedAt = &now
		a.DismissedReason = ""
		a.DismissedComment = ""
		a.DismissedAt = nil
	case "open":
		a.State = "open"
		a.DismissedReason = ""
		a.DismissedComment = ""
		a.DismissedAt = nil
		a.FixedAt = nil
	}
	a.UpdatedAt = now
	st.persistCodeScanningAlert(a)
	return nil
}

func validateCodeScanningTransition(currentState, newState, dismissedReason string) error {
	if newState != "" && newState != "open" && newState != "dismissed" && newState != "fixed" {
		return fmt.Errorf("invalid state %q", newState)
	}
	if newState == "dismissed" && !isValidDismissedReason(dismissedReason) {
		return fmt.Errorf("invalid dismissed_reason %q", dismissedReason)
	}
	if newState == "open" && currentState == "dismissed" {
		return nil
	}
	if newState == "fixed" && (currentState == "open" || currentState == "dismissed") {
		return nil
	}
	if newState == "dismissed" && (currentState == "open" || currentState == "fixed") {
		return nil
	}
	if newState == currentState {
		return nil
	}
	return fmt.Errorf("invalid transition from %q to %q", currentState, newState)
}

func isValidDismissedReason(r string) bool {
	switch r {
	case "false_positive", "won't_fix", "used_in_tests", "ignored":
		return true
	}
	return false
}

// CreateCodeScanningAnalysis records a new analysis run for a repo.
func (st *Store) CreateCodeScanningAnalysis(repoKey, ref, commitSHA, analysisKey, category, toolName string) *CodeScanningAnalysis {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.CodeScanningAnalysesByRepo[repoKey] == nil {
		st.CodeScanningAnalysesByRepo[repoKey] = make(map[int]*CodeScanningAnalysis)
	}

	now := time.Now().UTC()
	analysis := &CodeScanningAnalysis{
		ID:          st.NextCodeScanningAnalysisID,
		NodeID:      fmt.Sprintf("CSWA%d", st.NextCodeScanningAnalysisID),
		RepoKey:     repoKey,
		Ref:         ref,
		CommitSHA:   commitSHA,
		AnalysisKey: analysisKey,
		Category:    category,
		ToolName:    toolName,
		CreatedAt:   now,
	}
	st.NextCodeScanningAnalysisID++

	st.CodeScanningAnalyses[analysis.ID] = analysis
	st.CodeScanningAnalysesByRepo[repoKey][analysis.ID] = analysis
	st.persistCodeScanningAnalysis(analysis)
	return analysis
}

// GetCodeScanningAnalysis returns an analysis by ID scoped to a repo.
func (st *Store) GetCodeScanningAnalysis(repoKey string, id int) *CodeScanningAnalysis {
	st.mu.RLock()
	defer st.mu.RUnlock()
	a := st.CodeScanningAnalyses[id]
	if a != nil && a.RepoKey != repoKey {
		return nil
	}
	return a
}

// ListCodeScanningAnalyses returns analyses for a repo, optionally filtered by
// ref and tool_name.
func (st *Store) ListCodeScanningAnalyses(repoKey, ref, toolName string) []*CodeScanningAnalysis {
	st.mu.RLock()
	defer st.mu.RUnlock()

	byRepo := st.CodeScanningAnalysesByRepo[repoKey]
	out := make([]*CodeScanningAnalysis, 0, len(byRepo))
	for _, a := range byRepo {
		if ref != "" && a.Ref != ref {
			continue
		}
		if toolName != "" && a.ToolName != toolName {
			continue
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// DeleteCodeScanningAnalysis removes an analysis from the store.
func (st *Store) DeleteCodeScanningAnalysis(repoKey string, id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	a := st.CodeScanningAnalyses[id]
	if a == nil || a.RepoKey != repoKey {
		return false
	}
	delete(st.CodeScanningAnalyses, id)
	delete(st.CodeScanningAnalysesByRepo[repoKey], id)
	if st.persist != nil {
		st.persist.MustDelete("code_scanning_analyses", strconv.Itoa(id))
	}
	return true
}

// CreateSARIFUpload parses a base64-encoded SARIF payload, creates analyses
// and alerts, and returns the upload record. Processing is synchronous so the
// returned upload is always "complete".
func (st *Store) CreateSARIFUpload(repoKey string, payload map[string]interface{}) (*SARIFUpload, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	commitSHA, _ := payload["commit_sha"].(string)
	ref, _ := payload["ref"].(string)
	toolNameOverride, _ := payload["tool_name"].(string)

	if commitSHA == "" {
		return nil, fmt.Errorf("commit_sha is required")
	}
	if ref == "" {
		ref = "refs/heads/main"
	}

	sarifRaw, _ := payload["sarif"].(string)
	var sarif map[string]interface{}
	if sarifRaw != "" {
		decoded, err := base64.StdEncoding.DecodeString(sarifRaw)
		if err != nil {
			return nil, fmt.Errorf("sarif is not valid base64: %w", err)
		}
		if err := json.Unmarshal(decoded, &sarif); err != nil {
			return nil, fmt.Errorf("sarif is not valid JSON: %w", err)
		}
	} else {
		return nil, fmt.Errorf("sarif is required")
	}

	now := time.Now().UTC()
	uploadID := fmt.Sprintf("%s-%d", strings.ReplaceAll(repoKey, "/", "-"), now.UnixNano())
	upload := &SARIFUpload{
		ID:        uploadID,
		RepoKey:   repoKey,
		Status:    "complete",
		CreatedAt: now,
	}

	if st.SARIFUploads == nil {
		st.SARIFUploads = make(map[string]*SARIFUpload)
	}

	toolName := toolNameOverride
	runs, _ := sarif["runs"].([]interface{})
	if len(runs) > 0 {
		run, _ := runs[0].(map[string]interface{})
		toolName = extractSARIFToolName(run, toolName)
		results, _ := run["results"].([]interface{})
		if len(results) > 0 {
			analysisKey := fmt.Sprintf("%s:%s", toolName, ref)
			category := fmt.Sprintf("%s/%s", toolName, ref)
			analysis := st.createAnalysisAndAlertsLocked(repoKey, ref, commitSHA, analysisKey, category, toolName, results)
			analysis.SARIFUploadID = uploadID
			st.persistCodeScanningAnalysis(analysis)
		}
	}

	st.SARIFUploads[uploadID] = upload
	if st.persist != nil {
		st.persist.MustPut("sarif_uploads", uploadID, upload)
	}
	return upload, nil
}

func extractSARIFToolName(run map[string]interface{}, fallback string) string {
	tool, _ := run["tool"].(map[string]interface{})
	driver, _ := tool["driver"].(map[string]interface{})
	name, _ := driver["name"].(string)
	if name != "" {
		return name
	}
	return fallback
}

func (st *Store) createAnalysisAndAlertsLocked(repoKey, ref, commitSHA, analysisKey, category, toolName string, results []interface{}) *CodeScanningAnalysis {
	if st.CodeScanningAlertsByRepo[repoKey] == nil {
		st.CodeScanningAlertsByRepo[repoKey] = make(map[int]*CodeScanningAlert)
	}
	if st.CodeScanningNextNumber[repoKey] == 0 {
		st.CodeScanningNextNumber[repoKey] = 1
	}
	if st.CodeScanningAnalysesByRepo[repoKey] == nil {
		st.CodeScanningAnalysesByRepo[repoKey] = make(map[int]*CodeScanningAnalysis)
	}

	now := time.Now().UTC()
	analysis := &CodeScanningAnalysis{
		ID:          st.NextCodeScanningAnalysisID,
		NodeID:      fmt.Sprintf("CSWA%d", st.NextCodeScanningAnalysisID),
		RepoKey:     repoKey,
		Ref:         ref,
		CommitSHA:   commitSHA,
		AnalysisKey: analysisKey,
		Category:    category,
		ToolName:    toolName,
		CreatedAt:   now,
	}
	st.NextCodeScanningAnalysisID++
	st.CodeScanningAnalyses[analysis.ID] = analysis
	st.CodeScanningAnalysesByRepo[repoKey][analysis.ID] = analysis

	ruleSet := make(map[string]struct{})
	for _, r := range results {
		result, _ := r.(map[string]interface{})
		ruleID, _ := result["ruleId"].(string)
		if ruleID == "" {
			ruleID, _ = result["rule_id"].(string)
		}
		ruleSet[ruleID] = struct{}{}
	}
	analysis.ResultsCount = len(results)
	analysis.RulesCount = len(ruleSet)
	st.persistCodeScanningAnalysis(analysis)

	for _, r := range results {
		result, _ := r.(map[string]interface{})
		ruleID, _ := result["ruleId"].(string)
		if ruleID == "" {
			ruleID, _ = result["rule_id"].(string)
		}
		message := ""
		if msg, ok := result["message"].(map[string]interface{}); ok {
			message, _ = msg["text"].(string)
		}
		if message == "" {
			message, _ = result["message"].(string)
		}

		var instances []CodeScanningAlertInstance
		locations, _ := result["locations"].([]interface{})
		for _, loc := range locations {
			instance := codeScanningInstanceFromLocation(loc, ref, analysisKey, category, commitSHA)
			if instance != nil {
				instances = append(instances, *instance)
			}
		}
		if len(instances) == 0 {
			instances = append(instances, CodeScanningAlertInstance{
				Ref:         ref,
				AnalysisKey: analysisKey,
				Category:    category,
				State:       "open",
				CommitSHA:   commitSHA,
				Message:     message,
			})
		}

		number := st.CodeScanningNextNumber[repoKey]
		st.CodeScanningNextNumber[repoKey] = number + 1

		alert := &CodeScanningAlert{
			ID:              st.NextCodeScanningAlertID,
			NodeID:          fmt.Sprintf("CSWA%d", st.NextCodeScanningAlertID),
			Number:          number,
			RepoKey:         repoKey,
			RuleID:          ruleID,
			RuleDescription: message,
			ToolName:        toolName,
			State:           "open",
			Instances:       instances,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		st.NextCodeScanningAlertID++

		// Severity defaults to "warning" when the SARIF payload does not
		// include rule metadata.
		if alert.RuleSeverity == "" {
			alert.RuleSeverity = "warning"
		}

		st.CodeScanningAlerts[alert.ID] = alert
		st.CodeScanningAlertsByRepo[repoKey][number] = alert
		st.persistCodeScanningAlert(alert)
	}

	return analysis
}

func codeScanningInstanceFromLocation(loc interface{}, ref, analysisKey, category, commitSHA string) *CodeScanningAlertInstance {
	location, _ := loc.(map[string]interface{})
	if location == nil {
		return nil
	}
	physicalLocation, _ := location["physicalLocation"].(map[string]interface{})
	if physicalLocation == nil {
		physicalLocation, _ = location["physical_location"].(map[string]interface{})
	}
	if physicalLocation == nil {
		return nil
	}
	artifactLocation, _ := physicalLocation["artifactLocation"].(map[string]interface{})
	if artifactLocation == nil {
		artifactLocation, _ = physicalLocation["artifact_location"].(map[string]interface{})
	}
	path := ""
	if artifactLocation != nil {
		path, _ = artifactLocation["uri"].(string)
	}
	region, _ := physicalLocation["region"].(map[string]interface{})
	if region == nil {
		return nil
	}
	startLine := intNumber(region["startLine"])
	endLine := intNumber(region["endLine"])
	if endLine == 0 {
		endLine = startLine
	}
	startColumn := intNumber(region["startColumn"])
	endColumn := intNumber(region["endColumn"])

	var message string
	if msg, ok := location["message"].(map[string]interface{}); ok {
		message, _ = msg["text"].(string)
	}

	return &CodeScanningAlertInstance{
		Ref:         ref,
		AnalysisKey: analysisKey,
		Category:    category,
		State:       "open",
		CommitSHA:   commitSHA,
		Path:        path,
		StartLine:   startLine,
		EndLine:     endLine,
		StartColumn: startColumn,
		EndColumn:   endColumn,
		Message:     message,
	}
}

func intNumber(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i
		}
	}
	return 0
}

// GetSARIFUpload returns a SARIF upload by ID.
func (st *Store) GetSARIFUpload(repoKey, id string) *SARIFUpload {
	st.mu.RLock()
	defer st.mu.RUnlock()
	up := st.SARIFUploads[id]
	if up == nil || up.RepoKey != repoKey {
		return nil
	}
	return up
}

func (st *Store) persistCodeScanningAlert(a *CodeScanningAlert) {
	if st.persist != nil {
		st.persist.MustPut("code_scanning_alerts", strconv.Itoa(a.ID), a)
	}
}

func (st *Store) persistCodeScanningAnalysis(a *CodeScanningAnalysis) {
	if st.persist != nil {
		st.persist.MustPut("code_scanning_analyses", strconv.Itoa(a.ID), a)
	}
}
