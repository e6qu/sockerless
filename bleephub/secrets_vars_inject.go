package bleephub

import (
	"fmt"
	"strings"
)

// orgItemVisibleToRepo reports whether an organization-level secret or
// variable applies to a repository under real GitHub's visibility rules:
// "all" applies to every repo in the org, "private" only to private (and
// internal) repos, "selected" only to the explicitly selected repo IDs.
func orgItemVisibleToRepo(visibility string, selectedIDs []int, repo *Repo) bool {
	switch visibility {
	case "all":
		return true
	case "private":
		return repo != nil && repo.Private
	case "selected":
		if repo == nil {
			return false
		}
		for _, id := range selectedIDs {
			if id == repo.ID {
				return true
			}
		}
	}
	return false
}

// CollectJobSecretsAndVars resolves the Actions secrets and configuration
// variables a job running in repoFullName (optionally inside environment
// envName; "" for none) receives, exactly as real GitHub merges them:
// organization-level items apply first (filtered by their visibility
// against the repo), repository-level items override them, and
// environment-level items override both. Secrets and variables merge
// independently. The returned maps are fresh copies safe to hand to the
// runner-message builder.
func (s *Server) CollectJobSecretsAndVars(repoFullName, envName string) (secrets map[string]string, vars map[string]string, err error) {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	return s.collectJobSecretsAndVarsLocked(repoFullName, envName)
}

// collectJobSecretsAndVarsLocked is the lock-free core of
// CollectJobSecretsAndVars for callers already holding the store lock
// (job dispatch evaluates `if:` expressions under it).
func (s *Server) collectJobSecretsAndVarsLocked(repoFullName, envName string) (secrets map[string]string, vars map[string]string, err error) {
	secrets = make(map[string]string)
	vars = make(map[string]string)

	repo := s.store.ReposByName[repoFullName]
	if repo == nil {
		return nil, nil, fmt.Errorf("repository %q not found for Actions secrets and variables", repoFullName)
	}

	// Organization scope (lowest precedence). The owner segment is only an
	// org scope when it names an organization; user-owned repos have none.
	owner, _, _ := strings.Cut(repoFullName, "/")
	if org := s.store.OrgsByLogin[owner]; org != nil {
		for name, sec := range s.store.OrgSecrets[org.Login] {
			if orgItemVisibleToRepo(sec.Visibility, sec.SelectedRepoIDs, repo) {
				secrets[name] = sec.Value
			}
		}
		for name, v := range s.store.OrgVariables[org.Login] {
			if orgItemVisibleToRepo(v.Visibility, v.SelectedRepoIDs, repo) {
				vars[name] = v.Value
			}
		}
	}

	// Repository scope.
	for name, sec := range s.store.RepoSecrets[repoFullName] {
		secrets[name] = sec.Value
	}
	for name, v := range s.store.RepoVariables[repoFullName] {
		vars[name] = v.Value
	}

	// Environment scope (highest precedence).
	if envName != "" {
		key := envScopeKey(repoFullName, envName)
		for name, sec := range s.store.EnvSecrets[key] {
			secrets[name] = sec.Value
		}
		for name, v := range s.store.EnvVariables[key] {
			vars[name] = v.Value
		}
	}

	return secrets, vars, nil
}
