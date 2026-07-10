package bleephub

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// buildJobMessageFromDef builds a job message from a WorkflowDef-based job,
// supporting both run: and uses: steps.
func (s *Server) buildJobMessageFromDef(serverURL string, wf *Workflow, wfJob *WorkflowJob, planID, timelineID string, requestID int64, defaultImage string) (map[string]interface{}, error) {
	jd := wfJob.Def
	scopeID := uuid.New().String()
	jobToken := makeJWT(scopeID, "actions")

	// Determine the job container; empty means host mode (the run was
	// submitted with hostMode and the YAML declares no container) — the
	// runner then executes the job directly, like real GitHub. A bare
	// image string rides as-is; the object form (`container:` with
	// env/ports/volumes/options) becomes a full mapping token so none
	// of those fields drop.
	image := defaultImage
	var jobContainer interface{}
	if img := jd.ContainerImage(); img != "" {
		image = img
	}
	if cd := jd.ContainerObject(); cd != nil {
		jobContainer = containerSpecToken(cd.Image, cd.Env, cd.Ports, cd.Volumes, cd.Options)
	} else {
		jobContainer = jobContainerValue(image)
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
							"Value": templateToken(step.Run),
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
					"Value": templateToken(v),
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

	runID := strconv.Itoa(wf.RunID)

	// The github context (event metadata + defaults) is assembled by
	// githubRunnerContext below; the secrets/vars lookup must resolve a
	// real repository because repository, organization and environment
	// secrets are scoped by that persisted state.
	repoFullName := wf.RepoFullName

	// Build secrets context and mask array
	secretsPairs := make([]string, 0)
	maskArray := make([]interface{}, 0)

	// Always include GITHUB_TOKEN
	secretsPairs = append(secretsPairs, "GITHUB_TOKEN", jobToken)
	maskArray = append(maskArray, map[string]interface{}{"type": "regex", "value": jobToken})

	// Org → repo → environment secrets and variables merge (highest
	// scope wins); every secret value rides the mask list so the runner
	// scrubs it from logs. Jobs inside a reusable-workflow call with an
	// explicit `secrets:` map receive ONLY the mapped names.
	//
	// The official runner builds its `secrets` expression context from
	// message.Variables entries flagged isSecret (Variables.
	// ToSecretsContext) — NOT from contextData — so every secret also
	// rides the variables map under its own name.
	variables := map[string]interface{}{
		"system.github.job":                      varVal(wfJob.Key),
		"system.github.runid":                    varVal(runID),
		"system.github.token":                    varSecret(jobToken),
		"github_token":                           varSecret(jobToken),
		"GITHUB_TOKEN":                           varSecret(jobToken),
		"system.phaseDisplayName":                varVal(wfJob.DisplayName),
		"system.runnerGroupName":                 varVal("Default"),
		"DistributedTask.NewActionMetadata":      varVal("true"),
		"DistributedTask.EnableCompositeActions": varVal("true"),
	}
	varsPairs := make([]string, 0)
	if s != nil && s.store != nil && repoFullName != "" {
		secretsMap, varsMap, err := s.CollectJobSecretsAndVars(repoFullName, jd.EnvironmentName())
		if err != nil {
			return nil, err
		}
		if jd.Call != nil && jd.CallRole == "" && !jd.Call.SecretsInherit {
			secretsMap = remapCallSecrets(s, wf, jd.Call, secretsMap)
		}
		for _, name := range sortedKeys(secretsMap) {
			secretsPairs = append(secretsPairs, name, secretsMap[name])
			variables[name] = varSecret(secretsMap[name])
			maskArray = append(maskArray, map[string]interface{}{"type": "regex", "value": secretsMap[name]})
		}
		for _, name := range sortedKeys(varsMap) {
			varsPairs = append(varsPairs, name, varsMap[name])
		}
	}

	// Build inputs context: called jobs get the call's resolved (typed)
	// inputs; workflow_dispatch runs their typed inputs; else strings.
	var inputsCtx interface{}
	switch {
	case jd.Call != nil && jd.CallRole == "" && jd.Call.ResolvedInputs() != nil:
		inputsCtx = toPipelineContextData(anyMap(jd.Call.ResolvedInputs()))
	case len(wf.TypedInputs) > 0:
		inputsCtx = toPipelineContextData(anyMap(wf.TypedInputs))
	case len(wf.Inputs) > 0:
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
		"jobContainer":         jobContainer,
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
			"github": toPipelineContextData(githubRunnerContext(s, wf, wfJob, serverURL, jobToken)),
			"runner": dictContextData(
				"os", "Linux",
				"arch", "ARM64",
				"name", "test-runner",
				"tool_cache", "/opt/hostedtoolcache",
				"temp", "/home/runner/work/_temp",
			),
			"env":      dictContextData(envPairs...),
			"vars":     dictContextData(varsPairs...),
			"secrets":  dictContextData(secretsPairs...),
			"needs":    needsCtx,
			"inputs":   inputsCtx,
			"matrix":   matrixCtx,
			"strategy": nil,
		},
		"variables":            variables,
		"mask":                 maskArray,
		"steps":                steps,
		"workspace":            map[string]interface{}{},
		"defaults":             nil,
		"environmentVariables": nil,
		"actionsEnvironment":   nil,
		"fileTable":            []string{".github/workflows/ci.yml"},
	}, nil
}

// templateToken converts a workflow string into the runner's template
// token: a plain literal when it carries no ${{ }}, else a
// BasicExpression token — `format('...', expr...)` with {N}
// placeholders — which is the shape real GitHub sends so the RUNNER
// evaluates the expressions against its contexts (matrix, env, steps,
// ...). Literal-only emission left scripts with raw `${{ matrix.os }}`
// text reaching the shell.
func templateToken(s string) map[string]interface{} {
	if !strings.Contains(s, "${{") {
		return map[string]interface{}{"type": 0, "lit": s}
	}
	return map[string]interface{}{"type": 3, "expr": templateToFormatExpr(s)}
}

// templateToFormatExpr rewrites "a ${{ x }} b" into
// "format('a {0} b', x)"; a string that is exactly one template becomes
// the bare inner expression.
func templateToFormatExpr(s string) string {
	escape := func(part string) string {
		part = strings.ReplaceAll(part, "'", "''")
		part = strings.ReplaceAll(part, "{", "{{")
		part = strings.ReplaceAll(part, "}", "}}")
		return part
	}
	var fmtStr strings.Builder
	var exprs []string
	rest := s
	for {
		i := strings.Index(rest, "${{")
		if i < 0 {
			fmtStr.WriteString(escape(rest))
			break
		}
		j := strings.Index(rest[i:], "}}")
		if j < 0 {
			fmtStr.WriteString(escape(rest))
			break
		}
		fmtStr.WriteString(escape(rest[:i]))
		exprs = append(exprs, strings.TrimSpace(rest[i+3:i+j]))
		fmt.Fprintf(&fmtStr, "{%d}", len(exprs)-1)
		rest = rest[i+j+2:]
	}
	if len(exprs) == 1 && fmtStr.String() == "{0}" {
		return exprs[0]
	}
	return fmt.Sprintf("format('%s', %s)", fmtStr.String(), strings.Join(exprs, ", "))
}

// sortedKeys returns a map's keys in sorted order so context payloads
// are deterministic across runs.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// anyMap widens a typed-inputs map for toPipelineContextData.
func anyMap(m map[string]interface{}) map[string]interface{} { return m }

// githubRunnerContext assembles the full `github` context the runner
// receives: the server-side context map (including the triggering event
// payload) plus the runner-session keys.
func githubRunnerContext(s *Server, wf *Workflow, wfJob *WorkflowJob, serverURL, jobToken string) map[string]interface{} {
	m := s.githubContextMap(wf)
	m["server_url"] = serverURL
	m["api_url"] = serverURL
	m["job"] = wfJob.Key
	m["action"] = "__run"
	m["workspace"] = "/github/workspace"
	m["token"] = jobToken
	return m
}

// remapCallSecrets applies a reusable-workflow call's explicit `secrets:`
// map: the called job receives ONLY the mapped names, with each value
// template (`${{ secrets.X }}`) evaluated against the caller's secrets.
func remapCallSecrets(s *Server, wf *Workflow, binding *WorkflowCallBinding, callerSecrets map[string]string) map[string]string {
	secretsCtx := make(map[string]interface{}, len(callerSecrets))
	for k, v := range callerSecrets {
		secretsCtx[k] = v
	}
	ctx := &ExprContext{Contexts: map[string]interface{}{
		"secrets": secretsCtx,
		"github":  s.githubContextMap(wf),
	}}
	mapped := make(map[string]string, len(binding.SecretsMap))
	for name, tmpl := range binding.SecretsMap {
		val, err := EvalTemplate(tmpl, ctx)
		if err != nil {
			s.logger.Warn().Err(err).Str("secret", name).Str("workflow", binding.CalledPath).
				Msg("workflow_call secret template failed — omitting")
			continue
		}
		mapped[name] = val
	}
	return mapped
}

// toPipelineContextData converts a Go value into the runner's
// PipelineContextData JSON encoding: bare strings, {"t":3,"d":bool},
// {"t":4,"d":number}, {"t":1,"a":[...]} arrays, and
// {"t":2,"d":[{"k":...,"v":...}]} dictionaries.
func toPipelineContextData(v interface{}) interface{} {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return t
	case bool:
		return map[string]interface{}{"t": 3, "d": t}
	case float64:
		return map[string]interface{}{"t": 4, "d": t}
	case int:
		return map[string]interface{}{"t": 4, "d": float64(t)}
	case []interface{}:
		arr := make([]interface{}, 0, len(t))
		for _, item := range t {
			arr = append(arr, toPipelineContextData(item))
		}
		return map[string]interface{}{"t": 1, "a": arr}
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		entries := make([]map[string]interface{}, 0, len(t))
		for _, k := range keys {
			entries = append(entries, map[string]interface{}{"k": k, "v": toPipelineContextData(t[k])})
		}
		return map[string]interface{}{"t": 2, "d": entries}
	default:
		return fmt.Sprintf("%v", t)
	}
}

// buildServiceContainers converts parsed ServiceDefs to the runner's
// jobServiceContainers wire shape: a mapping TemplateToken of alias →
// container-spec mapping token. The official runner DESERIALIZES this
// field as a TemplateToken (mapping = {"type":2,"map":[{Key,Value}]}),
// not plain JSON — a raw map fails its template validation at job
// start ("The template is not valid. Unexpected value ”").
func buildServiceContainers(services map[string]*ServiceDef) interface{} {
	if len(services) == 0 {
		return nil
	}
	entries := make([]interface{}, 0, len(services))
	for _, name := range sortedServiceNames(services) {
		entries = append(entries, mappingEntry(name, containerSpecToken(
			services[name].Image, services[name].Env, services[name].Ports,
			services[name].Volumes, services[name].Options,
		)))
	}
	return mappingToken(entries)
}

// containerSpecToken builds the container-spec mapping token shared by
// object-form `container:` and each `services:` entry. Keys follow the
// runner's ContainerInfo reader: image / env / ports / volumes /
// options. Strings route through templateToken so ${{ }} expressions
// inside them still evaluate runner-side.
func containerSpecToken(image string, env map[string]string, ports []interface{}, volumes []string, options string) map[string]interface{} {
	entries := []interface{}{mappingEntry("image", templateToken(image))}
	if len(env) > 0 {
		envEntries := make([]interface{}, 0, len(env))
		for _, k := range sortedKeys(env) {
			envEntries = append(envEntries, mappingEntry(k, templateToken(env[k])))
		}
		entries = append(entries, mappingEntry("env", mappingToken(envEntries)))
	}
	if len(ports) > 0 {
		seq := make([]interface{}, 0, len(ports))
		for _, p := range ports {
			seq = append(seq, scalarToken(p))
		}
		entries = append(entries, mappingEntry("ports", map[string]interface{}{"type": 1, "seq": seq}))
	}
	if len(volumes) > 0 {
		seq := make([]interface{}, 0, len(volumes))
		for _, v := range volumes {
			seq = append(seq, templateToken(v))
		}
		entries = append(entries, mappingEntry("volumes", map[string]interface{}{"type": 1, "seq": seq}))
	}
	if options != "" {
		entries = append(entries, mappingEntry("options", templateToken(options)))
	}
	return mappingToken(entries)
}

// mappingToken wraps key/value entries as a mapping TemplateToken.
func mappingToken(entries []interface{}) map[string]interface{} {
	return map[string]interface{}{"type": 2, "map": entries}
}

// mappingEntry is one {Key, Value} pair of a mapping token.
func mappingEntry(key string, value interface{}) map[string]interface{} {
	return map[string]interface{}{
		"Key":   map[string]interface{}{"type": 0, "lit": key},
		"Value": value,
	}
}

// scalarToken encodes a YAML scalar (string / number / bool) as the
// matching literal TemplateToken — `ports:` entries in particular
// parse as numbers (`- 80`) or strings (`- "8080:80"`).
func scalarToken(v interface{}) map[string]interface{} {
	switch t := v.(type) {
	case string:
		return templateToken(t)
	case bool:
		return map[string]interface{}{"type": 5, "lit": t}
	case int, int64, float64:
		return map[string]interface{}{"type": 6, "lit": t}
	default:
		return templateToken(fmt.Sprintf("%v", t))
	}
}

func sortedServiceNames(m map[string]*ServiceDef) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// buildNeedsContext builds the "needs" PipelineContextData from completed
// dependency outputs. Jobs inside a reusable-workflow call see sibling
// needs under their unprefixed keys; the synthetic gate never appears.
func buildNeedsContext(wf *Workflow, wfJob *WorkflowJob) interface{} {
	if len(wfJob.Needs) == 0 {
		return dictContextData()
	}
	var binding *WorkflowCallBinding
	if wfJob.Def != nil && wfJob.Def.Call != nil && wfJob.Def.CallRole == "" {
		binding = wfJob.Def.Call
	}

	// Build a nested dict: needs.<job>.outputs.<name> = value, needs.<job>.result = "success"
	entries := make([]map[string]interface{}, 0, len(wfJob.Needs))
	for _, depKey := range wfJob.Needs {
		depJob, ok := wf.Jobs[depKey]
		if !ok {
			continue
		}
		if binding != nil {
			if depKey == binding.CallerKey+"/__call" {
				continue
			}
			depKey = strings.TrimPrefix(depKey, binding.CallerKey+"/")
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
			{"k": "result", "v": string(depJob.Result)},
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
