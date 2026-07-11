package bleephub

import (
	"strconv"
	"strings"
)

func (s *Server) deriveDependabotAlertsForRepository(repo *Repo) {
	if repo == nil {
		return
	}
	deps := s.currentDependencies(repo.ID, "refs/heads/"+repo.DefaultBranch, "")
	if len(deps) == 0 {
		return
	}
	for _, advisory := range s.store.listGlobalAdvisories() {
		s.deriveDependabotAlertsForRepositoryAdvisory(repo, deps, advisory)
	}
}

func (s *Server) deriveDependabotAlertsForPublishedAdvisory(advisory *SecurityAdvisory) {
	if advisory == nil || advisory.PublishedAt == nil || advisory.State != "published" {
		return
	}
	s.store.mu.RLock()
	repos := make([]*Repo, 0, len(s.store.Repos))
	for _, repo := range s.store.Repos {
		repos = append(repos, repo)
	}
	s.store.mu.RUnlock()

	for _, repo := range repos {
		deps := s.currentDependencies(repo.ID, "refs/heads/"+repo.DefaultBranch, "")
		if len(deps) == 0 {
			continue
		}
		s.deriveDependabotAlertsForRepositoryAdvisory(repo, deps, advisory)
	}
}

func (s *Server) deriveDependabotAlertsForRepositoryAdvisory(repo *Repo, deps map[string]*dependencyEntry, advisory *SecurityAdvisory) {
	if advisory == nil || advisory.PublishedAt == nil || advisory.State != "published" {
		return
	}
	for _, vulnerability := range advisory.Vulnerabilities {
		for purl, dep := range deps {
			ecosystem, packageName, version := parsePurl(purl)
			if !dependabotPackageMatches(vulnerability, ecosystem, packageName) {
				continue
			}
			if !dependabotVersionInRange(version, vulnerability.VulnerableVersionRange) {
				continue
			}
			s.store.CreateDependabotAlertIfNew(repo.FullName, vulnerability.PackageName, normalizeDependabotEcosystem(vulnerability.PackageEcosystem), dep.Manifest,
				advisory.GHSAID, advisory.CVEID, advisory.Severity, advisory.Summary, advisory.Description,
				vulnerability.VulnerableVersionRange, vulnerability.FirstPatchedVersion)
		}
	}
}

func dependabotPackageMatches(v SecurityAdvisoryVulnerability, ecosystem, packageName string) bool {
	return normalizeDependabotEcosystem(v.PackageEcosystem) == normalizeDependabotEcosystem(ecosystem) &&
		strings.EqualFold(v.PackageName, packageName)
}

func normalizeDependabotEcosystem(ecosystem string) string {
	switch strings.ToLower(ecosystem) {
	case "pypi":
		return "pip"
	default:
		return strings.ToLower(ecosystem)
	}
}

func dependabotVersionInRange(version, rangeExpr string) bool {
	version = strings.TrimSpace(version)
	if version == "" || strings.TrimSpace(rangeExpr) == "" {
		return false
	}
	for _, part := range strings.Split(rangeExpr, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !dependabotVersionMatchesConstraint(version, part) {
			return false
		}
	}
	return true
}

func dependabotVersionMatchesConstraint(version, constraint string) bool {
	for _, op := range []string{"<=", ">=", "<", ">", "="} {
		if strings.HasPrefix(constraint, op) {
			want := strings.TrimSpace(strings.TrimPrefix(constraint, op))
			cmp, ok := compareDependencyVersions(version, want)
			if !ok {
				return false
			}
			switch op {
			case "<":
				return cmp < 0
			case "<=":
				return cmp <= 0
			case ">":
				return cmp > 0
			case ">=":
				return cmp >= 0
			case "=":
				return cmp == 0
			}
		}
	}
	cmp, ok := compareDependencyVersions(version, constraint)
	return ok && cmp == 0
}

func compareDependencyVersions(left, right string) (int, bool) {
	leftParts, ok := dependencyVersionParts(left)
	if !ok {
		return 0, false
	}
	rightParts, ok := dependencyVersionParts(right)
	if !ok {
		return 0, false
	}
	max := len(leftParts)
	if len(rightParts) > max {
		max = len(rightParts)
	}
	for len(leftParts) < max {
		leftParts = append(leftParts, 0)
	}
	for len(rightParts) < max {
		rightParts = append(rightParts, 0)
	}
	for i := 0; i < max; i++ {
		switch {
		case leftParts[i] < rightParts[i]:
			return -1, true
		case leftParts[i] > rightParts[i]:
			return 1, true
		}
	}
	return 0, true
}

func dependencyVersionParts(version string) ([]int, bool) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if i := strings.IndexAny(version, "-+"); i >= 0 {
		version = version[:i]
	}
	raw := strings.Split(version, ".")
	if len(raw) == 0 {
		return nil, false
	}
	parts := make([]int, 0, len(raw))
	for _, part := range raw {
		if part == "" {
			return nil, false
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		parts = append(parts, n)
	}
	return parts, true
}
