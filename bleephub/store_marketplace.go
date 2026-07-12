package bleephub

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func marketplacePurchaseKey(listingSlug, accountType string, accountID int) string {
	return strings.ToLower(listingSlug) + ":" + strings.ToLower(accountType) + ":" + strconv.Itoa(accountID)
}

func cloneMarketplaceListing(listing *MarketplaceListing) *MarketplaceListing {
	if listing == nil {
		return nil
	}
	copy := *listing
	return &copy
}

func cloneMarketplacePlan(plan *MarketplacePlan) *MarketplacePlan {
	if plan == nil {
		return nil
	}
	copy := *plan
	copy.Bullets = append([]string{}, plan.Bullets...)
	return &copy
}

func cloneMarketplacePurchase(purchase *MarketplacePurchase) *MarketplacePurchase {
	if purchase == nil {
		return nil
	}
	copy := *purchase
	if purchase.UnitCount != nil {
		value := *purchase.UnitCount
		copy.UnitCount = &value
	}
	if purchase.NextBillingDate != nil {
		value := *purchase.NextBillingDate
		copy.NextBillingDate = &value
	}
	if purchase.UpdatedAt != nil {
		value := *purchase.UpdatedAt
		copy.UpdatedAt = &value
	}
	if purchase.FreeTrialEnds != nil {
		value := *purchase.FreeTrialEnds
		copy.FreeTrialEnds = &value
	}
	if purchase.InstallationID != nil {
		value := *purchase.InstallationID
		copy.InstallationID = &value
	}
	if purchase.PendingChange != nil {
		pending := *purchase.PendingChange
		if purchase.PendingChange.UnitCount != nil {
			value := *purchase.PendingChange.UnitCount
			pending.UnitCount = &value
		}
		copy.PendingChange = &pending
	}
	return &copy
}

func (st *Store) SaveMarketplaceListing(listing *MarketplaceListing) error {
	if listing == nil || listing.Slug == "" {
		return fmt.Errorf("marketplace listing slug is required")
	}
	copy := cloneMarketplaceListing(listing)
	copy.Slug = strings.ToLower(copy.Slug)
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()
	if st.Misc.persist != nil {
		if err := st.Misc.persist.Put("marketplace_listings", copy.Slug, copy); err != nil {
			return fmt.Errorf("persist marketplace listing: %w", err)
		}
	}
	st.Misc.marketplaceListings[copy.Slug] = copy
	return nil
}

func (st *Store) GetMarketplaceListing(slug string) *MarketplaceListing {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	return cloneMarketplaceListing(st.Misc.marketplaceListings[strings.ToLower(slug)])
}

func (st *Store) ListMarketplaceListings(publishedOnly bool) []*MarketplaceListing {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	out := make([]*MarketplaceListing, 0, len(st.Misc.marketplaceListings))
	for _, listing := range st.Misc.marketplaceListings {
		if publishedOnly && !listing.Published {
			continue
		}
		out = append(out, cloneMarketplaceListing(listing))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func (st *Store) ReserveMarketplaceHookID() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	id := st.NextHookID
	st.NextHookID++
	return id
}

func (st *Store) AddMarketplaceDelivery(listingSlug string, delivery *WebhookDelivery) {
	if delivery == nil {
		return
	}
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()
	listingSlug = strings.ToLower(listingSlug)
	delivery.ID = st.Misc.nextMarketplaceDeliveryID
	st.Misc.nextMarketplaceDeliveryID++
	list := append(st.Misc.marketplaceDeliveries[listingSlug], delivery)
	if len(list) > maxHookDeliveries {
		list = list[len(list)-maxHookDeliveries:]
	}
	if st.Misc.persist != nil {
		st.Misc.persist.MustPut("marketplace_deliveries", listingSlug, list)
	}
	st.Misc.marketplaceDeliveries[listingSlug] = list
}

func (st *Store) ListMarketplaceDeliveries(listingSlug string) []*WebhookDelivery {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	source := st.Misc.marketplaceDeliveries[strings.ToLower(listingSlug)]
	out := make([]*WebhookDelivery, len(source))
	copy(out, source)
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out
}

func (st *Store) CreateMarketplacePlan(plan *MarketplacePlan) (*MarketplacePlan, error) {
	if plan == nil || plan.ListingSlug == "" {
		return nil, fmt.Errorf("marketplace listing is required")
	}
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()
	listingSlug := strings.ToLower(plan.ListingSlug)
	if st.Misc.marketplaceListings[listingSlug] == nil {
		return nil, fmt.Errorf("marketplace listing not found")
	}
	copy := cloneMarketplacePlan(plan)
	copy.ID = st.Misc.nextMarketplacePlanID
	copy.Number = copy.ID
	copy.ListingSlug = listingSlug
	if st.Misc.persist != nil {
		if err := st.Misc.persist.Put("marketplace_plans", strconv.Itoa(copy.ID), copy); err != nil {
			return nil, fmt.Errorf("persist marketplace plan: %w", err)
		}
	}
	st.Misc.nextMarketplacePlanID++
	st.Misc.marketplacePlans[copy.ID] = copy
	return cloneMarketplacePlan(copy), nil
}

func (st *Store) ListMarketplacePlans(listingSlug string, publishedOnly bool) []*MarketplacePlan {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	listingSlug = strings.ToLower(listingSlug)
	out := make([]*MarketplacePlan, 0)
	for _, plan := range st.Misc.marketplacePlans {
		if plan.ListingSlug != listingSlug || (publishedOnly && plan.State != "published") {
			continue
		}
		out = append(out, cloneMarketplacePlan(plan))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

func (st *Store) GetMarketplacePlanForListing(listingSlug string, planID int) *MarketplacePlan {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	plan := st.Misc.marketplacePlans[planID]
	if plan == nil || plan.ListingSlug != strings.ToLower(listingSlug) {
		return nil
	}
	return cloneMarketplacePlan(plan)
}

func (st *Store) UpdateMarketplacePlan(plan *MarketplacePlan) error {
	if plan == nil || plan.ID <= 0 || plan.ListingSlug == "" {
		return fmt.Errorf("marketplace plan identity is required")
	}
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()
	existing := st.Misc.marketplacePlans[plan.ID]
	if existing == nil || existing.ListingSlug != strings.ToLower(plan.ListingSlug) {
		return fmt.Errorf("marketplace plan not found")
	}
	copy := cloneMarketplacePlan(plan)
	copy.ListingSlug, copy.Number = existing.ListingSlug, existing.Number
	if st.Misc.persist != nil {
		if err := st.Misc.persist.Put("marketplace_plans", strconv.Itoa(copy.ID), copy); err != nil {
			return fmt.Errorf("persist marketplace plan: %w", err)
		}
	}
	st.Misc.marketplacePlans[copy.ID] = copy
	return nil
}

func (st *Store) DeleteMarketplacePlan(listingSlug string, planID int) error {
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()
	plan := st.Misc.marketplacePlans[planID]
	if plan == nil || plan.ListingSlug != strings.ToLower(listingSlug) {
		return fmt.Errorf("marketplace plan not found")
	}
	for _, purchase := range st.Misc.marketplacePurchases {
		if purchase.ListingSlug == plan.ListingSlug && (purchase.PlanID == planID || purchase.PendingChange != nil && purchase.PendingChange.PlanID == planID) {
			return fmt.Errorf("marketplace plan has active purchases")
		}
	}
	if st.Misc.persist != nil {
		if err := st.Misc.persist.Delete("marketplace_plans", strconv.Itoa(planID)); err != nil {
			return fmt.Errorf("delete marketplace plan: %w", err)
		}
	}
	delete(st.Misc.marketplacePlans, planID)
	return nil
}

func (st *Store) DeleteMarketplaceListing(slug string) error {
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()
	slug = strings.ToLower(slug)
	if st.Misc.marketplaceListings[slug] == nil {
		return fmt.Errorf("marketplace listing not found")
	}
	for _, purchase := range st.Misc.marketplacePurchases {
		if purchase.ListingSlug == slug {
			return fmt.Errorf("marketplace listing has active purchases")
		}
	}
	deletes := []persistencePut{{bucket: "marketplace_listings", key: slug}}
	if len(st.Misc.marketplaceDeliveries[slug]) > 0 {
		deletes = append(deletes, persistencePut{bucket: "marketplace_deliveries", key: slug})
	}
	for id, plan := range st.Misc.marketplacePlans {
		if plan.ListingSlug == slug {
			deletes = append(deletes, persistencePut{bucket: "marketplace_plans", key: strconv.Itoa(id)})
		}
	}
	if err := st.Misc.persist.DeleteBatch(deletes...); err != nil {
		return fmt.Errorf("delete marketplace listing: %w", err)
	}
	delete(st.Misc.marketplaceListings, slug)
	delete(st.Misc.marketplaceDeliveries, slug)
	for id, plan := range st.Misc.marketplacePlans {
		if plan.ListingSlug == slug {
			delete(st.Misc.marketplacePlans, id)
		}
	}
	return nil
}

func (st *Store) SaveMarketplacePurchase(purchase *MarketplacePurchase) error {
	if purchase == nil || purchase.ListingSlug == "" || purchase.AccountID <= 0 || purchase.AccountType == "" {
		return fmt.Errorf("marketplace purchase identity is incomplete")
	}
	copy := cloneMarketplacePurchase(purchase)
	copy.ListingSlug = strings.ToLower(copy.ListingSlug)
	key := marketplacePurchaseKey(copy.ListingSlug, copy.AccountType, copy.AccountID)
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()
	plan := st.Misc.marketplacePlans[copy.PlanID]
	if plan == nil || plan.ListingSlug != copy.ListingSlug {
		return fmt.Errorf("marketplace plan not found for listing")
	}
	if st.Misc.persist != nil {
		if err := st.Misc.persist.Put("marketplace_purchases", key, copy); err != nil {
			return fmt.Errorf("persist marketplace purchase: %w", err)
		}
	}
	st.Misc.marketplacePurchases[key] = copy
	return nil
}

func copyMarketplaceStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

// CreateMarketplacePurchase atomically creates a Marketplace subscription and,
// for a GitHub App listing, its account installation. It reports whether the
// installation was newly created so webhook delivery can begin only after both
// records are durable.
func (st *Store) CreateMarketplacePurchase(listing *MarketplaceListing, account marketplaceBuyerAccount, purchase *MarketplacePurchase) (*Installation, bool, error) {
	if listing == nil || purchase == nil {
		return nil, false, fmt.Errorf("marketplace purchase state is required")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()

	key := marketplacePurchaseKey(listing.Slug, account.accountType, account.id)
	if st.Misc.marketplacePurchases[key] != nil {
		return nil, false, fmt.Errorf("marketplace purchase already exists")
	}
	plan := st.Misc.marketplacePlans[purchase.PlanID]
	if plan == nil || plan.ListingSlug != strings.ToLower(listing.Slug) {
		return nil, false, fmt.Errorf("marketplace plan not found for listing")
	}
	copy := cloneMarketplacePurchase(purchase)
	copy.ListingSlug = strings.ToLower(copy.ListingSlug)

	var installation *Installation
	created := false
	if listing.GitHubAppID != 0 {
		app := st.Apps[listing.GitHubAppID]
		if app == nil {
			return nil, false, fmt.Errorf("marketplace GitHub App not found")
		}
		for _, candidate := range st.Installations {
			if candidate.AppID == app.ID && candidate.TargetID == account.id && candidate.TargetType == account.accountType {
				installation = candidate
				break
			}
		}
		if installation == nil {
			now := time.Now().UTC()
			installation = &Installation{
				ID: st.NextInstallationID, AppID: app.ID, AppSlug: app.Slug,
				TargetType: account.accountType, TargetID: account.id, TargetLogin: account.login,
				Permissions: copyMarketplaceStringMap(app.Permissions), Events: append([]string(nil), app.Events...),
				RepositorySelection: "all", CreatedAt: now, UpdatedAt: now,
			}
			if user := st.UsersByLogin[account.login]; user != nil {
				installation.TargetNodeID, installation.TargetAvatarURL = user.NodeID, user.AvatarURL
			} else if org := st.OrgsByLogin[account.login]; org != nil {
				installation.TargetNodeID, installation.TargetAvatarURL = org.NodeID, org.AvatarURL
			}
			created = true
		}
		copy.InstallationID = &installation.ID
	}

	if st.persist != st.Misc.persist {
		return nil, false, fmt.Errorf("marketplace persistence coordinates do not match")
	}
	entries := []persistencePut{{bucket: "marketplace_purchases", key: key, value: copy}}
	if created {
		entries = append(entries, persistencePut{bucket: "installations", key: strconv.Itoa(installation.ID), value: installation})
	}
	if err := st.persist.PutBatch(entries...); err != nil {
		return nil, false, fmt.Errorf("persist Marketplace purchase: %w", err)
	}
	if created {
		st.Installations[installation.ID] = installation
		st.NextInstallationID++
	}
	st.Misc.marketplacePurchases[key] = copy
	return installation, created, nil
}

func (st *Store) GetMarketplacePurchase(listingSlug, accountType string, accountID int) *MarketplacePurchase {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	return cloneMarketplacePurchase(st.Misc.marketplacePurchases[marketplacePurchaseKey(listingSlug, accountType, accountID)])
}

func (st *Store) ListMarketplacePurchasesForListing(listingSlug string) []*MarketplacePurchase {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	listingSlug = strings.ToLower(listingSlug)
	out := make([]*MarketplacePurchase, 0)
	for _, purchase := range st.Misc.marketplacePurchases {
		if purchase.ListingSlug == listingSlug {
			out = append(out, cloneMarketplacePurchase(purchase))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := time.Time{}, time.Time{}
		if out[i].UpdatedAt != nil {
			left = *out[i].UpdatedAt
		}
		if out[j].UpdatedAt != nil {
			right = *out[j].UpdatedAt
		}
		return left.Before(right)
	})
	return out
}

func (st *Store) ListMarketplacePurchasesForAccount(accountType string, accountID int) []*MarketplacePurchase {
	st.Misc.mu.RLock()
	defer st.Misc.mu.RUnlock()
	out := make([]*MarketplacePurchase, 0)
	for _, purchase := range st.Misc.marketplacePurchases {
		if purchase.AccountID == accountID && purchase.AccountType == accountType {
			out = append(out, cloneMarketplacePurchase(purchase))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ListingSlug < out[j].ListingSlug })
	return out
}

func (st *Store) DeleteMarketplacePurchase(listingSlug, accountType string, accountID int) error {
	key := marketplacePurchaseKey(listingSlug, accountType, accountID)
	st.Misc.mu.Lock()
	defer st.Misc.mu.Unlock()
	if st.Misc.marketplacePurchases[key] == nil {
		return fmt.Errorf("marketplace purchase not found")
	}
	if st.Misc.persist != nil {
		if err := st.Misc.persist.Delete("marketplace_purchases", key); err != nil {
			return fmt.Errorf("delete marketplace purchase: %w", err)
		}
	}
	delete(st.Misc.marketplacePurchases, key)
	return nil
}
