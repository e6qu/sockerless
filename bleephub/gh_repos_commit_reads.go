package bleephub

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

// --- Single commit ---

func (s *Server) handleGetSingleCommit(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	stor := s.gitStorageForRepo(repo)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	hash, err := resolveGitRef(stor, strings.Trim(r.PathValue("ref"), "/"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	commit, err := object.GetCommit(stor, hash)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	base := s.baseURL(r)
	out := commitToJSON(commit, repo, base)
	out["author"] = s.commitSignatureUser(commit.Author)
	out["committer"] = s.commitSignatureUser(commit.Committer)

	files, additions, deletions, err := commitDiffEntries(commit, repo, base)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "diff computation failed")
		return
	}
	out["files"] = files
	out["stats"] = map[string]interface{}{
		"additions": additions,
		"deletions": deletions,
		"total":     additions + deletions,
	}
	writeJSON(w, http.StatusOK, out)
}

// commitSignatureUser resolves a git signature to a real bleephub account by
// email or login. Unresolvable signatures are null, as on real GitHub for
// commits authored under an email no account owns.
func (s *Server) commitSignatureUser(sig object.Signature) interface{} {
	if u := s.store.ResolveUserBySignature(sig.Name, sig.Email); u != nil {
		return userToJSON(u)
	}
	return nil
}

// commitDiffEntries computes the diff-entry list (with per-file patch text
// and addition/deletion counts) for a commit against its first parent, or
// against the empty tree for a root commit.
func commitDiffEntries(commit *object.Commit, repo *Repo, baseURL string) ([]map[string]interface{}, int, int, error) {
	parentTree := &object.Tree{}
	if commit.NumParents() > 0 {
		parent, err := commit.Parent(0)
		if err != nil {
			return nil, 0, 0, err
		}
		parentTree, err = parent.Tree()
		if err != nil {
			return nil, 0, 0, err
		}
	}
	headTree, err := commit.Tree()
	if err != nil {
		return nil, 0, 0, err
	}
	changes, err := object.DiffTree(parentTree, headTree)
	if err != nil {
		return nil, 0, 0, err
	}

	files := []map[string]interface{}{}
	totalAdd, totalDel := 0, 0
	for _, ch := range changes {
		status := "modified"
		switch {
		case ch.From.TreeEntry.Mode == 0:
			status = "added"
		case ch.To.TreeEntry.Mode == 0:
			status = "removed"
		case ch.From.TreeEntry.Hash == ch.To.TreeEntry.Hash:
			continue
		}

		name := ch.To.Name
		sha := ch.To.TreeEntry.Hash.String()
		if status == "removed" {
			name = ch.From.Name
			sha = ch.From.TreeEntry.Hash.String()
		}

		adds, dels := 0, 0
		var patchText string
		if patch, err := ch.Patch(); err == nil {
			stats := patch.Stats()
			if len(stats) > 0 {
				adds, dels = stats[0].Addition, stats[0].Deletion
			}
			full := patch.String()
			if idx := strings.Index(full, "\n@@"); idx >= 0 {
				patchText = strings.TrimSuffix(full[idx+1:], "\n")
			}
		}
		totalAdd += adds
		totalDel += dels

		entry := map[string]interface{}{
			"sha":          sha,
			"filename":     name,
			"status":       status,
			"additions":    adds,
			"deletions":    dels,
			"changes":      adds + dels,
			"blob_url":     baseURL + "/" + repo.FullName + "/blob/" + commit.Hash.String() + "/" + name,
			"raw_url":      baseURL + "/" + repo.FullName + "/raw/" + commit.Hash.String() + "/" + name,
			"contents_url": baseURL + "/api/v3/repos/" + repo.FullName + "/contents/" + name + "?ref=" + commit.Hash.String(),
		}
		if patchText != "" {
			entry["patch"] = patchText
		}
		files = append(files, entry)
	}
	sort.Slice(files, func(i, j int) bool {
		fi, _ := files[i]["filename"].(string)
		fj, _ := files[j]["filename"].(string)
		return fi < fj
	})
	return files, totalAdd, totalDel, nil
}

// --- Pull requests containing a commit ---

func (s *Server) handleListPullsForCommit(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	stor := s.gitStorageForRepo(repo)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	sha := r.PathValue("commit_sha")
	hash, err := resolveGitRef(stor, sha)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	base := s.baseURL(r)
	out := []map[string]interface{}{}
	for _, pr := range s.store.ListPullRequests(repo.ID, "all") {
		if pullRequestContainsCommit(stor, pr, hash) {
			out = append(out, pullRequestSimpleJSON(pr, s.store, base, repo.FullName))
		}
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

// pullRequestContainsCommit reports whether the commit is among the commits
// the pull request introduces (reachable from its head branch but not from
// its base branch, resolved live from the git storage).
func pullRequestContainsCommit(stor gitStorage.Storer, pr *PullRequest, hash plumbing.Hash) bool {
	headHash, err := resolveGitRef(stor, pr.HeadRefName)
	if err != nil {
		return false
	}
	baseHash, err := resolveGitRef(stor, pr.BaseRefName)
	if err != nil {
		return false
	}
	commits, err := commitsBetween(stor, baseHash, headHash)
	if err != nil {
		return false
	}
	for _, c := range commits {
		if c.Hash == hash {
			return true
		}
	}
	return false
}

// --- Branches where a commit is head ---

func (s *Server) handleListBranchesWhereHead(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	stor := s.gitStorageForRepo(repo)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	hash, err := resolveGitRef(stor, r.PathValue("commit_sha"))
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "No commit found for SHA: "+r.PathValue("commit_sha"))
		return
	}

	base := s.baseURL(r)
	out := []map[string]interface{}{}
	refs, err := stor.IterReferences()
	if err == nil {
		_ = refs.ForEach(func(ref *plumbing.Reference) error {
			if !ref.Name().IsBranch() || ref.Hash() != hash {
				return nil
			}
			out = append(out, map[string]interface{}{
				"name":      ref.Name().Short(),
				"protected": false,
				"commit": map[string]interface{}{
					"sha": ref.Hash().String(),
					"url": base + "/api/v3/repos/" + repo.FullName + "/commits/" + ref.Hash().String(),
				},
			})
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool {
		ni, _ := out[i]["name"].(string)
		nj, _ := out[j]["name"].(string)
		return ni < nj
	})
	writeJSON(w, http.StatusOK, out)
}

// --- Contributors ---

// contributorBucket aggregates one commit-author identity.
type contributorBucket struct {
	name          string
	email         string
	user          *User
	contributions int
	firstSeen     int // insertion order for stable output
}

// aggregateContributors walks the default branch history and groups commits
// by author identity, resolving identities to real accounts by email or
// login.
func (s *Server) aggregateContributors(repo *Repo) ([]*contributorBucket, bool) {
	commits, ok := s.defaultBranchCommits(repo)
	if !ok {
		return nil, false
	}
	byKey := map[string]*contributorBucket{}
	var order []*contributorBucket
	for _, c := range commits {
		key := strings.ToLower(c.Author.Email)
		if key == "" {
			key = c.Author.Name
		}
		b := byKey[key]
		if b == nil {
			b = &contributorBucket{name: c.Author.Name, email: c.Author.Email, firstSeen: len(order)}
			b.user = s.store.ResolveUserBySignature(c.Author.Name, c.Author.Email)
			byKey[key] = b
			order = append(order, b)
		}
		b.contributions++
	}
	return order, true
}

func (s *Server) handleListRepoContributors(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	buckets, ok := s.aggregateContributors(repo)
	if !ok || len(buckets) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	anon := r.URL.Query().Get("anon")
	includeAnon := anon == "1" || anon == "true"

	// Merge buckets that resolved to the same account.
	byUser := map[int]*contributorBucket{}
	var merged []*contributorBucket
	for _, b := range buckets {
		if b.user == nil {
			if includeAnon {
				merged = append(merged, b)
			}
			continue
		}
		if existing := byUser[b.user.ID]; existing != nil {
			existing.contributions += b.contributions
			continue
		}
		byUser[b.user.ID] = b
		merged = append(merged, b)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].contributions != merged[j].contributions {
			return merged[i].contributions > merged[j].contributions
		}
		return merged[i].firstSeen < merged[j].firstSeen
	})

	out := make([]map[string]interface{}, 0, len(merged))
	for _, b := range merged {
		if b.user != nil {
			entry := userToJSON(b.user)
			entry["contributions"] = b.contributions
			out = append(out, entry)
			continue
		}
		out = append(out, map[string]interface{}{
			"type":          "Anonymous",
			"name":          b.name,
			"email":         b.email,
			"contributions": b.contributions,
		})
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

// --- Statistics over the real commit history ---

// defaultBranchCommits returns every commit reachable from the repository's
// default branch. ok is false when the repo has no git data or no commits.
func (s *Server) defaultBranchCommits(repo *Repo) ([]*object.Commit, bool) {
	stor := s.gitStorageForRepo(repo)
	if stor == nil {
		return nil, false
	}
	ref, err := stor.Reference(plumbing.NewBranchReferenceName(repo.DefaultBranch))
	if err != nil {
		return nil, false
	}
	head, err := object.GetCommit(stor, ref.Hash())
	if err != nil {
		return nil, false
	}
	iter := object.NewCommitPreorderIter(head, nil, nil)
	defer iter.Close()
	var commits []*object.Commit
	_ = iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, c)
		return nil
	})
	return commits, len(commits) > 0
}

// weekStart returns midnight UTC of the Sunday beginning t's week — GitHub's
// statistics APIs bucket by Sunday-based weeks.
func weekStart(t time.Time) time.Time {
	t = t.UTC()
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return t.AddDate(0, 0, -int(t.Weekday()))
}

// commitLineStats returns additions/deletions of a commit against its first
// parent (or the empty tree). Merge/binary anomalies count as zero rather
// than failing the whole aggregation.
func commitLineStats(c *object.Commit) (int, int) {
	stats, err := c.Stats()
	if err != nil {
		return 0, 0
	}
	adds, dels := 0, 0
	for _, fs := range stats {
		adds += fs.Addition
		dels += fs.Deletion
	}
	return adds, dels
}

func (s *Server) handleStatsContributors(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	commits, ok := s.defaultBranchCommits(repo)
	if !ok {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	type weekCell struct{ a, d, c int }
	type authorAgg struct {
		user  *User
		total int
		weeks map[int64]*weekCell
	}
	byUser := map[int]*authorAgg{}
	var order []*authorAgg
	earliest := time.Now().UTC()
	for _, c := range commits {
		when := c.Committer.When.UTC()
		if when.Before(earliest) {
			earliest = when
		}
		user := s.store.ResolveUserBySignature(c.Author.Name, c.Author.Email)
		if user == nil {
			continue
		}
		agg := byUser[user.ID]
		if agg == nil {
			agg = &authorAgg{user: user, weeks: map[int64]*weekCell{}}
			byUser[user.ID] = agg
			order = append(order, agg)
		}
		wk := weekStart(when).Unix()
		cell := agg.weeks[wk]
		if cell == nil {
			cell = &weekCell{}
			agg.weeks[wk] = cell
		}
		adds, dels := commitLineStats(c)
		cell.a += adds
		cell.d += dels
		cell.c++
		agg.total++
	}

	first := weekStart(earliest)
	last := weekStart(time.Now())
	out := make([]map[string]interface{}, 0, len(order))
	for _, agg := range order {
		weeks := []map[string]interface{}{}
		for wk := first; !wk.After(last); wk = wk.AddDate(0, 0, 7) {
			cell := agg.weeks[wk.Unix()]
			if cell == nil {
				cell = &weekCell{}
			}
			weeks = append(weeks, map[string]interface{}{
				"w": wk.Unix(),
				"a": cell.a,
				"d": cell.d,
				"c": cell.c,
			})
		}
		out = append(out, map[string]interface{}{
			"author": userToJSON(agg.user),
			"total":  agg.total,
			"weeks":  weeks,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		ti, _ := out[i]["total"].(int)
		tj, _ := out[j]["total"].(int)
		return ti < tj
	})
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleStatsCodeFrequency(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	commits, ok := s.defaultBranchCommits(repo)
	if !ok {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	type cell struct{ a, d int }
	byWeek := map[int64]*cell{}
	for _, c := range commits {
		wk := weekStart(c.Committer.When).Unix()
		cl := byWeek[wk]
		if cl == nil {
			cl = &cell{}
			byWeek[wk] = cl
		}
		adds, dels := commitLineStats(c)
		cl.a += adds
		cl.d += dels
	}
	weeks := make([]int64, 0, len(byWeek))
	for wk := range byWeek {
		weeks = append(weeks, wk)
	}
	sort.Slice(weeks, func(i, j int) bool { return weeks[i] < weeks[j] })
	out := make([][]int64, 0, len(weeks))
	for _, wk := range weeks {
		out = append(out, []int64{wk, int64(byWeek[wk].a), -int64(byWeek[wk].d)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleStatsCommitActivity(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	commits, _ := s.defaultBranchCommits(repo)
	byWeekDay := map[int64]*[7]int{}
	for _, c := range commits {
		when := c.Committer.When.UTC()
		wk := weekStart(when).Unix()
		days := byWeekDay[wk]
		if days == nil {
			days = &[7]int{}
			byWeekDay[wk] = days
		}
		days[int(when.Weekday())]++
	}
	out := make([]map[string]interface{}, 0, 52)
	current := weekStart(time.Now())
	for i := 51; i >= 0; i-- {
		wk := current.AddDate(0, 0, -7*i)
		days := byWeekDay[wk.Unix()]
		if days == nil {
			days = &[7]int{}
		}
		total := 0
		dayList := make([]int, 7)
		for d := 0; d < 7; d++ {
			dayList[d] = days[d]
			total += days[d]
		}
		out = append(out, map[string]interface{}{
			"week":  wk.Unix(),
			"days":  dayList,
			"total": total,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleStatsParticipation(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	commits, _ := s.defaultBranchCommits(repo)
	all := make([]int, 52)
	owner := make([]int, 52)
	current := weekStart(time.Now())
	for _, c := range commits {
		wk := weekStart(c.Committer.When)
		weeksAgo := int(current.Sub(wk).Hours() / (24 * 7))
		if weeksAgo < 0 || weeksAgo > 51 {
			continue
		}
		idx := 51 - weeksAgo // index 0 = oldest week
		all[idx]++
		if repo.Owner != nil {
			if u := s.store.ResolveUserBySignature(c.Author.Name, c.Author.Email); u != nil && u.ID == repo.Owner.ID {
				owner[idx]++
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"all": all, "owner": owner})
}

func (s *Server) handleStatsPunchCard(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	commits, _ := s.defaultBranchCommits(repo)
	var counts [7][24]int
	for _, c := range commits {
		when := c.Committer.When.UTC()
		counts[int(when.Weekday())][when.Hour()]++
	}
	out := make([][]int, 0, 7*24)
	for day := 0; day < 7; day++ {
		for hour := 0; hour < 24; hour++ {
			out = append(out, []int{day, hour, counts[day][hour]})
		}
	}
	writeJSON(w, http.StatusOK, out)
}
