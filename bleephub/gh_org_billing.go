package bleephub

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Organization billing under the enhanced billing platform paths
// (/organizations/{org}/settings/billing/...): budgets are real persisted
// entities with full CRUD; the usage reports are computed from real stored
// state — GitHub Actions job run history recorded on the shared store — and
// are honestly empty when no billable usage exists.

// OrgBudgetAlerting is the alert configuration on a budget.
type OrgBudgetAlerting struct {
	WillAlert       bool     `json:"will_alert"`
	AlertRecipients []string `json:"alert_recipients"`
}

// OrgBudget is one organization spending budget.
type OrgBudget struct {
	ID                  string            `json:"id"`
	BudgetScope         string            `json:"budget_scope"` // organization | repository | multi_user_customer | user
	BudgetEntityName    string            `json:"budget_entity_name"`
	BudgetAmount        int               `json:"budget_amount"`
	PreventFurtherUsage bool              `json:"prevent_further_usage"`
	BudgetProductSKU    string            `json:"budget_product_sku"`
	BudgetType          string            `json:"budget_type"` // ProductPricing | SkuPricing
	BudgetAlerting      OrgBudgetAlerting `json:"budget_alerting"`
	CreatedAt           time.Time         `json:"created_at"`
}

var budgetScopes = map[string]bool{
	"organization":        true,
	"repository":          true,
	"multi_user_customer": true,
	"user":                true,
}

var budgetTypes = map[string]bool{
	"ProductPricing": true,
	"SkuPricing":     true,
}

func (s *Server) registerGHOrgBillingRoutes() {
	s.route("GET /api/v3/organizations/{org}/settings/billing/budgets", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleListOrgBudgets))
	s.route("POST /api/v3/organizations/{org}/settings/billing/budgets", s.requireOrgAdmin(scopeOrgAdministration, permWrite, s.handleCreateOrgBudget))
	s.route("GET /api/v3/organizations/{org}/settings/billing/budgets/{budget_id}", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleGetOrgBudget))
	s.route("PATCH /api/v3/organizations/{org}/settings/billing/budgets/{budget_id}", s.requireOrgAdmin(scopeOrgAdministration, permWrite, s.handleUpdateOrgBudget))
	s.route("DELETE /api/v3/organizations/{org}/settings/billing/budgets/{budget_id}", s.requireOrgAdmin(scopeOrgAdministration, permWrite, s.handleDeleteOrgBudget))

	s.route("GET /api/v3/organizations/{org}/settings/billing/usage", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleOrgBillingUsage))
	s.route("GET /api/v3/organizations/{org}/settings/billing/usage/summary", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleOrgBillingUsageSummary))
	s.route("GET /api/v3/organizations/{org}/settings/billing/premium_request/usage", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleOrgBillingPremiumRequestUsage))
	s.route("GET /api/v3/organizations/{org}/settings/billing/ai_credit/usage", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleOrgBillingAICreditUsage))
}

// ─── budget store methods ────────────────────────────────────────────────

func (st *Store) persistOrgBudgetsLocked(orgLogin string) {
	if st.persist == nil {
		return
	}
	if m := st.OrgBudgets[orgLogin]; len(m) > 0 {
		st.persist.MustPut("org_budgets", orgLogin, m)
	} else {
		st.persist.MustDelete("org_budgets", orgLogin)
	}
}

// CreateOrgBudget stores a new budget for the organization.
func (st *Store) CreateOrgBudget(orgLogin string, b *OrgBudget) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.OrgBudgets[orgLogin] == nil {
		st.OrgBudgets[orgLogin] = map[string]*OrgBudget{}
	}
	st.OrgBudgets[orgLogin][b.ID] = b
	st.persistOrgBudgetsLocked(orgLogin)
}

// GetOrgBudget returns a budget by ID, or nil.
func (st *Store) GetOrgBudget(orgLogin, id string) *OrgBudget {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.OrgBudgets[orgLogin][id]
}

// ListOrgBudgets returns the org's budgets ordered by creation time then ID.
func (st *Store) ListOrgBudgets(orgLogin string) []*OrgBudget {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]*OrgBudget, 0, len(st.OrgBudgets[orgLogin]))
	for _, b := range st.OrgBudgets[orgLogin] {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// UpdateOrgBudget applies fn to a budget under the write lock. Returns the
// updated budget, or nil when it does not exist.
func (st *Store) UpdateOrgBudget(orgLogin, id string, fn func(*OrgBudget)) *OrgBudget {
	st.mu.Lock()
	defer st.mu.Unlock()
	b := st.OrgBudgets[orgLogin][id]
	if b == nil {
		return nil
	}
	fn(b)
	st.persistOrgBudgetsLocked(orgLogin)
	return b
}

// DeleteOrgBudget removes a budget. Returns true if it existed.
func (st *Store) DeleteOrgBudget(orgLogin, id string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.OrgBudgets[orgLogin][id] == nil {
		return false
	}
	delete(st.OrgBudgets[orgLogin], id)
	st.persistOrgBudgetsLocked(orgLogin)
	return true
}

// ─── budget handlers ─────────────────────────────────────────────────────

type orgBudgetBody struct {
	BudgetAmount        *int    `json:"budget_amount"`
	PreventFurtherUsage *bool   `json:"prevent_further_usage"`
	BudgetScope         *string `json:"budget_scope"`
	BudgetEntityName    *string `json:"budget_entity_name"`
	BudgetType          *string `json:"budget_type"`
	BudgetProductSKU    *string `json:"budget_product_sku"`
	BudgetAlerting      *struct {
		WillAlert       *bool    `json:"will_alert"`
		AlertRecipients []string `json:"alert_recipients"`
	} `json:"budget_alerting"`
}

func budgetJSON(b *OrgBudget) map[string]interface{} {
	recipients := b.BudgetAlerting.AlertRecipients
	if recipients == nil {
		recipients = []string{}
	}
	return map[string]interface{}{
		"id":                    b.ID,
		"budget_scope":          b.BudgetScope,
		"budget_entity_name":    b.BudgetEntityName,
		"budget_amount":         b.BudgetAmount,
		"prevent_further_usage": b.PreventFurtherUsage,
		"budget_product_sku":    b.BudgetProductSKU,
		"budget_type":           b.BudgetType,
		"budget_alerting": map[string]interface{}{
			"will_alert":       b.BudgetAlerting.WillAlert,
			"alert_recipients": recipients,
		},
	}
}

func (s *Server) handleListOrgBudgets(w http.ResponseWriter, r *http.Request) {
	budgets := s.store.ListOrgBudgets(r.PathValue("org"))

	if scope := r.URL.Query().Get("scope"); scope != "" {
		filtered := budgets[:0]
		for _, b := range budgets {
			if b.BudgetScope == scope {
				filtered = append(filtered, b)
			}
		}
		budgets = filtered
	}

	// Each page returns up to 10 budgets; has_next_page carries the
	// pagination signal in the response body.
	page, perPage := 1, 10
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && v > 0 && v <= 10 {
		perPage = v
	}
	total := len(budgets)
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}

	out := make([]map[string]interface{}, 0, end-start)
	for _, b := range budgets[start:end] {
		out = append(out, budgetJSON(b))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"budgets":       out,
		"total_count":   total,
		"has_next_page": end < total,
	})
}

func (s *Server) handleCreateOrgBudget(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	var req orgBudgetBody
	if !decodeJSONBodyOptional(w, r, &req) {
		return
	}
	b := &OrgBudget{
		ID:          uuid.New().String(),
		BudgetScope: "organization",
		BudgetType:  "ProductPricing",
		CreatedAt:   time.Now().UTC(),
	}
	if req.BudgetScope != nil {
		if !budgetScopes[*req.BudgetScope] {
			writeGHValidationError(w, "Budget", "budget_scope", "invalid")
			return
		}
		b.BudgetScope = *req.BudgetScope
	}
	if req.BudgetType != nil {
		if !budgetTypes[*req.BudgetType] {
			writeGHValidationError(w, "Budget", "budget_type", "invalid")
			return
		}
		b.BudgetType = *req.BudgetType
	}
	if req.BudgetProductSKU == nil || *req.BudgetProductSKU == "" {
		writeGHValidationError(w, "Budget", "budget_product_sku", "missing_field")
		return
	}
	b.BudgetProductSKU = *req.BudgetProductSKU
	if req.BudgetAmount != nil {
		if *req.BudgetAmount < 0 {
			writeGHValidationError(w, "Budget", "budget_amount", "invalid")
			return
		}
		b.BudgetAmount = *req.BudgetAmount
	}
	if req.PreventFurtherUsage != nil {
		b.PreventFurtherUsage = *req.PreventFurtherUsage
	}
	if req.BudgetEntityName != nil {
		b.BudgetEntityName = *req.BudgetEntityName
	}
	switch b.BudgetScope {
	case "repository":
		name := b.BudgetEntityName
		if !strings.Contains(name, "/") {
			name = orgLogin + "/" + name
		}
		if s.store.GetRepoByFullName(name) == nil {
			writeGHValidationError(w, "Budget", "budget_entity_name", "invalid")
			return
		}
	case "user", "multi_user_customer":
		// The spec requires prevent_further_usage to be true for these scopes.
		if !b.PreventFurtherUsage {
			writeGHValidationError(w, "Budget", "prevent_further_usage", "invalid")
			return
		}
	}
	if req.BudgetAlerting != nil {
		if req.BudgetAlerting.WillAlert != nil {
			b.BudgetAlerting.WillAlert = *req.BudgetAlerting.WillAlert
		}
		b.BudgetAlerting.AlertRecipients = req.BudgetAlerting.AlertRecipients
	}

	s.store.CreateOrgBudget(orgLogin, b)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Budget created successfully",
		"budget":  budgetJSON(b),
	})
}

func (s *Server) handleGetOrgBudget(w http.ResponseWriter, r *http.Request) {
	b := s.store.GetOrgBudget(r.PathValue("org"), r.PathValue("budget_id"))
	if b == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, budgetJSON(b))
}

func (s *Server) handleUpdateOrgBudget(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	id := r.PathValue("budget_id")
	var req orgBudgetBody
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.BudgetScope != nil && !budgetScopes[*req.BudgetScope] {
		writeGHValidationError(w, "Budget", "budget_scope", "invalid")
		return
	}
	if req.BudgetType != nil && !budgetTypes[*req.BudgetType] {
		writeGHValidationError(w, "Budget", "budget_type", "invalid")
		return
	}
	if req.BudgetAmount != nil && *req.BudgetAmount < 0 {
		writeGHValidationError(w, "Budget", "budget_amount", "invalid")
		return
	}
	b := s.store.UpdateOrgBudget(orgLogin, id, func(b *OrgBudget) {
		if req.BudgetAmount != nil {
			b.BudgetAmount = *req.BudgetAmount
		}
		if req.PreventFurtherUsage != nil {
			b.PreventFurtherUsage = *req.PreventFurtherUsage
		}
		if req.BudgetScope != nil {
			b.BudgetScope = *req.BudgetScope
		}
		if req.BudgetEntityName != nil {
			b.BudgetEntityName = *req.BudgetEntityName
		}
		if req.BudgetType != nil {
			b.BudgetType = *req.BudgetType
		}
		if req.BudgetProductSKU != nil {
			b.BudgetProductSKU = *req.BudgetProductSKU
		}
		if req.BudgetAlerting != nil {
			if req.BudgetAlerting.WillAlert != nil {
				b.BudgetAlerting.WillAlert = *req.BudgetAlerting.WillAlert
			}
			if req.BudgetAlerting.AlertRecipients != nil {
				b.BudgetAlerting.AlertRecipients = req.BudgetAlerting.AlertRecipients
			}
		}
	})
	if b == nil {
		writeGHError(w, http.StatusNotFound, fmt.Sprintf("Budget with ID %s not found.", id))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Budget updated successfully",
		"budget":  budgetJSON(b),
	})
}

func (s *Server) handleDeleteOrgBudget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("budget_id")
	if !s.store.DeleteOrgBudget(r.PathValue("org"), id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Budget deleted successfully",
		"id":      id,
	})
}

// ─── usage reports ───────────────────────────────────────────────────────

// actionsUsageLine is one billable Actions usage line item: minutes consumed
// by workflow jobs of one repository on one date.
type actionsUsageLine struct {
	date     string
	repoName string
	minutes  int
}

// actionsPricePerMinute mirrors GitHub's Linux runner list price.
const actionsPricePerMinute = 0.008

// orgActionsUsageLines computes real Actions usage from the recorded
// workflow run history (current runs plus archived attempts): every
// completed job is billed per started minute, rounded up, exactly as GitHub
// meters Actions. Returns lines filtered to the requested year/month/day
// (zero month/day mean "whole year"/"whole month").
func (st *Store) orgActionsUsageLines(orgLogin string, year, month, day int) []actionsUsageLine {
	st.mu.RLock()
	defer st.mu.RUnlock()

	type key struct {
		date string
		repo string
	}
	minutes := map[key]int{}
	prefix := orgLogin + "/"
	addRun := func(wf *Workflow) {
		if !strings.HasPrefix(wf.RepoFullName, prefix) {
			return
		}
		repoName := strings.TrimPrefix(wf.RepoFullName, prefix)
		for _, job := range wf.Jobs {
			if job.StartedAt.IsZero() || job.CompletedAt.IsZero() || !job.CompletedAt.After(job.StartedAt) {
				continue
			}
			started := job.StartedAt.UTC()
			if started.Year() != year {
				continue
			}
			if month != 0 && int(started.Month()) != month {
				continue
			}
			if day != 0 && started.Day() != day {
				continue
			}
			mins := int(math.Ceil(job.CompletedAt.Sub(job.StartedAt).Minutes()))
			if mins < 1 {
				mins = 1
			}
			minutes[key{started.Format("2006-01-02"), repoName}] += mins
		}
	}
	for _, wf := range st.Workflows {
		addRun(wf)
	}
	for _, attempts := range st.WorkflowAttempts {
		for _, wf := range attempts {
			addRun(wf)
		}
	}

	out := make([]actionsUsageLine, 0, len(minutes))
	for k, mins := range minutes {
		out = append(out, actionsUsageLine{date: k.date, repoName: k.repo, minutes: mins})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].date != out[j].date {
			return out[i].date < out[j].date
		}
		return out[i].repoName < out[j].repoName
	})
	return out
}

// billingPeriod parses the year/month/day query parameters. defaultMonth
// selects the month-defaulting behavior of the summary/premium/AI reports.
func billingPeriod(w http.ResponseWriter, r *http.Request, defaultMonth bool) (year, month, day int, ok bool) {
	now := time.Now().UTC()
	year = now.Year()
	if defaultMonth {
		month = int(now.Month())
	}
	q := r.URL.Query()
	if v := q.Get("year"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1000 || n > 9999 {
			writeGHError(w, http.StatusBadRequest, "Invalid year")
			return 0, 0, 0, false
		}
		year = n
	}
	if v := q.Get("month"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 12 {
			writeGHError(w, http.StatusBadRequest, "Invalid month")
			return 0, 0, 0, false
		}
		month = n
	}
	if v := q.Get("day"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 31 {
			writeGHError(w, http.StatusBadRequest, "Invalid day")
			return 0, 0, 0, false
		}
		day = n
	}
	return year, month, day, true
}

func (s *Server) handleOrgBillingUsage(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	year, month, day, ok := billingPeriod(w, r, false)
	if !ok {
		return
	}
	lines := s.store.orgActionsUsageLines(orgLogin, year, month, day)
	items := make([]map[string]interface{}, 0, len(lines))
	for _, line := range lines {
		gross := float64(line.minutes) * actionsPricePerMinute
		items = append(items, map[string]interface{}{
			"date":             line.date,
			"product":          "actions",
			"sku":              "actions_linux",
			"quantity":         line.minutes,
			"unitType":         "Minutes",
			"pricePerUnit":     actionsPricePerMinute,
			"grossAmount":      gross,
			"discountAmount":   0.0,
			"netAmount":        gross,
			"organizationName": orgLogin,
			"repositoryName":   line.repoName,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"usageItems": items})
}

func billingTimePeriodJSON(year, month, day int) map[string]interface{} {
	out := map[string]interface{}{"year": year}
	if month != 0 {
		out["month"] = month
	}
	if day != 0 {
		out["day"] = day
	}
	return out
}

func (s *Server) handleOrgBillingUsageSummary(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	year, month, day, ok := billingPeriod(w, r, true)
	if !ok {
		return
	}
	q := r.URL.Query()
	repoFilter := q.Get("repository")
	productFilter := q.Get("product")
	skuFilter := q.Get("sku")

	lines := s.store.orgActionsUsageLines(orgLogin, year, month, day)
	var grossQuantity, grossAmount float64
	for _, line := range lines {
		if repoFilter != "" && line.repoName != repoFilter {
			continue
		}
		grossQuantity += float64(line.minutes)
		grossAmount += float64(line.minutes) * actionsPricePerMinute
	}

	items := []map[string]interface{}{}
	if grossQuantity > 0 &&
		(productFilter == "" || productFilter == "actions") &&
		(skuFilter == "" || skuFilter == "actions_linux") {
		items = append(items, map[string]interface{}{
			"product":          "actions",
			"sku":              "actions_linux",
			"unitType":         "Minutes",
			"pricePerUnit":     actionsPricePerMinute,
			"grossQuantity":    grossQuantity,
			"grossAmount":      grossAmount,
			"discountQuantity": 0.0,
			"discountAmount":   0.0,
			"netQuantity":      grossQuantity,
			"netAmount":        grossAmount,
		})
	}

	out := map[string]interface{}{
		"timePeriod":   billingTimePeriodJSON(year, month, day),
		"organization": orgLogin,
		"usageItems":   items,
	}
	if repoFilter != "" {
		out["repository"] = repoFilter
	}
	if productFilter != "" {
		out["product"] = productFilter
	}
	if skuFilter != "" {
		out["sku"] = skuFilter
	}
	writeJSON(w, http.StatusOK, out)
}

// handleOrgBillingPremiumRequestUsage reports Copilot premium request usage.
// bleephub runs no metered premium-request product, so the report is
// honestly empty for every period.
func (s *Server) handleOrgBillingPremiumRequestUsage(w http.ResponseWriter, r *http.Request) {
	s.writeOrgBillingMeteredAIUsage(w, r)
}

// handleOrgBillingAICreditUsage reports AI credit usage. bleephub runs no
// metered AI-credit product, so the report is honestly empty for every
// period.
func (s *Server) handleOrgBillingAICreditUsage(w http.ResponseWriter, r *http.Request) {
	s.writeOrgBillingMeteredAIUsage(w, r)
}

func (s *Server) writeOrgBillingMeteredAIUsage(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	year, month, day, ok := billingPeriod(w, r, true)
	if !ok {
		return
	}
	q := r.URL.Query()
	out := map[string]interface{}{
		"timePeriod":   billingTimePeriodJSON(year, month, day),
		"organization": orgLogin,
		"usageItems":   []map[string]interface{}{},
	}
	if v := q.Get("user"); v != "" {
		out["user"] = v
	}
	if v := q.Get("product"); v != "" {
		out["product"] = v
	}
	if v := q.Get("model"); v != "" {
		out["model"] = v
	}
	writeJSON(w, http.StatusOK, out)
}
