package bleephub

import (
	"net/http"
	"testing"
)

// TestDiscussionsGraphQL_Lifecycle exercises categories, discussions, comments,
// answers and permission checks through the GraphQL API.
func TestDiscussionsGraphQL_Lifecycle(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "discussions-gql",
	})
	repoData := decodeJSON(t, resp)
	owner, _ := repoData["owner"].(map[string]interface{})
	login, _ := owner["login"].(string)
	name, _ := repoData["name"].(string)
	repoNodeID, _ := repoData["node_id"].(string)

	// 1. Default categories exist.
	catQuery := `query($owner:String!,$name:String!){
		repository(owner:$owner,name:$name){
			discussionCategories(first:10){nodes{id,name,isAnswerable},totalCount}
		}
	}`
	cats := runDiscussionGQL(t, catQuery, map[string]interface{}{"owner": login, "name": name})
	repo, _ := cats["repository"].(map[string]interface{})
	catConn, _ := repo["discussionCategories"].(map[string]interface{})
	if tc, _ := catConn["totalCount"].(float64); tc != 5 {
		t.Fatalf("expected 5 default categories, got %v", catConn["totalCount"])
	}
	catNodes, _ := catConn["nodes"].([]interface{})
	var qaCatID string
	for _, n := range catNodes {
		cat, _ := n.(map[string]interface{})
		if cat["name"] == "Q&A" {
			qaCatID, _ = cat["id"].(string)
		}
	}
	if qaCatID == "" {
		t.Fatalf("expected Q&A category, got %v", catNodes)
	}

	// 2. Create discussion.
	create := `mutation($repo:ID!,$cat:ID!){createDiscussion(input:{repositoryId:$repo,categoryId:$cat,title:"Hello",body:"World\n\nDetails"}){discussion{number,id,title,bodyHTML,bodyText,category{name},viewerCanUpdate,viewerCanDelete}}}`
	createRes := runDiscussionGQL(t, create, map[string]interface{}{"repo": repoNodeID, "cat": qaCatID})
	createPayload, _ := createRes["createDiscussion"].(map[string]interface{})
	disc, _ := createPayload["discussion"].(map[string]interface{})
	if disc["title"] != "Hello" {
		t.Fatalf("expected title Hello, got %v", disc["title"])
	}
	discNodeID, _ := disc["id"].(string)
	discNumber := int(disc["number"].(float64))
	if bodyHTML, _ := disc["bodyHTML"].(string); bodyHTML == "" {
		t.Fatalf("expected bodyHTML, got empty")
	}
	if bodyText, _ := disc["bodyText"].(string); bodyText != "World\n\nDetails" {
		t.Fatalf("expected bodyText, got %q", bodyText)
	}
	catName, _ := disc["category"].(map[string]interface{})["name"].(string)
	if catName != "Q&A" {
		t.Fatalf("expected Q&A category, got %v", catName)
	}
	if v, _ := disc["viewerCanUpdate"].(bool); !v {
		t.Fatalf("expected viewerCanUpdate=true for author")
	}
	if v, _ := disc["viewerCanDelete"].(bool); !v {
		t.Fatalf("expected viewerCanDelete=true for author")
	}

	// 3. List discussions.
	listQuery := `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){discussions(first:10){nodes{number,title},totalCount}}}`
	listRes := runDiscussionGQL(t, listQuery, map[string]interface{}{"owner": login, "name": name})
	listRepo, _ := listRes["repository"].(map[string]interface{})
	listConn, _ := listRepo["discussions"].(map[string]interface{})
	if tc, _ := listConn["totalCount"].(float64); tc != 1 {
		t.Fatalf("expected 1 discussion, got %v", listConn["totalCount"])
	}

	// 4. Query by number.
	getQuery := `query($owner:String!,$name:String!,$num:Int!){
		repository(owner:$owner,name:$name){discussion(number:$num){number,title,comments(first:10){totalCount}}}
	}`
	getRes := runDiscussionGQL(t, getQuery, map[string]interface{}{"owner": login, "name": name, "num": discNumber})
	getRepo, _ := getRes["repository"].(map[string]interface{})
	getDisc, _ := getRepo["discussion"].(map[string]interface{})
	if getDisc["title"] != "Hello" {
		t.Fatalf("expected discussion title Hello, got %v", getDisc["title"])
	}

	// 5. Add top-level comment.
	addComment := `mutation($did:ID!){addDiscussionComment(input:{discussionId:$did,body:"First comment"}){comment{id,body,discussion{number},replies(first:10){totalCount}}}}`
	addRes := runDiscussionGQL(t, addComment, map[string]interface{}{"did": discNodeID})
	addPayload, _ := addRes["addDiscussionComment"].(map[string]interface{})
	comment, _ := addPayload["comment"].(map[string]interface{})
	commentNodeID, _ := comment["id"].(string)
	if comment["body"] != "First comment" {
		t.Fatalf("expected comment body, got %v", comment["body"])
	}

	// 6. Add reply.
	addReply := `mutation($did:ID!,$pid:ID!){addDiscussionComment(input:{discussionId:$did,body:"A reply",replyToId:$pid}){comment{id,body,replies(first:10){nodes{body},totalCount}}}}`
	replyRes := runDiscussionGQL(t, addReply, map[string]interface{}{"did": discNodeID, "pid": commentNodeID})
	replyPayload, _ := replyRes["addDiscussionComment"].(map[string]interface{})
	replyComment, _ := replyPayload["comment"].(map[string]interface{})
	replyNodeID, _ := replyComment["id"].(string)
	replies, _ := replyComment["replies"].(map[string]interface{})
	if tc, _ := replies["totalCount"].(float64); tc != 0 {
		t.Fatalf("expected no nested replies, got %v", tc)
	}

	// 7. Query comments with replies.
	commentsQuery := `query($owner:String!,$name:String!,$num:Int!){
		repository(owner:$owner,name:$name){discussion(number:$num){comments(first:10){nodes{id,body,replies(first:10){nodes{id,body},totalCount}},totalCount}}}
	}`
	commentsRes := runDiscussionGQL(t, commentsQuery, map[string]interface{}{"owner": login, "name": name, "num": discNumber})
	commentsRepo, _ := commentsRes["repository"].(map[string]interface{})
	commentsDisc, _ := commentsRepo["discussion"].(map[string]interface{})
	commentsConn, _ := commentsDisc["comments"].(map[string]interface{})
	if tc, _ := commentsConn["totalCount"].(float64); tc != 1 {
		t.Fatalf("expected 1 top-level comment, got %v", tc)
	}
	commentNodes, _ := commentsConn["nodes"].([]interface{})
	topComment, _ := commentNodes[0].(map[string]interface{})
	childReplies, _ := topComment["replies"].(map[string]interface{})
	if tc, _ := childReplies["totalCount"].(float64); tc != 1 {
		t.Fatalf("expected 1 reply, got %v", childReplies["totalCount"])
	}

	// 8. Mark reply as answer.
	markQuery := `mutation($cid:ID!){markDiscussionCommentAsAnswer(input:{commentId:$cid}){discussion{number}}}`
	markRes := runDiscussionGQL(t, markQuery, map[string]interface{}{"cid": replyNodeID})
	if _, ok := markRes["markDiscussionCommentAsAnswer"]; !ok {
		t.Fatalf("expected mark answer payload, got %v", markRes)
	}

	// 9. Unmark answer.
	unmarkQuery := `mutation($cid:ID!){unmarkDiscussionCommentAsAnswer(input:{commentId:$cid}){discussion{number}}}`
	unmarkRes := runDiscussionGQL(t, unmarkQuery, map[string]interface{}{"cid": replyNodeID})
	if _, ok := unmarkRes["unmarkDiscussionCommentAsAnswer"]; !ok {
		t.Fatalf("expected unmark answer payload, got %v", unmarkRes)
	}

	// 10. Update comment.
	upComment := `mutation($cid:ID!){updateDiscussionComment(input:{commentId:$cid,body:"Updated comment"}){comment{body}}}`
	upRes := runDiscussionGQL(t, upComment, map[string]interface{}{"cid": commentNodeID})
	upPayload, _ := upRes["updateDiscussionComment"].(map[string]interface{})
	upCommentMap, _ := upPayload["comment"].(map[string]interface{})
	if upCommentMap["body"] != "Updated comment" {
		t.Fatalf("expected updated comment body, got %v", upCommentMap["body"])
	}

	// 11. Delete comment.
	delComment := `mutation($cid:ID!){deleteDiscussionComment(input:{commentId:$cid}){clientMutationId}}`
	delCommentRes := runDiscussionGQL(t, delComment, map[string]interface{}{"cid": commentNodeID})
	if _, ok := delCommentRes["deleteDiscussionComment"]; !ok {
		t.Fatalf("expected delete comment payload, got %v", delCommentRes)
	}

	// 12. Update discussion.
	upDisc := `mutation($did:ID!){updateDiscussion(input:{discussionId:$did,title:"Updated",body:"New body"}){discussion{title,body}}}`
	upDiscRes := runDiscussionGQL(t, upDisc, map[string]interface{}{"did": discNodeID})
	upDiscPayload, _ := upDiscRes["updateDiscussion"].(map[string]interface{})
	upDiscMap, _ := upDiscPayload["discussion"].(map[string]interface{})
	if upDiscMap["title"] != "Updated" {
		t.Fatalf("expected updated title, got %v", upDiscMap["title"])
	}

	// 13. Delete discussion.
	delDisc := `mutation($did:ID!){deleteDiscussion(input:{discussionId:$did}){clientMutationId}}`
	delDiscRes := runDiscussionGQL(t, delDisc, map[string]interface{}{"did": discNodeID})
	if _, ok := delDiscRes["deleteDiscussion"]; !ok {
		t.Fatalf("expected delete discussion payload, got %v", delDiscRes)
	}

	// 14. List now empty.
	finalRes := runDiscussionGQL(t, listQuery, map[string]interface{}{"owner": login, "name": name})
	finalRepo, _ := finalRes["repository"].(map[string]interface{})
	finalConn, _ := finalRepo["discussions"].(map[string]interface{})
	if tc, _ := finalConn["totalCount"].(float64); tc != 0 {
		t.Fatalf("expected 0 discussions after delete, got %v", finalConn["totalCount"])
	}
}

// TestDiscussionsGraphQL_NotFound verifies missing discussions return a typed
// NOT_FOUND error that gh CLI recognizes.
func TestDiscussionsGraphQL_NotFound(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "discussions-notfound",
	})
	repoData := decodeJSON(t, resp)
	owner, _ := repoData["owner"].(map[string]interface{})
	login, _ := owner["login"].(string)
	name, _ := repoData["name"].(string)

	query := `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){discussion(number:99){number}}}`
	res := runDiscussionGQLExpectErrors(t, query, map[string]interface{}{"owner": login, "name": name})
	errs, _ := res["errors"].([]interface{})
	if len(errs) == 0 {
		t.Fatalf("expected errors, got %v", res)
	}
	e, _ := errs[0].(map[string]interface{})
	if e["type"] != "NOT_FOUND" {
		t.Fatalf("expected NOT_FOUND error type, got %v", e["type"])
	}
}

// TestDiscussionsGraphQL_ReactionsShape verifies reaction groups return the
// expected GitHub-shaped list with zero counts when no reactions exist.
func TestDiscussionsGraphQL_ReactionsShape(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "discussions-reactions",
	})
	repoData := decodeJSON(t, resp)
	owner, _ := repoData["owner"].(map[string]interface{})
	login, _ := owner["login"].(string)
	name, _ := repoData["name"].(string)
	repoNodeID, _ := repoData["node_id"].(string)

	catQuery := `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){discussionCategories(first:1){nodes{id}}}}`
	catRes := runDiscussionGQL(t, catQuery, map[string]interface{}{"owner": login, "name": name})
	catNode := firstNode(catRes, "repository", "discussionCategories")
	catID, _ := catNode["id"].(string)

	create := `mutation($repo:ID!,$cat:ID!){createDiscussion(input:{repositoryId:$repo,categoryId:$cat,title:"React",body:"body"}){discussion{id,reactionGroups{content,users{totalCount}}}}}`
	createRes := runDiscussionGQL(t, create, map[string]interface{}{"repo": repoNodeID, "cat": catID})
	createPayload, _ := createRes["createDiscussion"].(map[string]interface{})
	disc, _ := createPayload["discussion"].(map[string]interface{})
	groups, _ := disc["reactionGroups"].([]interface{})
	if len(groups) != 8 {
		t.Fatalf("expected 8 reaction groups, got %d", len(groups))
	}
	firstGroup, _ := groups[0].(map[string]interface{})
	if firstGroup["content"] != "THUMBS_UP" {
		t.Fatalf("expected first group THUMBS_UP, got %v", firstGroup["content"])
	}
	users, _ := firstGroup["users"].(map[string]interface{})
	if tc, _ := users["totalCount"].(float64); tc != 0 {
		t.Fatalf("expected zero reaction count, got %v", tc)
	}
}

func runDiscussionGQL(t *testing.T, query string, variables map[string]interface{}) map[string]interface{} {
	t.Helper()
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query":     query,
		"variables": variables,
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if errs, ok := data["errors"]; ok && errs != nil {
		t.Fatalf("unexpected graphql errors: %v", errs)
	}
	d, _ := data["data"].(map[string]interface{})
	if d == nil {
		t.Fatalf("expected data, got %v", data)
	}
	return d
}

func runDiscussionGQLExpectErrors(t *testing.T, query string, variables map[string]interface{}) map[string]interface{} {
	t.Helper()
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query":     query,
		"variables": variables,
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	return decodeJSON(t, resp)
}

func firstNode(data map[string]interface{}, path ...string) map[string]interface{} {
	cur := data
	for _, key := range path {
		next, _ := cur[key].(map[string]interface{})
		cur = next
	}
	nodes, _ := cur["nodes"].([]interface{})
	if len(nodes) == 0 {
		return nil
	}
	node, _ := nodes[0].(map[string]interface{})
	return node
}
