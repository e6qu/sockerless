package bleephub

// GitHub code quality setup REST surface (repository-scoped): the
// stored configuration for periodic code quality analysis.

import (
	"encoding/json"
	"net/http"
	"slices"
	"time"
)

// CodeQualitySetup is a repository's code quality configuration. Empty
// strings model the null runner_type / runner_label / schedule members.
type CodeQualitySetup struct {
	RepoFullName string     `json:"repo_full_name"`
	State        string     `json:"state"`
	Languages    []string   `json:"languages"`
	RunnerType   string     `json:"runner_type"`
	RunnerLabel  string     `json:"runner_label"`
	Schedule     string     `json:"schedule"`
	UpdatedAt    *time.Time `json:"updated_at"`
}

func cloneCodeQualitySetup(setup *CodeQualitySetup) *CodeQualitySetup {
	if setup == nil {
		return nil
	}
	cp := *setup
	cp.Languages = append([]string(nil), setup.Languages...)
	if setup.UpdatedAt != nil {
		updatedAt := *setup.UpdatedAt
		cp.UpdatedAt = &updatedAt
	}
	return &cp
}

// GetCodeQualitySetup returns the repository's code quality setup, or
// the unconfigured default.
func (st *Store) GetCodeQualitySetup(repoFullName string) *CodeQualitySetup {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if setup, ok := st.CodeQualitySetups[repoFullName]; ok && setup != nil {
		return cloneCodeQualitySetup(setup)
	}
	return &CodeQualitySetup{RepoFullName: repoFullName, State: "not-configured", Languages: []string{}}
}

// SetCodeQualitySetup stores the repository's code quality setup.
func (st *Store) SetCodeQualitySetup(setup *CodeQualitySetup) {
	st.mu.Lock()
	defer st.mu.Unlock()
	stored := cloneCodeQualitySetup(setup)
	st.CodeQualitySetups[setup.RepoFullName] = stored
	if st.persist != nil {
		st.persist.MustPut("code_quality_setups", setup.RepoFullName, stored)
	}
}

func (s *Server) registerGHCodeQualityRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/code-quality/setup", s.handleGetCodeQualitySetup)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/code-quality/setup", s.handleUpdateCodeQualitySetup)
}

func codeQualitySetupJSON(setup *CodeQualitySetup) map[string]interface{} {
	nullable := func(v string) interface{} {
		if v == "" {
			return nil
		}
		return v
	}
	var updatedAt interface{}
	if setup.UpdatedAt != nil {
		updatedAt = setup.UpdatedAt.Format(time.RFC3339)
	}
	languages := setup.Languages
	if languages == nil {
		languages = []string{}
	}
	return map[string]interface{}{
		"state":        setup.State,
		"languages":    languages,
		"runner_type":  nullable(setup.RunnerType),
		"runner_label": nullable(setup.RunnerLabel),
		"updated_at":   updatedAt,
		"schedule":     nullable(setup.Schedule),
	}
}

func (s *Server) handleGetCodeQualitySetup(w http.ResponseWriter, r *http.Request) {
	if ghUserFromContext(r.Context()) == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	writeJSON(w, http.StatusOK, codeQualitySetupJSON(s.store.GetCodeQualitySetup(repo.FullName)))
}

var codeQualityUpdateLanguages = []string{"csharp", "go", "java-kotlin", "javascript-typescript", "python", "ruby"}

func (s *Server) handleUpdateCodeQualitySetup(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have admin rights to Repository.")
		return
	}

	// The update schema forbids unknown members and requires at least
	// one of state / runner_type / runner_label / languages.
	var raw map[string]json.RawMessage
	if !decodeJSONBody(w, r, &raw) {
		return
	}
	if len(raw) == 0 {
		writeGHError(w, http.StatusUnprocessableEntity,
			"At least one of state, runner_type, runner_label, or languages must be provided.")
		return
	}
	for key := range raw {
		switch key {
		case "state", "runner_type", "runner_label", "languages":
		default:
			writeGHValidationError(w, "CodeQualitySetup", key, "invalid")
			return
		}
	}

	setup := s.store.GetCodeQualitySetup(repo.FullName)
	if v, ok := raw["state"]; ok {
		var state string
		if err := json.Unmarshal(v, &state); err != nil || (state != "configured" && state != "not-configured") {
			writeGHValidationError(w, "CodeQualitySetup", "state", "invalid")
			return
		}
		setup.State = state
	}
	if v, ok := raw["runner_type"]; ok {
		var runnerType string
		if err := json.Unmarshal(v, &runnerType); err != nil || (runnerType != "standard" && runnerType != "labeled") {
			writeGHValidationError(w, "CodeQualitySetup", "runner_type", "invalid")
			return
		}
		setup.RunnerType = runnerType
		if runnerType == "standard" {
			setup.RunnerLabel = ""
		}
	}
	if v, ok := raw["runner_label"]; ok {
		var label *string
		if err := json.Unmarshal(v, &label); err != nil {
			writeGHValidationError(w, "CodeQualitySetup", "runner_label", "invalid")
			return
		}
		if label == nil {
			setup.RunnerLabel = ""
		} else {
			setup.RunnerLabel = *label
		}
	}
	if v, ok := raw["languages"]; ok {
		var languages []string
		if err := json.Unmarshal(v, &languages); err != nil {
			writeGHValidationError(w, "CodeQualitySetup", "languages", "invalid")
			return
		}
		for _, lang := range languages {
			if !slices.Contains(codeQualityUpdateLanguages, lang) {
				writeGHValidationError(w, "CodeQualitySetup", "languages", "invalid")
				return
			}
		}
		setup.Languages = languages
	}
	if setup.RunnerType == "labeled" && setup.RunnerLabel == "" {
		writeGHValidationError(w, "CodeQualitySetup", "runner_label", "missing_field")
		return
	}
	// The periodic analysis schedule exists exactly while the setup is
	// configured; "weekly" is the only schedule GitHub offers.
	if setup.State == "configured" {
		setup.Schedule = "weekly"
	} else {
		setup.Schedule = ""
	}
	now := time.Now().UTC()
	setup.UpdatedAt = &now
	s.store.SetCodeQualitySetup(setup)
	// bleephub applies the configuration synchronously and runs no
	// analysis workflow, so the documented plain-success response (200
	// with an empty object) is the honest one — 202 is reserved for an
	// update that scheduled an analysis run.
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}
