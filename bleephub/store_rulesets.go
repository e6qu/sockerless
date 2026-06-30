package bleephub

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Ruleset is a GitHub repository ruleset.
type Ruleset struct {
	ID                   int                    `json:"id"`
	NodeID               string                 `json:"node_id"`
	RepoID               int                    `json:"repo_id"`
	Name                 string                 `json:"name"`
	Target               string                 `json:"target"` // branch, tag
	SourceType           string                 `json:"source_type"`
	Source               string                 `json:"source"`
	Enforcement          string                 `json:"enforcement"` // active, evaluate, disabled
	BypassActors         []RulesetBypassActor   `json:"bypass_actors"`
	CurrentUserCanBypass string                 `json:"current_user_can_bypass"`
	Conditions           RulesetConditions      `json:"conditions"`
	Rules                []Rule                 `json:"rules"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
	Versions             map[int]RulesetVersion `json:"-"`
	NextVersionID        int                    `json:"-"`
}

// RulesetBypassActor represents an actor that can bypass a ruleset.
type RulesetBypassActor struct {
	ActorID    int    `json:"actor_id"`
	ActorType  string `json:"actor_type"`
	BypassMode string `json:"bypass_mode"`
}

// RulesetConditions holds the conditions under which a ruleset applies.
type RulesetConditions struct {
	RefName RefNameCondition `json:"ref_name,omitempty"`
}

// RefNameCondition matches ref names.
type RefNameCondition struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

// Rule is a single rule inside a ruleset.
type Rule struct {
	Type       string                 `json:"type"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// RulesetVersion is a historical snapshot of a ruleset.
type RulesetVersion struct {
	VersionID int       `json:"version_id"`
	Ruleset   Ruleset   `json:"ruleset"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateRuleset creates and persists a new ruleset for a repository.
func (st *Store) CreateRuleset(repo *Repo, rs *Ruleset) *Ruleset {
	st.mu.Lock()
	defer st.mu.Unlock()

	rs.ID = st.NextRulesetID
	st.NextRulesetID++
	rs.NodeID = rulesetNodeID(rs.ID)
	rs.RepoID = repo.ID
	rs.SourceType = "Repository"
	rs.Source = repo.FullName
	rs.CurrentUserCanBypass = "never"
	if rs.Enforcement == "" {
		rs.Enforcement = "active"
	}
	if rs.Target == "" {
		rs.Target = "branch"
	}
	now := time.Now().UTC()
	rs.CreatedAt = now
	rs.UpdatedAt = now
	rs.Versions = map[int]RulesetVersion{}
	rs.NextVersionID = 1

	st.Rulesets[rs.ID] = rs
	st.persistRuleset(rs)
	return rs
}

// UpdateRuleset updates an existing ruleset and records a history snapshot.
func (st *Store) UpdateRuleset(repo *Repo, rs *Ruleset, updates *Ruleset) *Ruleset {
	st.mu.Lock()
	defer st.mu.Unlock()

	// Snapshot current state to history before mutating.
	snapshot := *rs
	snapshot.Versions = nil
	snapshot.NextVersionID = 0
	if rs.Versions == nil {
		rs.Versions = map[int]RulesetVersion{}
	}
	rs.Versions[rs.NextVersionID] = RulesetVersion{
		VersionID: rs.NextVersionID,
		Ruleset:   snapshot,
		CreatedAt: time.Now().UTC(),
	}
	rs.NextVersionID++

	if updates.Name != "" {
		rs.Name = updates.Name
	}
	if updates.Target != "" {
		rs.Target = updates.Target
	}
	if updates.Enforcement != "" {
		rs.Enforcement = updates.Enforcement
	}
	if updates.BypassActors != nil {
		rs.BypassActors = updates.BypassActors
	}
	if updates.CurrentUserCanBypass != "" {
		rs.CurrentUserCanBypass = updates.CurrentUserCanBypass
	}
	if len(updates.Conditions.RefName.Include) > 0 || len(updates.Conditions.RefName.Exclude) > 0 {
		rs.Conditions = updates.Conditions
	}
	if updates.Rules != nil {
		rs.Rules = updates.Rules
	}
	rs.UpdatedAt = time.Now().UTC()
	st.persistRuleset(rs)
	return rs
}

// DeleteRuleset removes a ruleset.
func (st *Store) DeleteRuleset(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, ok := st.Rulesets[id]; !ok {
		return false
	}
	delete(st.Rulesets, id)
	if st.persist != nil {
		st.persist.MustDelete("repo_rulesets", strconv.Itoa(id))
	}
	return true
}

// GetRuleset returns a ruleset by ID.
func (st *Store) GetRuleset(id int) *Ruleset {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Rulesets[id]
}

// ListRulesetsForRepo returns all rulesets for a repository, sorted by ID.
func (st *Store) ListRulesetsForRepo(repoID int) []*Ruleset {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*Ruleset
	for _, rs := range st.Rulesets {
		if rs.RepoID == repoID {
			out = append(out, rs)
		}
	}
	return out
}

// ListRulesForBranch evaluates active branch-targeting rulesets against a branch
// and returns the flattened rule objects GitHub's "list rules for a branch"
// endpoint produces.
func (st *Store) ListRulesForBranch(repo *Repo, branch string) []map[string]interface{} {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var out []map[string]interface{}
	for _, rs := range st.Rulesets {
		if rs.RepoID != repo.ID {
			continue
		}
		if rs.Enforcement == "disabled" {
			continue
		}
		if rs.Target != "" && rs.Target != "branch" {
			continue
		}
		if !rulesetMatchesBranch(rs, repo.DefaultBranch, branch) {
			continue
		}
		for _, rule := range rs.Rules {
			obj := map[string]interface{}{
				"type":                rule.Type,
				"ruleset_id":          rs.ID,
				"ruleset_source_type": rs.SourceType,
				"ruleset_source":      rs.Source,
			}
			if rule.Parameters != nil {
				obj["parameters"] = rule.Parameters
			}
			out = append(out, obj)
		}
	}
	return out
}

// GetRulesetHistory returns prior versions of a ruleset.
func (st *Store) GetRulesetHistory(rs *Ruleset) []RulesetVersion {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []RulesetVersion
	for _, v := range rs.Versions {
		out = append(out, v)
	}
	return out
}

// GetRulesetVersion returns a specific historical version.
func (st *Store) GetRulesetVersion(rs *Ruleset, versionID int) *RulesetVersion {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if v, ok := rs.Versions[versionID]; ok {
		return &v
	}
	return nil
}

func (st *Store) persistRuleset(rs *Ruleset) {
	if st.persist != nil {
		st.persist.MustPut("repo_rulesets", strconv.Itoa(rs.ID), rs)
	}
}

func rulesetNodeID(id int) string {
	return "RSR_" + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("ruleset:%d", id)))
}

func rulesetMatchesBranch(rs *Ruleset, defaultBranch, branch string) bool {
	cond := rs.Conditions.RefName
	if len(cond.Include) == 0 && len(cond.Exclude) == 0 {
		return true
	}
	included := false
	for _, pat := range cond.Include {
		if matchRefPattern(pat, defaultBranch, branch) {
			included = true
			break
		}
	}
	if !included {
		return false
	}
	for _, pat := range cond.Exclude {
		if matchRefPattern(pat, defaultBranch, branch) {
			return false
		}
	}
	return true
}

func matchRefPattern(pat, defaultBranch, branch string) bool {
	pat = strings.TrimPrefix(pat, "refs/heads/")
	switch pat {
	case "~ALL", "*":
		return true
	case "~DEFAULT_BRANCH":
		return branch == defaultBranch
	}
	// Very small glob subset: trailing *.
	if strings.HasSuffix(pat, "*") {
		return strings.HasPrefix(branch, strings.TrimSuffix(pat, "*"))
	}
	return branch == pat
}
