package bleephub

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Workflow represents a running multi-job workflow.
type Workflow struct {
	ID        string                    `json:"id"`
	Name      string                    `json:"name"`
	RunID     int                       `json:"runId"`
	RunNumber int                       `json:"runNumber"`
	Jobs      map[string]*WorkflowJob   `json:"jobs"`
	Env       map[string]string         `json:"env,omitempty"`
	Status    string                    `json:"status"`  // "running", "completed"
	Result    string                    `json:"result"`  // "success", "failure", "cancelled"
	CreatedAt time.Time                 `json:"createdAt"`
}

// WorkflowJob represents a single job within a workflow.
type WorkflowJob struct {
	Key          string                 `json:"key"`     // YAML key
	JobID        string                 `json:"jobId"`   // UUID, used as Job.ID
	DisplayName  string                 `json:"displayName"`
	Needs        []string               `json:"needs,omitempty"`
	Status       string                 `json:"status"`  // "pending", "queued", "running", "completed", "skipped"
	Result       string                 `json:"result"`  // "success", "failure", "cancelled", "skipped"
	Outputs      map[string]string      `json:"outputs,omitempty"`
	MatrixValues map[string]interface{} `json:"matrix,omitempty"`
	Def          *JobDef                `json:"-"`
}

// submitWorkflow creates a Workflow from a WorkflowDef and begins dispatching jobs.
func (s *Server) submitWorkflow(serverURL string, wf *WorkflowDef, defaultImage string) (*Workflow, error) {
	// Validate no cycles in the job dependency graph
	if err := validateJobGraph(wf); err != nil {
		return nil, err
	}

	s.store.mu.Lock()
	runID := s.store.NextRunID
	s.store.NextRunID++
	s.store.mu.Unlock()

	workflow := &Workflow{
		ID:        uuid.New().String(),
		Name:      wf.Name,
		RunID:     runID,
		RunNumber: runID,
		Jobs:      make(map[string]*WorkflowJob),
		Env:       wf.Env,
		Status:    "running",
		CreatedAt: time.Now(),
	}

	if workflow.Name == "" {
		workflow.Name = "workflow"
	}

	// Create WorkflowJobs for each JobDef
	for key, jd := range wf.Jobs {
		wfJob := &WorkflowJob{
			Key:         key,
			JobID:       uuid.New().String(),
			DisplayName: key,
			Needs:       jd.Needs,
			Status:      "pending",
			Outputs:     make(map[string]string),
			Def:         jd,
		}
		if jd.Name != "" {
			wfJob.DisplayName = jd.Name
		}

		// Extract matrix values from __matrix_ env prefix (set by expandMatrixJobs)
		if jd.Env != nil {
			matrixVals := make(map[string]interface{})
			for k, v := range jd.Env {
				if len(k) > 9 && k[:9] == "__matrix_" {
					matrixVals[k[9:]] = v
				}
			}
			if len(matrixVals) > 0 {
				wfJob.MatrixValues = matrixVals
			}
		}

		workflow.Jobs[key] = wfJob
	}

	// Store the workflow
	s.store.mu.Lock()
	s.store.Workflows[workflow.ID] = workflow
	s.store.mu.Unlock()

	// Dispatch root jobs (no dependencies)
	s.dispatchReadyJobs(workflow, serverURL, defaultImage)

	return workflow, nil
}

// dispatchReadyJobs finds pending jobs whose dependencies are all satisfied
// and dispatches them to the runner.
func (s *Server) dispatchReadyJobs(wf *Workflow, serverURL string, defaultImage string) {
	for _, wfJob := range wf.Jobs {
		if wfJob.Status != "pending" {
			continue
		}

		// Check all dependencies are completed
		allDepsOk := true
		anyDepFailed := false
		for _, dep := range wfJob.Needs {
			depJob, ok := wf.Jobs[dep]
			if !ok {
				allDepsOk = false
				break
			}
			if depJob.Status == "completed" || depJob.Status == "skipped" {
				if depJob.Result != "success" {
					anyDepFailed = true
				}
				continue
			}
			allDepsOk = false
			break
		}

		if !allDepsOk {
			continue
		}

		// If any dependency failed, skip this job
		if anyDepFailed {
			wfJob.Status = "skipped"
			wfJob.Result = "skipped"
			s.logger.Info().Str("job", wfJob.Key).Msg("skipping job (dependency failed)")
			continue
		}

		// Dispatch this job
		s.dispatchWorkflowJob(wf, wfJob, serverURL, defaultImage)
	}
}

// dispatchWorkflowJob builds and sends a job message to the runner.
func (s *Server) dispatchWorkflowJob(wf *Workflow, wfJob *WorkflowJob, serverURL, defaultImage string) {
	planID := uuid.New().String()
	timelineID := uuid.New().String()
	requestID := s.nextRequestID()

	msg := s.buildJobMessageFromDef(serverURL, wf, wfJob, planID, timelineID, requestID, defaultImage)
	msgJSON, _ := json.Marshal(msg)

	job := &Job{
		ID:          wfJob.JobID,
		RequestID:   requestID,
		PlanID:      planID,
		TimelineID:  timelineID,
		Status:      "queued",
		Message:     string(msgJSON),
		LockedUntil: time.Now().Add(1 * time.Hour),
	}

	s.store.mu.Lock()
	s.store.Jobs[wfJob.JobID] = job
	s.store.mu.Unlock()

	wfJob.Status = "queued"

	envelope := &TaskAgentMessage{
		MessageID:   s.nextMessageID(),
		MessageType: "PipelineAgentJobRequest",
		Body:        string(msgJSON),
	}

	if !s.sendMessageToAgent(envelope) {
		s.logger.Warn().Str("jobId", wfJob.JobID).Msg("no runner available, workflow job queued")
	}

	s.logger.Info().
		Str("workflow", wf.ID).
		Str("job", wfJob.Key).
		Str("jobId", wfJob.JobID).
		Msg("workflow job dispatched")
}

// onJobCompleted is called when a job finishes. It updates the workflow
// and dispatches any newly-ready dependent jobs.
func (s *Server) onJobCompleted(jobID, result string) {
	s.store.mu.RLock()
	var foundWf *Workflow
	var foundJob *WorkflowJob
	for _, wf := range s.store.Workflows {
		for _, wfJob := range wf.Jobs {
			if wfJob.JobID == jobID {
				foundWf = wf
				foundJob = wfJob
				break
			}
		}
		if foundWf != nil {
			break
		}
	}
	s.store.mu.RUnlock()

	if foundWf == nil {
		return // Not a workflow job
	}

	foundJob.Status = "completed"
	foundJob.Result = normalizeResult(result)

	s.logger.Info().
		Str("workflow", foundWf.ID).
		Str("job", foundJob.Key).
		Str("result", foundJob.Result).
		Msg("workflow job completed")

	// Dispatch any newly-ready jobs (this may also mark some as skipped)
	if foundWf.Env != nil {
		if serverURL, ok := foundWf.Env["__serverURL"]; ok {
			defaultImage := foundWf.Env["__defaultImage"]
			s.dispatchReadyJobs(foundWf, serverURL, defaultImage)
		}
	}

	// Check if all jobs are done (after dispatch, which may skip dependents)
	allDone := true
	anyFailed := false
	for _, wfJob := range foundWf.Jobs {
		if wfJob.Status != "completed" && wfJob.Status != "skipped" {
			allDone = false
		}
		if wfJob.Result == "failure" || wfJob.Result == "cancelled" {
			anyFailed = true
		}
	}

	if allDone {
		foundWf.Status = "completed"
		if anyFailed {
			foundWf.Result = "failure"
		} else {
			foundWf.Result = "success"
		}
		s.logger.Info().
			Str("workflow", foundWf.ID).
			Str("result", foundWf.Result).
			Msg("workflow completed")
	}
}

// normalizeResult converts runner result strings to consistent format.
func normalizeResult(result string) string {
	switch result {
	case "Succeeded", "succeeded":
		return "success"
	case "Failed", "failed":
		return "failure"
	case "Cancelled", "cancelled":
		return "cancelled"
	default:
		if result == "" {
			return "success"
		}
		return result
	}
}

// buildJobMessageFromDef builds a job message from a WorkflowDef-based job,
// supporting both run: and uses: steps.
func (s *Server) buildJobMessageFromDef(serverURL string, wf *Workflow, wfJob *WorkflowJob, planID, timelineID string, requestID int64, defaultImage string) map[string]interface{} {
	jd := wfJob.Def
	scopeID := uuid.New().String()
	jobToken := makeJWT(scopeID, "actions")

	// Determine container image
	image := defaultImage
	if img := jd.ContainerImage(); img != "" {
		image = img
	}
	if image == "" {
		image = "alpine:latest"
	}

	// Build steps
	steps := make([]map[string]interface{}, 0, len(jd.Steps))
	for i, step := range jd.Steps {
		stepID := uuid.New().String()

		if step.Run != "" {
			// Script step
			displayName := step.Name
			if displayName == "" {
				displayName = fmt.Sprintf("Run %s", truncateDisplay(step.Run, 40))
			}
			contextName := step.ID
			if contextName == "" {
				contextName = fmt.Sprintf("__run_%d", i+1)
			}
			steps = append(steps, map[string]interface{}{
				"type": "action",
				"id":   stepID,
				"name": contextName,
				"reference": map[string]interface{}{
					"type": "script",
				},
				"displayNameToken": displayName,
				"contextName":      contextName,
				"condition":        stepCondition(step.If),
				"inputs": map[string]interface{}{
					"type": 2,
					"map": []interface{}{
						map[string]interface{}{
							"Key":   map[string]interface{}{"type": 0, "lit": "script"},
							"Value": map[string]interface{}{"type": 0, "lit": step.Run},
						},
					},
				},
			})
		} else if step.Uses != "" {
			// Action step
			nameWithOwner, path, ref, isLocal := ParseActionRef(step.Uses)
			displayName := step.Name
			if displayName == "" {
				displayName = step.Uses
			}
			contextName := step.ID
			if contextName == "" {
				contextName = fmt.Sprintf("__action_%d", i+1)
			}

			var reference map[string]interface{}
			if isLocal {
				reference = map[string]interface{}{
					"type": "script",
					"path": path,
				}
			} else {
				reference = map[string]interface{}{
					"type":           "repository",
					"name":           nameWithOwner,
					"ref":            ref,
					"repositoryType": "GitHub",
				}
				if path != "" {
					reference["path"] = path
				}
			}

			// Build inputs MappingToken from with:
			inputEntries := make([]interface{}, 0, len(step.With))
			for k, v := range step.With {
				inputEntries = append(inputEntries, map[string]interface{}{
					"Key":   map[string]interface{}{"type": 0, "lit": k},
					"Value": map[string]interface{}{"type": 0, "lit": v},
				})
			}

			steps = append(steps, map[string]interface{}{
				"type":             "action",
				"id":               stepID,
				"name":             contextName,
				"reference":        reference,
				"displayNameToken": displayName,
				"contextName":      contextName,
				"condition":        stepCondition(step.If),
				"inputs": map[string]interface{}{
					"type": 2,
					"map":  inputEntries,
				},
			})
		}
	}

	// Build env context data
	envPairs := make([]string, 0)
	// Workflow-level env
	for k, v := range wf.Env {
		if k != "__serverURL" && k != "__defaultImage" {
			envPairs = append(envPairs, k, v)
		}
	}
	// Job-level env overrides
	for k, v := range jd.Env {
		envPairs = append(envPairs, k, v)
	}

	// Build needs context
	needsCtx := buildNeedsContext(wf, wfJob)

	// Build matrix context
	var matrixCtx interface{}
	if len(wfJob.MatrixValues) > 0 {
		matrixPairs := make([]string, 0, len(wfJob.MatrixValues)*2)
		for k, v := range wfJob.MatrixValues {
			matrixPairs = append(matrixPairs, k, fmt.Sprintf("%v", v))
		}
		matrixCtx = dictContextData(matrixPairs...)
	}

	runID := fmt.Sprintf("%d", wf.RunID)
	runNumber := fmt.Sprintf("%d", wf.RunNumber)

	return map[string]interface{}{
		"messageType": "PipelineAgentJobRequest",
		"plan": map[string]interface{}{
			"scopeIdentifier": scopeID,
			"planId":          planID,
			"planType":        "free",
			"planGroup":       "free",
			"version":         12,
			"owner": map[string]interface{}{
				"id":   0,
				"name": "Community",
			},
		},
		"timeline": map[string]interface{}{
			"id":       timelineID,
			"changeId": 1,
			"location": nil,
		},
		"jobId":          wfJob.JobID,
		"jobDisplayName": wfJob.DisplayName,
		"jobName":        wfJob.Key,
		"requestId":      requestID,
		"lockedUntil":    "0001-01-01T00:00:00",
		"jobContainer":         image,
		"jobServiceContainers": buildServiceContainers(jd.Services),
		"jobOutputs":           nil,
		"resources": map[string]interface{}{
			"endpoints": []map[string]interface{}{
				{
					"name": "SystemVssConnection",
					"url":  serverURL + "/",
					"authorization": map[string]interface{}{
						"scheme": "OAuth",
						"parameters": map[string]string{
							"AccessToken": jobToken,
						},
					},
					"data": map[string]string{
						"CacheServerUrl":    serverURL + "/",
						"ResultsServiceUrl": serverURL + "/",
					},
					"isShared": false,
					"isReady":  true,
				},
			},
			"repositories": []interface{}{},
			"containers":   []interface{}{},
		},
		"contextData": map[string]interface{}{
			"github": dictContextData(
				"server_url", serverURL,
				"api_url", serverURL,
				"repository", "bleephub/test",
				"repository_owner", "bleephub",
				"run_id", runID,
				"run_number", runNumber,
				"workflow", wf.Name,
				"job", wfJob.Key,
				"event_name", "push",
				"sha", "0000000000000000000000000000000000000000",
				"ref", "refs/heads/main",
				"action", "__run",
				"workspace", "/github/workspace",
				"token", jobToken,
			),
			"runner": dictContextData(
				"os", "Linux",
				"arch", "ARM64",
				"name", "test-runner",
				"tool_cache", "/opt/hostedtoolcache",
				"temp", "/home/runner/work/_temp",
			),
			"env":      dictContextData(envPairs...),
			"vars":     dictContextData(),
			"needs":    needsCtx,
			"inputs":   nil,
			"matrix":   matrixCtx,
			"strategy": nil,
		},
		"variables": map[string]interface{}{
			"system.github.job":                      varVal(wfJob.Key),
			"system.github.runid":                    varVal(runID),
			"system.github.token":                    varSecret(jobToken),
			"github_token":                           varSecret(jobToken),
			"system.phaseDisplayName":                varVal(wfJob.DisplayName),
			"system.runnerGroupName":                 varVal("Default"),
			"DistributedTask.NewActionMetadata":      varVal("true"),
			"DistributedTask.EnableCompositeActions": varVal("true"),
		},
		"mask":                 []interface{}{},
		"steps":                steps,
		"workspace":            map[string]interface{}{},
		"defaults":             nil,
		"environmentVariables": nil,
		"actionsEnvironment":   nil,
		"fileTable":            []string{".github/workflows/test.yml"},
	}
}

// buildServiceContainers converts parsed ServiceDefs to the runner's expected
// jobServiceContainers format: map of alias → container spec.
func buildServiceContainers(services map[string]*ServiceDef) interface{} {
	if len(services) == 0 {
		return nil
	}
	result := make(map[string]interface{}, len(services))
	for name, svc := range services {
		spec := map[string]interface{}{
			"image": svc.Image,
		}
		if len(svc.Env) > 0 {
			spec["environment"] = svc.Env
		}
		if len(svc.Ports) > 0 {
			spec["ports"] = svc.Ports
		}
		if len(svc.Volumes) > 0 {
			spec["volumes"] = svc.Volumes
		}
		if svc.Options != "" {
			spec["options"] = svc.Options
		}
		result[name] = spec
	}
	return result
}

// buildNeedsContext builds the "needs" PipelineContextData from completed
// dependency outputs.
func buildNeedsContext(wf *Workflow, wfJob *WorkflowJob) interface{} {
	if len(wfJob.Needs) == 0 {
		return dictContextData()
	}

	// Build a nested dict: needs.<job>.outputs.<name> = value, needs.<job>.result = "success"
	entries := make([]map[string]interface{}, 0, len(wfJob.Needs))
	for _, depKey := range wfJob.Needs {
		depJob, ok := wf.Jobs[depKey]
		if !ok {
			continue
		}

		// Build outputs sub-dict
		outputEntries := make([]map[string]interface{}, 0, len(depJob.Outputs))
		for k, v := range depJob.Outputs {
			outputEntries = append(outputEntries, map[string]interface{}{
				"k": k, "v": v,
			})
		}

		// Each dep is a dict with "result" and "outputs"
		depEntries := []map[string]interface{}{
			{"k": "result", "v": depJob.Result},
			{"k": "outputs", "v": map[string]interface{}{"t": 2, "d": outputEntries}},
		}

		entries = append(entries, map[string]interface{}{
			"k": depKey,
			"v": map[string]interface{}{"t": 2, "d": depEntries},
		})
	}

	return map[string]interface{}{"t": 2, "d": entries}
}

// stepCondition returns the condition string for a step.
func stepCondition(ifExpr string) string {
	if ifExpr != "" {
		return ifExpr
	}
	return "success()"
}

// validateJobGraph checks for cycles in the job dependency graph.
func validateJobGraph(wf *WorkflowDef) error {
	// Topological sort via DFS
	visited := make(map[string]int) // 0=unvisited, 1=visiting, 2=visited
	var visit func(key string) error
	visit = func(key string) error {
		if visited[key] == 2 {
			return nil
		}
		if visited[key] == 1 {
			return fmt.Errorf("cycle detected involving job %q", key)
		}
		visited[key] = 1

		jd, ok := wf.Jobs[key]
		if !ok {
			return fmt.Errorf("job %q references unknown dependency", key)
		}
		for _, dep := range jd.Needs {
			if _, ok := wf.Jobs[dep]; !ok {
				return fmt.Errorf("job %q needs unknown job %q", key, dep)
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		visited[key] = 2
		return nil
	}

	for key := range wf.Jobs {
		if err := visit(key); err != nil {
			return err
		}
	}
	return nil
}
