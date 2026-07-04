package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"testing"
)

func TestMarketplaceListingPlansShape(t *testing.T) {
	for _, path := range []string{
		"/api/v3/marketplace_listing/plans",
		"/api/v3/marketplace_listing/stubbed/plans",
	} {
		resp := ghGet(t, path, defaultToken)
		if resp.StatusCode != 200 {
			resp.Body.Close()
			t.Fatalf("%s status = %d", path, resp.StatusCode)
		}
		plans := decodeJSONArray(t, resp)
		if len(plans) == 0 {
			t.Fatalf("%s returned no plans", path)
		}
		plan := plans[0]
		if plan["name"] != "Free" || plan["price_model"] != "FREE" {
			t.Fatalf("%s plan = %v", path, plan)
		}
		wantURL := testBaseURL + "/api/v3/marketplace_listing/plans/1"
		if plan["url"] != wantURL || plan["accounts_url"] != wantURL+"/accounts" {
			t.Fatalf("%s plan URLs: url=%v accounts_url=%v", path, plan["url"], plan["accounts_url"])
		}
		if plan["number"] != float64(1) {
			t.Fatalf("%s plan number = %v", path, plan["number"])
		}
	}
}

func TestMarketplacePlanAccountsAndStubbedAccount(t *testing.T) {
	user := createTestUser(t, "marketplace-buyer")

	// Seed a real purchase through the internal seeding path.
	resp, err := authedPost("/internal/marketplace/purchases", "application/json",
		bytes.NewReader(mustJSON(map[string]interface{}{
			"account":       user.Login,
			"plan_id":       1,
			"billing_cycle": "monthly",
			"unit_count":    1,
		})))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("seed purchase: %d %s", resp.StatusCode, b)
	}
	var seeded map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&seeded); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	for _, path := range []string{
		"/api/v3/marketplace_listing/plans/1/accounts",
		"/api/v3/marketplace_listing/stubbed/plans/1/accounts",
	} {
		listResp := ghGet(t, path, defaultToken)
		if listResp.StatusCode != 200 {
			listResp.Body.Close()
			t.Fatalf("%s status = %d", path, listResp.StatusCode)
		}
		accounts := decodeJSONArray(t, listResp)
		var found map[string]interface{}
		for _, a := range accounts {
			if a["login"] == user.Login {
				found = a
			}
		}
		if found == nil {
			t.Fatalf("%s missing seeded purchaser", path)
		}
		if found["type"] != "User" || found["id"] != float64(user.ID) {
			t.Fatalf("%s account shape: %v", path, found)
		}
		purchase, _ := found["marketplace_purchase"].(map[string]interface{})
		if purchase == nil || purchase["billing_cycle"] != "monthly" {
			t.Fatalf("%s purchase shape: %v", path, found["marketplace_purchase"])
		}
		plan, _ := purchase["plan"].(map[string]interface{})
		if plan == nil || plan["name"] != "Free" || plan["accounts_url"] == nil {
			t.Fatalf("%s nested plan: %v", path, purchase["plan"])
		}
	}

	// GET the account directly (stubbed + production variants).
	for _, path := range []string{
		"/api/v3/marketplace_listing/accounts/" + strconv.Itoa(user.ID),
		"/api/v3/marketplace_listing/stubbed/accounts/" + strconv.Itoa(user.ID),
	} {
		acctResp := ghGet(t, path, defaultToken)
		if acctResp.StatusCode != 200 {
			acctResp.Body.Close()
			t.Fatalf("%s status = %d", path, acctResp.StatusCode)
		}
		acct := decodeJSON(t, acctResp)
		if acct["login"] != user.Login || acct["id"] != float64(user.ID) {
			t.Fatalf("%s account = %v", path, acct)
		}
	}
}

func TestMarketplaceNotFound(t *testing.T) {
	// Unknown plan → 404 for its accounts listing.
	resp := ghGet(t, "/api/v3/marketplace_listing/plans/999/accounts", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown plan accounts status = %d, want 404", resp.StatusCode)
	}
	// An account with no purchase → 404, not a fabricated purchase.
	resp = ghGet(t, "/api/v3/marketplace_listing/stubbed/accounts/999999", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("purchase-less account status = %d, want 404", resp.StatusCode)
	}
}
