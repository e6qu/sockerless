package bleephub

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// knownIssueEventTypes are the event discriminators the renderers special-case;
// fuzzing draws from these plus arbitrary strings so both the typed arms and
// the default arm are exercised.
var knownIssueEventTypes = []string{
	"labeled", "unlabeled", "assigned", "unassigned", "milestoned",
	"demilestoned", "renamed", "review_requested", "review_request_removed",
	"locked", "unlocked", "closed", "reopened", "merged", "referenced",
	"", "bogus_event",
}

// FuzzTimelineEventRender drives fuzzed IssueEvent field combinations through
// the three event serializers (repo-level, per-issue union, timeline union).
// Referenced entities (actor, label, assignee, milestone, reviewer) are left
// dangling by fuzzing their IDs so the "missing referenced user/commit" path
// is covered. Invariant: no panic, and every rendered map marshals to JSON.
func FuzzTimelineEventRender(f *testing.F) {
	st := NewStore()
	st.SeedDefaultUser()
	admin := st.UsersByLogin["admin"]
	// One real label / milestone so resolved arms are exercised too.
	st.Labels[1] = &IssueLabel{ID: 1, Name: "bug", Color: "f00"}
	st.Milestones[1] = &Milestone{ID: 1, Title: "v1"}

	// eventIdx selects the event string; the ints dangle references.
	f.Add(0, admin.ID, 1, 1, 1, "from", "to", "resolved")
	f.Add(6, 99999, 0, -1, 424242, "", "", "")
	f.Add(3, admin.ID, 0, 0, 1, "old name", "new name", "off-topic")
	f.Add(17, 0, 999, 999, 999, "\x00", "￿", "spam")
	f.Add(9, admin.ID, 1, 0, 0, "a", "b", "too heated")

	f.Fuzz(func(t *testing.T, eventIdx, actorID, labelID, assigneeID, milestoneID int, renameFrom, renameTo, lockReason string) {
		ev := knownIssueEventTypes[((eventIdx%len(knownIssueEventTypes))+len(knownIssueEventTypes))%len(knownIssueEventTypes)]
		e := &IssueEvent{
			ID:                  1,
			NodeID:              "IE_test",
			RepoID:              1,
			ParentType:          "issue",
			IssueID:             1,
			ActorID:             actorID,
			Event:               ev,
			CommitID:            "deadbeef",
			CreatedAt:           time.Unix(0, 0).UTC(),
			LabelID:             labelID,
			AssigneeID:          assigneeID,
			AssignerID:          actorID,
			MilestoneID:         milestoneID,
			RequestedReviewerID: assigneeID,
			LockReason:          lockReason,
			RenameFrom:          renameFrom,
			RenameTo:            renameTo,
		}
		for _, render := range []func(*IssueEvent, *Store, string, string) map[string]interface{}{
			issueEventToJSON, issueEventForIssueToJSON, issueEventForTimelineToJSON,
		} {
			out := render(e, st, "http://x", "admin/repo")
			if _, err := json.Marshal(out); err != nil {
				t.Fatalf("event %q rendered a non-JSON-marshalable map: %v", ev, err)
			}
			if out["event"] != ev {
				t.Fatalf("renderer dropped event discriminator: got %v want %q", out["event"], ev)
			}
		}
	})
}

// FuzzCommittedTimelineEvent drives the PR committed-event derivation with
// fuzzed commit metadata (message, author/committer identity, parent count).
// A commit object with arbitrary fields must render to a JSON-marshalable
// timeline-committed event without panicking on empty signatures or huge
// parent lists.
func FuzzCommittedTimelineEvent(f *testing.F) {
	f.Add("initial commit", "Alice", "alice@example.com", 0)
	f.Add("", "", "", 1)
	f.Add("merge\n\nbody", "Bob", "bob@x", 2)
	f.Add("\x00￿", "n", "e", 64)

	f.Fuzz(func(t *testing.T, message, name, email string, nParents int) {
		if nParents < 0 {
			nParents = 0
		}
		if nParents > 256 {
			nParents = 256
		}
		parents := make([]plumbing.Hash, 0, nParents)
		for i := 0; i < nParents; i++ {
			parents = append(parents, plumbing.NewHash("0000000000000000000000000000000000000000"))
		}
		c := &object.Commit{
			Hash:         plumbing.NewHash("1111111111111111111111111111111111111111"),
			Message:      message,
			Author:       object.Signature{Name: name, Email: email, When: time.Unix(0, 0)},
			Committer:    object.Signature{Name: name, Email: email, When: time.Unix(0, 0)},
			TreeHash:     plumbing.NewHash("2222222222222222222222222222222222222222"),
			ParentHashes: parents,
		}
		out := timelineCommittedEventJSON(c, "admin/repo", "http://x")
		if _, err := json.Marshal(out); err != nil {
			t.Fatalf("committed event not JSON-marshalable: %v", err)
		}
		if out["event"] != "committed" {
			t.Fatalf("committed event discriminator = %v", out["event"])
		}
	})
}
