package bleephub

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var marketplaceSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (s *Server) registerGHMarketplaceRoutes() {
	s.route("GET /settings/apps/{publisher}/marketplace", s.handleGetMarketplaceListingSettings)
	s.route("PUT /settings/apps/{publisher}/marketplace", s.handlePutMarketplaceListingSettings)
	s.route("POST /settings/apps/{publisher}/marketplace/plans", s.handleCreateMarketplacePlanSettings)
	s.route("PUT /settings/apps/{publisher}/marketplace/plans/{plan_id}", s.handleUpdateMarketplacePlanSettings)
	s.route("DELETE /settings/apps/{publisher}/marketplace/plans/{plan_id}", s.handleDeleteMarketplacePlanSettings)
	s.route("DELETE /settings/apps/{publisher}/marketplace", s.handleDeleteMarketplaceListingSettings)
	s.route("GET /settings/apps/{publisher}/marketplace/deliveries", s.handleListMarketplaceDeliveriesSettings)
	s.route("GET /settings/oauth-apps/{publisher}/marketplace", s.handleGetMarketplaceListingSettings)
	s.route("PUT /settings/oauth-apps/{publisher}/marketplace", s.handlePutMarketplaceListingSettings)
	s.route("POST /settings/oauth-apps/{publisher}/marketplace/plans", s.handleCreateMarketplacePlanSettings)
	s.route("PUT /settings/oauth-apps/{publisher}/marketplace/plans/{plan_id}", s.handleUpdateMarketplacePlanSettings)
	s.route("DELETE /settings/oauth-apps/{publisher}/marketplace/plans/{plan_id}", s.handleDeleteMarketplacePlanSettings)
	s.route("DELETE /settings/oauth-apps/{publisher}/marketplace", s.handleDeleteMarketplaceListingSettings)
	s.route("GET /settings/oauth-apps/{publisher}/marketplace/deliveries", s.handleListMarketplaceDeliveriesSettings)

	s.route("GET /ui-data/marketplace/listings", s.handleListMarketplaceBrowser)
	s.route("GET /ui-data/settings/apps/{publisher}/marketplace", s.handleGetMarketplaceListingBrowserSettings)
	s.route("GET /ui-data/marketplace/listings/{listing_slug}", s.handleGetMarketplaceBrowser)
	s.route("GET /ui-data/marketplace/accounts", s.handleListMarketplaceBuyerAccounts)
	s.route("GET /ui-data/marketplace/subscriptions", s.handleListMarketplaceSubscriptionsBrowser)
	s.route("POST /ui-data/marketplace/listings/{listing_slug}/purchase", s.handlePurchaseMarketplaceBrowser)
	s.route("PATCH /ui-data/marketplace/listings/{listing_slug}/subscription", s.handleChangeMarketplaceSubscriptionBrowser)
	s.route("DELETE /ui-data/marketplace/listings/{listing_slug}/subscription", s.handleCancelMarketplaceSubscriptionBrowser)
}

func (s *Server) handleGetMarketplaceListingBrowserSettings(w http.ResponseWriter, r *http.Request) {
	user, _ := s.marketplaceBrowserUser(w, r)
	if user == nil {
		return
	}
	publisher, ok := s.marketplacePublisherForSettings(w, r, user)
	if !ok {
		return
	}
	listing := s.marketplaceListingForSettingsPublisher(publisher)
	var value interface{}
	if listing != nil {
		value = marketplaceListingSettingsJSON(listing, s.store.ListMarketplacePlans(listing.Slug, false), s.baseURL(r))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"listing": value})
}

func (s *Server) handleListMarketplaceBuyerAccounts(w http.ResponseWriter, r *http.Request) {
	user, _ := s.marketplaceBrowserUser(w, r)
	if user == nil {
		return
	}
	rows := []map[string]interface{}{{"id": user.ID, "login": user.Login, "type": "User", "avatar_url": user.AvatarURL}}
	for _, org := range s.store.ListOrgsByUser(user.ID) {
		if canAdminOrg(s.store, user, org) {
			rows = append(rows, map[string]interface{}{"id": org.ID, "login": org.Login, "type": "Organization", "avatar_url": org.AvatarURL})
		}
	}
	writeJSON(w, http.StatusOK, rows)
}

type marketplacePublisher struct {
	githubApp *App
	oauthApp  *OAuthApp
}

func (publisher marketplacePublisher) matches(listing *MarketplaceListing) bool {
	if listing == nil {
		return false
	}
	if publisher.githubApp != nil {
		return listing.GitHubAppID == publisher.githubApp.ID && listing.OAuthAppClientID == ""
	}
	return publisher.oauthApp != nil && listing.OAuthAppClientID == publisher.oauthApp.ClientID && listing.GitHubAppID == 0
}

func (s *Server) marketplacePublisherForSettings(w http.ResponseWriter, r *http.Request, user *User) (marketplacePublisher, bool) {
	id := r.PathValue("publisher")
	if strings.HasPrefix(r.URL.Path, "/settings/oauth-apps/") {
		app := s.store.GetOAuthApp(id)
		if app == nil || (app.OwnerID != user.ID && !user.SiteAdmin) {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return marketplacePublisher{}, false
		}
		return marketplacePublisher{oauthApp: app}, true
	}
	app := s.store.GetAppBySlug(id)
	if app == nil || (app.OwnerID != user.ID && !user.SiteAdmin) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return marketplacePublisher{}, false
	}
	return marketplacePublisher{githubApp: app}, true
}

func (s *Server) marketplaceListingForPublisher(w http.ResponseWriter, r *http.Request) *MarketplaceListing {
	publisher := marketplacePublisher{githubApp: ghAppFromContext(r.Context())}
	if publisher.githubApp == nil {
		scheme, credential := authScheme(r.Header.Get("Authorization"))
		if scheme == "basic" {
			raw, err := base64.StdEncoding.DecodeString(credential)
			if err == nil {
				clientID, secret, found := strings.Cut(string(raw), ":")
				if found {
					if app := s.store.VerifyOAuthAppSecret(clientID, secret); app != nil {
						publisher.oauthApp = app
					}
				}
			}
		}
	}
	if publisher.githubApp == nil && publisher.oauthApp == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return nil
	}
	for _, listing := range s.store.ListMarketplaceListings(false) {
		if publisher.matches(listing) {
			return listing
		}
	}
	writeGHError(w, http.StatusNotFound, "Marketplace listing not found")
	return nil
}

func (s *Server) marketplaceListingForSettingsPublisher(publisher marketplacePublisher) *MarketplaceListing {
	for _, listing := range s.store.ListMarketplaceListings(false) {
		if publisher.matches(listing) {
			return listing
		}
	}
	return nil
}

func marketplaceListingJSON(listing *MarketplaceListing, plans []*MarketplacePlan, baseURL string) map[string]interface{} {
	planRows := make([]map[string]interface{}, 0, len(plans))
	for _, plan := range plans {
		planRows = append(planRows, marketplacePlanToJSON(plan, baseURL))
	}
	return map[string]interface{}{
		"slug": listing.Slug, "name": listing.Name, "description": listing.Description,
		"full_description": listing.FullDescription, "setup_url": nullOrString(listing.SetupURL),
		"installation_url": nullOrString(listing.InstallationURL), "github_app_id": nullOrInt(listing.GitHubAppID),
		"oauth_app_client_id": nullOrString(listing.OAuthAppClientID), "published": listing.Published,
		"created_at": listing.CreatedAt.UTC().Format(time.RFC3339), "updated_at": listing.UpdatedAt.UTC().Format(time.RFC3339),
		"plans": planRows,
	}
}

func marketplaceListingSettingsJSON(listing *MarketplaceListing, plans []*MarketplacePlan, baseURL string) map[string]interface{} {
	row := marketplaceListingJSON(listing, plans, baseURL)
	row["webhook_url"] = nullOrString(listing.WebhookURL)
	row["webhook_content_type"] = listing.WebhookContentType
	row["webhook_active"] = listing.WebhookActive
	row["webhook_id"] = nullOrInt(listing.WebhookID)
	return row
}

func nullOrInt(value int) interface{} {
	if value == 0 {
		return nil
	}
	return value
}

func (s *Server) handleGetMarketplaceListingSettings(w http.ResponseWriter, r *http.Request) {
	user, _ := s.personalAccessTokenWebUser(w, r)
	if user == nil {
		return
	}
	publisher, ok := s.marketplacePublisherForSettings(w, r, user)
	if !ok {
		return
	}
	listing := s.marketplaceListingForSettingsPublisher(publisher)
	if listing == nil {
		writeGHError(w, http.StatusNotFound, "Marketplace listing not found")
		return
	}
	writeJSON(w, http.StatusOK, marketplaceListingSettingsJSON(listing, s.store.ListMarketplacePlans(listing.Slug, false), s.baseURL(r)))
}

func validMarketplaceURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func (s *Server) handlePutMarketplaceListingSettings(w http.ResponseWriter, r *http.Request) {
	user, r := s.personalAccessTokenWebUser(w, r)
	if user == nil {
		return
	}
	publisher, ok := s.marketplacePublisherForSettings(w, r, user)
	if !ok {
		return
	}
	var req struct {
		Slug               string `json:"slug"`
		Name               string `json:"name"`
		Description        string `json:"description"`
		FullDescription    string `json:"full_description"`
		SetupURL           string `json:"setup_url"`
		InstallationURL    string `json:"installation_url"`
		WebhookURL         string `json:"webhook_url"`
		WebhookSecret      string `json:"webhook_secret"`
		WebhookContentType string `json:"webhook_content_type"`
		WebhookActive      bool   `json:"webhook_active"`
		Published          bool   `json:"published"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if publisher.githubApp != nil {
		req.Slug = publisher.githubApp.Slug
	}
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	if !marketplaceSlugPattern.MatchString(req.Slug) || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Description) == "" {
		writeGHValidationError(w, "MarketplaceListing", "slug, name, and description", "invalid")
		return
	}
	if req.SetupURL != "" && !validMarketplaceURL(req.SetupURL) || req.InstallationURL != "" && !validMarketplaceURL(req.InstallationURL) {
		writeGHValidationError(w, "MarketplaceListing", "setup_url or installation_url", "invalid")
		return
	}
	if req.WebhookContentType == "" {
		req.WebhookContentType = "json"
	}
	if req.WebhookURL != "" && !validMarketplaceURL(req.WebhookURL) || (req.WebhookContentType != "json" && req.WebhookContentType != "form") {
		writeGHValidationError(w, "MarketplaceListing", "webhook", "invalid")
		return
	}
	existing := s.marketplaceListingForSettingsPublisher(publisher)
	if conflicting := s.store.GetMarketplaceListing(req.Slug); conflicting != nil && (existing == nil || conflicting.Slug != existing.Slug) {
		writeGHValidationError(w, "MarketplaceListing", "slug", "already_exists")
		return
	}
	if existing != nil && existing.Slug != req.Slug {
		writeGHValidationError(w, "MarketplaceListing", "slug", "immutable")
		return
	}
	if req.Published {
		if publisher.githubApp != nil && req.SetupURL == "" || publisher.oauthApp != nil && req.InstallationURL == "" {
			writeGHValidationError(w, "MarketplaceListing", "setup_url or installation_url", "missing_field")
			return
		}
		if len(s.store.ListMarketplacePlans(req.Slug, true)) == 0 {
			writeGHValidationError(w, "MarketplaceListing", "plans", "missing_field")
			return
		}
		if req.WebhookURL == "" || !req.WebhookActive {
			writeGHValidationError(w, "MarketplaceListing", "webhook", "missing_field")
			return
		}
	}
	now := time.Now().UTC()
	listing := &MarketplaceListing{
		Slug: req.Slug, Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description),
		FullDescription: req.FullDescription, SetupURL: req.SetupURL, InstallationURL: req.InstallationURL,
		WebhookURL: req.WebhookURL, WebhookSecret: req.WebhookSecret,
		WebhookContentType: req.WebhookContentType, WebhookActive: req.WebhookActive,
		Published: req.Published, CreatedAt: now, UpdatedAt: now,
	}
	webhookChanged := existing == nil || existing.WebhookURL != listing.WebhookURL || existing.WebhookContentType != listing.WebhookContentType || existing.WebhookActive != listing.WebhookActive || (req.WebhookSecret != "" && req.WebhookSecret != existing.WebhookSecret)
	if existing != nil {
		listing.CreatedAt = existing.CreatedAt
		listing.WebhookID = existing.WebhookID
		if listing.WebhookSecret == "" {
			listing.WebhookSecret = existing.WebhookSecret
		}
	}
	if listing.WebhookURL != "" && listing.WebhookID == 0 {
		listing.WebhookID = s.store.ReserveMarketplaceHookID()
	}
	if publisher.githubApp != nil {
		listing.GitHubAppID = publisher.githubApp.ID
	} else {
		listing.OAuthAppClientID = publisher.oauthApp.ClientID
	}
	if err := s.store.SaveMarketplaceListing(listing); err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if webhookChanged && listing.WebhookURL != "" && listing.WebhookActive {
		s.emitMarketplacePing(listing, user)
	}
	status := http.StatusCreated
	if existing != nil {
		status = http.StatusOK
	}
	writeJSON(w, status, marketplaceListingSettingsJSON(listing, s.store.ListMarketplacePlans(listing.Slug, false), s.baseURL(r)))
}

func (s *Server) handleCreateMarketplacePlanSettings(w http.ResponseWriter, r *http.Request) {
	user, r := s.personalAccessTokenWebUser(w, r)
	if user == nil {
		return
	}
	publisher, ok := s.marketplacePublisherForSettings(w, r, user)
	if !ok {
		return
	}
	listing := s.marketplaceListingForSettingsPublisher(publisher)
	if listing == nil {
		writeGHError(w, http.StatusNotFound, "Marketplace listing not found")
		return
	}
	plan := s.decodeMarketplacePlanSettings(w, r, listing.Slug)
	if plan == nil {
		return
	}
	plan, err := s.store.CreateMarketplacePlan(plan)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, marketplacePlanToJSON(plan, s.baseURL(r)))
}

func (s *Server) decodeMarketplacePlanSettings(w http.ResponseWriter, r *http.Request, listingSlug string) *MarketplacePlan {
	var req MarketplacePlan
	if !decodeJSONBody(w, r, &req) {
		return nil
	}
	req.ListingSlug = listingSlug
	req.Name, req.Description = strings.TrimSpace(req.Name), strings.TrimSpace(req.Description)
	req.PriceModel, req.UnitName = strings.ToUpper(req.PriceModel), strings.TrimSpace(req.UnitName)
	if req.State == "" {
		req.State = "draft"
	}
	if req.Name == "" || req.MonthlyPriceInCents < 0 || req.YearlyPriceInCents < 0 ||
		(req.PriceModel != "FREE" && req.PriceModel != "FLAT_RATE" && req.PriceModel != "PER_UNIT") ||
		(req.State != "draft" && req.State != "published") ||
		(req.PriceModel == "FREE" && (req.MonthlyPriceInCents != 0 || req.YearlyPriceInCents != 0)) ||
		(req.PriceModel == "PER_UNIT" && req.UnitName == "") {
		writeGHValidationError(w, "MarketplaceListingPlan", "plan", "invalid")
		return nil
	}
	req.Bullets = append([]string(nil), req.Bullets...)
	return &req
}

func (s *Server) handleUpdateMarketplacePlanSettings(w http.ResponseWriter, r *http.Request) {
	user, r := s.personalAccessTokenWebUser(w, r)
	if user == nil {
		return
	}
	publisher, ok := s.marketplacePublisherForSettings(w, r, user)
	if !ok {
		return
	}
	listing := s.marketplaceListingForSettingsPublisher(publisher)
	if listing == nil {
		writeGHError(w, http.StatusNotFound, "Marketplace listing not found")
		return
	}
	plan := s.decodeMarketplacePlanSettings(w, r, listing.Slug)
	if plan == nil {
		return
	}
	plan.ID, _ = strconv.Atoi(r.PathValue("plan_id"))
	if s.store.GetMarketplacePlanForListing(listing.Slug, plan.ID) == nil {
		writeGHError(w, http.StatusNotFound, "Marketplace plan not found")
		return
	}
	if listing.Published && plan.State != "published" {
		published := 0
		for _, candidate := range s.store.ListMarketplacePlans(listing.Slug, true) {
			if candidate.ID != plan.ID {
				published++
			}
		}
		if published == 0 {
			writeGHValidationError(w, "MarketplaceListingPlan", "state", "published listing requires a published plan")
			return
		}
	}
	if err := s.store.UpdateMarketplacePlan(plan); err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, marketplacePlanToJSON(s.store.GetMarketplacePlanForListing(listing.Slug, plan.ID), s.baseURL(r)))
}

func (s *Server) handleDeleteMarketplacePlanSettings(w http.ResponseWriter, r *http.Request) {
	user, _ := s.personalAccessTokenWebUser(w, r)
	if user == nil {
		return
	}
	publisher, ok := s.marketplacePublisherForSettings(w, r, user)
	if !ok {
		return
	}
	listing := s.marketplaceListingForSettingsPublisher(publisher)
	planID, _ := strconv.Atoi(r.PathValue("plan_id"))
	if listing == nil || s.store.GetMarketplacePlanForListing(listing.Slug, planID) == nil {
		writeGHError(w, http.StatusNotFound, "Marketplace plan not found")
		return
	}
	if listing.Published && len(s.store.ListMarketplacePlans(listing.Slug, true)) == 1 && s.store.GetMarketplacePlanForListing(listing.Slug, planID).State == "published" {
		writeGHValidationError(w, "MarketplaceListingPlan", "plan", "published listing requires a published plan")
		return
	}
	if err := s.store.DeleteMarketplacePlan(listing.Slug, planID); err != nil {
		writeGHValidationError(w, "MarketplaceListingPlan", "plan", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteMarketplaceListingSettings(w http.ResponseWriter, r *http.Request) {
	user, _ := s.personalAccessTokenWebUser(w, r)
	if user == nil {
		return
	}
	publisher, ok := s.marketplacePublisherForSettings(w, r, user)
	if !ok {
		return
	}
	listing := s.marketplaceListingForSettingsPublisher(publisher)
	if listing == nil {
		writeGHError(w, http.StatusNotFound, "Marketplace listing not found")
		return
	}
	if err := s.store.DeleteMarketplaceListing(listing.Slug); err != nil {
		writeGHValidationError(w, "MarketplaceListing", "listing", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListMarketplaceDeliveriesSettings(w http.ResponseWriter, r *http.Request) {
	user, _ := s.personalAccessTokenWebUser(w, r)
	if user == nil {
		return
	}
	publisher, ok := s.marketplacePublisherForSettings(w, r, user)
	if !ok {
		return
	}
	listing := s.marketplaceListingForSettingsPublisher(publisher)
	if listing == nil {
		writeGHError(w, http.StatusNotFound, "Marketplace listing not found")
		return
	}
	deliveries := s.store.ListMarketplaceDeliveries(listing.Slug)
	rows := make([]map[string]interface{}, 0, len(deliveries))
	for _, delivery := range deliveries {
		rows = append(rows, deliveryFullJSON(delivery))
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) marketplaceBrowserUser(w http.ResponseWriter, r *http.Request) (*User, *http.Request) {
	return s.personalAccessTokenWebUser(w, r)
}

func (s *Server) handleListMarketplaceBrowser(w http.ResponseWriter, r *http.Request) {
	if user, _ := s.marketplaceBrowserUser(w, r); user == nil {
		return
	}
	listings := s.store.ListMarketplaceListings(true)
	rows := make([]map[string]interface{}, 0, len(listings))
	for _, listing := range listings {
		rows = append(rows, marketplaceListingJSON(listing, s.store.ListMarketplacePlans(listing.Slug, true), s.baseURL(r)))
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleGetMarketplaceBrowser(w http.ResponseWriter, r *http.Request) {
	if user, _ := s.marketplaceBrowserUser(w, r); user == nil {
		return
	}
	listing := s.store.GetMarketplaceListing(r.PathValue("listing_slug"))
	if listing == nil || !listing.Published {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, marketplaceListingJSON(listing, s.store.ListMarketplacePlans(listing.Slug, true), s.baseURL(r)))
}

type marketplaceBuyerAccount struct {
	id          int
	login       string
	accountType string
}

func (s *Server) marketplaceBuyerAccount(w http.ResponseWriter, user *User, login string) (marketplaceBuyerAccount, bool) {
	if login == "" || strings.EqualFold(login, user.Login) {
		return marketplaceBuyerAccount{id: user.ID, login: user.Login, accountType: "User"}, true
	}
	org := s.store.GetOrg(login)
	if org == nil || !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return marketplaceBuyerAccount{}, false
	}
	return marketplaceBuyerAccount{id: org.ID, login: org.Login, accountType: "Organization"}, true
}

func marketplaceBillingDate(now time.Time, cycle string) time.Time {
	if cycle == "yearly" {
		return now.AddDate(1, 0, 0)
	}
	return now.AddDate(0, 1, 0)
}

func marketplacePlanPrice(plan *MarketplacePlan, cycle string) int {
	if cycle == "yearly" {
		return plan.YearlyPriceInCents
	}
	return plan.MonthlyPriceInCents * 12
}

func (s *Server) handlePurchaseMarketplaceBrowser(w http.ResponseWriter, r *http.Request) {
	user, r := s.marketplaceBrowserUser(w, r)
	if user == nil {
		return
	}
	s.marketplaceMu.Lock()
	defer s.marketplaceMu.Unlock()
	listing := s.store.GetMarketplaceListing(r.PathValue("listing_slug"))
	if listing == nil || !listing.Published {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Account      string `json:"account"`
		PlanID       int    `json:"plan_id"`
		BillingCycle string `json:"billing_cycle"`
		UnitCount    int    `json:"unit_count"`
		FreeTrial    bool   `json:"free_trial"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	account, ok := s.marketplaceBuyerAccount(w, user, req.Account)
	if !ok {
		return
	}
	if s.store.GetMarketplacePurchase(listing.Slug, account.accountType, account.id) != nil {
		writeGHValidationError(w, "MarketplacePurchase", "account", "already_exists")
		return
	}
	plan := s.store.GetMarketplacePlanForListing(listing.Slug, req.PlanID)
	if plan == nil || plan.State != "published" || (req.BillingCycle != "monthly" && req.BillingCycle != "yearly") ||
		(plan.PriceModel == "PER_UNIT" && req.UnitCount <= 0) || (req.FreeTrial && !plan.HasFreeTrial) {
		writeGHValidationError(w, "MarketplacePurchase", "plan", "invalid")
		return
	}
	now := time.Now().UTC()
	unitCount := req.UnitCount
	if plan.PriceModel != "PER_UNIT" {
		unitCount = 0
	}
	nextBilling := marketplaceBillingDate(now, req.BillingCycle)
	purchase := &MarketplacePurchase{
		ListingSlug: listing.Slug, AccountID: account.id, AccountType: account.accountType,
		BillingCycle: req.BillingCycle, PlanID: plan.ID, PlanName: plan.Name, OnFreeTrial: req.FreeTrial,
		UnitCount: &unitCount, NextBillingDate: &nextBilling, UpdatedAt: &now,
	}
	if req.FreeTrial {
		trialEnd := now.AddDate(0, 0, 14)
		purchase.FreeTrialEnds = &trialEnd
		purchase.NextBillingDate = &trialEnd
	}
	installation, installationCreated, err := s.store.CreateMarketplacePurchase(listing, account, purchase)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if installation != nil {
		purchase.InstallationID = &installation.ID
		if installationCreated {
			s.emitInstallationEvent(s.store.GetApp(listing.GitHubAppID), "created", installation)
		}
	}
	s.emitMarketplacePurchase(listing, "purchased", purchase, nil, user)
	writeJSON(w, http.StatusCreated, s.marketplaceBrowserSubscriptionJSON(purchase, plan, listing, account.login, s.baseURL(r)))
}

func (s *Server) marketplaceBrowserSubscriptionJSON(purchase *MarketplacePurchase, plan *MarketplacePlan, listing *MarketplaceListing, accountLogin, baseURL string) map[string]interface{} {
	row := s.marketplaceAccountJSON(purchase, plan, baseURL)
	row["listing"] = marketplaceListingJSON(listing, s.store.ListMarketplacePlans(listing.Slug, true), baseURL)
	row["account_login"] = accountLogin
	setupURL := listing.SetupURL
	if setupURL == "" {
		setupURL = listing.InstallationURL
	}
	if setupURL != "" {
		parsed, _ := url.Parse(setupURL)
		query := parsed.Query()
		query.Set("marketplace_listing_plan_id", strconv.Itoa(plan.ID))
		if purchase.InstallationID != nil {
			query.Set("installation_id", strconv.Itoa(*purchase.InstallationID))
		}
		parsed.RawQuery = query.Encode()
		row["setup_url"] = parsed.String()
	} else {
		row["setup_url"] = nil
	}
	return row
}

func (s *Server) handleListMarketplaceSubscriptionsBrowser(w http.ResponseWriter, r *http.Request) {
	user, _ := s.marketplaceBrowserUser(w, r)
	if user == nil {
		return
	}
	for _, listing := range s.store.ListMarketplaceListings(false) {
		s.reconcileMarketplacePurchases(listing.Slug)
	}
	type accountRef struct {
		typeName string
		id       int
		login    string
	}
	accounts := []accountRef{{typeName: "User", id: user.ID, login: user.Login}}
	for _, org := range s.store.ListOrgsByUser(user.ID) {
		if canAdminOrg(s.store, user, org) {
			accounts = append(accounts, accountRef{typeName: "Organization", id: org.ID, login: org.Login})
		}
	}
	rows := []map[string]interface{}{}
	for _, account := range accounts {
		for _, purchase := range s.store.ListMarketplacePurchasesForAccount(account.typeName, account.id) {
			listing := s.store.GetMarketplaceListing(purchase.ListingSlug)
			plan := s.store.GetMarketplacePlanForListing(purchase.ListingSlug, purchase.PlanID)
			if listing == nil || plan == nil {
				writeGHError(w, http.StatusInternalServerError, "Marketplace subscription references missing listing state")
				return
			}
			rows = append(rows, s.marketplaceBrowserSubscriptionJSON(purchase, plan, listing, account.login, s.baseURL(r)))
		}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleChangeMarketplaceSubscriptionBrowser(w http.ResponseWriter, r *http.Request) {
	user, r := s.marketplaceBrowserUser(w, r)
	if user == nil {
		return
	}
	s.marketplaceMu.Lock()
	defer s.marketplaceMu.Unlock()
	listing := s.store.GetMarketplaceListing(r.PathValue("listing_slug"))
	if listing == nil || !listing.Published {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.reconcileMarketplacePurchasesLocked(listing.Slug)
	var req struct {
		Account      string `json:"account"`
		PlanID       int    `json:"plan_id"`
		BillingCycle string `json:"billing_cycle"`
		UnitCount    int    `json:"unit_count"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	account, ok := s.marketplaceBuyerAccount(w, user, req.Account)
	if !ok {
		return
	}
	purchase := s.store.GetMarketplacePurchase(listing.Slug, account.accountType, account.id)
	newPlan := s.store.GetMarketplacePlanForListing(listing.Slug, req.PlanID)
	if purchase == nil || newPlan == nil || newPlan.State != "published" || (req.BillingCycle != "monthly" && req.BillingCycle != "yearly") ||
		(newPlan.PriceModel == "PER_UNIT" && req.UnitCount <= 0) {
		writeGHValidationError(w, "MarketplacePurchase", "plan", "invalid")
		return
	}
	oldPlan := s.store.GetMarketplacePlanForListing(listing.Slug, purchase.PlanID)
	if oldPlan == nil {
		writeGHError(w, http.StatusInternalServerError, "Current Marketplace plan not found")
		return
	}
	previous := cloneMarketplacePurchase(purchase)
	now := time.Now().UTC()
	oldUnits, newUnits := 0, req.UnitCount
	if purchase.UnitCount != nil {
		oldUnits = *purchase.UnitCount
	}
	immediate := marketplacePlanPrice(newPlan, req.BillingCycle) > marketplacePlanPrice(oldPlan, purchase.BillingCycle) ||
		(newPlan.ID == oldPlan.ID && newUnits > oldUnits) || (purchase.BillingCycle == "monthly" && req.BillingCycle == "yearly")
	if immediate {
		purchase.PlanID, purchase.PlanName, purchase.BillingCycle = newPlan.ID, newPlan.Name, req.BillingCycle
		purchase.UnitCount = &newUnits
		purchase.PendingChange = nil
		purchase.OnFreeTrial = false
		purchase.FreeTrialEnds = nil
		nextBilling := marketplaceBillingDate(now, req.BillingCycle)
		purchase.NextBillingDate = &nextBilling
		purchase.UpdatedAt = &now
	} else {
		effective := marketplaceBillingDate(now, purchase.BillingCycle)
		if purchase.NextBillingDate != nil && purchase.NextBillingDate.After(now) {
			effective = *purchase.NextBillingDate
		}
		purchase.PendingChange = &MarketplacePendingChange{
			PlanID: newPlan.ID, BillingCycle: req.BillingCycle, UnitCount: &newUnits, EffectiveDate: effective, ActorID: user.ID,
		}
		purchase.UpdatedAt = &now
	}
	if err := s.store.SaveMarketplacePurchase(purchase); err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if immediate {
		s.emitMarketplacePurchase(listing, "changed", purchase, previous, user)
	}
	writeJSON(w, http.StatusOK, s.marketplaceBrowserSubscriptionJSON(purchase, s.store.GetMarketplacePlanForListing(listing.Slug, purchase.PlanID), listing, account.login, s.baseURL(r)))
}

func (s *Server) handleCancelMarketplaceSubscriptionBrowser(w http.ResponseWriter, r *http.Request) {
	user, _ := s.marketplaceBrowserUser(w, r)
	if user == nil {
		return
	}
	s.marketplaceMu.Lock()
	defer s.marketplaceMu.Unlock()
	listing := s.store.GetMarketplaceListing(r.PathValue("listing_slug"))
	if listing == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.reconcileMarketplacePurchasesLocked(listing.Slug)
	account, ok := s.marketplaceBuyerAccount(w, user, r.URL.Query().Get("account"))
	if !ok {
		return
	}
	purchase := s.store.GetMarketplacePurchase(listing.Slug, account.accountType, account.id)
	if purchase == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	plan := s.store.GetMarketplacePlanForListing(listing.Slug, purchase.PlanID)
	if plan == nil {
		writeGHError(w, http.StatusInternalServerError, "Current Marketplace plan not found")
		return
	}
	now := time.Now().UTC()
	if purchase.OnFreeTrial || plan.PriceModel == "FREE" {
		if err := s.store.DeleteMarketplacePurchase(listing.Slug, account.accountType, account.id); err != nil {
			writeGHError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.emitMarketplacePurchase(listing, "cancelled", purchase, nil, user)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	effective := marketplaceBillingDate(now, purchase.BillingCycle)
	if purchase.NextBillingDate != nil && purchase.NextBillingDate.After(now) {
		effective = *purchase.NextBillingDate
	}
	purchase.PendingChange = &MarketplacePendingChange{EffectiveDate: effective, Cancellation: true, ActorID: user.ID}
	purchase.UpdatedAt = &now
	if err := s.store.SaveMarketplacePurchase(purchase); err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, s.marketplaceBrowserSubscriptionJSON(purchase, plan, listing, account.login, s.baseURL(r)))
}

func marketplaceAccountWebhookJSON(purchase *MarketplacePurchase, login string) map[string]interface{} {
	return map[string]interface{}{
		"id": purchase.AccountID, "login": login, "type": purchase.AccountType,
	}
}

func (s *Server) marketplacePurchaseWebhookJSON(listing *MarketplaceListing, purchase *MarketplacePurchase, baseURL string) map[string]interface{} {
	plan := s.store.GetMarketplacePlanForListing(listing.Slug, purchase.PlanID)
	login := ""
	if purchase.AccountType == "Organization" {
		if org := s.store.GetOrgByID(purchase.AccountID); org != nil {
			login = org.Login
		}
	} else if user := s.store.GetUserByID(purchase.AccountID); user != nil {
		login = user.Login
	}
	return map[string]interface{}{
		"account": marketplaceAccountWebhookJSON(purchase, login), "billing_cycle": purchase.BillingCycle,
		"unit_count": purchase.UnitCount, "on_free_trial": purchase.OnFreeTrial,
		"free_trial_ends_on": purchase.FreeTrialEnds, "next_billing_date": purchase.NextBillingDate,
		"plan": marketplacePlanToJSON(plan, baseURL),
	}
}

func (s *Server) emitMarketplacePurchase(listing *MarketplaceListing, action string, purchase, previous *MarketplacePurchase, sender *User) {
	payload := map[string]interface{}{
		"action": action, "effective_date": time.Now().UTC().Format(time.RFC3339),
		"marketplace_purchase": s.marketplacePurchaseWebhookJSON(listing, purchase, s.baseURLFromConfig()),
		"sender":               userToJSON(sender),
	}
	if previous != nil {
		payload["previous_marketplace_purchase"] = s.marketplacePurchaseWebhookJSON(listing, previous, s.baseURLFromConfig())
	}
	s.emitMarketplaceWebhook(listing, "marketplace_purchase", action, payload)
}

func (s *Server) emitMarketplacePing(listing *MarketplaceListing, sender *User) {
	payload := map[string]interface{}{
		"zen": "Keep it logically awesome.", "hook_id": listing.WebhookID,
		"hook": map[string]interface{}{"type": "Marketplace", "id": listing.WebhookID, "active": listing.WebhookActive,
			"config": map[string]interface{}{"url": listing.WebhookURL, "content_type": listing.WebhookContentType}},
		"sender": userToJSON(sender),
	}
	s.emitMarketplaceWebhook(listing, "ping", "", payload)
}

func (s *Server) emitMarketplaceWebhook(listing *MarketplaceListing, event, action string, payload map[string]interface{}) {
	if listing.WebhookURL == "" || !listing.WebhookActive {
		return
	}
	hook := &Webhook{ID: listing.WebhookID, URL: listing.WebhookURL, Secret: listing.WebhookSecret,
		ContentType: listing.WebhookContentType, Active: true, MarketplaceSlug: listing.Slug}
	go func() {
		delivery := s.doDeliverAttempt(hook, event, action, uuid.New().String(), mustMarshal(payload), false)
		s.store.AddMarketplaceDelivery(listing.Slug, delivery)
	}()
}

func (s *Server) baseURLFromConfig() string {
	if s.externalURL != "" {
		return strings.TrimSuffix(s.externalURL, "/")
	}
	return "http://localhost:5555"
}

func (s *Server) reconcileMarketplacePurchases(listingSlug string) {
	s.marketplaceMu.Lock()
	defer s.marketplaceMu.Unlock()
	s.reconcileMarketplacePurchasesLocked(listingSlug)
}

func (s *Server) reconcileMarketplacePurchasesLocked(listingSlug string) {
	listing := s.store.GetMarketplaceListing(listingSlug)
	if listing == nil {
		return
	}
	now := time.Now().UTC()
	for _, purchase := range s.store.ListMarketplacePurchasesForListing(listingSlug) {
		pending := purchase.PendingChange
		if pending == nil || pending.EffectiveDate.After(now) {
			continue
		}
		previous := cloneMarketplacePurchase(purchase)
		if pending.Cancellation {
			freePlan := (*MarketplacePlan)(nil)
			for _, plan := range s.store.ListMarketplacePlans(listingSlug, true) {
				if plan.PriceModel == "FREE" {
					freePlan = plan
					break
				}
			}
			if freePlan == nil {
				if s.store.DeleteMarketplacePurchase(listingSlug, purchase.AccountType, purchase.AccountID) == nil {
					s.emitMarketplacePurchase(listing, "cancelled", previous, nil, s.store.GetUserByID(pending.ActorID))
				}
				continue
			}
			purchase.PlanID, purchase.PlanName = freePlan.ID, freePlan.Name
			purchase.BillingCycle = "monthly"
			zero := 0
			purchase.UnitCount = &zero
		} else {
			plan := s.store.GetMarketplacePlanForListing(listingSlug, pending.PlanID)
			if plan == nil {
				continue
			}
			purchase.PlanID, purchase.PlanName = plan.ID, plan.Name
			purchase.BillingCycle, purchase.UnitCount = pending.BillingCycle, pending.UnitCount
		}
		purchase.PendingChange = nil
		purchase.UpdatedAt = &now
		nextBilling := marketplaceBillingDate(now, purchase.BillingCycle)
		purchase.NextBillingDate = &nextBilling
		if s.store.SaveMarketplacePurchase(purchase) == nil {
			action := "changed"
			if pending.Cancellation {
				action = "cancelled"
			}
			s.emitMarketplacePurchase(listing, action, purchase, previous, s.store.GetUserByID(pending.ActorID))
		}
	}
}
