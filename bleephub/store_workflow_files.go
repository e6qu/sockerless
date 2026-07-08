package bleephub

import (
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// WorkflowFile is the file-level workflow entity (the YAML on disk),
// distinct from the run-level [Workflow]. introduces this so
// `GET /api/v3/repos/{o}/{r}/actions/workflows` can return the
// GitHub-shape file listing — and the new dispatch endpoint can
// reconstruct the YAML to submit a fresh run.
//
// Source provenance:
//   - "submitted"  — auto-registered by handleSubmitWorkflow when the
//     YAML lands at /api/v3/bleephub/workflow.
//   - "discovered" — auto-discovered from the repo's git storage by
//     walking `.github/workflows/*.yml` at HEAD.
//
// Either source can register the same (repo, path) pair; the latter
// registration wins (so a fresh git push refreshes the cached YAML).
// The json tags shape the persisted row only — REST responses go
// through workflowFileJSON / workflowFileView, never this struct.
// Every field must round-trip: a restored row with an empty
// RepoFullName or YAML is unusable (dispatch 422s on it).
type WorkflowFile struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	State        string    `json:"state"`
	RepoFullName string    `json:"repo_full_name"`
	NodeID       string    `json:"node_id"`
	BadgeURL     string    `json:"badge_url"`
	YAML         string    `json:"yaml"`
	Source       string    `json:"source"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// stableWorkflowFileID returns the GitHub-shape int64 ID derived from
// (repo, path). FNV-1a 64-bit, sign-bit masked. Same shape as
// stableJobID (gh_actions_rest.go) so the two ID schemes don't trip
// over each other in tests.
func stableWorkflowFileID(repoFullName, path string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(repoFullName + "\x00" + path))
	return int64(h.Sum64() & 0x7fffffffffffffff)
}

// RegisterWorkflowFile creates-or-updates the WorkflowFile row keyed
// by (repo, path). Idempotent: the latest call wins on YAML/Name; the
// CreatedAt timestamp is preserved across updates so the GitHub-shape
// `created_at` field stays stable.
func (st *Store) RegisterWorkflowFile(repoFullName, path, name, yamlBody, source string) *WorkflowFile {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.WorkflowFiles == nil {
		st.WorkflowFiles = map[int64]*WorkflowFile{}
	}
	id := stableWorkflowFileID(repoFullName, path)
	now := time.Now().UTC()
	if existing, ok := st.WorkflowFiles[id]; ok {
		existing.Name = name
		existing.YAML = yamlBody
		existing.Source = source
		existing.UpdatedAt = now
		if st.persist != nil {
			st.persist.MustPut("workflow_files", strconv.FormatInt(id, 10), existing)
		}
		return existing
	}
	wf := &WorkflowFile{
		ID:           id,
		Name:         name,
		Path:         path,
		State:        "active",
		RepoFullName: repoFullName,
		NodeID:       "WF_" + path,
		YAML:         yamlBody,
		Source:       source,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	st.WorkflowFiles[id] = wf
	if st.persist != nil {
		st.persist.MustPut("workflow_files", strconv.FormatInt(id, 10), wf)
	}
	return wf
}

// GetWorkflowFile returns the WorkflowFile keyed by (repo, id).
// Returns nil if not present OR if the entry's repo doesn't match
// (stable IDs are global; the repo check guards against ID-collision
// across repos, which FNV makes unlikely but not impossible).
func (st *Store) GetWorkflowFile(repoFullName string, id int64) *WorkflowFile {
	st.mu.RLock()
	defer st.mu.RUnlock()
	wf, ok := st.WorkflowFiles[id]
	if !ok {
		return nil
	}
	if wf.RepoFullName != repoFullName {
		return nil
	}
	return wf
}

// ListWorkflowFiles returns every WorkflowFile registered for the
// given repo, ordered by ID so paginated clients see a stable list.
func (st *Store) ListWorkflowFiles(repoFullName string) []*WorkflowFile {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*WorkflowFile
	for _, wf := range st.WorkflowFiles {
		if wf.RepoFullName == repoFullName {
			out = append(out, wf)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// DiscoverWorkflowFilesFromGit walks the repo's default branch and registers
// any `.github/workflows/*.{yml,yaml}` file as a WorkflowFile with
// source="discovered". Idempotent — re-discovers on every call so a fresh push
// picks up new YAMLs.
//
// No-ops for repos that have no git storage, no default branch ref, or an empty
// tree; returns the count of files registered.
func (st *Store) DiscoverWorkflowFilesFromGit(repoFullName string) int {
	st.mu.RLock()
	storer := st.GitStorages[repoFullName]
	defaultBranch := "main"
	if repo := st.ReposByName[repoFullName]; repo != nil && repo.DefaultBranch != "" {
		defaultBranch = repo.DefaultBranch
	}
	st.mu.RUnlock()
	if storer == nil {
		return 0
	}
	repo, err := git.Open(storer, nil)
	if err != nil {
		return 0
	}
	headRef, err := repo.Head()
	if err != nil {
		headRef, err = repo.Storer.Reference(plumbing.NewBranchReferenceName(defaultBranch))
		if err != nil {
			return 0
		}
	}
	commit, err := repo.CommitObject(headRef.Hash())
	if err != nil {
		// Try resolving the ref as a tag instead.
		tag, terr := repo.TagObject(headRef.Hash())
		if terr != nil {
			return 0
		}
		commit, err = tag.Commit()
		if err != nil {
			return 0
		}
	}
	tree, err := commit.Tree()
	if err != nil {
		return 0
	}
	count := 0
	_ = tree.Files().ForEach(func(f *object.File) error {
		if !isWorkflowYAMLPath(f.Name) {
			return nil
		}
		body, _ := f.Contents()
		name := workflowDisplayName(body, f.Name)
		st.RegisterWorkflowFile(repoFullName, f.Name, name, body, "discovered")
		count++
		return nil
	})
	return count
}

func isWorkflowYAMLPath(p string) bool {
	if !strings.HasPrefix(p, ".github/workflows/") {
		return false
	}
	rest := p[len(".github/workflows/"):]
	if strings.Contains(rest, "/") {
		// Subdirectories under .github/workflows/ aren't workflow files
		// in real GitHub either.
		return false
	}
	return strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml")
}

// workflowDisplayName extracts the workflow's `name:` field from the
// YAML body (matches GitHub's display in the Actions UI). Falls back
// to the file's basename when `name:` is absent.
func workflowDisplayName(yamlBody, path string) string {
	def, err := ParseWorkflow([]byte(yamlBody))
	if err == nil && def.Name != "" {
		return def.Name
	}
	base := path
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	for _, ext := range []string{".yml", ".yaml"} {
		if strings.HasSuffix(base, ext) {
			base = strings.TrimSuffix(base, ext)
			break
		}
	}
	if base == "" {
		return "workflow"
	}
	return base
}
