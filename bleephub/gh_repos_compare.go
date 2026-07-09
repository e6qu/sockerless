package bleephub

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

// resolveGitRef turns a user-supplied ref name (branch, tag, full ref, or SHA)
// into a commit hash. It returns an error describing the resolution failure so
// callers can choose a 404/422 response.
func resolveGitRef(stor gitStorage.Storer, ref string) (plumbing.Hash, error) {
	if ref == "" {
		return plumbing.ZeroHash, errors.New("ref is empty")
	}

	// 1. Full reference name.
	if strings.HasPrefix(ref, "refs/") {
		r, err := stor.Reference(plumbing.ReferenceName(ref))
		if err == nil {
			return refHash(r, stor)
		}
	}

	// 2. Branch or tag shorthand.
	for _, name := range []plumbing.ReferenceName{
		plumbing.NewBranchReferenceName(ref),
		plumbing.NewTagReferenceName(ref),
	} {
		r, err := stor.Reference(name)
		if err == nil {
			return refHash(r, stor)
		}
	}

	// 3. Short or full SHA. plumbing.NewHash accepts hex and returns ZeroHash
	// on invalid input; IsZero prevents treating garbage as zero hash.
	h := plumbing.NewHash(ref)
	if !h.IsZero() {
		if _, err := object.GetCommit(stor, h); err == nil {
			return h, nil
		}
	}

	return plumbing.ZeroHash, fmt.Errorf("ref not found: %s", ref)
}

func refHash(r *plumbing.Reference, stor gitStorage.Storer) (plumbing.Hash, error) {
	if r.Type() == plumbing.SymbolicReference {
		target, err := stor.Reference(r.Target())
		if err != nil {
			return plumbing.ZeroHash, err
		}
		return refHash(target, stor)
	}
	h := r.Hash()
	if tag, err := object.GetTag(stor, h); err == nil {
		if tag.TargetType != plumbing.CommitObject {
			return plumbing.ZeroHash, fmt.Errorf("tag %s points to %s, not commit", r.Name(), tag.TargetType)
		}
		return tag.Target, nil
	}
	return h, nil
}

// findMergeBase returns the nearest common ancestor of a and b. A simple
// ancestor-set algorithm is sufficient for bleephub's linear/short-history
// repositories; it walks parents from a, then walks from b until it hits the
// set. If none exists it returns ZeroHash.
func findMergeBase(stor gitStorage.Storer, a, b plumbing.Hash) (plumbing.Hash, error) {
	if a == b {
		return a, nil
	}

	ancestors := map[plumbing.Hash]bool{}
	walk := func(start plumbing.Hash) error {
		commit, err := object.GetCommit(stor, start)
		if err != nil {
			return err
		}
		iter := object.NewCommitPreorderIter(commit, nil, nil)
		defer iter.Close()
		return iter.ForEach(func(c *object.Commit) error {
			ancestors[c.Hash] = true
			return nil
		})
	}
	if err := walk(a); err != nil {
		return plumbing.ZeroHash, err
	}

	commit, err := object.GetCommit(stor, b)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	iter := object.NewCommitPreorderIter(commit, nil, nil)
	defer iter.Close()
	found := plumbing.ZeroHash
	_ = iter.ForEach(func(c *object.Commit) error {
		if ancestors[c.Hash] {
			found = c.Hash
			return errors.New("stop")
		}
		return nil
	})
	return found, nil
}

// commitsBetween returns the commits reachable from head but not from base,
// ordered from newest to oldest (head first). If base is not an ancestor of
// head the result is the full history reachable from head.
func commitsBetween(stor gitStorage.Storer, base, head plumbing.Hash) ([]*object.Commit, error) {
	baseCommit, err := object.GetCommit(stor, base)
	if err != nil {
		return nil, err
	}
	exclude := map[plumbing.Hash]bool{baseCommit.Hash: true}
	iter := object.NewCommitPreorderIter(baseCommit, nil, nil)
	_ = iter.ForEach(func(c *object.Commit) error {
		exclude[c.Hash] = true
		return nil
	})
	iter.Close()

	headCommit, err := object.GetCommit(stor, head)
	if err != nil {
		return nil, err
	}
	iter = object.NewCommitPreorderIter(headCommit, nil, nil)
	defer iter.Close()

	var commits []*object.Commit
	_ = iter.ForEach(func(c *object.Commit) error {
		if exclude[c.Hash] {
			return nil
		}
		commits = append(commits, c)
		return nil
	})
	return commits, nil
}

func commitToJSON(c *object.Commit, repo *Repo, baseURL string) map[string]interface{} {
	if c == nil {
		return nil
	}
	commitURL := baseURL + "/repos/" + repo.FullName + "/commits/" + c.Hash.String()
	htmlURL := "/" + repo.FullName + "/commit/" + c.Hash.String()
	parents := make([]map[string]interface{}, 0, len(c.ParentHashes))
	for _, h := range c.ParentHashes {
		parents = append(parents, map[string]interface{}{
			"sha":      h.String(),
			"url":      baseURL + "/repos/" + repo.FullName + "/commits/" + h.String(),
			"html_url": "/" + repo.FullName + "/commit/" + h.String(),
		})
	}
	return map[string]interface{}{
		"sha":          c.Hash.String(),
		"node_id":      "MDY6Q29tbWl0" + c.Hash.String(),
		"url":          commitURL,
		"html_url":     htmlURL,
		"comments_url": commitURL + "/comments",
		"author":       map[string]interface{}{},
		"committer":    map[string]interface{}{},
		"parents":      parents,
		"commit": map[string]interface{}{
			"url":           commitURL,
			"message":       strings.TrimSpace(c.Message),
			"comment_count": 0,
			"author": map[string]interface{}{
				"name":  c.Author.Name,
				"email": c.Author.Email,
				"date":  c.Author.When.Format(time.RFC3339),
			},
			"committer": map[string]interface{}{
				"name":  c.Committer.Name,
				"email": c.Committer.Email,
				"date":  c.Committer.When.Format(time.RFC3339),
			},
			"tree": map[string]interface{}{
				"sha": c.TreeHash.String(),
				"url": baseURL + "/repos/" + repo.FullName + "/git/trees/" + c.TreeHash.String(),
			},
			"verification": map[string]interface{}{
				"verified":    false,
				"reason":      "unsigned",
				"signature":   nil,
				"payload":     nil,
				"verified_at": nil,
			},
		},
	}
}

// compareFiles walks the diff between baseTree and headTree and returns the
// file entries GitHub's compare API emits, plus total additions/deletions/
// changes.
func compareFiles(baseTree, headTree *object.Tree, headCommit *object.Commit, repo *Repo, baseURL string) ([]map[string]interface{}, int, int, int, error) {
	changes, err := object.DiffTree(baseTree, headTree)
	if err != nil {
		return nil, 0, 0, 0, err
	}

	var files []map[string]interface{}
	var totalAdd, totalDel, totalChange int
	for _, ch := range changes {
		var status string
		switch ch.To.TreeEntry.Mode {
		case 0:
			status = "removed"
		default:
			if ch.From.TreeEntry.Mode == 0 {
				status = "added"
			} else if ch.From.TreeEntry.Hash == ch.To.TreeEntry.Hash {
				status = "unchanged"
			} else {
				status = "modified"
			}
		}

		adds, dels, err := changeStats(ch)
		if err != nil {
			return nil, 0, 0, 0, err
		}
		if status == "unchanged" {
			continue
		}
		totalAdd += adds
		totalDel += dels
		totalChange++

		name := ch.To.Name
		if name == "" {
			name = ch.From.Name
		}
		sha := ch.To.TreeEntry.Hash.String()
		if sha == "" {
			sha = ch.From.TreeEntry.Hash.String()
		}
		files = append(files, map[string]interface{}{
			"filename":     name,
			"status":       status,
			"additions":    adds,
			"deletions":    dels,
			"changes":      adds + dels,
			"sha":          sha,
			"blob_url":     baseURL + "/" + repo.FullName + "/blob/" + headCommit.Hash.String() + "/" + name,
			"raw_url":      baseURL + "/" + repo.FullName + "/raw/" + headCommit.Hash.String() + "/" + name,
			"contents_url": baseURL + "/api/v3/repos/" + repo.FullName + "/contents/" + name + "?ref=" + headCommit.Hash.String(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		fi, _ := files[i]["filename"].(string)
		fj, _ := files[j]["filename"].(string)
		return fi < fj
	})
	return files, totalAdd, totalDel, totalChange, nil
}

func changeStats(ch *object.Change) (int, int, error) {
	patch, err := ch.Patch()
	if err != nil {
		return 0, 0, err
	}
	stats := patch.Stats()
	if len(stats) == 0 {
		return 0, 0, nil
	}
	return stats[0].Addition, stats[0].Deletion, nil
}

func (s *Server) handleCompareRefs(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	user := ghUserFromContext(r.Context())
	if repo.Private && !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	stor := s.store.GetGitStorage(owner, name)
	if stor == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"url":               "",
			"html_url":          "",
			"permalink_url":     "",
			"diff_url":          "",
			"patch_url":         "",
			"base_commit":       nil,
			"merge_base_commit": nil,
			"status":            "identical",
			"ahead_by":          0,
			"behind_by":         0,
			"total_commits":     0,
			"commits":           []interface{}{},
			"files":             []interface{}{},
		})
		return
	}

	baseRef := r.PathValue("range")
	headRef := ""
	if i := strings.Index(baseRef, "..."); i >= 0 {
		headRef = baseRef[i+3:]
		baseRef = baseRef[:i]
	}
	if baseRef == "" || headRef == "" {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	baseHash, err := resolveGitRef(stor, baseRef)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	headHash, err := resolveGitRef(stor, headRef)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	baseCommit, _ := object.GetCommit(stor, baseHash)
	headCommit, _ := object.GetCommit(stor, headHash)

	mergeBase, err := findMergeBase(stor, baseHash, headHash)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "merge base lookup failed")
		return
	}
	mergeBaseCommit, _ := object.GetCommit(stor, mergeBase)

	status := "diverged"
	aheadBy, behindBy := 0, 0
	if baseHash == headHash {
		status = "identical"
	} else if mergeBase == headHash {
		status = "behind"
		aheadCommits, _ := commitsBetween(stor, headHash, baseHash)
		behindBy = len(aheadCommits)
	} else if mergeBase == baseHash {
		status = "ahead"
		aheadCommits, _ := commitsBetween(stor, baseHash, headHash)
		aheadBy = len(aheadCommits)
	} else {
		aheadCommits, _ := commitsBetween(stor, mergeBase, headHash)
		behindCommits, _ := commitsBetween(stor, mergeBase, baseHash)
		aheadBy = len(aheadCommits)
		behindBy = len(behindCommits)
	}

	var commits []map[string]interface{}
	if status == "ahead" || status == "diverged" {
		commitObjs, _ := commitsBetween(stor, baseHash, headHash)
		for _, c := range commitObjs {
			commits = append(commits, commitToJSON(c, repo, s.baseURL(r)))
		}
	}

	var files []map[string]interface{}
	if mergeBaseCommit != nil && headCommit != nil {
		baseTree, _ := mergeBaseCommit.Tree()
		headTree, _ := headCommit.Tree()
		if baseTree != nil && headTree != nil {
			files, _, _, _, _ = compareFiles(baseTree, headTree, headCommit, repo, s.baseURL(r))
		}
	}

	url := s.baseURL(r) + "/" + repo.FullName + "/compare/" + baseRef + "..." + headRef
	out := map[string]interface{}{
		"url":               url,
		"html_url":          url,
		"permalink_url":     url,
		"diff_url":          url + ".diff",
		"patch_url":         url + ".patch",
		"base_commit":       commitToJSON(baseCommit, repo, s.baseURL(r)),
		"merge_base_commit": commitToJSON(mergeBaseCommit, repo, s.baseURL(r)),
		"status":            status,
		"ahead_by":          aheadBy,
		"behind_by":         behindBy,
		"total_commits":     len(commits),
		"commits":           commits,
		"files":             files,
	}
	writeJSON(w, http.StatusOK, out)
}

// flattenTree returns a map of full file paths to tree entries for all
// non-directory entries in t.
func flattenTree(t *object.Tree) (map[string]object.TreeEntry, error) {
	out := map[string]object.TreeEntry{}
	if t == nil {
		return out, nil
	}
	w := object.NewTreeWalker(t, true, nil)
	defer w.Close()
	for {
		name, entry, err := w.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if entry.Mode == filemode.Dir {
			continue
		}
		out[name] = entry
	}
	return out, nil
}

// threeWayMergePaths combines file maps from the merge base, our side, and
// their side. It returns a conflict error when both sides change the same path
// differently.
func threeWayMergePaths(base, ours, theirs map[string]object.TreeEntry) (map[string]object.TreeEntry, error) {
	result := map[string]object.TreeEntry{}
	seen := map[string]bool{}

	sameEntry := func(a, b object.TreeEntry) bool {
		return a.Mode == b.Mode && a.Hash == b.Hash
	}

	for p := range base {
		seen[p] = true
		b := base[p]
		o, hasOurs := ours[p]
		t, hasTheirs := theirs[p]

		switch {
		case !hasOurs && !hasTheirs:
			result[p] = b
		case !hasOurs:
			if sameEntry(b, t) {
				result[p] = b
			} else {
				return nil, fmt.Errorf("merge conflict: %s", p)
			}
		case !hasTheirs:
			if sameEntry(b, o) {
				result[p] = b
			} else {
				return nil, fmt.Errorf("merge conflict: %s", p)
			}
		case sameEntry(o, t):
			result[p] = o
		case sameEntry(b, o):
			result[p] = t
		case sameEntry(b, t):
			result[p] = o
		default:
			return nil, fmt.Errorf("merge conflict: %s", p)
		}
	}

	for p := range ours {
		if seen[p] {
			continue
		}
		seen[p] = true
		o := ours[p]
		t, hasTheirs := theirs[p]
		_, hasBase := base[p]
		if hasBase {
			continue // already handled
		}
		if !hasTheirs || sameEntry(o, t) {
			result[p] = o
		} else {
			return nil, fmt.Errorf("merge conflict: %s", p)
		}
	}

	for p := range theirs {
		if seen[p] {
			continue
		}
		result[p] = theirs[p]
	}

	return result, nil
}

// storeTreeObject encodes a tree and persists it in storer storage.
func storeTreeObject(stor storer.EncodedObjectStorer, entries []object.TreeEntry) (plumbing.Hash, error) {
	tree := &object.Tree{Entries: entries}
	obj := stor.NewEncodedObject()
	if err := tree.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return stor.SetEncodedObject(obj)
}

// buildTreeFromPaths reconstructs a root tree from a flat file path map and
// stores all intermediate trees.
func buildTreeFromPaths(stor storer.EncodedObjectStorer, files map[string]object.TreeEntry) (plumbing.Hash, error) {
	type node struct {
		children map[string]*node
		entry    *object.TreeEntry
	}
	root := &node{children: map[string]*node{}}

	add := func(p string, entry object.TreeEntry) {
		dir := path.Dir(p)
		name := path.Base(p)
		n := root
		if dir != "." {
			for _, seg := range strings.Split(dir, "/") {
				if n.children[seg] == nil {
					n.children[seg] = &node{children: map[string]*node{}}
				}
				n = n.children[seg]
			}
		}
		n.children[name] = &node{entry: &entry}
	}
	for p, e := range files {
		add(p, e)
	}

	var build func(*node) (plumbing.Hash, error)
	build = func(n *node) (plumbing.Hash, error) {
		var entries []object.TreeEntry
		for name, child := range n.children {
			if child.entry != nil {
				ent := *child.entry
				ent.Name = name
				entries = append(entries, ent)
			} else {
				hash, err := build(child)
				if err != nil {
					return plumbing.ZeroHash, err
				}
				entries = append(entries, object.TreeEntry{
					Name: name,
					Mode: filemode.Dir,
					Hash: hash,
				})
			}
		}
		sort.Slice(entries, func(i, j int) bool {
			return object.TreeEntrySorter(entries).Less(i, j)
		})
		return storeTreeObject(stor, entries)
	}

	return build(root)
}

// performMerge creates a merge commit updating baseRef to incorporate headHash.
// headName is used in the default merge message. It fast-forwards when
// possible, otherwise performs a three-way merge. It returns the new commit
// hash, "true" for a fast-forward, or an error.
func performMerge(stor gitStorage.Storer, baseRef plumbing.ReferenceName, headHash plumbing.Hash, headName, message string, sig *object.Signature) (plumbing.Hash, bool, error) {
	baseRefObj, err := stor.Reference(baseRef)
	if err != nil {
		return plumbing.ZeroHash, false, fmt.Errorf("resolve base ref: %w", err)
	}
	baseHash := baseRefObj.Hash()

	if baseHash == headHash {
		return baseHash, true, nil
	}

	mergeBase, err := findMergeBase(stor, baseHash, headHash)
	if err != nil {
		return plumbing.ZeroHash, false, err
	}
	if mergeBase.IsZero() {
		return plumbing.ZeroHash, false, errors.New("no merge base")
	}

	if mergeBase == headHash {
		// Already merged.
		return baseHash, true, nil
	}
	if mergeBase == baseHash {
		// Fast-forward: point base at head.
		if err := stor.SetReference(plumbing.NewHashReference(baseRef, headHash)); err != nil {
			return plumbing.ZeroHash, false, err
		}
		return headHash, true, nil
	}

	// True three-way merge.
	mergedTreeHash, err := threeWayMergedTree(stor, mergeBase, baseHash, headHash)
	if err != nil {
		return plumbing.ZeroHash, false, err
	}

	if message == "" {
		message = fmt.Sprintf("Merge branch '%s'", headName)
	}
	commitHash, err := writeCommit(stor, &object.Commit{
		Author:       *sig,
		Committer:    *sig,
		Message:      message,
		TreeHash:     mergedTreeHash,
		ParentHashes: []plumbing.Hash{baseHash, headHash},
	})
	if err != nil {
		return plumbing.ZeroHash, false, err
	}
	if err := stor.SetReference(plumbing.NewHashReference(baseRef, commitHash)); err != nil {
		return plumbing.ZeroHash, false, err
	}
	return commitHash, false, nil
}

// threeWayMergedTree merges the trees of ours and theirs relative to their
// common ancestor mergeBase and stores the resulting tree, returning its
// hash. A conflicting path yields the "merge conflict" error from
// threeWayMergePaths.
func threeWayMergedTree(stor gitStorage.Storer, mergeBase, ours, theirs plumbing.Hash) (plumbing.Hash, error) {
	files := map[string]map[string]object.TreeEntry{}
	for name, hash := range map[string]plumbing.Hash{"base": mergeBase, "ours": ours, "theirs": theirs} {
		commit, err := object.GetCommit(stor, hash)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		tree, err := commit.Tree()
		if err != nil {
			return plumbing.ZeroHash, err
		}
		flat, err := flattenTree(tree)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		files[name] = flat
	}
	mergedFiles, err := threeWayMergePaths(files["base"], files["ours"], files["theirs"])
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return buildTreeFromPaths(stor, mergedFiles)
}

// writeCommit encodes a commit object into storage and returns its hash.
func writeCommit(stor gitStorage.Storer, commit *object.Commit) (plumbing.Hash, error) {
	obj := stor.NewEncodedObject()
	if err := commit.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return stor.SetEncodedObject(obj)
}

// performMergeCommit creates a two-parent merge commit updating baseRef to
// incorporate headHash, the way GitHub's "merge" method merges a pull
// request: unlike performMerge it never fast-forwards — a merge commit is
// created even when base is an ancestor of head. Returns the merge commit
// hash.
func performMergeCommit(stor gitStorage.Storer, baseRef plumbing.ReferenceName, headHash plumbing.Hash, message string, sig *object.Signature) (plumbing.Hash, error) {
	baseRefObj, err := stor.Reference(baseRef)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("resolve base ref: %w", err)
	}
	baseHash := baseRefObj.Hash()

	mergeBase, err := findMergeBase(stor, baseHash, headHash)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if mergeBase.IsZero() {
		return plumbing.ZeroHash, errors.New("no merge base")
	}
	if mergeBase == headHash {
		// Nothing to merge: head is already contained in base.
		return baseHash, nil
	}

	var treeHash plumbing.Hash
	if mergeBase == baseHash {
		headCommit, err := object.GetCommit(stor, headHash)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		treeHash = headCommit.TreeHash
	} else {
		treeHash, err = threeWayMergedTree(stor, mergeBase, baseHash, headHash)
		if err != nil {
			return plumbing.ZeroHash, err
		}
	}

	commitHash, err := writeCommit(stor, &object.Commit{
		Author:       *sig,
		Committer:    *sig,
		Message:      message,
		TreeHash:     treeHash,
		ParentHashes: []plumbing.Hash{baseHash, headHash},
	})
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if err := stor.SetReference(plumbing.NewHashReference(baseRef, commitHash)); err != nil {
		return plumbing.ZeroHash, err
	}
	return commitHash, nil
}

// performSquashMerge condenses everything reachable from headHash but not
// from baseRef into a single commit on baseRef, the way GitHub's squash
// merge does: one commit whose tree is the merged result and whose sole
// parent is the previous base head. Returns the squash commit hash.
func performSquashMerge(stor gitStorage.Storer, baseRef plumbing.ReferenceName, headHash plumbing.Hash, message string, author, committer *object.Signature) (plumbing.Hash, error) {
	baseRefObj, err := stor.Reference(baseRef)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("resolve base ref: %w", err)
	}
	baseHash := baseRefObj.Hash()

	mergeBase, err := findMergeBase(stor, baseHash, headHash)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if mergeBase.IsZero() {
		return plumbing.ZeroHash, errors.New("no merge base")
	}
	if mergeBase == headHash {
		// Nothing to squash: head is already contained in base.
		return baseHash, nil
	}

	var treeHash plumbing.Hash
	if mergeBase == baseHash {
		headCommit, err := object.GetCommit(stor, headHash)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		treeHash = headCommit.TreeHash
	} else {
		treeHash, err = threeWayMergedTree(stor, mergeBase, baseHash, headHash)
		if err != nil {
			return plumbing.ZeroHash, err
		}
	}

	commitHash, err := writeCommit(stor, &object.Commit{
		Author:       *author,
		Committer:    *committer,
		Message:      message,
		TreeHash:     treeHash,
		ParentHashes: []plumbing.Hash{baseHash},
	})
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if err := stor.SetReference(plumbing.NewHashReference(baseRef, commitHash)); err != nil {
		return plumbing.ZeroHash, err
	}
	return commitHash, nil
}

// performRebaseMerge replays the commits reachable from headHash but not
// from baseRef on top of baseRef, the way GitHub's rebase merge does: each
// commit keeps its author and message but gets a new parent chain and the
// merger as committer. Returns the new base head (the rebased tip).
func performRebaseMerge(stor gitStorage.Storer, baseRef plumbing.ReferenceName, headHash plumbing.Hash, committer *object.Signature) (plumbing.Hash, error) {
	baseRefObj, err := stor.Reference(baseRef)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("resolve base ref: %w", err)
	}
	baseHash := baseRefObj.Hash()

	mergeBase, err := findMergeBase(stor, baseHash, headHash)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if mergeBase.IsZero() {
		return plumbing.ZeroHash, errors.New("no merge base")
	}
	if mergeBase == headHash {
		// Nothing to replay: head is already contained in base.
		return baseHash, nil
	}

	commits, err := commitsBetween(stor, mergeBase, headHash)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	// commitsBetween is newest-first; replay oldest-first.
	newParent := baseHash
	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]
		var treeHash plumbing.Hash
		if len(c.ParentHashes) != 1 {
			// Merge commits within the range are dropped, as on GitHub.
			continue
		}
		if c.ParentHashes[0] == newParent {
			treeHash = c.TreeHash
		} else {
			// Replay the commit's change (parent → commit) onto the new
			// base side via a three-way tree merge.
			treeHash, err = threeWayMergedTree(stor, c.ParentHashes[0], newParent, c.Hash)
			if err != nil {
				return plumbing.ZeroHash, err
			}
		}
		newParent, err = writeCommit(stor, &object.Commit{
			Author:       c.Author,
			Committer:    *committer,
			Message:      c.Message,
			TreeHash:     treeHash,
			ParentHashes: []plumbing.Hash{newParent},
		})
		if err != nil {
			return plumbing.ZeroHash, err
		}
	}
	if err := stor.SetReference(plumbing.NewHashReference(baseRef, newParent)); err != nil {
		return plumbing.ZeroHash, err
	}
	return newParent, nil
}

func (s *Server) handleMergeRefs(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have push access to Repository.")
		return
	}

	var req struct {
		Base          string `json:"base"`
		Head          string `json:"head"`
		CommitMessage string `json:"commit_message"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Base == "" || req.Head == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "base and head are required")
		return
	}

	stor := s.store.GetGitStorage(owner, name)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	headHash, err := resolveGitRef(stor, req.Head)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	baseRef := plumbing.NewBranchReferenceName(req.Base)
	baseRefObj, err := stor.Reference(baseRef)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	baseHash := baseRefObj.Hash()

	// Already merged: head is an ancestor of base.
	mergeBase, err := findMergeBase(stor, baseHash, headHash)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "merge base lookup failed")
		return
	}
	if mergeBase == headHash {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	sig := repoSignature("GitHub", "noreply@github.com")
	commitHash, _, err := performMerge(stor, baseRef, headHash, req.Head, req.CommitMessage, sig)
	if err != nil {
		if strings.Contains(err.Error(), "merge conflict") {
			writeGHError(w, http.StatusConflict, "Merge conflict")
			return
		}
		writeGHError(w, http.StatusInternalServerError, "Merge failed")
		return
	}

	mergeCommit, err := object.GetCommit(stor, commitHash)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "Merge failed")
		return
	}

	s.store.UpdateRepo(owner, name, func(r *Repo) {
		r.PushedAt = time.Now().UTC()
	})
	writeJSON(w, http.StatusCreated, commitToJSON(mergeCommit, repo, s.baseURL(r)))
}

// copyGitStorage copies all encoded objects and references from src to dst.
// It is used to implement repository forks while keeping the copy independent
// of the original.
func copyGitStorage(src, dst gitStorage.Storer) error {
	if err := copyGitObjects(src, dst); err != nil {
		return err
	}

	refIter, err := src.IterReferences()
	if err != nil {
		return err
	}
	defer refIter.Close()
	return refIter.ForEach(func(ref *plumbing.Reference) error {
		return dst.SetReference(ref)
	})
}

func copyGitObjects(src, dst gitStorage.Storer) error {
	for _, t := range []plumbing.ObjectType{plumbing.CommitObject, plumbing.TreeObject, plumbing.BlobObject, plumbing.TagObject} {
		iter, err := src.IterEncodedObjects(t)
		if err != nil {
			return err
		}
		if err := iter.ForEach(func(obj plumbing.EncodedObject) error {
			newObj := dst.NewEncodedObject()
			newObj.SetType(obj.Type())
			newObj.SetSize(obj.Size())
			w, err := newObj.Writer()
			if err != nil {
				return err
			}
			r, err := obj.Reader()
			if err != nil {
				return err
			}
			if _, err := io.Copy(w, r); err != nil {
				r.Close()
				return err
			}
			r.Close()
			if err := w.Close(); err != nil {
				return err
			}
			_, err = dst.SetEncodedObject(newObj)
			return err
		}); err != nil {
			iter.Close()
			return err
		}
		iter.Close()
	}
	return nil
}
