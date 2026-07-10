package bleephub

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

// Codespace represents a GitHub Codespace backed by a local Docker container.
type Codespace struct {
	ID                     int       `json:"id"`
	Name                   string    `json:"name"`
	OwnerLogin             string    `json:"owner_login"`
	RepoKey                string    `json:"repo_key,omitempty"`
	GitRef                 string    `json:"git_ref"`
	MachineName            string    `json:"machine_name"`
	MachineDisplayName     string    `json:"machine_display_name"`
	MachineType            string    `json:"machine_type"`
	DisplayName            string    `json:"display_name"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
	LastUsedAt             time.Time `json:"last_used_at"`
	State                  string    `json:"state"`
	ContainerID            string    `json:"container_id"`
	ContainerName          string    `json:"container_name"`
	DevcontainerPath       string    `json:"devcontainer_path"`
	ImageName              string    `json:"image_name"`
	RetentionPeriodMinutes int       `json:"retention_period_minutes"`
	WorkspaceMount         string    `json:"workspace_mount,omitempty"`
	// LatestExport records the most recent export of this codespace.
	LatestExport *CodespaceExport `json:"latest_export,omitempty"`
}

// CodespaceExport captures one export of a codespace to a repository
// branch. GitHub addresses export details with the id "latest".
type CodespaceExport struct {
	ID          string    `json:"id"`
	State       string    `json:"state"`
	Branch      string    `json:"branch"`
	SHA         string    `json:"sha"`
	CompletedAt time.Time `json:"completed_at"`
}

// CodespaceSecret is a user/repo/org-level Codespaces secret.
type CodespaceSecret struct {
	Name            string    `json:"name"`
	Key             string    `json:"key"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	SelectedRepoIDs []int     `json:"selected_repository_ids,omitempty"`
	Visibility      string    `json:"visibility,omitempty"`
}

// codespaceMachine describes a machine type offered for Codespaces,
// including the per-machine resources the codespace-machine schema
// reports (cpus, memory_in_bytes, storage_in_bytes).
type codespaceMachine struct {
	Name         string
	DisplayName  string
	Type         string // "standard" or "premium"
	CPUs         int
	MemoryBytes  int64
	StorageBytes int64
}

const codespaceGiB = int64(1) << 30

var codespaceMachines = []codespaceMachine{
	{Name: "basicLinux32", DisplayName: "2 cores, 4 GB RAM, 32 GB storage", Type: "standard", CPUs: 2, MemoryBytes: 4 * codespaceGiB, StorageBytes: 32 * codespaceGiB},
	{Name: "standardLinux32", DisplayName: "4 cores, 8 GB RAM, 32 GB storage", Type: "standard", CPUs: 4, MemoryBytes: 8 * codespaceGiB, StorageBytes: 32 * codespaceGiB},
	{Name: "premiumLinux64", DisplayName: "8 cores, 16 GB RAM, 64 GB storage", Type: "premium", CPUs: 8, MemoryBytes: 16 * codespaceGiB, StorageBytes: 64 * codespaceGiB},
	{Name: "largeLinux64", DisplayName: "16 cores, 32 GB RAM, 64 GB storage", Type: "premium", CPUs: 16, MemoryBytes: 32 * codespaceGiB, StorageBytes: 64 * codespaceGiB},
}

// codespaceMachineByName resolves a catalog machine by name; unknown
// names fall back to the default machine (CreateCodespace snaps every
// codespace onto a catalog entry, so lookups always resolve).
func codespaceMachineByName(name string) codespaceMachine {
	for _, m := range codespaceMachines {
		if m.Name == name {
			return m
		}
	}
	return codespaceDefaultMachine()
}

const (
	codespaceContainerPrefix = "bleephub-codespace-"
	codespaceDefaultImage    = "mcr.microsoft.com/devcontainers/universal:latest"
)

// persistCodespaceLocked writes a codespace row through to persistence.
// Caller must hold st.mu.
func (st *Store) persistCodespaceLocked(cs *Codespace) {
	if st.persist != nil {
		st.persist.MustPut("codespaces", strconv.Itoa(cs.ID), cs)
	}
}

// persistCodespaceSecretScopeLocked writes a whole secret scope through
// to persistence — the scope map is the bucket row, mirroring the
// Dependabot secret buckets. Caller must hold st.mu.
func (st *Store) persistCodespaceSecretScopeLocked(scope string) {
	if st.persist == nil {
		return
	}
	m := st.CodespaceSecrets[scope]
	if len(m) == 0 {
		st.persist.MustDelete("codespace_secrets", scope)
		return
	}
	st.persist.MustPut("codespace_secrets", scope, m)
}

// CreateCodespace records a new codespace and starts its backing container.
func (st *Store) CreateCodespace(ownerLogin, repoKey, gitRef, machineName, displayName string) (*Codespace, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	name, err := generateCodespaceName(repoKey)
	if err != nil {
		return nil, err
	}
	machine := codespaceDefaultMachine()
	for _, m := range codespaceMachines {
		if m.Name == machineName {
			machine = m
			break
		}
	}
	if displayName == "" {
		displayName = name
	}

	image := codespaceDefaultImage
	devcontainerPath := ""
	if repoKey != "" {
		if img, path, ok := st.resolveDevcontainerLocked(repoKey, gitRef); ok {
			image = img
			devcontainerPath = path
		}
	}

	cs := &Codespace{
		ID:                 st.NextCodespaceID,
		Name:               name,
		OwnerLogin:         ownerLogin,
		RepoKey:            repoKey,
		GitRef:             gitRef,
		MachineName:        machine.Name,
		MachineDisplayName: machine.DisplayName,
		MachineType:        machine.Type,
		DisplayName:        displayName,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
		LastUsedAt:         time.Now().UTC(),
		State:              "Creating",
		ImageName:          image,
		DevcontainerPath:   devcontainerPath,
	}

	repoDir, cleanup, err := st.prepareWorkspaceLocked(repoKey, gitRef)
	if err != nil {
		return nil, fmt.Errorf("prepare workspace: %w", err)
	}
	cs.WorkspaceMount = repoDir

	containerName := codespaceContainerName(cs.Name)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	containerID, err := dockerRunCodespace(ctx, containerName, image, repoDir, repoNameFromRepoKey(repoKey))
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("docker run: %w", err)
	}
	cs.ContainerID = containerID
	cs.ContainerName = containerName
	cs.State = dockerStateToCodespaceState(containerID)
	st.Codespaces[cs.ID] = cs
	st.CodespacesByName[cs.Name] = cs
	st.NextCodespaceID++
	st.persistCodespaceLocked(cs)
	return cs, nil
}

// GetCodespace returns a codespace by ID.
func (st *Store) GetCodespace(id int) *Codespace {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Codespaces[id]
}

// GetCodespaceByName returns a codespace by its unique name.
func (st *Store) GetCodespaceByName(name string) *Codespace {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.CodespacesByName[name]
}

// ListCodespacesByOwner returns all codespaces owned by a user.
func (st *Store) ListCodespacesByOwner(ownerLogin string) []*Codespace {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*Codespace
	for _, cs := range st.Codespaces {
		if cs.OwnerLogin == ownerLogin {
			out = append(out, cs)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// ListCodespacesByRepo returns all codespaces for a repository.
func (st *Store) ListCodespacesByRepo(repoKey string) []*Codespace {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*Codespace
	for _, cs := range st.Codespaces {
		if cs.RepoKey == repoKey {
			out = append(out, cs)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// DeleteCodespace stops and removes the backing container and deletes the record.
func (st *Store) DeleteCodespace(id int) (bool, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	cs := st.Codespaces[id]
	if cs == nil {
		return false, nil
	}
	if err := st.deleteCodespaceRuntimeLocked(cs); err != nil {
		return true, err
	}
	delete(st.Codespaces, id)
	delete(st.CodespacesByName, cs.Name)
	if st.persist != nil {
		st.persist.MustDelete("codespaces", strconv.Itoa(id))
	}
	return true, nil
}

func (st *Store) deleteCodespaceRuntimeLocked(cs *Codespace) error {
	if cs.ContainerID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		err := dockerRemoveContainer(ctx, cs.ContainerID)
		cancel()
		if err != nil {
			return fmt.Errorf("docker remove: %w", err)
		}
	}
	if cs.WorkspaceMount != "" && !pathIsUnderDir(cs.WorkspaceMount, os.TempDir()) {
		return fmt.Errorf("refusing to remove workspace outside temp dir: %s", cs.WorkspaceMount)
	}
	if cs.WorkspaceMount != "" {
		if err := os.RemoveAll(cs.WorkspaceMount); err != nil {
			return fmt.Errorf("remove workspace: %w", err)
		}
	}
	return nil
}

func pathIsUnderDir(path, dir string) bool {
	cleanPath := filepath.Clean(path)
	cleanDir := filepath.Clean(dir)
	if cleanPath == cleanDir {
		return true
	}
	return strings.HasPrefix(cleanPath, cleanDir+string(os.PathSeparator))
}

// UpdateCodespace updates mutable fields of a codespace.
func (st *Store) UpdateCodespace(id int, displayName, machineName string, retention int) (*Codespace, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	cs := st.Codespaces[id]
	if cs == nil {
		return nil, false
	}
	if displayName != "" {
		cs.DisplayName = displayName
	}
	if machineName != "" {
		for _, m := range codespaceMachines {
			if m.Name == machineName {
				cs.MachineName = m.Name
				cs.MachineDisplayName = m.DisplayName
				cs.MachineType = m.Type
				break
			}
		}
	}
	if retention > 0 {
		cs.RetentionPeriodMinutes = retention
	}
	cs.UpdatedAt = time.Now().UTC()
	cs.LastUsedAt = time.Now().UTC()
	st.persistCodespaceLocked(cs)
	return cs, true
}

// RefreshCodespaceState queries Docker for the current state of a codespace.
func (st *Store) RefreshCodespaceState(id int) string {
	st.mu.Lock()
	defer st.mu.Unlock()
	cs := st.Codespaces[id]
	if cs == nil {
		return ""
	}
	if cs.ContainerID == "" {
		cs.State = "Unavailable"
		st.persistCodespaceLocked(cs)
		return cs.State
	}
	cs.State = dockerStateToCodespaceState(cs.ContainerID)
	st.persistCodespaceLocked(cs)
	return cs.State
}

// SetCodespaceContainerState sets the recorded state and container ID.
func (st *Store) SetCodespaceContainerState(id int, containerID, state string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	cs := st.Codespaces[id]
	if cs == nil {
		return
	}
	cs.ContainerID = containerID
	cs.State = state
	st.persistCodespaceLocked(cs)
}

// SetCodespaceState records the observed state of a codespace; markUsed
// additionally bumps LastUsedAt (a successful start counts as use).
func (st *Store) SetCodespaceState(id int, state string, markUsed bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	cs := st.Codespaces[id]
	if cs == nil {
		return
	}
	cs.State = state
	if markUsed {
		cs.LastUsedAt = time.Now().UTC()
	}
	st.persistCodespaceLocked(cs)
}

// --- secret helpers ---

func codespaceSecretScopeKey(scope, key string) string { return scope + "\x1f" + key }

// CreateCodespaceSecret creates or updates a codespaces secret in a scope.
func (st *Store) CreateCodespaceSecret(scope, name, value, visibility string, selectedRepoIDs []int) *CodespaceSecret {
	st.mu.Lock()
	defer st.mu.Unlock()
	m := st.CodespaceSecrets[scope]
	if m == nil {
		m = make(map[string]*CodespaceSecret)
		st.CodespaceSecrets[scope] = m
	}
	now := time.Now().UTC()
	key := strings.ToUpper(name)
	if existing := m[name]; existing != nil {
		existing.UpdatedAt = now
		existing.SelectedRepoIDs = selectedRepoIDs
		existing.Visibility = visibility
		st.persistCodespaceSecretScopeLocked(scope)
		return existing
	}
	sec := &CodespaceSecret{
		Name:            name,
		Key:             key,
		CreatedAt:       now,
		UpdatedAt:       now,
		SelectedRepoIDs: selectedRepoIDs,
		Visibility:      visibility,
	}
	m[name] = sec
	st.persistCodespaceSecretScopeLocked(scope)
	return sec
}

// GetCodespaceSecret returns a secret by scope+name.
func (st *Store) GetCodespaceSecret(scope, name string) *CodespaceSecret {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if m := st.CodespaceSecrets[scope]; m != nil {
		return m[name]
	}
	return nil
}

// ListCodespaceSecrets returns secrets in a scope sorted by name.
func (st *Store) ListCodespaceSecrets(scope string) []*CodespaceSecret {
	st.mu.RLock()
	defer st.mu.RUnlock()
	m := st.CodespaceSecrets[scope]
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*CodespaceSecret, len(names))
	for i, n := range names {
		out[i] = m[n]
	}
	return out
}

// DeleteCodespaceSecret removes a secret.
func (st *Store) DeleteCodespaceSecret(scope, name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	m := st.CodespaceSecrets[scope]
	if m == nil || m[name] == nil {
		return false
	}
	delete(m, name)
	st.persistCodespaceSecretScopeLocked(scope)
	return true
}

// SetCodespaceSecretSelectedRepos updates the selected repositories for an org secret.
func (st *Store) SetCodespaceSecretSelectedRepos(scope, name string, ids []int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	m := st.CodespaceSecrets[scope]
	if m == nil || m[name] == nil {
		return false
	}
	m[name].SelectedRepoIDs = ids
	m[name].UpdatedAt = time.Now().UTC()
	st.persistCodespaceSecretScopeLocked(scope)
	return true
}

// AddCodespaceSecretSelectedRepo adds one repository to a secret's
// selected list; adding an already-selected repository is a no-op.
func (st *Store) AddCodespaceSecretSelectedRepo(scope, name string, repoID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	m := st.CodespaceSecrets[scope]
	if m == nil || m[name] == nil {
		return false
	}
	sec := m[name]
	for _, id := range sec.SelectedRepoIDs {
		if id == repoID {
			return true
		}
	}
	sec.SelectedRepoIDs = append(sec.SelectedRepoIDs, repoID)
	sec.UpdatedAt = time.Now().UTC()
	st.persistCodespaceSecretScopeLocked(scope)
	return true
}

// RemoveCodespaceSecretSelectedRepo removes one repository from a
// secret's selected list; removing an unselected repository is a no-op.
func (st *Store) RemoveCodespaceSecretSelectedRepo(scope, name string, repoID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	m := st.CodespaceSecrets[scope]
	if m == nil || m[name] == nil {
		return false
	}
	sec := m[name]
	for i, id := range sec.SelectedRepoIDs {
		if id == repoID {
			sec.SelectedRepoIDs = append(sec.SelectedRepoIDs[:i], sec.SelectedRepoIDs[i+1:]...)
			sec.UpdatedAt = time.Now().UTC()
			st.persistCodespaceSecretScopeLocked(scope)
			return true
		}
	}
	return true
}

// ─── export + publish ───────────────────────────────────────────────────

// Codespace export / publish failure modes surfaced to handlers.
var (
	errCodespaceNotFound     = fmt.Errorf("codespace not found")
	errCodespaceNoRepository = fmt.Errorf("codespace has no repository")
	errCodespacePublished    = fmt.Errorf("codespace already has a repository")
	errRepoNameTaken         = fmt.Errorf("repository name already exists")
)

// ExportCodespace exports the codespace's current git ref to a new
// branch (codespace-<name>) in its repository — the same state
// transition GitHub performs when exporting unpushed codespace changes —
// and records the export details under the id "latest".
func (st *Store) ExportCodespace(id int) (*CodespaceExport, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	cs := st.Codespaces[id]
	if cs == nil {
		return nil, errCodespaceNotFound
	}
	if cs.RepoKey == "" {
		return nil, errCodespaceNoRepository
	}
	stor := st.GitStorages[cs.RepoKey]
	if stor == nil {
		return nil, fmt.Errorf("git storage not found for %s", cs.RepoKey)
	}
	refName := cs.GitRef
	if refName == "" {
		if repo := st.ReposByName[cs.RepoKey]; repo != nil {
			refName = repo.DefaultBranch
		} else {
			refName = "main"
		}
	}
	ref, err := stor.Reference(plumbing.NewBranchReferenceName(refName))
	if err != nil {
		return nil, fmt.Errorf("resolve ref %s: %w", refName, err)
	}
	branch := "codespace-" + cs.Name
	if err := stor.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), ref.Hash())); err != nil {
		return nil, fmt.Errorf("create branch %s: %w", branch, err)
	}
	export := &CodespaceExport{
		ID:          "latest",
		State:       "succeeded",
		Branch:      branch,
		SHA:         ref.Hash().String(),
		CompletedAt: time.Now().UTC(),
	}
	cs.LatestExport = export
	cs.UpdatedAt = time.Now().UTC()
	st.persistCodespaceLocked(cs)
	return export, nil
}

// PublishCodespace creates a repository for an unpublished codespace and
// associates the codespace with it.
func (st *Store) PublishCodespace(id int, owner *User, name string, private bool) (*Codespace, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	cs := st.Codespaces[id]
	if cs == nil {
		return nil, errCodespaceNotFound
	}
	if cs.RepoKey != "" {
		return nil, errCodespacePublished
	}
	if name == "" {
		name = cs.Name
	}
	repo := st.createRepoLocked(owner.Login+"/"+name, name, "", private, owner.ID, "User", owner)
	if repo == nil {
		return nil, errRepoNameTaken
	}
	cs.RepoKey = repo.FullName
	cs.UpdatedAt = time.Now().UTC()
	st.persistCodespaceLocked(cs)
	return cs, nil
}

// --- internal helpers ---

func codespaceDefaultMachine() codespaceMachine {
	return codespaceMachines[1]
}

func generateCodespaceName(repoKey string) (string, error) {
	return generateCodespaceNameWithReader(repoKey, rand.Reader)
}

func generateCodespaceNameWithReader(repoKey string, random io.Reader) (string, error) {
	b := make([]byte, 4)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", fmt.Errorf("generate codespace name: read random bytes: %w", err)
	}
	suffix := fmt.Sprintf("%07s", fmt.Sprintf("%x", b))[:7]
	repoName := repoNameFromRepoKey(repoKey)
	if repoName == "" {
		return "github-" + suffix, nil
	}
	return fmt.Sprintf("github-%s-%s", repoName, suffix), nil
}

func repoNameFromRepoKey(repoKey string) string {
	_, repo, ok := splitRepoFullName(repoKey)
	if !ok {
		return ""
	}
	return repo
}

func codespaceContainerName(codespaceName string) string {
	return codespaceContainerPrefix + codespaceName
}

func dockerStateToCodespaceState(containerID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	status, err := dockerContainerStatus(ctx, containerID)
	if err != nil {
		return "Unavailable"
	}
	switch status {
	case "running":
		return "Available"
	case "created", "paused":
		return "Shutdown"
	case "exited", "dead":
		return "Shutdown"
	default:
		return "Unavailable"
	}
}

// resolveDevcontainerLocked reads .devcontainer/devcontainer.json from the
// repo's default branch (or gitRef) and extracts the image if present.
// Caller must hold st.mu (reads GitStorages).
func (st *Store) resolveDevcontainerLocked(repoKey, gitRef string) (image, path string, ok bool) {
	stor := st.GitStorages[repoKey]
	if stor == nil {
		return "", "", false
	}
	if gitRef == "" {
		if repo := st.ReposByName[repoKey]; repo != nil {
			gitRef = repo.DefaultBranch
		} else {
			gitRef = "main"
		}
	}

	for _, p := range []string{".devcontainer/devcontainer.json", ".devcontainer.json"} {
		data, err := readGitFile(stor, gitRef, p)
		if err != nil || len(data) == 0 {
			continue
		}
		var cfg struct {
			Image string `json:"image"`
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}
		if cfg.Image != "" {
			return cfg.Image, p, true
		}
	}
	return "", "", false
}

// prepareWorkspaceLocked returns a host directory that can be mounted as
// /workspaces/<repo>. For filesystem-backed git storage it returns the git
// directory directly; for in-memory storage it exports the ref into a temp dir.
// The returned cleanup func removes any temp directory created.
func (st *Store) prepareWorkspaceLocked(repoKey, gitRef string) (dir string, cleanup func(), err error) {
	cleanup = func() {}
	if repoKey == "" {
		return "", cleanup, nil
	}

	repo := st.ReposByName[repoKey]
	if repo == nil {
		return "", cleanup, fmt.Errorf("repo not found")
	}
	if gitRef == "" {
		gitRef = repo.DefaultBranch
	}

	if GitDataDir() != "" {
		dir := filepath.Join(GitDataDir(), filepath.FromSlash(repoKey))
		if _, err := os.Stat(dir); err == nil {
			return dir, cleanup, nil
		}
	}

	stor := st.GitStorages[repoKey]
	if stor == nil {
		return "", cleanup, fmt.Errorf("git storage not found")
	}

	tmpDir, err := os.MkdirTemp("", "bleephub-codespace-"+repo.Name+"-*")
	if err != nil {
		return "", cleanup, fmt.Errorf("mkdirtemp: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	if err := exportGitRef(stor, gitRef, tmpDir); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("export git ref: %w", err)
	}
	return tmpDir, cleanup, nil
}

func readGitFile(stor gitStorage.Storer, refName, path string) ([]byte, error) {
	branchRef := plumbing.NewBranchReferenceName(refName)
	ref, err := stor.Reference(branchRef)
	if err != nil {
		return nil, err
	}
	commit, err := object.GetCommit(stor, ref.Hash())
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	entry, err := tree.FindEntry(path)
	if err != nil {
		return nil, err
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

func exportGitRef(stor gitStorage.Storer, refName, dst string) error {
	branchRef := plumbing.NewBranchReferenceName(refName)
	ref, err := stor.Reference(branchRef)
	if err != nil {
		return err
	}
	commit, err := object.GetCommit(stor, ref.Hash())
	if err != nil {
		return err
	}
	tree, err := commit.Tree()
	if err != nil {
		return err
	}
	return tree.Files().ForEach(func(f *object.File) error {
		mode, err := f.Mode.ToOSFileMode()
		if err != nil {
			return err
		}
		full := filepath.Join(dst, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		content, err := f.Contents()
		if err != nil {
			return err
		}
		return os.WriteFile(full, []byte(content), mode)
	})
}

// --- docker helpers ---

func dockerRunCodespace(ctx context.Context, name, image, repoDir, repoName string) (string, error) {
	if repoName == "" {
		repoName = "workspace"
	}
	if err := ensureDockerImage(ctx, image); err != nil {
		return "", fmt.Errorf("pull image %q: %w", image, err)
	}

	args := []string{
		"run", "-d",
		"--name", name,
		"--hostname", name,
	}
	if repoDir != "" {
		args = append(args, "-v", repoDir+":/workspaces/"+repoName)
	}
	args = append(args, image, "sleep", "1000000")

	out, err := runDockerCLI(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func dockerStartContainer(ctx context.Context, id string) error {
	out, err := runDockerCLI(ctx, "start", id)
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}

func dockerStopContainer(ctx context.Context, id string) error {
	out, err := runDockerCLI(ctx, "stop", "--time", "30", id)
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}

func dockerRemoveContainer(ctx context.Context, id string) error {
	out, err := runDockerCLI(ctx, "rm", "-f", "-v", id)
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}

func dockerContainerStatus(ctx context.Context, id string) (string, error) {
	out, err := runDockerCLI(ctx, "inspect", "-f", "{{.State.Status}}", id)
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func ensureDockerImage(ctx context.Context, image string) error {
	out, err := runDockerCLI(ctx, "image", "inspect", image)
	if err == nil && len(out) > 0 {
		return nil
	}
	out, err = runDockerCLI(ctx, "pull", image)
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}

func runDockerCLI(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	return cmd.CombinedOutput()
}
