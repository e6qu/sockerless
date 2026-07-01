package bleephub

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// MigrationCommon holds the fields shared by user and organization migrations.
// It is the canonical response payload for GitHub's Migration object.
type MigrationCommon struct {
	ID                   int       `json:"id"`
	NodeID               string    `json:"node_id"`
	GUID                 string    `json:"guid"`
	State                string    `json:"state"`
	Repositories         []string  `json:"repositories"`
	LockRepositories     bool      `json:"lock_repositories"`
	ExcludeMetadata      bool      `json:"exclude_metadata"`
	ExcludeGitData       bool      `json:"exclude_git_data"`
	ExcludeAttachments   bool      `json:"exclude_attachments"`
	ExcludeReleases      bool      `json:"exclude_releases"`
	ExcludeOwnerProjects bool      `json:"exclude_owner_projects"`
	OrgMetadataOnly      bool      `json:"org_metadata_only"`
	URL                  string    `json:"url"`
	HTMLURL              string    `json:"html_url"`
	ArchiveURL           string    `json:"archive_url"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	ExportedAt           time.Time `json:"exported_at"`

	// Internal state omitted from API responses.
	LockedRepos    map[string]bool `json:"-"`
	ArchiveDeleted bool            `json:"-"`
}

// UserMigration is a user-scoped GitHub migration export.
type UserMigration struct {
	MigrationCommon
	UserID int `json:"-"`
}

// OrgMigration is an organization-scoped GitHub migration export.
type OrgMigration struct {
	MigrationCommon
	OrgLogin string `json:"-"`
}

func migrationNodeID(id int) string {
	return fmt.Sprintf("M_kgDO%08d", id)
}

// userMigrationRecord is the persistence view of a UserMigration. It keeps the
// internal state fields that must survive a restart but are not exposed in the
// REST response.
type userMigrationRecord struct {
	MigrationCommon
	UserID         int             `json:"user_id"`
	LockedRepos    map[string]bool `json:"locked_repos"`
	ArchiveDeleted bool            `json:"archive_deleted"`
}

func userMigrationToRecord(m *UserMigration) *userMigrationRecord {
	mc := m.MigrationCommon
	mc.LockedRepos = nil
	mc.ArchiveDeleted = false
	return &userMigrationRecord{
		MigrationCommon: mc,
		UserID:          m.UserID,
		LockedRepos:     m.LockedRepos,
		ArchiveDeleted:  m.ArchiveDeleted,
	}
}

func recordToUserMigration(r *userMigrationRecord) *UserMigration {
	mc := r.MigrationCommon
	mc.LockedRepos = r.LockedRepos
	mc.ArchiveDeleted = r.ArchiveDeleted
	return &UserMigration{MigrationCommon: mc, UserID: r.UserID}
}

// orgMigrationRecord is the persistence view of an OrgMigration.
type orgMigrationRecord struct {
	MigrationCommon
	OrgLogin       string          `json:"org_login"`
	LockedRepos    map[string]bool `json:"locked_repos"`
	ArchiveDeleted bool            `json:"archive_deleted"`
}

func orgMigrationToRecord(m *OrgMigration) *orgMigrationRecord {
	mc := m.MigrationCommon
	mc.LockedRepos = nil
	mc.ArchiveDeleted = false
	return &orgMigrationRecord{
		MigrationCommon: mc,
		OrgLogin:        m.OrgLogin,
		LockedRepos:     m.LockedRepos,
		ArchiveDeleted:  m.ArchiveDeleted,
	}
}

func recordToOrgMigration(r *orgMigrationRecord) *OrgMigration {
	mc := r.MigrationCommon
	mc.LockedRepos = r.LockedRepos
	mc.ArchiveDeleted = r.ArchiveDeleted
	return &OrgMigration{MigrationCommon: mc, OrgLogin: r.OrgLogin}
}

// CreateUserMigration starts a new user migration export.
func (st *Store) CreateUserMigration(userID int, repos []string, lock, exMeta, exGit, exAttach, exRel, exOwnerProj, orgMetaOnly bool) *UserMigration {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now().UTC()
	id := st.NextUserMigrationID
	m := &UserMigration{
		MigrationCommon: MigrationCommon{
			ID:                   id,
			NodeID:               migrationNodeID(id),
			GUID:                 uuid.New().String(),
			State:                "exported",
			Repositories:         repos,
			LockRepositories:     lock,
			ExcludeMetadata:      exMeta,
			ExcludeGitData:       exGit,
			ExcludeAttachments:   exAttach,
			ExcludeReleases:      exRel,
			ExcludeOwnerProjects: exOwnerProj,
			OrgMetadataOnly:      orgMetaOnly,
			CreatedAt:            now,
			UpdatedAt:            now,
			ExportedAt:           now,
			LockedRepos:          map[string]bool{},
		},
		UserID: userID,
	}
	if lock {
		for _, fullName := range repos {
			if repo := st.ReposByName[fullName]; repo != nil {
				m.LockedRepos[repo.Name] = true
			}
		}
	}
	st.UserMigrations[id] = m
	st.NextUserMigrationID++
	st.persistUserMigration(m)
	return m
}

// GetUserMigration returns a user migration by ID, or nil.
func (st *Store) GetUserMigration(id int) *UserMigration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.UserMigrations[id]
}

// ListUserMigrations returns a user's migrations, newest first.
func (st *Store) ListUserMigrations(userID int) []*UserMigration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*UserMigration
	for _, m := range st.UserMigrations {
		if m.UserID == userID {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// DeleteUserMigrationArchive marks a user migration archive as deleted.
func (st *Store) DeleteUserMigrationArchive(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	m := st.UserMigrations[id]
	if m == nil {
		return false
	}
	m.ArchiveDeleted = true
	m.UpdatedAt = time.Now().UTC()
	st.persistUserMigration(m)
	return true
}

// UnlockUserMigrationRepo unlocks a single repository in a user migration.
func (st *Store) UnlockUserMigrationRepo(id int, repoName string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	m := st.UserMigrations[id]
	if m == nil || !m.LockedRepos[repoName] {
		return false
	}
	delete(m.LockedRepos, repoName)
	m.UpdatedAt = time.Now().UTC()
	st.persistUserMigration(m)
	return true
}

// IsUserMigrationRepoLocked reports whether a repository is still locked.
func (st *Store) IsUserMigrationRepoLocked(id int, repoName string) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	m := st.UserMigrations[id]
	if m == nil {
		return false
	}
	return m.LockedRepos[repoName]
}

// CreateOrgMigration starts a new organization migration export.
func (st *Store) CreateOrgMigration(orgLogin string, repos []string, lock, exMeta, exGit, exAttach, exRel, exOwnerProj, orgMetaOnly bool) *OrgMigration {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now().UTC()
	id := st.NextOrgMigrationID
	m := &OrgMigration{
		MigrationCommon: MigrationCommon{
			ID:                   id,
			NodeID:               migrationNodeID(id),
			GUID:                 uuid.New().String(),
			State:                "exported",
			Repositories:         repos,
			LockRepositories:     lock,
			ExcludeMetadata:      exMeta,
			ExcludeGitData:       exGit,
			ExcludeAttachments:   exAttach,
			ExcludeReleases:      exRel,
			ExcludeOwnerProjects: exOwnerProj,
			OrgMetadataOnly:      orgMetaOnly,
			CreatedAt:            now,
			UpdatedAt:            now,
			ExportedAt:           now,
			LockedRepos:          map[string]bool{},
		},
		OrgLogin: orgLogin,
	}
	if lock {
		for _, fullName := range repos {
			if repo := st.ReposByName[fullName]; repo != nil {
				m.LockedRepos[repo.Name] = true
			}
		}
	}
	st.OrgMigrations[id] = m
	st.NextOrgMigrationID++
	st.persistOrgMigration(m)
	return m
}

// GetOrgMigration returns an org migration by ID, or nil.
func (st *Store) GetOrgMigration(id int) *OrgMigration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.OrgMigrations[id]
}

// ListOrgMigrations returns an organization's migrations, newest first.
func (st *Store) ListOrgMigrations(orgLogin string) []*OrgMigration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*OrgMigration
	for _, m := range st.OrgMigrations {
		if m.OrgLogin == orgLogin {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// DeleteOrgMigrationArchive marks an organization migration archive as deleted.
func (st *Store) DeleteOrgMigrationArchive(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	m := st.OrgMigrations[id]
	if m == nil {
		return false
	}
	m.ArchiveDeleted = true
	m.UpdatedAt = time.Now().UTC()
	st.persistOrgMigration(m)
	return true
}

// UnlockOrgMigrationRepo unlocks a single repository in an org migration.
func (st *Store) UnlockOrgMigrationRepo(id int, repoName string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	m := st.OrgMigrations[id]
	if m == nil || !m.LockedRepos[repoName] {
		return false
	}
	delete(m.LockedRepos, repoName)
	m.UpdatedAt = time.Now().UTC()
	st.persistOrgMigration(m)
	return true
}

// IsOrgMigrationRepoLocked reports whether a repository is still locked.
func (st *Store) IsOrgMigrationRepoLocked(id int, repoName string) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	m := st.OrgMigrations[id]
	if m == nil {
		return false
	}
	return m.LockedRepos[repoName]
}

func (st *Store) persistUserMigration(m *UserMigration) {
	if st.persist != nil {
		st.persist.MustPut("user_migrations", strconv.Itoa(m.ID), userMigrationToRecord(m))
	}
}

func (st *Store) persistOrgMigration(m *OrgMigration) {
	if st.persist != nil {
		st.persist.MustPut("org_migrations", strconv.Itoa(m.ID), orgMigrationToRecord(m))
	}
}
