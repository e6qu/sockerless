package sdktests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	github "github.com/google/go-github/v88/github"
)

func marketplaceJSON(t *testing.T, method, path, token string, body interface{}, out interface{}) int {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := rawHTTP.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode %s %s response (%s): %v", method, path, raw, err)
		}
	}
	return resp.StatusCode
}

func TestMarketplaceOfficialSoftwareDevelopmentKit(t *testing.T) {
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer sink.Close()
	var app struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
		PEM  string `json:"pem"`
	}
	if status := createGitHubAppViaManifest(t, uniqueName("marketplace-sdk"), map[string]string{"contents": "read"}, &app); status != http.StatusCreated {
		t.Fatalf("GitHub App manifest conversion status = %d", status)
	}
	settingsPath := "/settings/apps/" + app.Slug + "/marketplace"
	listing := map[string]interface{}{
		"name": "Marketplace SDK App", "description": "Official client Marketplace coverage",
		"setup_url": sink.URL + "/setup", "webhook_url": sink.URL, "webhook_secret": "sdk-secret",
		"webhook_content_type": "json", "webhook_active": true, "published": false,
	}
	if status := marketplaceJSON(t, http.MethodPut, settingsPath, adminToken, listing, nil); status != http.StatusCreated {
		t.Fatalf("create Marketplace listing status = %d", status)
	}
	var plan github.MarketplacePlan
	if status := marketplaceJSON(t, http.MethodPost, settingsPath+"/plans", adminToken,
		map[string]interface{}{"name": "SDK Team", "description": "Typed plan", "price_model": "FLAT_RATE", "monthly_price_in_cents": 1500, "yearly_price_in_cents": 15000, "state": "published"}, &plan); status != http.StatusCreated {
		t.Fatalf("create Marketplace plan status = %d", status)
	}
	listing["published"] = true
	if status := marketplaceJSON(t, http.MethodPut, settingsPath, adminToken, listing, nil); status != http.StatusOK {
		t.Fatalf("publish Marketplace listing status = %d", status)
	}
	var purchase map[string]interface{}
	if status := marketplaceJSON(t, http.MethodPost, "/ui-data/marketplace/listings/"+app.Slug+"/purchase", adminToken,
		map[string]interface{}{"plan_id": plan.GetID(), "billing_cycle": "monthly"}, &purchase); status != http.StatusCreated {
		t.Fatalf("purchase Marketplace plan status = %d", status)
	}

	appClient := ghClient(t, signAppJWT(t, app.PEM, app.ID))
	plans, _, err := appClient.Marketplace.ListPlans(ctx(), nil)
	if err != nil {
		t.Fatalf("Marketplace.ListPlans: %v", err)
	}
	if len(plans) != 1 || plans[0].GetID() != plan.GetID() || plans[0].GetName() != "SDK Team" {
		t.Fatalf("Marketplace.ListPlans = %+v", plans)
	}
	accounts, _, err := appClient.Marketplace.ListPlanAccountsForPlan(ctx(), plan.GetID(), nil)
	if err != nil {
		t.Fatalf("Marketplace.ListPlanAccountsForPlan: %v", err)
	}
	if len(accounts) != 1 || accounts[0].GetLogin() != "admin" || accounts[0].GetMarketplacePurchase().GetPlan().GetID() != plan.GetID() {
		t.Fatalf("Marketplace.ListPlanAccountsForPlan = %+v", accounts)
	}
	account, _, err := appClient.Marketplace.GetPlanAccountForAccount(ctx(), accounts[0].GetID())
	if err != nil {
		t.Fatalf("Marketplace.GetPlanAccountForAccount: %v", err)
	}
	if account.GetLogin() != "admin" || account.GetMarketplacePurchase().GetBillingCycle() != "monthly" {
		t.Fatalf("Marketplace.GetPlanAccountForAccount = %+v", account)
	}
	purchases, _, err := client.Marketplace.ListMarketplacePurchasesForUser(ctx(), nil)
	if err != nil {
		t.Fatalf("Marketplace.ListMarketplacePurchasesForUser: %v", err)
	}
	found := false
	for _, candidate := range purchases {
		found = found || candidate.GetPlan().GetID() == plan.GetID()
	}
	if !found {
		t.Fatalf("Marketplace.ListMarketplacePurchasesForUser omitted plan %d: %s", plan.GetID(), fmt.Sprint(purchases))
	}
}
