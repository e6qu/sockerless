package bleephub

import (
	"strconv"
	"time"
)

// CheckRun is a single check execution attached to a git ref (commit SHA).
// Mirrors GitHub's Checks API shape. Created and updated by a GitHub App's
// installation token; visible to anyone with read access to the repo.
type CheckRun struct {
	ID          int64           `json:"id"`
	NodeID      string          `json:"node_id"`
	HeadSHA     string          `json:"head_sha"`
	ExternalID  string          `json:"external_id"`
	Name        string          `json:"name"`
	Status      string          `json:"status"`     // queued, in_progress, completed
	Conclusion  string          `json:"conclusion"` // success, failure, neutral, cancelled, skipped, timed_out, action_required, stale, startup_failure
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Output      *CheckRunOutput `json:"output,omitempty"`
	DetailsURL  string          `json:"details_url"`
	AppID       int             `json:"app_id"`
	SuiteID     int64           `json:"check_suite_id"`
	// RepoKey carries a real json name so persistence round-trips it
	// (post-reload commit lookups match on it). Client responses never
	// marshal this struct — checkRunToJSON emits an explicit map.
	RepoKey string `json:"repo_key"`
}

// CheckRunOutput is the title/summary/text/annotations bundle attached to a CheckRun.
type CheckRunOutput struct {
	Title            string             `json:"title,omitempty"`
	Summary          string             `json:"summary,omitempty"`
	Text             string             `json:"text,omitempty"`
	AnnotationsCount int                `json:"annotations_count"`
	Annotations      []*CheckAnnotation `json:"annotations"` // persisted with the run; rendered only via the annotations list endpoint
	Images           []*CheckImage      `json:"images,omitempty"`
}

// CheckAnnotation is a per-line annotation attached to a CheckRun's output.
type CheckAnnotation struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	StartColumn     *int   `json:"start_column,omitempty"`
	EndColumn       *int   `json:"end_column,omitempty"`
	AnnotationLevel string `json:"annotation_level"` // notice, warning, failure
	Message         string `json:"message"`
	Title           string `json:"title,omitempty"`
	RawDetails      string `json:"raw_details,omitempty"`
}

// CheckImage attaches an image (e.g. coverage report screenshot) to a CheckRun.
type CheckImage struct {
	Alt      string `json:"alt"`
	ImageURL string `json:"image_url"`
	Caption  string `json:"caption,omitempty"`
}

// CheckSuite groups CheckRuns by (repo, head_sha, app).
type CheckSuite struct {
	ID                   int64  `json:"id"`
	NodeID               string `json:"node_id"`
	HeadBranch           string `json:"head_branch"`
	HeadSHA              string `json:"head_sha"`
	Status               string `json:"status"`
	Conclusion           string `json:"conclusion"`
	AppID                int    `json:"app_id"`
	WorkflowRunID        int    `json:"workflow_run_id,omitempty"`
	WorkflowRunBackendID string `json:"workflow_run_backend_id,omitempty"`
	WorkflowName         string `json:"workflow_name,omitempty"`
	WorkflowFileID       int64  `json:"workflow_file_id,omitempty"`
	WorkflowFilePath     string `json:"workflow_file_path,omitempty"`
	// RepoKey carries a real json name so persistence round-trips it;
	// client responses go through checkSuiteToJSON (explicit map).
	RepoKey   string    `json:"repo_key"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CheckSuitePref controls auto-trigger of CheckSuites for a (repo, app) pair.
type CheckSuitePref struct {
	AppID   int  `json:"app_id"`
	Setting bool `json:"setting"`
}

// CreateCheckSuite creates or returns an existing suite for the (repoKey, headSHA, appID) tuple.
func (st *Store) CreateCheckSuite(repoKey, headBranch, headSHA string, appID int) *CheckSuite {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, s := range st.CheckSuites {
		if s.RepoKey == repoKey && s.HeadSHA == headSHA && s.AppID == appID {
			return s
		}
	}
	id := st.NextCheckSuiteID
	st.NextCheckSuiteID++
	now := time.Now().UTC()
	s := &CheckSuite{
		ID:         id,
		NodeID:     "CS_" + headSHA[:min(8, len(headSHA))],
		HeadBranch: headBranch,
		HeadSHA:    headSHA,
		Status:     "queued",
		AppID:      appID,
		RepoKey:    repoKey,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	st.CheckSuites[id] = s
	if st.persist != nil {
		st.persist.MustPut("check_suites", strconv.FormatInt(id, 10), s)
	}
	return s
}

// GetCheckSuite returns a suite by ID, or nil.
func (st *Store) GetCheckSuite(id int64) *CheckSuite {
	st.mu.RLock()
	defer st.mu.RUnlock()
	suite := st.CheckSuites[id]
	if suite == nil {
		return nil
	}
	// Return a snapshot, not the live pointer: the engine mutates the stored
	// suite's fields under the lock, so a caller that reads fields off the
	// returned pointer after RUnlock would race those writes.
	cp := *suite
	return &cp
}

// ListCheckSuitesForCommit returns every suite recorded against (repoKey, headSHA),
// optionally filtered by appID (0 = no filter).
func (st *Store) ListCheckSuitesForCommit(repoKey, headSHA string, appID int) []*CheckSuite {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := []*CheckSuite{}
	for _, s := range st.CheckSuites {
		if s.RepoKey != repoKey || s.HeadSHA != headSHA {
			continue
		}
		if appID > 0 && s.AppID != appID {
			continue
		}
		// Snapshot, not the live pointer — the engine mutates suite fields
		// under the lock; callers read fields off the result after RUnlock.
		cp := *s
		out = append(out, &cp)
	}
	return out
}

// CreateCheckRun inserts a new check run. If suiteID is 0, finds-or-creates a suite for the SHA.
func (st *Store) CreateCheckRun(repoKey, headSHA, name string, appID int, suiteID int64) *CheckRun {
	st.mu.Lock()
	defer st.mu.Unlock()

	if suiteID == 0 {
		// inline suite create (mirror logic from CreateCheckSuite without re-locking)
		for _, s := range st.CheckSuites {
			if s.RepoKey == repoKey && s.HeadSHA == headSHA && s.AppID == appID {
				suiteID = s.ID
				break
			}
		}
		if suiteID == 0 {
			suiteID = st.NextCheckSuiteID
			st.NextCheckSuiteID++
			now := time.Now().UTC()
			st.CheckSuites[suiteID] = &CheckSuite{
				ID:        suiteID,
				NodeID:    "CS_" + headSHA[:min(8, len(headSHA))],
				HeadSHA:   headSHA,
				Status:    "queued",
				AppID:     appID,
				RepoKey:   repoKey,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if st.persist != nil {
				st.persist.MustPut("check_suites", strconv.FormatInt(suiteID, 10), st.CheckSuites[suiteID])
			}
		}
	}

	id := st.NextCheckRunID
	st.NextCheckRunID++
	now := time.Now().UTC()
	cr := &CheckRun{
		ID:        id,
		NodeID:    "CR_" + headSHA[:min(8, len(headSHA))],
		HeadSHA:   headSHA,
		Name:      name,
		Status:    "queued",
		StartedAt: now,
		AppID:     appID,
		SuiteID:   suiteID,
		RepoKey:   repoKey,
	}
	st.CheckRuns[id] = cr
	if st.persist != nil {
		st.persist.MustPut("check_runs", strconv.FormatInt(id, 10), cr)
	}
	return cr
}

// GetCheckRun returns a check run by ID, or nil.
func (st *Store) GetCheckRun(id int64) *CheckRun {
	st.mu.RLock()
	defer st.mu.RUnlock()
	cr := st.CheckRuns[id]
	if cr == nil {
		return nil
	}
	// Snapshot, not the live pointer — see GetCheckSuite.
	cp := *cr
	return &cp
}

// UpdateCheckSuite mutates a check suite via callback. Returns false if not found.
func (st *Store) UpdateCheckSuite(id int64, fn func(*CheckSuite)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	suite := st.CheckSuites[id]
	if suite == nil {
		return false
	}
	fn(suite)
	suite.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("check_suites", strconv.FormatInt(id, 10), suite)
	}
	return true
}

// UpdateCheckRun mutates a check run via callback. Returns false if not found.
func (st *Store) UpdateCheckRun(id int64, fn func(*CheckRun)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	cr := st.CheckRuns[id]
	if cr == nil {
		return false
	}
	fn(cr)
	if st.persist != nil {
		st.persist.MustPut("check_runs", strconv.FormatInt(id, 10), cr)
	}
	return true
}

// ListCheckRunsForCommit returns every CheckRun for (repoKey, headSHA), optional filters.
func (st *Store) ListCheckRunsForCommit(repoKey, headSHA, status, conclusion string, appID int) []*CheckRun {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := []*CheckRun{}
	for _, cr := range st.CheckRuns {
		if cr.RepoKey != repoKey || cr.HeadSHA != headSHA {
			continue
		}
		if status != "" && cr.Status != status {
			continue
		}
		if conclusion != "" && cr.Conclusion != conclusion {
			continue
		}
		if appID > 0 && cr.AppID != appID {
			continue
		}
		// Snapshot, not the live pointer — see ListCheckSuitesForCommit.
		cp := *cr
		out = append(out, &cp)
	}
	return out
}

// ListCheckRunsForSuite returns every CheckRun in a suite.
func (st *Store) ListCheckRunsForSuite(suiteID int64) []*CheckRun {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := []*CheckRun{}
	for _, cr := range st.CheckRuns {
		if cr.SuiteID == suiteID {
			// Snapshot, not the live pointer — see ListCheckSuitesForCommit.
			cp := *cr
			out = append(out, &cp)
		}
	}
	return out
}

// SetCheckSuitePreferences replaces the per-app auto-trigger flags for a repo.
func (st *Store) SetCheckSuitePreferences(repoKey string, prefs []*CheckSuitePref) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.CheckSuitePrefs == nil {
		st.CheckSuitePrefs = make(map[string][]*CheckSuitePref)
	}
	st.CheckSuitePrefs[repoKey] = append([]*CheckSuitePref(nil), prefs...)
	if st.persist != nil {
		st.persist.MustPut("check_suite_prefs", repoKey, st.CheckSuitePrefs[repoKey])
	}
}

// GetCheckSuitePreferences returns the configured auto-trigger flags, or empty.
func (st *Store) GetCheckSuitePreferences(repoKey string) []*CheckSuitePref {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.CheckSuitePrefs[repoKey]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
