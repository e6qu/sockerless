package bleephub

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

// maxWorkflowCallDepth bounds reusable-workflow nesting: GitHub allows
// connecting up to four levels of workflows (the caller plus three
// nested calls).
const maxWorkflowCallDepth = 4

// WorkflowCallBinding links the jobs produced by one reusable-workflow
// call. The gate node resolves the caller's `with:` templates once its
// dependencies finish; called jobs read the resolved inputs and secret
// configuration; the collector maps the called workflow's outputs onto
// the public caller job key.
type WorkflowCallBinding struct {
	// CalledPath identifies the called workflow (for errors/logging).
	CalledPath string
	// With holds the caller's raw input templates; InputDefs the called
	// workflow's declarations (typing + defaults).
	With      map[string]string
	InputDefs map[string]*WorkflowInputDef
	// SecretsInherit / SecretsMap mirror the caller job's `secrets:`.
	SecretsMap     map[string]string
	SecretsInherit bool
	// OutputDefs holds the called workflow's output value templates.
	OutputDefs map[string]string
	// CalledJobKeys lists the expanded keys of the called workflow's
	// jobs (collector aggregation + output evaluation scope); CallerKey
	// is the public caller job key (the needs-context prefix).
	CalledJobKeys []string
	CallerKey     string

	// ResolvedInputs is filled by the gate node when it completes:
	// the typed `inputs` context the called jobs run with.
	mu             sync.Mutex
	resolvedInputs map[string]interface{}
}

// SetResolvedInputs stores the typed inputs resolved at gate completion.
func (b *WorkflowCallBinding) SetResolvedInputs(in map[string]interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resolvedInputs = in
}

// ResolvedInputs returns the typed inputs resolved at gate completion
// (nil until the gate ran).
func (b *WorkflowCallBinding) ResolvedInputs() map[string]interface{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.resolvedInputs
}

// expandReusableWorkflows replaces every `uses:` job in def with the
// called workflow's jobs plus two synthetic server-completed nodes:
//
//	<caller>/__call  — the gate: carries the caller job's needs/if and
//	                   resolves `with:` templates when it completes
//	<caller>/<k>     — each called job (display "caller / called");
//	                   roots need the gate
//	<caller>         — the collector: needs every called job, computes
//	                   the workflow_call outputs, and serves downstream
//	                   `needs.<caller>` edges under the original key
//
// repoKey provides the resolution context for "./" references; depth
// starts at 1 for the top-level workflow.
func (s *Server) expandReusableWorkflows(def *WorkflowDef, repoKey string, depth int) (*WorkflowDef, error) {
	hasCalls := false
	for _, jd := range def.Jobs {
		if jd.Uses != "" {
			hasCalls = true
			break
		}
	}
	if !hasCalls {
		return def, nil
	}
	if depth >= maxWorkflowCallDepth {
		return nil, fmt.Errorf("reusable workflows nested deeper than %d levels", maxWorkflowCallDepth)
	}

	out := &WorkflowDef{
		Name:        def.Name,
		Env:         def.Env,
		Concurrency: def.Concurrency,
		Jobs:        make(map[string]*JobDef, len(def.Jobs)),
	}
	for key, jd := range def.Jobs {
		if jd.Uses == "" {
			out.Jobs[key] = jd
			continue
		}
		if err := s.expandOneCall(out, key, jd, repoKey, depth); err != nil {
			return nil, fmt.Errorf("job %q: %w", key, err)
		}
	}
	if err := validateJobGraph(out); err != nil {
		return nil, fmt.Errorf("reusable-workflow expansion produced an invalid graph: %w", err)
	}
	return out, nil
}

// expandOneCall expands a single `uses:` job into out.
func (s *Server) expandOneCall(out *WorkflowDef, callerKey string, caller *JobDef, repoKey string, depth int) error {
	calledRepo, calledPath, calledYAML, err := s.resolveCalledWorkflow(repoKey, caller.Uses)
	if err != nil {
		return err
	}

	calledOn, err := ParseWorkflowOn(calledYAML)
	if err != nil {
		return fmt.Errorf("called workflow %s: %w", calledPath, err)
	}
	callDef, isCallable := calledOn["workflow_call"]
	if !isCallable {
		return fmt.Errorf("called workflow %s does not declare on: workflow_call", calledPath)
	}
	var inputDefs map[string]*WorkflowInputDef
	var outputDefs map[string]string
	var secretDefs map[string]*WorkflowCallSecretDef
	if callDef != nil {
		inputDefs = callDef.Inputs
		outputDefs = callDef.Outputs
		secretDefs = callDef.Secrets
	}

	// Validate the caller's inputs/secrets against the declarations now —
	// real GitHub rejects the run at creation for these.
	for name := range caller.With {
		if _, ok := inputDefs[name]; !ok {
			return fmt.Errorf("called workflow %s does not define input %q", calledPath, name)
		}
	}
	for name, def := range inputDefs {
		if def != nil && def.Required && def.Default == nil {
			if _, ok := caller.With[name]; !ok {
				return fmt.Errorf("called workflow %s requires input %q", calledPath, name)
			}
		}
	}
	if !caller.SecretsInherit {
		for name, def := range secretDefs {
			if def != nil && def.Required {
				if _, ok := caller.SecretsMap[name]; !ok {
					return fmt.Errorf("called workflow %s requires secret %q", calledPath, name)
				}
			}
		}
	}

	calledDef, err := ParseWorkflow(calledYAML)
	if err != nil {
		return fmt.Errorf("called workflow %s: %w", calledPath, err)
	}
	calledDef = expandMatrixJobs(calledDef)
	calledDef, err = s.expandReusableWorkflows(calledDef, calledRepo, depth+1)
	if err != nil {
		return fmt.Errorf("called workflow %s: %w", calledPath, err)
	}

	binding := &WorkflowCallBinding{
		CallerKey:      callerKey,
		CalledPath:     calledPath,
		With:           caller.With,
		InputDefs:      inputDefs,
		SecretsMap:     caller.SecretsMap,
		SecretsInherit: caller.SecretsInherit,
		OutputDefs:     outputDefs,
	}

	callerDisplay := caller.Name
	if callerDisplay == "" {
		callerDisplay = callerKey
	}

	gateKey := callerKey + "/__call"
	out.Jobs[gateKey] = &JobDef{
		Name:            callerDisplay,
		Needs:           caller.Needs,
		If:              caller.If,
		Env:             caller.Env, // carries __matrix_* for `with:` evaluation
		Call:            binding,
		ServerCompleted: true,
		CallRole:        "gate",
	}

	collectorNeeds := []string{gateKey}
	for k, cjd := range calledDef.Jobs {
		childKey := callerKey + "/" + k
		child := *cjd // shallow copy; nested expansion already ran
		childDisplay := cjd.Name
		if childDisplay == "" {
			childDisplay = k
		}
		child.Name = callerDisplay + " / " + childDisplay
		// Internal needs point at sibling called jobs; roots wait on the gate.
		if len(cjd.Needs) == 0 {
			child.Needs = []string{gateKey}
		} else {
			prefixed := make([]string, 0, len(cjd.Needs))
			for _, n := range cjd.Needs {
				prefixed = append(prefixed, callerKey+"/"+n)
			}
			child.Needs = prefixed
		}
		if child.Call == nil {
			child.Call = binding
		}
		out.Jobs[childKey] = &child
		binding.CalledJobKeys = append(binding.CalledJobKeys, childKey)
		collectorNeeds = append(collectorNeeds, childKey)
	}

	out.Jobs[callerKey] = &JobDef{
		Name:            callerDisplay,
		Needs:           collectorNeeds,
		Call:            binding,
		ServerCompleted: true,
		CallRole:        "collector",
	}
	return nil
}

// resolveCalledWorkflow loads the YAML a `uses:` reference points at:
// "./.github/workflows/x.yml" resolves in the caller's repo at HEAD;
// "owner/repo/.github/workflows/x.yml@ref" resolves in another repo on
// this server at the given ref.
func (s *Server) resolveCalledWorkflow(repoKey, uses string) (calledRepo, calledPath string, yaml []byte, err error) {
	if strings.HasPrefix(uses, "./") {
		if repoKey == "" {
			return "", "", nil, fmt.Errorf("local reusable workflow %q needs a repository context", uses)
		}
		path := strings.TrimPrefix(uses, "./")
		parts := splitRepoKeyParts(repoKey)
		stor := s.store.GetGitStorage(parts[0], parts[1])
		if stor == nil {
			return "", "", nil, fmt.Errorf("no git storage for %s", repoKey)
		}
		content, err := gitFileAtRef(stor, "", path)
		if err != nil {
			return "", "", nil, fmt.Errorf("reusable workflow %s not found in %s: %w", path, repoKey, err)
		}
		return repoKey, path, content, nil
	}

	nameWithOwner, path, ref, isLocal := ParseActionRef(uses)
	if isLocal || nameWithOwner == "" || path == "" {
		return "", "", nil, fmt.Errorf("invalid reusable workflow reference %q", uses)
	}
	parts := splitRepoKeyParts(nameWithOwner)
	stor := s.store.GetGitStorage(parts[0], parts[1])
	if stor == nil {
		return "", "", nil, fmt.Errorf("repository %s not found on this server", nameWithOwner)
	}
	content, err := gitFileAtRef(stor, ref, path)
	if err != nil {
		return "", "", nil, fmt.Errorf("reusable workflow %s@%s not found in %s: %w", path, ref, nameWithOwner, err)
	}
	return nameWithOwner, path, content, nil
}

// gitFileAtRef reads one file from git storage at a ref (branch, tag, or
// sha); empty ref means HEAD.
func gitFileAtRef(stor gitStorage.Storer, ref, path string) ([]byte, error) {
	var hash plumbing.Hash
	if ref == "" {
		sha := resolveRefSha(stor, "")
		if sha == "0000000000000000000000000000000000000000" {
			return nil, fmt.Errorf("HEAD did not resolve to a commit")
		}
		hash = plumbing.NewHash(sha)
	} else {
		candidates := []plumbing.ReferenceName{
			plumbing.ReferenceName(ref),
			plumbing.NewBranchReferenceName(ref),
			plumbing.NewTagReferenceName(ref),
		}
		for _, name := range candidates {
			if r, err := stor.Reference(name); err == nil {
				hash = r.Hash()
				break
			}
		}
		if hash.IsZero() && len(ref) == 40 {
			hash = plumbing.NewHash(ref)
		}
		if hash.IsZero() {
			return nil, fmt.Errorf("ref %q not found", ref)
		}
	}

	commit, err := object.GetCommit(stor, hash)
	if err != nil {
		return nil, fmt.Errorf("resolve commit: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	entry, err := tree.FindEntry(path)
	if err != nil {
		return nil, fmt.Errorf("file %q not found", path)
	}
	blob, err := object.GetBlob(stor, entry.Hash)
	if err != nil {
		return nil, err
	}
	r, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// completeServerJob finishes a synthetic gate/collector node in place.
// Callers hold the store write lock.
func (s *Server) completeServerJobLocked(wf *Workflow, wfJob *WorkflowJob) {
	jd := wfJob.Def
	switch jd.CallRole {
	case "gate":
		if !s.resolveCallInputsLocked(wf, wfJob) {
			wfJob.Status = JobStatusCompleted
			wfJob.Result = ResultFailure
			break
		}
		wfJob.Status = JobStatusCompleted
		wfJob.Result = ResultSuccess
	case "collector":
		s.collectCallOutputsLocked(wf, wfJob)
	default:
		wfJob.Status = JobStatusCompleted
		wfJob.Result = ResultSuccess
	}
	wfJob.CompletedAt = time.Now()
	if wfJob.StartedAt.IsZero() {
		wfJob.StartedAt = wfJob.CompletedAt
	}
}

// resolveCallInputsLocked evaluates the caller's `with:` templates with
// the contexts available to jobs.<id>.with (github, needs, vars, inputs,
// matrix) and applies the called workflow's defaults and typing.
func (s *Server) resolveCallInputsLocked(wf *Workflow, gate *WorkflowJob) bool {
	binding := gate.Def.Call
	if binding == nil {
		return true
	}
	ctx, err := s.jobExprContext(wf, gate)
	if err != nil {
		s.logger.Warn().Err(err).Str("workflow", binding.CalledPath).
			Msg("workflow_call input context failed")
		return false
	}
	if len(gate.MatrixValues) > 0 {
		matrixCtx := make(map[string]interface{}, len(gate.MatrixValues))
		for k, v := range gate.MatrixValues {
			matrixCtx[k] = v
		}
		ctx.Contexts["matrix"] = matrixCtx
	}

	resolved := make(map[string]interface{}, len(binding.InputDefs))
	for name, tmpl := range binding.With {
		val, err := EvalTemplate(tmpl, ctx)
		if err != nil {
			s.logger.Warn().Err(err).Str("input", name).Str("workflow", binding.CalledPath).
				Msg("workflow_call input template failed — passing empty")
			val = ""
		}
		resolved[name] = typedCallInput(binding.InputDefs[name], val)
	}
	for name, def := range binding.InputDefs {
		if _, ok := resolved[name]; ok || def == nil {
			continue
		}
		if def.Default != nil {
			resolved[name] = normalizeYAMLValue(def.Default)
		} else if def.Type == "boolean" {
			resolved[name] = false
		}
	}
	binding.SetResolvedInputs(resolved)
	return true
}

// typedCallInput converts a resolved string input to the declared type.
func typedCallInput(def *WorkflowInputDef, val string) interface{} {
	if def == nil {
		return val
	}
	switch def.Type {
	case "boolean":
		return val == "true"
	case "number":
		if f, err := parseJSONNumber(val); err == nil {
			return f
		}
		return val
	default:
		return val
	}
}

// collectCallOutputsLocked finishes a collector node: outputs evaluate
// the workflow_call output templates against the called jobs' results;
// the node reports skipped when every called job skipped (mirroring the
// caller job's state on real GitHub).
func (s *Server) collectCallOutputsLocked(wf *Workflow, collector *WorkflowJob) {
	binding := collector.Def.Call
	allSkipped := true
	jobsCtx := make(map[string]interface{})
	if binding != nil {
		for _, key := range binding.CalledJobKeys {
			called, ok := wf.Jobs[key]
			if !ok {
				continue
			}
			if called.Status != JobStatusSkipped {
				allSkipped = false
			}
			outputs := make(map[string]interface{}, len(called.Outputs))
			for k, v := range called.Outputs {
				outputs[k] = v
			}
			// Output templates address jobs by their UNPREFIXED key.
			shortKey := strings.TrimPrefix(key, binding.CallerKey+"/")
			jobsCtx[shortKey] = map[string]interface{}{
				"result":  string(called.Result),
				"outputs": outputs,
			}
		}
	}

	if allSkipped {
		collector.Status = JobStatusSkipped
		collector.Result = ResultSkipped
		return
	}

	collector.Status = JobStatusCompleted
	collector.Result = ResultSuccess
	if binding == nil || len(binding.OutputDefs) == 0 {
		return
	}
	ctx := &ExprContext{Contexts: map[string]interface{}{
		"jobs":   jobsCtx,
		"github": s.githubContextMap(wf),
		"inputs": binding.ResolvedInputs(),
	}}
	for name, tmpl := range binding.OutputDefs {
		val, err := EvalTemplate(tmpl, ctx)
		if err != nil {
			s.logger.Warn().Err(err).Str("output", name).Msg("workflow_call output template failed")
			continue
		}
		collector.Outputs[name] = val
	}
}

// parseJSONNumber parses a numeric string the way expression numbers work.
func parseJSONNumber(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%g", &f)
	return f, err
}
