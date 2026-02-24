package bleephub

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

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

	// Use event metadata from workflow, with defaults
	eventName := wf.EventName
	if eventName == "" {
		eventName = "push"
	}
	ref := wf.Ref
	if ref == "" {
		ref = "refs/heads/main"
	}
	sha := wf.Sha
	if sha == "" {
		sha = "0000000000000000000000000000000000000000"
	}
	repoFullName := wf.RepoFullName
	if repoFullName == "" {
		repoFullName = "bleephub/test"
	}
	repoOwner := repoFullName
	if idx := strings.Index(repoOwner, "/"); idx >= 0 {
		repoOwner = repoOwner[:idx]
	}

	// Build secrets context and mask array
	secretsPairs := make([]string, 0)
	maskArray := make([]interface{}, 0)

	// Always include GITHUB_TOKEN
	secretsPairs = append(secretsPairs, "GITHUB_TOKEN", jobToken)
	maskArray = append(maskArray, map[string]interface{}{"type": "regex", "value": jobToken})

	// Look up repo secrets
	if s != nil && s.store != nil {
		s.store.mu.RLock()
		if secrets, ok := s.store.RepoSecrets[repoFullName]; ok {
			for _, sec := range secrets {
				secretsPairs = append(secretsPairs, sec.Name, sec.Value)
				maskArray = append(maskArray, map[string]interface{}{"type": "regex", "value": sec.Value})
			}
		}
		s.store.mu.RUnlock()
	}

	// Build inputs context
	var inputsCtx interface{}
	if len(wf.Inputs) > 0 {
		inputsPairs := make([]string, 0, len(wf.Inputs)*2)
		for k, v := range wf.Inputs {
			inputsPairs = append(inputsPairs, k, v)
		}
		inputsCtx = dictContextData(inputsPairs...)
	}

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
		"jobId":                wfJob.JobID,
		"jobDisplayName":       wfJob.DisplayName,
		"jobName":              wfJob.Key,
		"requestId":            requestID,
		"lockedUntil":          "0001-01-01T00:00:00",
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
				"repository", repoFullName,
				"repository_owner", repoOwner,
				"run_id", runID,
				"run_number", runNumber,
				"workflow", wf.Name,
				"job", wfJob.Key,
				"event_name", eventName,
				"sha", sha,
				"ref", ref,
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
			"secrets":  dictContextData(secretsPairs...),
			"needs":    needsCtx,
			"inputs":   inputsCtx,
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
		"mask":                 maskArray,
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

// dictContextData builds a PipelineContextData DictionaryContextData.
// Args are alternating key, value strings.
func dictContextData(kvs ...string) map[string]interface{} {
	entries := make([]map[string]interface{}, 0, len(kvs)/2)
	for i := 0; i+1 < len(kvs); i += 2 {
		entries = append(entries, map[string]interface{}{
			"k": kvs[i],
			"v": kvs[i+1], // String values are bare JSON strings
		})
	}
	return map[string]interface{}{
		"t": 2,
		"d": entries,
	}
}

func varVal(value string) map[string]interface{} {
	return map[string]interface{}{
		"value":    value,
		"isSecret": false,
	}
}

func varSecret(value string) map[string]interface{} {
	return map[string]interface{}{
		"value":    value,
		"isSecret": true,
	}
}

func truncateDisplay(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
