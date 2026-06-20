package bleephub

import (
	"testing"
)

// Embedded GraphQL connections (PullRequest.reviews/comments, Issue.comments)
// must honor the Relay connection args (first/after) like the top-level
// connections do — slicing the node list and emitting a correct
// pageInfo.hasNextPage + endCursor — not return every node with a hardcoded
// hasNextPage:false. The runner cell sends `first:100`, which must still
// return everything, so this test also pins the all-in-one-page case.

// seedPRReviews submits n reviews against a PR via REST.
func seedPRReviews(t *testing.T, owner, name string, prNum, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		resp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/pulls/"+itoa(prNum)+"/reviews", defaultToken,
			map[string]interface{}{"body": "review " + itoa(i), "event": "COMMENT"})
		if resp.StatusCode != 200 {
			resp.Body.Close()
			t.Fatalf("review %d create status = %d", i, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// queryPRReviews fetches PullRequest.reviews with the given connection-arg
// fragment (e.g. `first: 2` or `first: 2, after: "<cursor>"`) and returns the
// reviews connection map.
func queryPRReviews(t *testing.T, owner, name string, prNum int, connArgs string) map[string]interface{} {
	t.Helper()
	query := `query PRReviews($owner: String!, $repo: String!, $pr_number: Int!) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $pr_number) {
				reviews(` + connArgs + `) {
					nodes { id, body, state }
					totalCount
					pageInfo { hasNextPage, hasPreviousPage, startCursor, endCursor }
				}
			}
		}
	}`
	d := gqlData(t, query, map[string]interface{}{"owner": owner, "repo": name, "pr_number": prNum})
	repo, _ := d["repository"].(map[string]interface{})
	pr, _ := repo["pullRequest"].(map[string]interface{})
	if pr == nil {
		t.Fatalf("pullRequest null: %v", d)
	}
	reviews, _ := pr["reviews"].(map[string]interface{})
	if reviews == nil {
		t.Fatalf("reviews null: %v", pr)
	}
	return reviews
}

func TestPRGraphQL_ReviewsConnectionPagination(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-reviewpaginate")
	prNum, _ := sweepPR(t, owner, name, "paginate reviews")

	const total = 5
	seedPRReviews(t, owner, name, prNum, total)

	// --- Page 1: first:2 returns exactly 2, hasNextPage:true, real endCursor ---
	page1 := queryPRReviews(t, owner, name, prNum, "first: 2")
	nodes1, _ := page1["nodes"].([]interface{})
	if len(nodes1) != 2 {
		t.Fatalf("first:2 returned %d nodes, want 2", len(nodes1))
	}
	if tc, _ := page1["totalCount"].(float64); int(tc) != total {
		t.Errorf("totalCount = %v, want %d", page1["totalCount"], total)
	}
	pi1, _ := page1["pageInfo"].(map[string]interface{})
	if hn, _ := pi1["hasNextPage"].(bool); !hn {
		t.Errorf("page1 hasNextPage = %v, want true", pi1["hasNextPage"])
	}
	if hp, _ := pi1["hasPreviousPage"].(bool); hp {
		t.Errorf("page1 hasPreviousPage = %v, want false", pi1["hasPreviousPage"])
	}
	endCursor1, _ := pi1["endCursor"].(string)
	if endCursor1 == "" {
		t.Fatalf("page1 endCursor empty, want a non-null cursor: %v", pi1)
	}

	// --- Page 2: after:<endCursor> returns the next slice ---
	page2 := queryPRReviews(t, owner, name, prNum, `first: 2, after: "`+endCursor1+`"`)
	nodes2, _ := page2["nodes"].([]interface{})
	if len(nodes2) != 2 {
		t.Fatalf("page2 returned %d nodes, want 2", len(nodes2))
	}
	pi2, _ := page2["pageInfo"].(map[string]interface{})
	if hn, _ := pi2["hasNextPage"].(bool); !hn {
		t.Errorf("page2 hasNextPage = %v, want true (1 review remains)", pi2["hasNextPage"])
	}
	if hp, _ := pi2["hasPreviousPage"].(bool); !hp {
		t.Errorf("page2 hasPreviousPage = %v, want true", pi2["hasPreviousPage"])
	}
	// Page 2 must be disjoint from page 1.
	idOf := func(n interface{}) string {
		m, _ := n.(map[string]interface{})
		s, _ := m["id"].(string)
		return s
	}
	seen := map[string]bool{idOf(nodes1[0]): true, idOf(nodes1[1]): true}
	if seen[idOf(nodes2[0])] || seen[idOf(nodes2[1])] {
		t.Errorf("page2 overlaps page1: %v vs %v", nodes2, nodes1)
	}

	// --- Page 3: the final single review ---
	endCursor2, _ := pi2["endCursor"].(string)
	page3 := queryPRReviews(t, owner, name, prNum, `first: 2, after: "`+endCursor2+`"`)
	nodes3, _ := page3["nodes"].([]interface{})
	if len(nodes3) != 1 {
		t.Fatalf("page3 returned %d nodes, want 1", len(nodes3))
	}
	pi3, _ := page3["pageInfo"].(map[string]interface{})
	if hn, _ := pi3["hasNextPage"].(bool); hn {
		t.Errorf("page3 hasNextPage = %v, want false (list exhausted)", pi3["hasNextPage"])
	}

	// --- first:100 (the runner-cell shape) returns everything, one page ---
	all := queryPRReviews(t, owner, name, prNum, "first: 100")
	allNodes, _ := all["nodes"].([]interface{})
	if len(allNodes) != total {
		t.Fatalf("first:100 returned %d nodes, want %d", len(allNodes), total)
	}
	piAll, _ := all["pageInfo"].(map[string]interface{})
	if hn, _ := piAll["hasNextPage"].(bool); hn {
		t.Errorf("first:100 hasNextPage = %v, want false (all fit on one page)", piAll["hasNextPage"])
	}
	if hp, _ := piAll["hasPreviousPage"].(bool); hp {
		t.Errorf("first:100 hasPreviousPage = %v, want false", piAll["hasPreviousPage"])
	}
}

// queryIssueComments fetches Issue.comments with the given connection-arg
// fragment and returns the comments connection map.
func queryIssueComments(t *testing.T, owner, name string, issueNum int, connArgs string) map[string]interface{} {
	t.Helper()
	query := `query IssueComments($owner: String!, $repo: String!, $n: Int!) {
		repository(owner: $owner, name: $repo) {
			issue(number: $n) {
				comments(` + connArgs + `) {
					nodes { id, body }
					totalCount
					pageInfo { hasNextPage, hasPreviousPage, startCursor, endCursor }
				}
			}
		}
	}`
	d := gqlData(t, query, map[string]interface{}{"owner": owner, "repo": name, "n": issueNum})
	repo, _ := d["repository"].(map[string]interface{})
	issue, _ := repo["issue"].(map[string]interface{})
	if issue == nil {
		t.Fatalf("issue null: %v", d)
	}
	comments, _ := issue["comments"].(map[string]interface{})
	if comments == nil {
		t.Fatalf("comments null: %v", issue)
	}
	return comments
}

func TestIssueGraphQL_CommentsConnectionPagination(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-commentpaginate")

	// Create an issue via REST.
	resp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/issues", defaultToken,
		map[string]interface{}{"title": "paginate comments"})
	issueData := decodeJSON(t, resp)
	numF, ok := issueData["number"].(float64)
	if !ok {
		t.Fatalf("issue create failed: %v", issueData)
	}
	issueNum := int(numF)

	const total = 5
	for i := 0; i < total; i++ {
		r := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/issues/"+itoa(issueNum)+"/comments", defaultToken,
			map[string]interface{}{"body": "comment " + itoa(i)})
		if r.StatusCode != 201 && r.StatusCode != 200 {
			r.Body.Close()
			t.Fatalf("comment %d create status = %d", i, r.StatusCode)
		}
		r.Body.Close()
	}

	// Page 1: first:2 → 2 nodes, hasNextPage:true, real endCursor.
	page1 := queryIssueComments(t, owner, name, issueNum, "first: 2")
	if n, _ := page1["nodes"].([]interface{}); len(n) != 2 {
		t.Fatalf("first:2 returned %d comments, want 2", len(n))
	}
	pi1, _ := page1["pageInfo"].(map[string]interface{})
	if hn, _ := pi1["hasNextPage"].(bool); !hn {
		t.Errorf("page1 hasNextPage = %v, want true", pi1["hasNextPage"])
	}
	endCursor, _ := pi1["endCursor"].(string)
	if endCursor == "" {
		t.Fatalf("page1 endCursor empty: %v", pi1)
	}

	// Page 2: after:<cursor> → next 2, disjoint from page 1.
	page1Nodes, _ := page1["nodes"].([]interface{})
	page2 := queryIssueComments(t, owner, name, issueNum, `first: 2, after: "`+endCursor+`"`)
	page2Nodes, _ := page2["nodes"].([]interface{})
	if len(page2Nodes) != 2 {
		t.Fatalf("page2 returned %d comments, want 2", len(page2Nodes))
	}
	bodyOf := func(n interface{}) string {
		m, _ := n.(map[string]interface{})
		s, _ := m["body"].(string)
		return s
	}
	seen := map[string]bool{bodyOf(page1Nodes[0]): true, bodyOf(page1Nodes[1]): true}
	if seen[bodyOf(page2Nodes[0])] || seen[bodyOf(page2Nodes[1])] {
		t.Errorf("page2 overlaps page1: %v vs %v", page2Nodes, page1Nodes)
	}

	// first:100 (runner-cell shape) returns everything in one page.
	all := queryIssueComments(t, owner, name, issueNum, "first: 100")
	if n, _ := all["nodes"].([]interface{}); len(n) != total {
		t.Fatalf("first:100 returned %d comments, want %d", len(n), total)
	}
	piAll, _ := all["pageInfo"].(map[string]interface{})
	if hn, _ := piAll["hasNextPage"].(bool); hn {
		t.Errorf("first:100 hasNextPage = %v, want false", piAll["hasNextPage"])
	}
}
