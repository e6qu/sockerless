package bleephub

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitStorage "github.com/go-git/go-git/v5/storage"
	"gopkg.in/yaml.v3"
)

// TriggerDef is one event entry under a workflow's `on:` with its
// filters. A nil *TriggerDef (event listed without filters) means
// "match every activity the event's defaults cover".
type TriggerDef struct {
	Branches       []string
	BranchesIgnore []string
	Tags           []string
	TagsIgnore     []string
	Paths          []string
	PathsIgnore    []string
	Types          []string
	// Inputs carries workflow_dispatch / workflow_call input declarations.
	Inputs map[string]*WorkflowInputDef
	// Crons carries the cron lines of `on: schedule:`.
	Crons []string
	// Outputs carries workflow_call output declarations: name → the
	// `${{ jobs.<id>.outputs.<o> }}` value template.
	Outputs map[string]string
	// Secrets carries workflow_call secret declarations.
	Secrets map[string]*WorkflowCallSecretDef
}

// WorkflowCallSecretDef is a declared workflow_call secret.
type WorkflowCallSecretDef struct {
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

// workflowCallOutputDef is the YAML shape of one workflow_call output.
type workflowCallOutputDef struct {
	Description string `yaml:"description"`
	Value       string `yaml:"value"`
}

// WorkflowInputDef is a declared workflow_dispatch / workflow_call input.
type WorkflowInputDef struct {
	Description string        `yaml:"description"`
	Required    bool          `yaml:"required"`
	Default     interface{}   `yaml:"default"`
	Type        string        `yaml:"type"` // string | choice | boolean | number | environment
	Options     []interface{} `yaml:"options"`
}

// triggerEvent describes a concrete event occurrence to match against
// workflow `on:` definitions.
type triggerEvent struct {
	Type   string // "push", "pull_request", ...
	Action string // activity type ("opened", ...) or repository_dispatch event_type
	Ref    string // full ref ("refs/heads/main", "refs/tags/v1")
	// ChangedFiles lists paths touched by the event; valid only when
	// ChangedFilesKnown (path filters pass open when a diff can't be
	// computed, matching GitHub's behavior for new-branch pushes).
	ChangedFiles      []string
	ChangedFilesKnown bool
}

// defaultActivityTypes are the activity types an event matches when its
// trigger declares no `types:` filter — GitHub's documented defaults for
// the events where the default is NOT "all types".
var defaultActivityTypes = map[string][]string{
	"pull_request":        {"opened", "synchronize", "reopened"},
	"pull_request_target": {"opened", "synchronize", "reopened"},
}

// ParseWorkflowOn extracts the structured `on:` definition from workflow
// YAML: event name → filters (nil when the event has no filters).
func ParseWorkflowOn(yamlContent []byte) (map[string]*TriggerDef, error) {
	var raw struct {
		On yaml.Node `yaml:"on"`
	}
	if err := yaml.Unmarshal(yamlContent, &raw); err != nil {
		return nil, fmt.Errorf("parse workflow on: %w", err)
	}
	out := map[string]*TriggerDef{}
	node := &raw.On
	switch node.Kind {
	case 0:
		// No `on:` at all. YAML quirk: an unquoted `on:` key parses as
		// boolean true — callers see an empty map either way.
		return out, nil
	case yaml.ScalarNode:
		var s string
		if err := node.Decode(&s); err != nil {
			return nil, fmt.Errorf("parse workflow on: %w", err)
		}
		if s != "" && s != "true" {
			out[s] = nil
		}
		return out, nil
	case yaml.SequenceNode:
		var events []string
		if err := node.Decode(&events); err != nil {
			return nil, fmt.Errorf("parse workflow on: %w", err)
		}
		for _, e := range events {
			out[e] = nil
		}
		return out, nil
	case yaml.MappingNode:
		var m map[string]yaml.Node
		if err := node.Decode(&m); err != nil {
			return nil, fmt.Errorf("parse workflow on: %w", err)
		}
		for event, val := range m {
			td, err := parseTriggerDef(event, &val)
			if err != nil {
				return nil, fmt.Errorf("on.%s: %w", event, err)
			}
			out[event] = td
		}
		return out, nil
	default:
		return nil, fmt.Errorf("on: must be a string, list, or map")
	}
}

func parseTriggerDef(event string, node *yaml.Node) (*TriggerDef, error) {
	if node.Kind == 0 || node.Tag == "!!null" {
		return nil, nil
	}
	if event == "schedule" {
		var entries []struct {
			Cron string `yaml:"cron"`
		}
		if err := node.Decode(&entries); err != nil {
			return nil, fmt.Errorf("schedule must be a list of {cron: ...}: %w", err)
		}
		td := &TriggerDef{}
		for _, e := range entries {
			if e.Cron != "" {
				td.Crons = append(td.Crons, e.Cron)
			}
		}
		return td, nil
	}
	var raw struct {
		Branches       []string                          `yaml:"branches"`
		BranchesIgnore []string                          `yaml:"branches-ignore"`
		Tags           []string                          `yaml:"tags"`
		TagsIgnore     []string                          `yaml:"tags-ignore"`
		Paths          []string                          `yaml:"paths"`
		PathsIgnore    []string                          `yaml:"paths-ignore"`
		Types          []string                          `yaml:"types"`
		Inputs         map[string]*WorkflowInputDef      `yaml:"inputs"`
		Outputs        map[string]*workflowCallOutputDef `yaml:"outputs"`
		Secrets        map[string]*WorkflowCallSecretDef `yaml:"secrets"`
	}
	if err := node.Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid trigger filters: %w", err)
	}
	td := &TriggerDef{
		Branches:       raw.Branches,
		BranchesIgnore: raw.BranchesIgnore,
		Tags:           raw.Tags,
		TagsIgnore:     raw.TagsIgnore,
		Paths:          raw.Paths,
		PathsIgnore:    raw.PathsIgnore,
		Types:          raw.Types,
		Inputs:         raw.Inputs,
		Secrets:        raw.Secrets,
	}
	if len(raw.Outputs) > 0 {
		td.Outputs = make(map[string]string, len(raw.Outputs))
		for name, o := range raw.Outputs {
			if o != nil {
				td.Outputs[name] = o.Value
			}
		}
	}
	if len(td.Branches) > 0 && len(td.BranchesIgnore) > 0 {
		return nil, fmt.Errorf("branches and branches-ignore cannot be used together")
	}
	if len(td.Tags) > 0 && len(td.TagsIgnore) > 0 {
		return nil, fmt.Errorf("tags and tags-ignore cannot be used together")
	}
	if len(td.Paths) > 0 && len(td.PathsIgnore) > 0 {
		return nil, fmt.Errorf("paths and paths-ignore cannot be used together")
	}
	return td, nil
}

// workflowTriggersOn decides whether a workflow (by its parsed `on:`)
// fires for a concrete event. Invalid filter combinations were rejected
// at parse; an event absent from `on:` never fires.
func workflowTriggersOn(on map[string]*TriggerDef, ev triggerEvent) bool {
	td, ok := on[ev.Type]
	if !ok {
		return false
	}

	// Activity types: explicit list, else the event's documented default
	// (events without an entry in defaultActivityTypes match any action).
	types := defaultActivityTypes[ev.Type]
	if td != nil && len(td.Types) > 0 {
		types = td.Types
	}
	if len(types) > 0 && ev.Action != "" {
		matched := false
		for _, t := range types {
			if t == ev.Action {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if td == nil {
		return true
	}

	// Branch / tag filters. For push events they apply to the pushed ref;
	// for pull_request events the branches filter applies to the BASE
	// branch (ev.Ref carries the base ref for PR events at the call site).
	isTag := strings.HasPrefix(ev.Ref, "refs/tags/")
	branchName := strings.TrimPrefix(ev.Ref, "refs/heads/")
	tagName := strings.TrimPrefix(ev.Ref, "refs/tags/")
	hasBranchFilter := len(td.Branches) > 0 || len(td.BranchesIgnore) > 0
	hasTagFilter := len(td.Tags) > 0 || len(td.TagsIgnore) > 0

	if isTag {
		// A push trigger filtered to branches only never matches tags.
		if hasBranchFilter && !hasTagFilter {
			return false
		}
		if len(td.Tags) > 0 && !filterPatternsMatch(td.Tags, tagName) {
			return false
		}
		if len(td.TagsIgnore) > 0 && filterPatternsMatch(td.TagsIgnore, tagName) {
			return false
		}
	} else if ev.Ref != "" {
		if hasTagFilter && !hasBranchFilter {
			return false
		}
		if len(td.Branches) > 0 && !filterPatternsMatch(td.Branches, branchName) {
			return false
		}
		if len(td.BranchesIgnore) > 0 && filterPatternsMatch(td.BranchesIgnore, branchName) {
			return false
		}
	}

	// Path filters: when the diff is unknown (new branch, shallow data)
	// the filter passes open, matching GitHub.
	if len(td.Paths) > 0 && ev.ChangedFilesKnown {
		any := false
		for _, f := range ev.ChangedFiles {
			if filterPatternsMatch(td.Paths, f) {
				any = true
				break
			}
		}
		if !any {
			return false
		}
	}
	if len(td.PathsIgnore) > 0 && ev.ChangedFilesKnown {
		allIgnored := true
		for _, f := range ev.ChangedFiles {
			if !filterPatternsMatch(td.PathsIgnore, f) {
				allIgnored = false
				break
			}
		}
		if allIgnored && len(ev.ChangedFiles) > 0 {
			return false
		}
	}

	return true
}

// filterPatternsMatch evaluates GitHub filter patterns in order: a
// matching pattern includes the value, a later matching `!pattern`
// excludes it again (and a yet-later positive match re-includes).
func filterPatternsMatch(patterns []string, value string) bool {
	matched := false
	for _, p := range patterns {
		if neg, ok := strings.CutPrefix(p, "!"); ok {
			if matched && filterPatternMatch(neg, value) {
				matched = false
			}
			continue
		}
		if !matched && filterPatternMatch(p, value) {
			matched = true
		}
	}
	return matched
}

var filterPatternCache = struct {
	m map[string]*regexp.Regexp
}{m: map[string]*regexp.Regexp{}}

// filterPatternMatch matches one GitHub filter pattern: `*` (any except
// '/'), `**` (any), `?` / `+` (zero-or-one / one-or-more of the
// preceding token), `[...]` character classes.
func filterPatternMatch(pattern, value string) bool {
	re, err := compileFilterPattern(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(value)
}

func compileFilterPattern(pattern string) (*regexp.Regexp, error) {
	if re, ok := filterPatternCache.m[pattern]; ok {
		return re, nil
	}
	var sb strings.Builder
	sb.WriteString("^(?:")
	lastAtom := ""
	emit := func(atom string) {
		sb.WriteString(lastAtom)
		lastAtom = atom
	}
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				emit("(?s:.*)")
				i++
			} else {
				emit("[^/]*")
			}
		case '?':
			if lastAtom == "" {
				emit(regexp.QuoteMeta("?"))
			} else {
				lastAtom = "(?:" + lastAtom + ")?"
			}
		case '+':
			if lastAtom == "" {
				emit(regexp.QuoteMeta("+"))
			} else {
				lastAtom = "(?:" + lastAtom + ")+"
			}
		case '[':
			end := strings.IndexByte(pattern[i:], ']')
			if end <= 1 {
				emit(regexp.QuoteMeta(string(c)))
				continue
			}
			emit(pattern[i : i+end+1])
			i += end
		default:
			emit(regexp.QuoteMeta(string(c)))
		}
	}
	sb.WriteString(lastAtom)
	sb.WriteString(")$")
	re, err := regexp.Compile(sb.String())
	if err != nil {
		return nil, err
	}
	filterPatternCache.m[pattern] = re
	return re, nil
}

// changedFilesBetween computes the paths touched between two commits in
// git storage. ok is false when the diff cannot be computed (zero/unknown
// shas) — path filters then pass open like real GitHub.
func changedFilesBetween(stor gitStorage.Storer, beforeSha, afterSha string) (files []string, ok bool) {
	const zeroSha = "0000000000000000000000000000000000000000"
	if beforeSha == "" || afterSha == "" || beforeSha == zeroSha || afterSha == zeroSha {
		return nil, false
	}
	beforeCommit, err := object.GetCommit(stor, plumbing.NewHash(beforeSha))
	if err != nil {
		return nil, false
	}
	afterCommit, err := object.GetCommit(stor, plumbing.NewHash(afterSha))
	if err != nil {
		return nil, false
	}
	beforeTree, err := beforeCommit.Tree()
	if err != nil {
		return nil, false
	}
	afterTree, err := afterCommit.Tree()
	if err != nil {
		return nil, false
	}
	changes, err := object.DiffTree(beforeTree, afterTree)
	if err != nil {
		return nil, false
	}
	seen := map[string]bool{}
	for _, ch := range changes {
		for _, name := range []string{ch.From.Name, ch.To.Name} {
			if name != "" && !seen[name] {
				seen[name] = true
				files = append(files, name)
			}
		}
	}
	return files, true
}

// normalizeYAMLValue maps YAML-decoded scalars into the expression value
// space (ints become float64 like every other expression number).
func normalizeYAMLValue(v interface{}) interface{} {
	switch t := v.(type) {
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case uint64:
		return float64(t)
	default:
		return v
	}
}

// firePullRequestSynchronize emits pull_request "synchronize" (webhook +
// workflow triggers) for every open PR whose head branch just received a
// push — real GitHub's behavior for pushes to a PR's source branch.
func (s *Server) firePullRequestSynchronize(repo *Repo, repoKey, branch string) {
	s.store.mu.RLock()
	var prs []*PullRequest
	for _, pr := range s.store.PullRequests {
		if pullRequestHeadRepoID(pr) == repo.ID && pr.State == "OPEN" && pr.HeadRefName == branch {
			prs = append(prs, pr)
		}
	}
	s.store.mu.RUnlock()

	for _, pr := range prs {
		baseRepo := s.store.GetRepoByID(pr.RepoID)
		if baseRepo == nil {
			continue
		}
		payload := buildPullRequestPayload(s.store, baseRepo, pr, nil, "synchronize")
		s.emitWebhookEvent(baseRepo.FullName, "pull_request", "synchronize", payload)
		s.triggerWorkflowsForEvent(baseRepo.FullName, "pull_request", "synchronize", "refs/heads/"+pr.HeadRefName, payload)
	}
}

// resolveBranchSha resolves a branch name to its commit sha in git
// storage; empty when unknown.
func resolveBranchSha(stor gitStorage.Storer, branch string) string {
	if stor == nil || branch == "" {
		return ""
	}
	ref, err := stor.Reference(plumbing.NewBranchReferenceName(branch))
	if err != nil {
		return ""
	}
	return ref.Hash().String()
}
