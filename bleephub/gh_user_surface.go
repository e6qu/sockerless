package bleephub

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// User-account surface: profile updates, email addresses, account
// interaction limits, GitHub Marketplace purchases, billing usage
// reports, profile hovercards, and cross-repository issue listing.

func (s *Server) registerGHUserSurfaceRoutes() {
	s.route("PATCH /api/v3/user", s.handleUpdateAuthenticatedUser)
	s.route("GET /api/v3/user/{account_id}", s.handleGetUserByAccountID)

	// Email addresses.
	s.route("POST /api/v3/user/emails", s.handleAddUserEmails)
	s.route("DELETE /api/v3/user/emails", s.handleDeleteUserEmails)
	s.route("GET /api/v3/user/public_emails", s.handleListPublicUserEmails)
	s.route("PATCH /api/v3/user/email/visibility", s.handleSetUserEmailVisibility)

	// SSH signing keys (single-key read; list/create/delete live in
	// gh_misc_endpoints.go).
	s.route("GET /api/v3/user/ssh_signing_keys/{ssh_signing_key_id}", s.handleGetMySSHSigningKey)

	// Account interaction limits.
	s.route("GET /api/v3/user/interaction-limits", s.handleGetUserInteractionLimits)
	s.route("PUT /api/v3/user/interaction-limits", s.handleSetUserInteractionLimits)
	s.route("DELETE /api/v3/user/interaction-limits", s.handleDeleteUserInteractionLimits)

	// GitHub Marketplace purchases.
	s.route("GET /api/v3/user/marketplace_purchases", s.handleListUserMarketplacePurchases)
	s.route("GET /api/v3/user/marketplace_purchases/stubbed", s.handleListUserMarketplacePurchases)

	// Cross-repository issue listing for the authenticated user.
	s.route("GET /api/v3/user/issues", s.handleListAuthUserIssues)

	// Profile hovercard.
	s.route("GET /api/v3/users/{username}/hovercard", s.handleGetUserHovercard)

	// Enhanced billing platform usage reports.
	s.route("GET /api/v3/users/{username}/settings/billing/usage", s.handleUserBillingUsage)
	s.route("GET /api/v3/users/{username}/settings/billing/usage/summary", s.handleUserBillingUsageSummary)
	s.route("GET /api/v3/users/{username}/settings/billing/ai_credit/usage", s.handleUserBillingAICreditUsage)
	s.route("GET /api/v3/users/{username}/settings/billing/premium_request/usage", s.handleUserBillingPremiumRequestUsage)
}

// ─── Profile (PATCH /user, GET /user/{account_id}) ──────────────────────

func (s *Server) handleUpdateAuthenticatedUser(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	var req struct {
		Name            *string `json:"name"`
		Email           *string `json:"email"`
		Blog            *string `json:"blog"`
		TwitterUsername *string `json:"twitter_username"`
		Company         *string `json:"company"`
		Location        *string `json:"location"`
		Hireable        *bool   `json:"hireable"`
		Bio             *string `json:"bio"`
	}
	if !decodeJSONBodyOptional(w, r, &req) {
		return
	}
	updated := s.store.UpdateUserProfile(user.ID, func(u *User) {
		if req.Name != nil {
			u.Name = *req.Name
		}
		if req.Email != nil && *req.Email != "" {
			s.store.setPrimaryEmailLocked(u, *req.Email)
		}
		if req.Blog != nil {
			u.Blog = *req.Blog
		}
		if req.TwitterUsername != nil {
			u.TwitterUsername = *req.TwitterUsername
		}
		if req.Company != nil {
			u.Company = *req.Company
		}
		if req.Location != nil {
			u.Location = *req.Location
		}
		if req.Hireable != nil {
			u.Hireable = req.Hireable
		}
		if req.Bio != nil {
			u.Bio = *req.Bio
		}
	})
	if updated == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.privateUserJSON(updated))
}

func (s *Server) handleGetUserByAccountID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("account_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	user := s.store.GetUserByID(id)
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	// Looking up your own account yields the private view, as on GET /user.
	if viewer := ghUserFromContext(r.Context()); viewer != nil && viewer.ID == user.ID {
		writeJSON(w, http.StatusOK, s.privateUserJSON(user))
		return
	}
	writeJSON(w, http.StatusOK, s.fullUserJSON(user))
}

// privateUserJSON renders GitHub's `private-user` schema — the
// authenticated user's own account view. The private counters are
// derived live from store state; two_factor_authentication is false
// because bleephub does not model two-factor authentication.
func (s *Server) privateUserJSON(u *User) map[string]interface{} {
	out := s.fullUserJSON(u)
	out["user_view_type"] = "private"
	privateRepos := s.store.CountPrivateRepos(u.Login)
	out["owned_private_repos"] = privateRepos
	out["total_private_repos"] = privateRepos
	out["private_gists"] = s.store.CountSecretGists(u.ID)
	out["collaborators"] = s.store.CountRepoCollaboratorsForOwner(u.Login)
	out["disk_usage"] = s.store.DiskUsageKBForOwner(u.Login)
	out["two_factor_authentication"] = false
	return out
}

// CountPrivateRepos returns the number of private repositories owned by
// the given account login.
func (st *Store) CountPrivateRepos(login string) int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	prefix := login + "/"
	n := 0
	for name, r := range st.ReposByName {
		if strings.HasPrefix(name, prefix) && r.Private {
			n++
		}
	}
	return n
}

// CountSecretGists returns the number of secret (non-public) gists the
// user owns.
func (st *Store) CountSecretGists(userID int) int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	n := 0
	for _, g := range st.Gists {
		if g.OwnerID == userID && !g.Public {
			n++
		}
	}
	return n
}

// CountRepoCollaboratorsForOwner returns the number of distinct
// collaborators across the account's repositories.
func (st *Store) CountRepoCollaboratorsForOwner(login string) int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	prefix := login + "/"
	distinct := map[string]bool{}
	for repoKey, collabs := range st.RepoCollaborators {
		if !strings.HasPrefix(repoKey, prefix) {
			continue
		}
		for collab := range collabs {
			distinct[collab] = true
		}
	}
	return len(distinct)
}

// DiskUsageKBForOwner sums the on-disk size of the account's
// repositories in kilobytes (memory-backed git storage occupies no disk).
func (st *Store) DiskUsageKBForOwner(login string) int64 {
	st.mu.RLock()
	prefix := login + "/"
	var names []string
	for name := range st.ReposByName {
		if strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	st.mu.RUnlock()
	var total int64
	for _, name := range names {
		total += st.RepoSize(name)
	}
	return total
}

// ─── Email addresses ─────────────────────────────────────────────────────

// userEmailJSON renders one email address in GitHub's `email` schema.
// An unset visibility is null on the wire.
func userEmailJSON(e UserEmail) map[string]interface{} {
	return map[string]interface{}{
		"email":      e.Email,
		"primary":    e.Primary,
		"verified":   e.Verified,
		"visibility": nullableString(e.Visibility),
	}
}

// decodeEmailsBody decodes the flexible request body GitHub accepts for
// POST/DELETE /user/emails: {"emails": [...]}, a bare JSON array, or a
// single JSON string.
func decodeEmailsBody(r *http.Request) ([]string, bool) {
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return nil, false
	}
	var obj struct {
		Emails []string `json:"emails"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Emails != nil {
		return obj.Emails, true
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, true
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil && single != "" {
		return []string{single}, true
	}
	return nil, false
}

func (s *Server) handleAddUserEmails(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	emails, ok := decodeEmailsBody(r)
	if !ok || len(emails) == 0 {
		writeGHValidationError(w, "Email", "emails", "missing_field")
		return
	}
	for _, e := range emails {
		if strings.TrimSpace(e) == "" {
			writeGHValidationError(w, "Email", "emails", "invalid")
			return
		}
	}
	added, ok := s.store.AddUserEmails(user.ID, emails)
	if !ok {
		writeGHValidationError(w, "Email", "emails", "already_exists")
		return
	}
	out := make([]map[string]interface{}, len(added))
	for i, e := range added {
		out[i] = userEmailJSON(e)
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleDeleteUserEmails(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	emails, ok := decodeEmailsBody(r)
	if !ok || len(emails) == 0 {
		writeGHValidationError(w, "Email", "emails", "missing_field")
		return
	}
	switch s.store.DeleteUserEmails(user.ID, emails) {
	case deleteEmailsOK:
		w.WriteHeader(http.StatusNoContent)
	case deleteEmailsPrimary:
		writeGHValidationError(w, "Email", "emails", "invalid")
	default:
		writeGHError(w, http.StatusNotFound, "Not Found")
	}
}

func (s *Server) handleListPublicUserEmails(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	out := []map[string]interface{}{}
	for _, e := range s.store.ListUserEmails(user.ID) {
		if e.Visibility == "public" {
			out = append(out, userEmailJSON(e))
		}
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func (s *Server) handleSetUserEmailVisibility(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	var req struct {
		Visibility string `json:"visibility"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Visibility != "public" && req.Visibility != "private" {
		writeGHValidationError(w, "Email", "visibility", "invalid")
		return
	}
	updated := s.store.SetPrimaryEmailVisibility(user.ID, req.Visibility)
	if updated == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	out := make([]map[string]interface{}, len(updated))
	for i, e := range updated {
		out[i] = userEmailJSON(e)
	}
	writeJSON(w, http.StatusOK, out)
}

// ─── Email store methods ─────────────────────────────────────────────────

// materializeEmailsLocked seeds the multi-email list from the legacy
// single Email field the first time email state is touched. Caller must
// hold st.mu.
func materializeEmailsLocked(u *User) {
	if len(u.Emails) == 0 && u.Email != "" {
		u.Emails = []UserEmail{{Email: u.Email, Primary: true, Verified: true, Visibility: "private"}}
	}
}

func (st *Store) persistUserLocked(u *User) {
	if st.persist != nil {
		st.persist.MustPut("users", strconv.Itoa(u.ID), u)
	}
}

// ListUserEmails returns the user's email addresses, primary first.
func (st *Store) ListUserEmails(userID int) []UserEmail {
	st.mu.Lock()
	defer st.mu.Unlock()
	u := st.Users[userID]
	if u == nil {
		return nil
	}
	materializeEmailsLocked(u)
	out := make([]UserEmail, len(u.Emails))
	copy(out, u.Emails)
	return out
}

// AddUserEmails appends new email addresses to the user's account.
// Returns (nil, false) when any address is already registered, matching
// real GitHub's 422 on duplicates.
func (st *Store) AddUserEmails(userID int, emails []string) ([]UserEmail, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	u := st.Users[userID]
	if u == nil {
		return nil, false
	}
	materializeEmailsLocked(u)
	for _, addr := range emails {
		for _, existing := range u.Emails {
			if strings.EqualFold(existing.Email, addr) {
				return nil, false
			}
		}
	}
	added := make([]UserEmail, 0, len(emails))
	for _, addr := range emails {
		e := UserEmail{Email: addr, Primary: false, Verified: true}
		u.Emails = append(u.Emails, e)
		added = append(added, e)
	}
	u.UpdatedAt = time.Now().UTC()
	st.persistUserLocked(u)
	return added, true
}

type deleteEmailsResult int

const (
	deleteEmailsOK deleteEmailsResult = iota
	deleteEmailsNotFound
	deleteEmailsPrimary
)

// DeleteUserEmails removes email addresses from the user's account. The
// primary address cannot be removed.
func (st *Store) DeleteUserEmails(userID int, emails []string) deleteEmailsResult {
	st.mu.Lock()
	defer st.mu.Unlock()
	u := st.Users[userID]
	if u == nil {
		return deleteEmailsNotFound
	}
	materializeEmailsLocked(u)
	for _, addr := range emails {
		found := false
		for _, existing := range u.Emails {
			if strings.EqualFold(existing.Email, addr) {
				if existing.Primary {
					return deleteEmailsPrimary
				}
				found = true
				break
			}
		}
		if !found {
			return deleteEmailsNotFound
		}
	}
	kept := u.Emails[:0]
	for _, existing := range u.Emails {
		remove := false
		for _, addr := range emails {
			if strings.EqualFold(existing.Email, addr) {
				remove = true
				break
			}
		}
		if !remove {
			kept = append(kept, existing)
		}
	}
	u.Emails = kept
	u.UpdatedAt = time.Now().UTC()
	st.persistUserLocked(u)
	return deleteEmailsOK
}

// SetPrimaryEmailVisibility updates the visibility of the primary email
// address and returns the updated entries, or nil when the user has no
// primary email.
func (st *Store) SetPrimaryEmailVisibility(userID int, visibility string) []UserEmail {
	st.mu.Lock()
	defer st.mu.Unlock()
	u := st.Users[userID]
	if u == nil {
		return nil
	}
	materializeEmailsLocked(u)
	var updated []UserEmail
	for i := range u.Emails {
		if u.Emails[i].Primary {
			u.Emails[i].Visibility = visibility
			updated = append(updated, u.Emails[i])
		}
	}
	if updated == nil {
		return nil
	}
	u.UpdatedAt = time.Now().UTC()
	st.persistUserLocked(u)
	return updated
}

// setPrimaryEmailLocked changes the account's primary email address
// (PATCH /user `email`). Caller must hold st.mu.
func (st *Store) setPrimaryEmailLocked(u *User, email string) {
	u.Email = email
	materializeEmailsLocked(u)
	for i := range u.Emails {
		if u.Emails[i].Primary {
			u.Emails[i].Email = email
			return
		}
	}
	u.Emails = append(u.Emails, UserEmail{Email: email, Primary: true, Verified: true, Visibility: "private"})
}

// UpdateUserProfile applies fn to the user under the store lock, bumps
// UpdatedAt, and persists. Returns nil when the user does not exist.
func (st *Store) UpdateUserProfile(userID int, fn func(*User)) *User {
	st.mu.Lock()
	defer st.mu.Unlock()
	u := st.Users[userID]
	if u == nil {
		return nil
	}
	fn(u)
	u.UpdatedAt = time.Now().UTC()
	st.persistUserLocked(u)
	return u
}

// ─── SSH signing key single read ─────────────────────────────────────────

func (s *Server) handleGetMySSHSigningKey(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	id, err := strconv.Atoi(r.PathValue("ssh_signing_key_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	for _, entry := range s.store.ListUserSSHSigningKeys(user.ID) {
		if sshSigningKeyEntryID(entry) == id {
			writeJSON(w, http.StatusOK, entry)
			return
		}
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

// ─── Account interaction limits ──────────────────────────────────────────

// interactionLimitExpiry maps GitHub's interaction-expiry enum to the
// moment the restriction lapses.
func interactionLimitExpiry(expiry string, from time.Time) (time.Time, bool) {
	switch expiry {
	case "", "one_day":
		return from.Add(24 * time.Hour), true
	case "three_days":
		return from.Add(3 * 24 * time.Hour), true
	case "one_week":
		return from.Add(7 * 24 * time.Hour), true
	case "one_month":
		return from.AddDate(0, 1, 0), true
	case "six_months":
		return from.AddDate(0, 6, 0), true
	}
	return time.Time{}, false
}

func isInteractionGroup(limit string) bool {
	switch limit {
	case "existing_users", "contributors_only", "collaborators_only":
		return true
	}
	return false
}

func (s *Server) handleGetUserInteractionLimits(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	limit, expiresAt := s.store.GetUserInteractionLimit(user.ID)
	if limit == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"limit":      limit,
		"origin":     "user",
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleSetUserInteractionLimits(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	var req struct {
		Limit  string `json:"limit"`
		Expiry string `json:"expiry"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Limit == "" {
		writeGHValidationError(w, "InteractionLimit", "limit", "missing_field")
		return
	}
	if !isInteractionGroup(req.Limit) {
		writeGHValidationError(w, "InteractionLimit", "limit", "invalid")
		return
	}
	expiresAt, ok := interactionLimitExpiry(req.Expiry, time.Now().UTC())
	if !ok {
		writeGHValidationError(w, "InteractionLimit", "expiry", "invalid")
		return
	}
	if !s.store.SetUserInteractionLimit(user.ID, req.Limit, &expiresAt) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"limit":      req.Limit,
		"origin":     "user",
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

func (s *Server) handleDeleteUserInteractionLimits(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	if !s.store.SetUserInteractionLimit(user.ID, "", nil) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetUserInteractionLimit records (or clears, with limit == "") the
// account-level interaction limit.
func (st *Store) SetUserInteractionLimit(userID int, limit string, expiresAt *time.Time) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	u := st.Users[userID]
	if u == nil {
		return false
	}
	u.InteractionLimit = limit
	u.InteractionLimitExpiry = expiresAt
	st.persistUserLocked(u)
	return true
}

// GetUserInteractionLimit returns the active limit and its expiry, or
// ("", zero) when no unexpired limit is set.
func (st *Store) GetUserInteractionLimit(userID int) (string, time.Time) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	u := st.Users[userID]
	if u == nil || u.InteractionLimit == "" || u.InteractionLimitExpiry == nil {
		return "", time.Time{}
	}
	if time.Now().After(*u.InteractionLimitExpiry) {
		return "", time.Time{}
	}
	return u.InteractionLimit, *u.InteractionLimitExpiry
}

// ─── GitHub Marketplace purchases ────────────────────────────────────────

func (s *Server) handleListUserMarketplacePurchases(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	for _, listing := range s.store.ListMarketplaceListings(false) {
		s.reconcileMarketplacePurchases(listing.Slug)
	}
	out := []map[string]interface{}{}
	for _, purchase := range s.store.ListMarketplacePurchasesForAccount("User", user.ID) {
		plan := s.store.GetMarketplacePlanForListing(purchase.ListingSlug, purchase.PlanID)
		if plan == nil {
			writeGHError(w, http.StatusInternalServerError, "Marketplace plan not found for purchase")
			return
		}
		out = append(out, s.userMarketplacePurchaseJSON(purchase, plan, user, s.baseURL(r)))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func (s *Server) userMarketplacePurchaseJSON(p *MarketplacePurchase, plan *MarketplacePlan, account *User, baseURL string) map[string]interface{} {
	var nextBilling, updatedAt, freeTrialEnds interface{}
	if p.NextBillingDate != nil {
		nextBilling = p.NextBillingDate.UTC().Format(time.RFC3339)
	}
	if p.UpdatedAt != nil {
		updatedAt = p.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if p.FreeTrialEnds != nil {
		freeTrialEnds = p.FreeTrialEnds.UTC().Format(time.RFC3339)
	}
	var unitCount interface{}
	if p.UnitCount != nil {
		unitCount = *p.UnitCount
	}
	return map[string]interface{}{
		"billing_cycle":      p.BillingCycle,
		"next_billing_date":  nextBilling,
		"unit_count":         unitCount,
		"on_free_trial":      p.OnFreeTrial,
		"free_trial_ends_on": freeTrialEnds,
		"updated_at":         updatedAt,
		"account": map[string]interface{}{
			"url":     baseURL + "/api/v3/users/" + account.Login,
			"id":      account.ID,
			"type":    account.Type,
			"node_id": account.NodeID,
			"login":   account.Login,
		},
		"plan": marketplacePlanJSON(plan, baseURL),
	}
}

// marketplacePlanJSON renders the full marketplace-listing-plan schema.
// The plan number is its listing identifier, which bleephub keys by ID.
func marketplacePlanJSON(p *MarketplacePlan, baseURL string) map[string]interface{} {
	planURL := baseURL + "/api/v3/marketplace_listing/plans/" + strconv.Itoa(p.ID)
	return map[string]interface{}{
		"url":                    planURL,
		"accounts_url":           planURL + "/accounts",
		"id":                     p.ID,
		"number":                 p.ID,
		"name":                   p.Name,
		"description":            p.Description,
		"monthly_price_in_cents": p.MonthlyPriceInCents,
		"yearly_price_in_cents":  p.YearlyPriceInCents,
		"price_model":            p.PriceModel,
		"has_free_trial":         p.HasFreeTrial,
		"unit_name":              nil,
		"state":                  p.State,
		"bullets":                append([]string{}, p.Bullets...),
	}
}

// ─── Profile hovercard ───────────────────────────────────────────────────

func (s *Server) handleGetUserHovercard(w http.ResponseWriter, r *http.Request) {
	viewer := ghUserFromContext(r.Context())
	if viewer == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	user := s.store.LookupUserByLogin(r.PathValue("username"))
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	subjectType := r.URL.Query().Get("subject_type")
	subjectID := r.URL.Query().Get("subject_id")
	if subjectType != "" {
		switch subjectType {
		case "organization", "repository", "issue", "pull_request":
		default:
			writeGHValidationError(w, "Hovercard", "subject_type", "invalid")
			return
		}
		if subjectID == "" {
			writeGHValidationError(w, "Hovercard", "subject_id", "missing_field")
			return
		}
	}

	contexts := []map[string]interface{}{}
	for _, orgLogin := range s.store.ActiveOrgLoginsForUser(user.ID) {
		contexts = append(contexts, map[string]interface{}{
			"message": "Member of " + orgLogin,
			"octicon": "organization",
		})
	}
	if subjectType == "repository" {
		if repoID, err := strconv.Atoi(subjectID); err == nil {
			if repo := s.store.GetRepoByID(repoID); repo != nil && repo.OwnerID == user.ID {
				contexts = append(contexts, map[string]interface{}{
					"message": "Owns this repository",
					"octicon": "repo",
				})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"contexts": contexts})
}

// ActiveOrgLoginsForUser returns the logins of organizations where the
// user holds an active membership, sorted alphabetically.
// ─── GET /user/issues ────────────────────────────────────────────────────

type issueWithRepo struct {
	issue *Issue
	repo  *Repo
}

// ListUserFilteredIssues returns issues visible through GET /user/issues
// for the given filter (assigned, created, mentioned, subscribed, repos,
// all). Repository read access is checked by the caller.
func (st *Store) ListUserFilteredIssues(user *User, filter string) []issueWithRepo {
	st.mu.RLock()
	defer st.mu.RUnlock()

	subscribed := func(repoID int) bool {
		sub := st.RepoSubscriptions[repoSubscriptionKey(user.ID, repoID)]
		return sub != nil && sub.Subscribed
	}
	matches := func(issue *Issue, repo *Repo) bool {
		assigned := false
		for _, aid := range issue.AssigneeIDs {
			if aid == user.ID {
				assigned = true
				break
			}
		}
		created := issue.AuthorID == user.ID
		mentioned := strings.Contains(issue.Body, "@"+user.Login)
		switch filter {
		case "created":
			return created
		case "mentioned":
			return mentioned
		case "subscribed":
			return subscribed(repo.ID)
		case "repos":
			return repo.OwnerID == user.ID
		case "all":
			return assigned || created || mentioned || subscribed(repo.ID)
		default: // "assigned"
			return assigned
		}
	}

	var out []issueWithRepo
	for _, issue := range st.Issues {
		repo := st.Repos[issue.RepoID]
		if repo == nil {
			continue
		}
		if matches(issue, repo) {
			out = append(out, issueWithRepo{issue: issue, repo: repo})
		}
	}
	return out
}

// CountIssueComments returns the number of conversation comments on an issue.
func (st *Store) CountIssueComments(issueID int) int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	n := 0
	for _, c := range st.Comments {
		if c.ParentType == "issue" && c.IssueID == issueID {
			n++
		}
	}
	return n
}

func (s *Server) handleListAuthUserIssues(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	q := r.URL.Query()
	filter := q.Get("filter")
	if filter == "" {
		filter = "assigned"
	}
	state := q.Get("state")
	if state == "" {
		state = "open"
	}

	pairs := s.store.ListUserFilteredIssues(user, filter)

	var filtered []issueWithRepo
	since := parseSinceTime(r)
	var labelNames []string
	if labelsParam := q.Get("labels"); labelsParam != "" {
		labelNames = strings.Split(labelsParam, ",")
	}
	for _, p := range pairs {
		if !canReadRepo(s.store, user, p.repo) {
			continue
		}
		switch state {
		case "open":
			if p.issue.State != "OPEN" {
				continue
			}
		case "closed":
			if p.issue.State != "CLOSED" {
				continue
			}
		}
		if !since.IsZero() && p.issue.UpdatedAt.Before(since) {
			continue
		}
		if len(labelNames) > 0 && !issueHasAllLabels(s.store, p.issue, labelNames, p.repo.ID) {
			continue
		}
		filtered = append(filtered, p)
	}

	sortKey := q.Get("sort")
	ascending := q.Get("direction") == "asc"
	sort.Slice(filtered, func(i, j int) bool {
		a, b := filtered[i].issue, filtered[j].issue
		var less bool
		switch sortKey {
		case "updated":
			less = a.UpdatedAt.Before(b.UpdatedAt)
		case "comments":
			less = s.store.CountIssueComments(a.ID) < s.store.CountIssueComments(b.ID)
		default: // "created"
			less = a.CreatedAt.Before(b.CreatedAt)
		}
		if ascending {
			return less
		}
		return !less
	})

	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(filtered))
	for _, p := range filtered {
		item := issueToJSON(p.issue, s.store, base, p.repo.FullName)
		item["repository"] = repoToJSON(p.repo, s.store, base)
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

// ─── Enhanced billing platform usage reports ─────────────────────────────

// billingUsageItem is one metered usage line derived from real run state.
type billingUsageItem struct {
	Date         time.Time
	Product      string
	SKU          string
	RepoFullName string
	Quantity     int
	UnitType     string
	PricePerUnit float64
}

// actionsLinuxPricePerMinute is GitHub's published Linux-runner
// per-minute price used on billing usage reports.
const actionsLinuxPricePerMinute = 0.008

// ActionsBillingUsageForOwner derives GitHub Actions usage line items
// from completed workflow-run jobs in repositories owned by the account.
// Quantities are per-job minutes rounded up, matching GitHub's metering.
func (st *Store) ActionsBillingUsageForOwner(ownerLogin string) []billingUsageItem {
	st.mu.RLock()
	defer st.mu.RUnlock()
	prefix := ownerLogin + "/"
	var out []billingUsageItem
	for _, wf := range st.Workflows {
		if !strings.HasPrefix(wf.RepoFullName, prefix) {
			continue
		}
		for _, job := range wf.Jobs {
			if job.StartedAt.IsZero() || job.CompletedAt.IsZero() || job.CompletedAt.Before(job.StartedAt) {
				continue
			}
			minutes := int(math.Ceil(job.CompletedAt.Sub(job.StartedAt).Minutes()))
			if minutes < 1 {
				minutes = 1
			}
			out = append(out, billingUsageItem{
				Date:         job.StartedAt.UTC(),
				Product:      "Actions",
				SKU:          "Actions Linux",
				RepoFullName: wf.RepoFullName,
				Quantity:     minutes,
				UnitType:     "minutes",
				PricePerUnit: actionsLinuxPricePerMinute,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}

// billingTimeFilter holds parsed year/month/day query parameters.
type billingTimeFilter struct {
	year, month, day int
}

// parseBillingTimeFilter reads year/month/day query params. A missing
// year defaults to the current year; when defaultMonth is set, a missing
// month defaults to the current month.
func parseBillingTimeFilter(r *http.Request, defaultMonth bool) (billingTimeFilter, error) {
	q := r.URL.Query()
	now := time.Now().UTC()
	f := billingTimeFilter{year: now.Year()}
	if defaultMonth {
		f.month = int(now.Month())
	}
	for _, p := range []struct {
		name string
		dst  *int
	}{{"year", &f.year}, {"month", &f.month}, {"day", &f.day}} {
		if v := q.Get(p.name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return f, fmt.Errorf("invalid %s parameter", p.name)
			}
			*p.dst = n
		}
	}
	return f, nil
}

func (f billingTimeFilter) matches(t time.Time) bool {
	if f.year != 0 && t.Year() != f.year {
		return false
	}
	if f.month != 0 && int(t.Month()) != f.month {
		return false
	}
	if f.day != 0 && t.Day() != f.day {
		return false
	}
	return true
}

// resolveBillingUser authorizes the billing report request: only the
// account owner or a site administrator can read a user's usage.
func (s *Server) resolveBillingUser(w http.ResponseWriter, r *http.Request) *User {
	viewer := ghUserFromContext(r.Context())
	if viewer == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return nil
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil || (target.ID != viewer.ID && !viewer.SiteAdmin) {
		writeGHError(w, http.StatusForbidden, "Forbidden")
		return nil
	}
	return target
}

// filteredBillingItems applies time/repository/product/sku query filters.
func (s *Server) filteredBillingItems(w http.ResponseWriter, r *http.Request, user *User, defaultMonth bool) ([]billingUsageItem, *billingTimeFilter) {
	f, err := parseBillingTimeFilter(r, defaultMonth)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, err.Error())
		return nil, nil
	}
	q := r.URL.Query()
	repoFilter := q.Get("repository")
	productFilter := q.Get("product")
	skuFilter := q.Get("sku")
	items := s.store.ActionsBillingUsageForOwner(user.Login)
	out := items[:0]
	for _, it := range items {
		if !f.matches(it.Date) {
			continue
		}
		if repoFilter != "" && !strings.EqualFold(it.RepoFullName, repoFilter) {
			continue
		}
		if productFilter != "" && !strings.EqualFold(it.Product, productFilter) {
			continue
		}
		if skuFilter != "" && !strings.EqualFold(it.SKU, skuFilter) {
			continue
		}
		out = append(out, it)
	}
	return out, &f
}

func (s *Server) handleUserBillingUsage(w http.ResponseWriter, r *http.Request) {
	user := s.resolveBillingUser(w, r)
	if user == nil {
		return
	}
	items, f := s.filteredBillingItems(w, r, user, false)
	if f == nil {
		return
	}
	usageItems := make([]map[string]interface{}, 0, len(items))
	for _, it := range items {
		gross := float64(it.Quantity) * it.PricePerUnit
		usageItems = append(usageItems, map[string]interface{}{
			"date":           it.Date.Format("2006-01-02"),
			"product":        it.Product,
			"sku":            it.SKU,
			"quantity":       it.Quantity,
			"unitType":       it.UnitType,
			"pricePerUnit":   it.PricePerUnit,
			"grossAmount":    gross,
			"discountAmount": 0.0,
			"netAmount":      gross,
			"repositoryName": it.RepoFullName,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"usageItems": usageItems})
}

func (s *Server) handleUserBillingUsageSummary(w http.ResponseWriter, r *http.Request) {
	user := s.resolveBillingUser(w, r)
	if user == nil {
		return
	}
	items, f := s.filteredBillingItems(w, r, user, true)
	if f == nil {
		return
	}
	type aggKey struct{ product, sku string }
	agg := map[aggKey]*struct {
		quantity int
		unitType string
		price    float64
	}{}
	var order []aggKey
	for _, it := range items {
		k := aggKey{it.Product, it.SKU}
		a := agg[k]
		if a == nil {
			a = &struct {
				quantity int
				unitType string
				price    float64
			}{unitType: it.UnitType, price: it.PricePerUnit}
			agg[k] = a
			order = append(order, k)
		}
		a.quantity += it.Quantity
	}
	usageItems := make([]map[string]interface{}, 0, len(order))
	for _, k := range order {
		a := agg[k]
		gross := float64(a.quantity) * a.price
		usageItems = append(usageItems, map[string]interface{}{
			"product":          k.product,
			"sku":              k.sku,
			"unitType":         a.unitType,
			"pricePerUnit":     a.price,
			"grossQuantity":    float64(a.quantity),
			"grossAmount":      gross,
			"discountQuantity": 0.0,
			"discountAmount":   0.0,
			"netQuantity":      float64(a.quantity),
			"netAmount":        gross,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"timePeriod": userBillingTimePeriodJSON(*f),
		"user":       user.Login,
		"usageItems": usageItems,
	})
}

// handleUserBillingAICreditUsage and handleUserBillingPremiumRequestUsage
// report metered AI-credit and premium-request usage. bleephub models no
// AI-credit-consuming or premium-request-consuming products, so the
// faithful report carries zero usage items.
func (s *Server) handleUserBillingAICreditUsage(w http.ResponseWriter, r *http.Request) {
	s.writeEmptyModelUsageReport(w, r)
}

func (s *Server) handleUserBillingPremiumRequestUsage(w http.ResponseWriter, r *http.Request) {
	s.writeEmptyModelUsageReport(w, r)
}

func (s *Server) writeEmptyModelUsageReport(w http.ResponseWriter, r *http.Request) {
	user := s.resolveBillingUser(w, r)
	if user == nil {
		return
	}
	f, err := parseBillingTimeFilter(r, true)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"timePeriod": userBillingTimePeriodJSON(f),
		"user":       user.Login,
		"usageItems": []map[string]interface{}{},
	})
}

func userBillingTimePeriodJSON(f billingTimeFilter) map[string]interface{} {
	out := map[string]interface{}{"year": f.year}
	if f.month != 0 {
		out["month"] = f.month
	}
	if f.day != 0 {
		out["day"] = f.day
	}
	return out
}
