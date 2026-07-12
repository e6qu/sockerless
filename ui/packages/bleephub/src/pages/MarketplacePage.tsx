import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { InlineError, Spinner } from "@sockerless/ui-core/components";
import { Link, useParams } from "react-router";
import {
  cancelMarketplacePlan,
  changeMarketplacePlan,
  fetchMarketplaceAccounts,
  fetchMarketplaceListing,
  fetchMarketplaceListings,
  fetchMarketplaceSubscriptions,
  purchaseMarketplacePlan,
} from "../api.js";
import type { GithubMarketplaceListing, GithubMarketplacePlan } from "../types.js";
import { Blankslate, Box, Button, ErrorBanner, FormLabel, StateLabel } from "../components/ui.js";
import { CheckIcon, OrganizationIcon, PackageIcon, SearchIcon } from "../components/octicons.js";

const MARKETPLACE_COLORS = [
  "var(--color-brand-purple)",
  "var(--color-brand-blue)",
  "var(--color-brand-cyan)",
  "var(--color-brand-pink)",
  "var(--color-brand-gold)",
];

export function MarketplacePage() {
  const { slug } = useParams<{ slug?: string }>();
  return slug ? <MarketplaceDetail slug={slug} /> : <MarketplaceDirectory />;
}

function MarketplaceHero({ subscriptions }: { subscriptions: number }) {
  return (
    <section className="marketplace-hero mb-6">
      <div>
        <div className="mb-2 inline-flex items-center gap-2 marketplace-eyebrow">
          <PackageIcon size={17} /> GitHub Marketplace
        </div>
        <h1>Build more. Ship brighter.</h1>
        <p>
          Discover GitHub Apps and OAuth Apps that extend your workflow, with familiar installation,
          billing, trials, and publisher-managed plans.
        </p>
      </div>
      <div className="marketplace-hero-stat">
        <b>{subscriptions}</b>
        <span>active subscription{subscriptions === 1 ? "" : "s"}</span>
      </div>
    </section>
  );
}

function MarketplaceDirectory() {
  const listings = useQuery({ queryKey: ["marketplace", "listings"], queryFn: fetchMarketplaceListings });
  const subscriptions = useQuery({ queryKey: ["marketplace", "subscriptions"], queryFn: fetchMarketplaceSubscriptions });
  const [search, setSearch] = useState("");
  const visible = useMemo(() => {
    const query = search.trim().toLowerCase();
    return (listings.data ?? []).filter((listing) => !query || `${listing.name} ${listing.description}`.toLowerCase().includes(query));
  }, [listings.data, search]);

  if (listings.isLoading || subscriptions.isLoading) return <Spinner label="loading Marketplace" />;
  if (listings.isError || subscriptions.isError) return <InlineError title="Marketplace unavailable" detail={String(listings.error || subscriptions.error)} />;

  return (
    <div>
      <MarketplaceHero subscriptions={subscriptions.data?.length ?? 0} />
      <div className="marketplace-layout">
        <aside className="marketplace-sidebar">
          <h2>Discover</h2>
          <a href="#all-apps" className="marketplace-category active">All apps</a>
          <a href="#developer-tools" className="marketplace-category">Developer tools</a>
          <a href="#security" className="marketplace-category">Security</a>
          <a href="#education" className="marketplace-category">Education</a>
          <Link to="/ui/apps" className="marketplace-publisher-link">Publish an app →</Link>
        </aside>
        <main id="all-apps" className="min-w-0">
          <div className="marketplace-toolbar">
            <div>
              <h2>Apps for every workflow</h2>
              <p>{visible.length} published integration{visible.length === 1 ? "" : "s"}</p>
            </div>
            <label className="marketplace-search">
              <SearchIcon size={16} />
              <input aria-label="Search Marketplace" type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search apps" />
            </label>
          </div>
          {visible.length === 0 ? (
            <Blankslate title="No apps matched">Try another Marketplace search.</Blankslate>
          ) : (
            <div className="marketplace-grid">
              {visible.map((listing, index) => <ListingCard key={listing.slug} listing={listing} color={MARKETPLACE_COLORS[index % MARKETPLACE_COLORS.length]} />)}
            </div>
          )}
          {(subscriptions.data?.length ?? 0) > 0 && (
            <section className="mt-8">
              <h2 className="mb-3" style={{ fontSize: "1.1rem", fontWeight: 650 }}>Your Marketplace purchases</h2>
              <div className="grid gap-3 md:grid-cols-2">
                {subscriptions.data?.map((subscription) => (
                  <Link key={`${subscription.listing.slug}:${subscription.account_login}`} to={`/ui/marketplace/${subscription.listing.slug}`} style={{ color: "inherit", textDecoration: "none" }}>
                    <Box style={{ height: "100%", boxShadow: "var(--shadow-resting)" }}>
                      <div className="flex items-center justify-between gap-3" style={{ padding: "1rem" }}>
                        <div><b>{subscription.listing.name}</b><p className="mt-1 marketplace-muted">{subscription.account_login} · {subscription.marketplace_purchase.plan.name}</p></div>
                        <StateLabel state={subscription.marketplace_pending_change ? "draft" : "open"}>{subscription.marketplace_pending_change ? "Changes scheduled" : "Active"}</StateLabel>
                      </div>
                    </Box>
                  </Link>
                ))}
              </div>
            </section>
          )}
        </main>
      </div>
    </div>
  );
}

function ListingCard({ listing, color }: { listing: GithubMarketplaceListing; color: string }) {
  const free = listing.plans.some((plan) => plan.price_model === "FREE");
  const trial = listing.plans.some((plan) => plan.has_free_trial);
  return (
    <Link to={`/ui/marketplace/${listing.slug}`} className="marketplace-card">
      <div className="marketplace-card-accent" style={{ background: color }} />
      <div className="marketplace-card-body">
        <div className="marketplace-app-mark" style={{ background: `color-mix(in srgb, ${color} 18%, var(--color-surface))`, color }}>
          {listing.name.slice(0, 1).toUpperCase()}
        </div>
        <div className="min-w-0">
          <h3>{listing.name}</h3>
          <p>{listing.description}</p>
          <div className="marketplace-tags">
            {free && <span>Free plan</span>}
            {trial && <span>Free trial</span>}
            <span>{listing.github_app_id ? "GitHub App" : "OAuth App"}</span>
          </div>
        </div>
      </div>
    </Link>
  );
}

function MarketplaceDetail({ slug }: { slug: string }) {
  const client = useQueryClient();
  const listing = useQuery({ queryKey: ["marketplace", "listing", slug], queryFn: () => fetchMarketplaceListing(slug) });
  const accounts = useQuery({ queryKey: ["marketplace", "accounts"], queryFn: fetchMarketplaceAccounts });
  const subscriptions = useQuery({ queryKey: ["marketplace", "subscriptions"], queryFn: fetchMarketplaceSubscriptions });
  const [account, setAccount] = useState("");
  const [planID, setPlanID] = useState<number | null>(null);
  const [billingCycle, setBillingCycle] = useState<"monthly" | "yearly">("monthly");
  const [unitCount, setUnitCount] = useState(1);
  const [freeTrial, setFreeTrial] = useState(false);
  const [setupURL, setSetupURL] = useState<string | null>(null);

  const selectedAccount = account || accounts.data?.[0]?.login || "";
  const current = subscriptions.data?.find((subscription) => subscription.listing.slug === slug && subscription.account_login === selectedAccount);
  const selectedPlanID = planID ?? current?.marketplace_purchase.plan.id ?? listing.data?.plans[0]?.id ?? null;
  const selectedPlan = listing.data?.plans.find((plan) => plan.id === selectedPlanID);
  const invalidate = async () => {
    await client.invalidateQueries({ queryKey: ["marketplace"] });
  };
  const save = useMutation({
    mutationFn: async () => {
      if (!selectedPlan) throw new Error("Choose a Marketplace plan");
      const payload = { account: selectedAccount, plan_id: selectedPlan.id, billing_cycle: billingCycle, unit_count: unitCount };
      if (current) return changeMarketplacePlan(slug, payload);
      return purchaseMarketplacePlan(slug, { ...payload, free_trial: freeTrial });
    },
    onSuccess: async (subscription) => { setSetupURL(subscription.setup_url); await invalidate(); },
  });
  const cancel = useMutation({
    mutationFn: () => cancelMarketplacePlan(slug, selectedAccount),
    onSuccess: invalidate,
  });

  if (listing.isLoading || accounts.isLoading || subscriptions.isLoading) return <Spinner label="loading Marketplace listing" />;
  if (listing.isError || accounts.isError || subscriptions.isError) return <InlineError title="Marketplace listing unavailable" detail={String(listing.error || accounts.error || subscriptions.error)} />;
  if (!listing.data) return <Blankslate title="Listing not found" />;

  return (
    <div>
      <Link to="/ui/marketplace" className="marketplace-back">← GitHub Marketplace</Link>
      <section className="marketplace-detail-header">
        <div className="marketplace-detail-mark">{listing.data.name.slice(0, 1).toUpperCase()}</div>
        <div><div className="marketplace-eyebrow">{listing.data.github_app_id ? "GitHub App" : "OAuth App"}</div><h1>{listing.data.name}</h1><p>{listing.data.description}</p></div>
      </section>
      <div className="marketplace-detail-layout">
        <main>
          <Box header={<b>About this integration</b>}>
            <div style={{ padding: "1.25rem" }}><p style={{ lineHeight: 1.65 }}>{listing.data.full_description || listing.data.description}</p></div>
          </Box>
          <h2 className="mt-6 mb-3" style={{ fontSize: "1.15rem", fontWeight: 650 }}>Choose a plan</h2>
          <div className="grid gap-3">
            {listing.data.plans.map((plan) => (
              <PlanChoice key={plan.id} plan={plan} selected={selectedPlanID === plan.id} cycle={billingCycle} onSelect={() => setPlanID(plan.id)} />
            ))}
          </div>
        </main>
        <aside>
          <Box header={<b>{current ? "Manage your plan" : "Complete your order"}</b>} style={{ boxShadow: "var(--shadow-floating)" }}>
            <div style={{ padding: "1rem" }}>
              <FormLabel id="marketplace-account">Billing account</FormLabel>
              <select id="marketplace-account" className="w-full" value={selectedAccount} onChange={(event) => { setAccount(event.target.value); setPlanID(null); setSetupURL(null); }}>
                {accounts.data?.map((item) => <option key={`${item.type}:${item.id}`} value={item.login}>{item.login} ({item.type === "Organization" ? "organization" : "personal"})</option>)}
              </select>
              <div className="mt-4">
                <FormLabel id="marketplace-cycle">Billing cycle</FormLabel>
                <div id="marketplace-cycle" className="marketplace-cycle" role="group" aria-label="Billing cycle">
                  <button type="button" className={billingCycle === "monthly" ? "active" : ""} onClick={() => setBillingCycle("monthly")}>Monthly</button>
                  <button type="button" className={billingCycle === "yearly" ? "active" : ""} onClick={() => setBillingCycle("yearly")}>Yearly <span>save more</span></button>
                </div>
              </div>
              {selectedPlan?.price_model === "PER_UNIT" && <div className="mt-4"><FormLabel id="marketplace-units">{selectedPlan.unit_name || "Units"}</FormLabel><input id="marketplace-units" className="w-full" type="number" min={1} value={unitCount} onChange={(event) => setUnitCount(Number(event.target.value))} /></div>}
              {!current && selectedPlan?.has_free_trial && <label className="mt-4 flex items-start gap-2 marketplace-checkbox"><input type="checkbox" checked={freeTrial} onChange={(event) => setFreeTrial(event.target.checked)} /><span><b>Start with a 14-day free trial</b><small>You can cancel immediately during the trial.</small></span></label>}
              {current?.marketplace_pending_change && <div className="marketplace-pending mt-4"><b>{current.marketplace_pending_change.cancellation ? "Cancellation scheduled" : "Plan change scheduled"}</b><span>Effective {new Date(current.marketplace_pending_change.effective_date).toLocaleDateString()}</span></div>}
              {(save.error || cancel.error) && <div className="mt-4"><ErrorBanner>{String(save.error || cancel.error)}</ErrorBanner></div>}
              <Button className="mt-5 w-full" variant="primary" disabled={!selectedPlan || save.isPending || !selectedAccount} onClick={() => save.mutate()}>
                {current ? "Update plan" : "Complete order and begin installation"}
              </Button>
              {setupURL && <a className="marketplace-setup mt-3" href={setupURL}>Continue to {listing.data.name} setup →</a>}
              {current && <Button className="mt-2 w-full" variant="danger" disabled={cancel.isPending} onClick={() => { if (confirm(`Cancel ${listing.data.name} for ${selectedAccount}?`)) cancel.mutate(); }}>Cancel plan</Button>}
              <p className="marketplace-fine-print">Plan changes follow GitHub Marketplace billing boundaries. Upgrades begin now; downgrades and paid cancellations begin next cycle.</p>
            </div>
          </Box>
        </aside>
      </div>
    </div>
  );
}

function PlanChoice({ plan, selected, cycle, onSelect }: { plan: GithubMarketplacePlan; selected: boolean; cycle: "monthly" | "yearly"; onSelect: () => void }) {
  const amount = cycle === "yearly" ? plan.yearly_price_in_cents : plan.monthly_price_in_cents;
  return (
    <button type="button" className={`marketplace-plan ${selected ? "selected" : ""}`} onClick={onSelect} aria-pressed={selected}>
      <span className="marketplace-plan-radio">{selected && <CheckIcon size={14} />}</span>
      <span className="marketplace-plan-copy"><b>{plan.name}</b><small>{plan.description}</small><span>{plan.bullets.map((bullet) => <em key={bullet}><CheckIcon size={13} /> {bullet}</em>)}</span></span>
      <span className="marketplace-price">{amount === 0 ? <b>Free</b> : <><b>${(amount / 100).toLocaleString()}</b><small>/{cycle === "yearly" ? "year" : "month"}{plan.price_model === "PER_UNIT" ? `/${plan.unit_name || "unit"}` : ""}</small></>}</span>
    </button>
  );
}
