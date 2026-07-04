package bleephub

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func postMarkdown(t *testing.T, body interface{}) (int, string) {
	t.Helper()
	resp := ghPost(t, "/api/v3/markdown", defaultToken, body)
	defer resp.Body.Close()
	rendered, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(rendered)
}

func TestMarkdownRender(t *testing.T) {
	status, html := postMarkdown(t, map[string]interface{}{
		"text": "# Title\n\nSome **bold** text and ~~gone~~.",
	})
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	for _, want := range []string{"<h1>Title</h1>", "<strong>bold</strong>", "<del>gone</del>"} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered HTML missing %q:\n%s", want, html)
		}
	}
	// markdown mode renders soft line breaks as newlines, not <br>.
	status, html = postMarkdown(t, map[string]interface{}{"text": "line1\nline2"})
	if status != 200 || strings.Contains(html, "<br") {
		t.Fatalf("markdown mode must not hard-wrap; status=%d html=%s", status, html)
	}
}

func TestMarkdownGFMModeReferences(t *testing.T) {
	createTestUser(t, "md-mention-user")
	repoKey := createTestRepo(t)

	// A real issue for #1 to resolve against.
	resp := ghPost(t, "/api/v3/repos/"+repoKey+"/issues", defaultToken, map[string]interface{}{
		"title": "markdown reference target",
	})
	issue := decodeJSONWithStatus(t, resp, 201)
	num := int(issue["number"].(float64))
	if num != 1 {
		t.Fatalf("expected issue number 1, got %d", num)
	}

	status, html := postMarkdown(t, map[string]interface{}{
		"text":    "Hi @md-mention-user, see #1 and #999 and @no-such-user-xyz.\n\n`code #1`\n\nline1\nline2",
		"mode":    "gfm",
		"context": repoKey,
	})
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(html, `class="user-mention" href="`+testBaseURL+`/md-mention-user"`) {
		t.Fatalf("existing user mention not linked:\n%s", html)
	}
	if strings.Contains(html, "no-such-user-xyz</a>") {
		t.Fatalf("nonexistent user mention must not be linked:\n%s", html)
	}
	if !strings.Contains(html, `href="`+testBaseURL+`/`+repoKey+`/issues/1"`) {
		t.Fatalf("existing issue reference not linked:\n%s", html)
	}
	if strings.Contains(html, "issues/999") {
		t.Fatalf("nonexistent issue reference must not be linked:\n%s", html)
	}
	// References inside code spans stay literal.
	if !strings.Contains(html, "<code") || strings.Contains(html, `<code class="language-`) {
		t.Fatalf("expected a code span:\n%s", html)
	}
	if strings.Contains(html, `<code>code <a`) {
		t.Fatalf("code span content must not be linkified:\n%s", html)
	}
	// gfm mode renders hard line breaks.
	if !strings.Contains(html, "<br") {
		t.Fatalf("gfm mode must hard-wrap:\n%s", html)
	}
}

func TestMarkdownValidation(t *testing.T) {
	status, _ := postMarkdown(t, map[string]interface{}{"mode": "gfm"})
	if status != 422 {
		t.Fatalf("missing text status = %d, want 422", status)
	}
	status, _ = postMarkdown(t, map[string]interface{}{"text": "x", "mode": "bogus"})
	if status != 422 {
		t.Fatalf("invalid mode status = %d, want 422", status)
	}
}

func TestMarkdownRaw(t *testing.T) {
	req, err := http.NewRequest("POST", testBaseURL+"/api/v3/markdown/raw", strings.NewReader("**raw** and @admin"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "token "+defaultToken)
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("raw status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("raw content-type = %q", ct)
	}
	html := string(body)
	if !strings.Contains(html, "<strong>raw</strong>") {
		t.Fatalf("raw render missing bold:\n%s", html)
	}
	// Raw rendering is markdown mode: no mention linkification.
	if strings.Contains(html, "user-mention") {
		t.Fatalf("markdown/raw must not linkify mentions:\n%s", html)
	}
}
