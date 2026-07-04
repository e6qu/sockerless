package bleephub

import (
	"strings"
	"testing"
)

func TestCodesOfConductList(t *testing.T) {
	resp := ghGet(t, "/api/v3/codes_of_conduct", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list status = %d", resp.StatusCode)
	}
	list := decodeJSONArray(t, resp)
	if len(list) != 2 {
		t.Fatalf("catalog has %d entries, want 2", len(list))
	}
	keys := map[string]bool{}
	for _, c := range list {
		key, _ := c["key"].(string)
		keys[key] = true
		if c["url"] != testBaseURL+"/api/v3/codes_of_conduct/"+key {
			t.Fatalf("url = %v", c["url"])
		}
		if _, hasBody := c["body"]; hasBody {
			t.Fatal("list entries must not carry the body")
		}
	}
	if !keys["contributor_covenant"] || !keys["citizen_code_of_conduct"] {
		t.Fatalf("catalog keys = %v", keys)
	}
}

func TestCodesOfConductGetByKey(t *testing.T) {
	resp := ghGet(t, "/api/v3/codes_of_conduct/contributor_covenant", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("get status = %d", resp.StatusCode)
	}
	coc := decodeJSON(t, resp)
	if coc["name"] != "Contributor Covenant" {
		t.Fatalf("name = %v", coc["name"])
	}
	body, _ := coc["body"].(string)
	if !strings.Contains(body, "# Contributor Covenant Code of Conduct") ||
		!strings.Contains(body, "Our Pledge") {
		t.Fatal("body is not the real Contributor Covenant text")
	}

	resp = ghGet(t, "/api/v3/codes_of_conduct/citizen_code_of_conduct", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("citizen get status = %d", resp.StatusCode)
	}
	coc = decodeJSON(t, resp)
	body, _ = coc["body"].(string)
	if !strings.Contains(body, "# Citizen Code of Conduct") {
		t.Fatal("body is not the real Citizen Code of Conduct text")
	}

	resp = ghGet(t, "/api/v3/codes_of_conduct/no_such_code", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown key status = %d, want 404", resp.StatusCode)
	}
}
