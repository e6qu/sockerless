package bleephub

import (
	"testing"
	"time"
)

func TestMarketplaceStatePersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", dir)
	persistence, err := NewPersistence()
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	if err := store.SetPersistence(persistence); err != nil {
		t.Fatal(err)
	}
	store.SeedDefaultUser()
	admin := store.LookupUserByLogin("admin")
	app, err := store.CreateAppE(admin.ID, "Persistent Marketplace App", "", map[string]string{"contents": "read"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	listing := &MarketplaceListing{
		Slug: app.Slug, Name: app.Name, Description: "Persistent listing", SetupURL: "https://example.test/setup",
		GitHubAppID: app.ID, WebhookURL: "https://example.test/hook", WebhookContentType: "json", WebhookActive: true,
		Published: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.SaveMarketplaceListing(listing); err != nil {
		t.Fatal(err)
	}
	plan, err := store.CreateMarketplacePlan(&MarketplacePlan{
		ListingSlug: listing.Slug, Name: "Team", Description: "Persistent plan", PriceModel: "FLAT_RATE",
		MonthlyPriceInCents: 900, YearlyPriceInCents: 9000, State: "published",
	})
	if err != nil {
		t.Fatal(err)
	}
	nextBilling := now.AddDate(0, 1, 0)
	purchase := &MarketplacePurchase{
		ListingSlug: listing.Slug, AccountID: admin.ID, AccountType: "User", BillingCycle: "monthly",
		PlanID: plan.ID, PlanName: plan.Name, NextBillingDate: &nextBilling, UpdatedAt: &now,
		PendingChange: &MarketplacePendingChange{EffectiveDate: nextBilling, Cancellation: true, ActorID: admin.ID},
	}
	installation, created, err := store.CreateMarketplacePurchase(listing,
		marketplaceBuyerAccount{id: admin.ID, login: admin.Login, accountType: "User"}, purchase)
	if err != nil || !created || installation == nil {
		t.Fatalf("create persisted Marketplace purchase: installation=%#v created=%v err=%v", installation, created, err)
	}
	if err := persistence.Close(); err != nil {
		t.Fatal(err)
	}

	persistence, err = NewPersistence()
	if err != nil {
		t.Fatal(err)
	}
	defer persistence.Close()
	reloaded := NewStore()
	if err := reloaded.SetPersistence(persistence); err != nil {
		t.Fatal(err)
	}
	gotListing := reloaded.GetMarketplaceListing(listing.Slug)
	gotPlan := reloaded.GetMarketplacePlanForListing(listing.Slug, plan.ID)
	gotPurchase := reloaded.GetMarketplacePurchase(listing.Slug, "User", admin.ID)
	gotInstallation := reloaded.GetInstallation(installation.ID)
	if gotListing == nil || gotPlan == nil || gotPurchase == nil || gotInstallation == nil {
		t.Fatalf("reloaded Marketplace state: listing=%#v plan=%#v purchase=%#v installation=%#v", gotListing, gotPlan, gotPurchase, gotInstallation)
	}
	if gotPurchase.PendingChange == nil || !gotPurchase.PendingChange.Cancellation || gotPurchase.InstallationID == nil || *gotPurchase.InstallationID != installation.ID {
		t.Fatalf("reloaded Marketplace purchase = %#v", gotPurchase)
	}
}

func TestMarketplacePurchaseStorageFailureLeavesNoInstallationOrPurchase(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", dir)
	persistence, err := NewPersistence()
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	if err := store.SetPersistence(persistence); err != nil {
		t.Fatal(err)
	}
	store.SeedDefaultUser()
	admin := store.LookupUserByLogin("admin")
	app, err := store.CreateAppE(admin.ID, "Atomic Marketplace App", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	listing := &MarketplaceListing{Slug: app.Slug, Name: app.Name, Description: "Atomic listing", GitHubAppID: app.ID, CreatedAt: now, UpdatedAt: now}
	if err := store.SaveMarketplaceListing(listing); err != nil {
		t.Fatal(err)
	}
	plan, err := store.CreateMarketplacePlan(&MarketplacePlan{ListingSlug: listing.Slug, Name: "Free", PriceModel: "FREE", State: "published"})
	if err != nil {
		t.Fatal(err)
	}
	if err := persistence.Close(); err != nil {
		t.Fatal(err)
	}

	purchase := &MarketplacePurchase{ListingSlug: listing.Slug, AccountID: admin.ID, AccountType: "User", BillingCycle: "monthly", PlanID: plan.ID, PlanName: plan.Name, UpdatedAt: &now}
	if _, _, err := store.CreateMarketplacePurchase(listing, marketplaceBuyerAccount{id: admin.ID, login: admin.Login, accountType: "User"}, purchase); err == nil {
		t.Fatal("Marketplace purchase unexpectedly succeeded after durable storage closed")
	}
	if store.GetMarketplacePurchase(listing.Slug, "User", admin.ID) != nil {
		t.Fatal("failed Marketplace purchase mutated subscription memory")
	}
	if got := store.ListAppInstallations(app.ID); len(got) != 0 {
		t.Fatalf("failed Marketplace purchase left installations: %#v", got)
	}
}
