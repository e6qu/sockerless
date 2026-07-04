package bleephub

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestEmojisCatalog(t *testing.T) {
	resp := ghGet(t, "/api/v3/emojis", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("emojis status = %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if len(data) < 1900 {
		t.Fatalf("emoji catalog has %d entries, want the full gemoji set (>= 1900)", len(data))
	}
	thumbsUp, _ := data["+1"].(string)
	if !strings.HasPrefix(thumbsUp, testBaseURL+"/images/icons/emoji/unicode/1f44d.png") {
		t.Fatalf("+1 = %q, want a %s-hosted unicode/1f44d.png URL", thumbsUp, testBaseURL)
	}
	octocat, _ := data["octocat"].(string)
	if octocat != testBaseURL+"/images/icons/emoji/octocat.png?v8" {
		t.Fatalf("octocat = %q, want custom-emoji path on the instance host", octocat)
	}
}

func TestZenQuote(t *testing.T) {
	quotes := map[string]bool{}
	for _, q := range zenQuotes {
		quotes[q] = true
	}
	for i := 0; i < 5; i++ {
		resp := ghGet(t, "/api/v3/zen", defaultToken)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("zen status = %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
			t.Fatalf("zen content-type = %q", ct)
		}
		if !quotes[string(body)] {
			t.Fatalf("zen quote %q not in GitHub's zen quote set", body)
		}
	}
}

// TestOctocatSpeech verifies the ASCII art is byte-identical to real
// GitHub's GET /octocat output for a valid `s` parameter.
func TestOctocatSpeech(t *testing.T) {
	resp := ghGet(t, "/api/v3/octocat?s=Hello", defaultToken)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("octocat status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/octocat-stream" {
		t.Fatalf("octocat content-type = %q", ct)
	}
	// Captured verbatim from GET https://api.github.com/octocat?s=Hello.
	want := "\n" +
		"               MMM.           .MMM\n" +
		"               MMMMMMMMMMMMMMMMMMM\n" +
		"               MMMMMMMMMMMMMMMMMMM      _______\n" +
		"              MMMMMMMMMMMMMMMMMMMMM    |       |\n" +
		"             MMMMMMMMMMMMMMMMMMMMMMM   | Hello |\n" +
		"            MMMMMMMMMMMMMMMMMMMMMMMM   |_   ___|\n" +
		"            MMMM::- -:::::::- -::MMMM    |/\n" +
		"             MM~:~ 00~:::::~ 00~:~MM\n" +
		"        .. MMMMM::.00:::+:::.00::MMMMM ..\n" +
		"              .MM::::: ._. :::::MM.\n" +
		"                 MMMM;:::::;MMMM\n" +
		"          -MM        MMMMMMM\n" +
		"          ^  M+     MMMMMMMMM\n" +
		"              MMMMMMM MM MM MM\n" +
		"                   MM MM MM MM\n" +
		"                   MM MM MM MM\n" +
		"                .~~MM~MM~MM~MM~~.\n" +
		"             ~~~~MM:~MM~~~MM~:MM~~~~\n" +
		"            ~~~~~~==~==~~~==~==~~~~~~\n" +
		"             ~~~~~~==~==~==~==~~~~~~\n" +
		"                 :~==~==~==~==~~\n"
	if string(body) != want {
		t.Fatalf("octocat art mismatch:\ngot:\n%s\nwant:\n%s", body, want)
	}
}

// TestOctocatInvalidSpeechFallsBackToZen mirrors real GitHub: an `s` value
// containing characters outside GitHub's accepted set (e.g. a period) is
// ignored and a random zen quote fills the speech bubble.
func TestOctocatInvalidSpeechFallsBackToZen(t *testing.T) {
	resp := ghGet(t, "/api/v3/octocat?s=a.b", defaultToken)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("octocat status = %d", resp.StatusCode)
	}
	if strings.Contains(string(body), "| a.b |") {
		t.Fatal("octocat rendered a speech value real GitHub rejects")
	}
	found := false
	for _, q := range zenQuotes {
		if strings.Contains(string(body), "| "+q+" |") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("octocat bubble does not contain a zen quote:\n%s", body)
	}
}

func TestRESTAPIVersions(t *testing.T) {
	resp := ghGet(t, "/api/v3/versions", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("versions status = %d", resp.StatusCode)
	}
	var versions []string
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		t.Fatalf("decode versions: %v", err)
	}
	if len(versions) != 1 || versions[0] != "2022-11-28" {
		t.Fatalf("versions = %v, want [2022-11-28]", versions)
	}
}

func TestCredentialsRevoke(t *testing.T) {
	user := createTestUser(t, "revoke-target")
	token := testServer.store.CreateToken(user.ID, "repo").Value

	// The token works before revocation.
	resp := ghGet(t, "/api/v3/user", token)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("pre-revocation GET /user = %d", resp.StatusCode)
	}

	// Revoke it (the endpoint itself needs no authentication, matching
	// GitHub) along with an unknown credential, which is silently accepted.
	resp = ghPost(t, "/api/v3/credentials/revoke", "", map[string]interface{}{
		"credentials": []string{token, "ghp_doesnotexist000000000000000000000000"},
	})
	resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Fatalf("revoke status = %d, want 202", resp.StatusCode)
	}

	resp = ghGet(t, "/api/v3/user", token)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("post-revocation GET /user = %d, want 401", resp.StatusCode)
	}
}

func TestCredentialsRevokeValidation(t *testing.T) {
	resp := ghPost(t, "/api/v3/credentials/revoke", "", map[string]interface{}{
		"credentials": []string{},
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("empty credentials status = %d, want 422", resp.StatusCode)
	}
}
