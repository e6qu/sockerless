package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type marketplaceTestListing struct {
	slug       string
	appID      int
	appJWT     string
	freePlanID int
	paidPlanID int
}

func publishMarketplaceOAuthApp(t *testing.T, name, slug, webhookURL string) (clientID, clientSecret string, planID int) {
	t.Helper()
	form := url.Values{
		"name": {name}, "description": {"Marketplace OAuth integration"},
		"url": {"https://example.test/oauth-app"}, "callback_url": {"https://example.test/oauth/callback"},
	}
	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/settings/oauth-apps/new", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "token "+defaultToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	created := requireMarketplaceStatus(t, mustDoMarketplaceRequest(t, req), http.StatusCreated)
	clientID, clientSecret = created["client_id"].(string), created["client_secret"].(string)
	settingsPath := "/settings/oauth-apps/" + clientID + "/marketplace"
	body := map[string]interface{}{
		"slug": slug, "name": name, "description": "A real OAuth App Marketplace integration",
		"installation_url": webhookURL + "/install", "webhook_url": webhookURL,
		"webhook_secret": "oauth-marketplace-secret", "webhook_content_type": "json", "webhook_active": true,
		"published": false,
	}
	requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodPut, settingsPath, "token "+defaultToken, body), http.StatusCreated)
	plan := requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodPost, settingsPath+"/plans", "token "+defaultToken,
		map[string]interface{}{"name": "OAuth Free", "description": "OAuth plan", "price_model": "FREE", "state": "published"}), http.StatusCreated)
	body["published"] = true
	requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodPut, settingsPath, "token "+defaultToken, body), http.StatusOK)
	return clientID, clientSecret, int(plan["id"].(float64))
}

func mustDoMarketplaceRequest(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func marketplaceRequest(t *testing.T, method, path, authorization string, body interface{}) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, testBaseURL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func requireMarketplaceStatus(t *testing.T, resp *http.Response, status int) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != status {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("Marketplace response status = %d, want %d: %s", resp.StatusCode, status, raw)
	}
	if status == http.StatusNoContent {
		return nil
	}
	var value map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func publishMarketplaceGitHubApp(t *testing.T, name, webhookURL string) marketplaceTestListing {
	t.Helper()
	app := createGitHubAppViaManifest(t, name, map[string]string{"contents": "read"}, []string{"push"})
	slug := app["slug"].(string)
	settingsPath := "/settings/apps/" + slug + "/marketplace"
	listingBody := map[string]interface{}{
		"name": name, "description": "A production Marketplace integration",
		"full_description": "Automates a real GitHub workflow from installation through billing.",
		"setup_url":        webhookURL + "/setup", "webhook_url": webhookURL,
		"webhook_secret": "marketplace-secret", "webhook_content_type": "json", "webhook_active": true,
		"published": false,
	}
	requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodPut, settingsPath, "token "+defaultToken, listingBody), http.StatusCreated)

	createPlan := func(body map[string]interface{}) int {
		created := requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodPost, settingsPath+"/plans", "token "+defaultToken, body), http.StatusCreated)
		return int(created["id"].(float64))
	}
	freePlanID := createPlan(map[string]interface{}{
		"name": "Community", "description": "For open source projects", "price_model": "FREE",
		"monthly_price_in_cents": 0, "yearly_price_in_cents": 0, "state": "published",
		"bullets": []string{"Unlimited public repositories", "Community support"},
	})
	paidPlanID := createPlan(map[string]interface{}{
		"name": "Team", "description": "For growing teams", "price_model": "FLAT_RATE",
		"monthly_price_in_cents": 1200, "yearly_price_in_cents": 12000, "has_free_trial": true,
		"state": "published", "bullets": []string{"Private repositories", "Priority support"},
	})
	listingBody["published"] = true
	published := requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodPut, settingsPath, "token "+defaultToken, listingBody), http.StatusOK)
	if published["slug"] != slug || published["published"] != true {
		t.Fatalf("published Marketplace listing = %v", published)
	}
	appJWT, err := signAppJWT(app["pem"].(string), int(app["id"].(float64)), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return marketplaceTestListing{slug: slug, appID: int(app["id"].(float64)), appJWT: appJWT, freePlanID: freePlanID, paidPlanID: paidPlanID}
}

func TestMarketplacePublisherBuyerAndBillingLifecycle(t *testing.T) {
	type delivery struct {
		event, action, signature, targetType string
		body                                 map[string]interface{}
	}
	deliveries := make(chan delivery, 4)
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		deliveries <- delivery{event: r.Header.Get("X-GitHub-Event"), action: r.Header.Get("X-GitHub-Hook-ID"),
			signature: r.Header.Get("X-Hub-Signature-256"), targetType: r.Header.Get("X-GitHub-Hook-Installation-Target-Type"), body: body}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer sink.Close()

	listing := publishMarketplaceGitHubApp(t, "Marketplace Lifecycle App", sink.URL)
	buyer, buyerToken := userSurfaceUser(t, "marketplace-lifecycle-buyer")
	nextPurchaseDelivery := func() delivery {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			select {
			case got := <-deliveries:
				if got.event == "marketplace_purchase" {
					return got
				}
			case <-deadline:
				t.Fatal("timed out waiting for Marketplace purchase webhook")
			}
		}
	}

	listResp := marketplaceRequest(t, http.MethodGet, "/ui-data/marketplace/listings", "token "+buyerToken, nil)
	listings := decodeJSONArray(t, listResp)
	found := false
	for _, row := range listings {
		if row["slug"] == listing.slug {
			found = true
			if _, leaked := row["webhook_url"]; leaked {
				t.Fatalf("public Marketplace listing leaked publisher webhook settings: %v", row)
			}
		}
	}
	if !found {
		t.Fatalf("Marketplace browse did not include %q: %v", listing.slug, listings)
	}

	purchase := requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodPost,
		"/ui-data/marketplace/listings/"+listing.slug+"/purchase", "token "+buyerToken,
		map[string]interface{}{"plan_id": listing.freePlanID, "billing_cycle": "monthly"}), http.StatusCreated)
	if purchase["account_login"] != buyer.Login || purchase["setup_url"] == nil {
		t.Fatalf("Marketplace purchase handoff = %v", purchase)
	}
	marketplacePurchase := purchase["marketplace_purchase"].(map[string]interface{})
	if marketplacePurchase["is_installed"] != true {
		t.Fatalf("Marketplace purchase did not install GitHub App: %v", marketplacePurchase)
	}
	installationID := *testServer.store.GetMarketplacePurchase(listing.slug, "User", buyer.ID).InstallationID
	installation := testServer.store.GetInstallation(installationID)
	if installation == nil || installation.AppID != listing.appID || installation.TargetID != buyer.ID {
		t.Fatalf("Marketplace installation = %#v", installation)
	}

	got := nextPurchaseDelivery()
	if got.body["action"] != "purchased" || !strings.HasPrefix(got.signature, "sha256=") {
		t.Fatalf("purchase webhook = %+v", got)
	}
	if got.targetType != "" {
		t.Fatalf("Marketplace webhook advertised installation target %q", got.targetType)
	}
	if got.action == "" || got.action == "0" {
		t.Fatalf("Marketplace webhook hook id = %q", got.action)
	}

	for _, path := range []string{"/api/v3/marketplace_listing/plans", "/api/v3/marketplace_listing/stubbed/plans"} {
		plans := decodeJSONArray(t, marketplaceRequest(t, http.MethodGet, path, "Bearer "+listing.appJWT, nil))
		if len(plans) != 2 || plans[0]["url"] == nil || plans[0]["accounts_url"] == nil {
			t.Fatalf("%s plans = %v", path, plans)
		}
	}
	accountPath := "/api/v3/marketplace_listing/accounts/" + strconv.Itoa(buyer.ID)
	account := requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodGet, accountPath, "Bearer "+listing.appJWT, nil), http.StatusOK)
	if account["login"] != buyer.Login {
		t.Fatalf("publisher account lookup = %v", account)
	}

	changed := requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodPatch,
		"/ui-data/marketplace/listings/"+listing.slug+"/subscription", "token "+buyerToken,
		map[string]interface{}{"plan_id": listing.paidPlanID, "billing_cycle": "monthly"}), http.StatusOK)
	if changed["marketplace_pending_change"] != nil {
		t.Fatalf("paid upgrade was not immediate: %v", changed)
	}
	got = nextPurchaseDelivery()
	if got.body["action"] != "changed" || got.body["previous_marketplace_purchase"] == nil {
		t.Fatalf("change webhook = %+v", got)
	}
	var deliveryRows []map[string]interface{}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp := marketplaceRequest(t, http.MethodGet,
			"/settings/apps/"+listing.slug+"/marketplace/deliveries", "token "+defaultToken, nil)
		deliveryRows = decodeJSONArray(t, resp)
		if len(deliveryRows) >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(deliveryRows) < 3 || deliveryRows[0]["event"] != "marketplace_purchase" {
		t.Fatalf("Marketplace delivery history = %v", deliveryRows)
	}

	cancelled := requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodDelete,
		"/ui-data/marketplace/listings/"+listing.slug+"/subscription", "token "+buyerToken, nil), http.StatusAccepted)
	if cancelled["marketplace_pending_change"] == nil {
		t.Fatalf("paid cancellation did not wait for billing boundary: %v", cancelled)
	}

	userPurchases := decodeJSONArray(t, marketplaceRequest(t, http.MethodGet, "/api/v3/user/marketplace_purchases", "token "+buyerToken, nil))
	if len(userPurchases) != 1 || userPurchases[0]["plan"].(map[string]interface{})["id"] != float64(listing.paidPlanID) {
		t.Fatalf("user Marketplace purchases = %v", userPurchases)
	}

	unauthorized := marketplaceRequest(t, http.MethodGet, "/api/v3/marketplace_listing/plans", "token "+defaultToken, nil)
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("personal access token Marketplace publisher status = %d, want 401", unauthorized.StatusCode)
	}
}

func TestMarketplacePublicWorkflowsHaveNoOperatorIngress(t *testing.T) {
	for _, route := range testServer.routePatterns {
		if strings.Contains(route, "/internal/marketplace") {
			t.Fatalf("Marketplace operator ingress route remained registered: %s", route)
		}
	}
	harness, err := os.ReadFile("test/run-gh-test.sh")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(harness, []byte("/internal/marketplace")) {
		t.Fatal("official GitHub command-line interface Marketplace coverage used operator ingress")
	}
}

func TestMarketplaceOrganizationIdentityDoesNotCollideWithUserID(t *testing.T) {
	server := newTestServer()
	admin := server.store.LookupUserByLogin("admin")
	org := server.store.CreateOrg(admin, "same-id-marketplace-org", "Same ID Organization", "")
	if org.ID != admin.ID {
		t.Fatalf("test requires colliding user/organization ids, got user=%d org=%d", admin.ID, org.ID)
	}
	now := time.Now().UTC()
	listing := &MarketplaceListing{Slug: "identity-app", Name: "Identity App", Description: "Identity", CreatedAt: now, UpdatedAt: now}
	if err := server.store.SaveMarketplaceListing(listing); err != nil {
		t.Fatal(err)
	}
	plan, err := server.store.CreateMarketplacePlan(&MarketplacePlan{ListingSlug: listing.Slug, Name: "Free", PriceModel: "FREE", State: "published"})
	if err != nil {
		t.Fatal(err)
	}
	row := server.marketplaceAccountJSON(&MarketplacePurchase{ListingSlug: listing.Slug, AccountID: org.ID, AccountType: "Organization", PlanID: plan.ID}, plan, "http://bleephub.test")
	if row["type"] != "Organization" || row["login"] != org.Login {
		t.Fatalf("organization Marketplace identity = %v", row)
	}
}

func TestMarketplaceNotFoundIsScopedToAuthenticatedPublisher(t *testing.T) {
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer sink.Close()
	listing := publishMarketplaceGitHubApp(t, "Marketplace Scope App", sink.URL)
	app := testServer.store.GetApp(listing.appID)
	resp := marketplaceRequest(t, http.MethodGet, "/api/v3/marketplace_listing/plans",
		"Basic "+basicHeader(app.ClientID, app.ClientSecret), nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GitHub App Basic Marketplace authentication status = %d, want 401", resp.StatusCode)
	}

	resp = marketplaceRequest(t, http.MethodGet, "/api/v3/marketplace_listing/plans/999/accounts", "Bearer "+listing.appJWT, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown plan accounts status = %d, want 404", resp.StatusCode)
	}
	resp = marketplaceRequest(t, http.MethodGet, "/api/v3/marketplace_listing/stubbed/accounts/999999", "Bearer "+listing.appJWT, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("purchase-less account status = %d, want 404", resp.StatusCode)
	}
}

func TestMarketplacePublisherEditsPlansAndDeletesUnusedListing(t *testing.T) {
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer sink.Close()
	listing := publishMarketplaceGitHubApp(t, "Marketplace Publisher App", sink.URL)
	settingsPath := "/settings/apps/marketplace-publisher-app/marketplace"
	updated := requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodPut,
		settingsPath+"/plans/"+strconv.Itoa(listing.paidPlanID), "token "+defaultToken,
		map[string]interface{}{"name": "Enterprise", "description": "Updated publisher plan", "price_model": "FLAT_RATE", "monthly_price_in_cents": 2400, "yearly_price_in_cents": 24000, "state": "published", "bullets": []string{"Enterprise support"}}), http.StatusOK)
	if updated["name"] != "Enterprise" || updated["number"] != float64(listing.paidPlanID) {
		t.Fatalf("updated Marketplace plan = %v", updated)
	}
	requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodDelete,
		settingsPath+"/plans/"+strconv.Itoa(listing.freePlanID), "token "+defaultToken, nil), http.StatusNoContent)
	requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodDelete, settingsPath, "token "+defaultToken, nil), http.StatusNoContent)
	resp := marketplaceRequest(t, http.MethodGet, settingsPath, "token "+defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted Marketplace listing status = %d, want 404", resp.StatusCode)
	}
}

func TestMarketplaceOAuthAppUsesListingWebhookAndIndependentSubscriptionIdentity(t *testing.T) {
	deliveries := make(chan map[string]interface{}, 2)
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-GitHub-Event") != "marketplace_purchase" || !strings.HasPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256=") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		deliveries <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer sink.Close()

	clientID, clientSecret, oauthPlanID := publishMarketplaceOAuthApp(t, "Marketplace OAuth App", "marketplace-oauth-app", sink.URL)
	githubListing := publishMarketplaceGitHubApp(t, "Marketplace Identity App", sink.URL)
	buyer, token := userSurfaceUser(t, "marketplace-multi-app-buyer")

	for slug, planID := range map[string]int{"marketplace-oauth-app": oauthPlanID, githubListing.slug: githubListing.freePlanID} {
		requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodPost, "/ui-data/marketplace/listings/"+slug+"/purchase", "token "+token,
			map[string]interface{}{"plan_id": planID, "billing_cycle": "monthly"}), http.StatusCreated)
	}
	subscriptions := decodeJSONArray(t, marketplaceRequest(t, http.MethodGet, "/ui-data/marketplace/subscriptions", "token "+token, nil))
	if len(subscriptions) != 2 {
		t.Fatalf("independent Marketplace subscriptions = %v", subscriptions)
	}
	seen := map[string]bool{}
	for _, subscription := range subscriptions {
		listing := subscription["listing"].(map[string]interface{})
		seen[listing["slug"].(string)] = true
	}
	if !seen["marketplace-oauth-app"] || !seen[githubListing.slug] {
		t.Fatalf("Marketplace subscription listing identities = %v", seen)
	}

	basic := "Basic " + basicHeader(clientID, clientSecret)
	plans := decodeJSONArray(t, marketplaceRequest(t, http.MethodGet, "/api/v3/marketplace_listing/plans", basic, nil))
	if len(plans) != 1 || plans[0]["id"] != float64(oauthPlanID) {
		t.Fatalf("OAuth App scoped Marketplace plans = %v", plans)
	}
	account := requireMarketplaceStatus(t, marketplaceRequest(t, http.MethodGet,
		"/api/v3/marketplace_listing/accounts/"+strconv.Itoa(buyer.ID), basic, nil), http.StatusOK)
	if account["login"] != buyer.Login || account["marketplace_purchase"].(map[string]interface{})["is_installed"] != false {
		t.Fatalf("OAuth App Marketplace account = %v", account)
	}

	for range 2 {
		select {
		case delivery := <-deliveries:
			if delivery["action"] != "purchased" {
				t.Fatalf("Marketplace purchase delivery = %v", delivery)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for independent Marketplace purchase webhook")
		}
	}
}
