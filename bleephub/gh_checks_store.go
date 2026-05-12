package bleephub

import "time"

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
	RepoKey     string          `json:"-"`
}

// CheckRunOutput is the title/summary/text/annotations bundle attached to a CheckRun.
type CheckRunOutput struct {
	Title            string             `json:"title,omitempty"`
	Summary          string             `json:"summary,omitempty"`
	Text             string             `json:"text,omitempty"`
	AnnotationsCount int                `json:"annotations_count"`
	Annotations      []*CheckAnnotation `json:"-"`
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
	ID         int64     `json:"id"`
	NodeID     string    `json:"node_id"`
	HeadBranch string    `json:"head_branch"`
	HeadSHA    string    `json:"head_sha"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	AppID      int       `json:"app_id"`
	RepoKey    string    `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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
	now := time.Now()
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
	return s
}

// GetCheckSuite returns a suite by ID, or nil.
func (st *Store) GetCheckSuite(id int64) *CheckSuite {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.CheckSuites[id]
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
		out = append(out, s)
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
			now := time.Now()
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
		}
	}

	id := st.NextCheckRunID
	st.NextCheckRunID++
	now := time.Now()
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
	return cr
}

// GetCheckRun returns a check run by ID, or nil.
func (st *Store) GetCheckRun(id int64) *CheckRun {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.CheckRuns[id]
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
		out = append(out, cr)
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
			out = append(out, cr)
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
