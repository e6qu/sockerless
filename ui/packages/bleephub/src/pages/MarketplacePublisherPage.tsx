import { useState, type FormEvent, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { InlineError, Spinner } from "@sockerless/ui-core/components";
import { Link, useParams } from "react-router";
import {
  createMarketplacePlanSettings,
  fetchMarketplaceListingSettings,
  isNotFound,
  saveMarketplaceListingSettings,
  type MarketplaceListingSettingsPayload,
} from "../api.js";
import type { GithubMarketplaceListingSettings } from "../types.js";
import { Box, Button, ErrorBanner, FormLabel, PageTitle, StateLabel } from "../components/ui.js";
import { PackageIcon, PlusIcon } from "../components/octicons.js";

export function MarketplacePublisherPage() {
  const { publisher = "" } = useParams<{ publisher: string }>();
  const query = useQuery({ queryKey: ["marketplace", "publisher", publisher], queryFn: () => fetchMarketplaceListingSettings(publisher), retry: false });
  if (query.isLoading) return <Spinner label="loading Marketplace listing settings" />;
  if (query.isError && !isNotFound(query.error)) return <InlineError title="Marketplace listing settings unavailable" detail={String(query.error)} />;
  return <PublisherEditor key={`${publisher}:${query.data?.updated_at ?? "new"}`} publisher={publisher} listing={query.data ?? undefined} />;
}

function PublisherEditor({ publisher, listing }: { publisher: string; listing?: GithubMarketplaceListingSettings }) {
  const client = useQueryClient();
  const [form, setForm] = useState<MarketplaceListingSettingsPayload>({
    name: listing?.name ?? "",
    description: listing?.description ?? "",
    full_description: listing?.full_description ?? "",
    setup_url: listing?.setup_url ?? "",
    installation_url: listing?.installation_url ?? "",
    webhook_url: listing?.webhook_url ?? "",
    webhook_secret: "",
    webhook_content_type: listing?.webhook_content_type ?? "json",
    webhook_active: listing?.webhook_active ?? true,
    published: listing?.published ?? false,
  });
  const [showPlan, setShowPlan] = useState(false);
  const save = useMutation({
    mutationFn: () => saveMarketplaceListingSettings(publisher, form),
    onSuccess: async () => client.invalidateQueries({ queryKey: ["marketplace"] }),
  });

  return (
    <div>
      <div className="mb-4"><Link to="/ui/apps" className="marketplace-back">← Apps & installations</Link></div>
      <PageTitle
        icon={<PackageIcon size={24} />}
        title={`${listing ? "Manage" : "Create"} Marketplace listing`}
        meta={<>Publish <b>{publisher}</b> with real plans, installation handoff, and a dedicated GitHub Marketplace webhook.</>}
        actions={listing && <StateLabel state={listing.published ? "open" : "draft"}>{listing.published ? "Published" : "Draft"}</StateLabel>}
      />
      <div className="marketplace-publisher-layout">
        <form onSubmit={(event) => { event.preventDefault(); save.mutate(); }}>
          <Box header={<b>Listing details</b>}>
            <div className="grid gap-4" style={{ padding: "1rem" }}>
              <Field label="Listing name" id="marketplace-listing-name"><input id="marketplace-listing-name" className="w-full" required value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} /></Field>
              <Field label="Short description" id="marketplace-listing-description"><input id="marketplace-listing-description" className="w-full" required value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} /></Field>
              <Field label="Full description" id="marketplace-listing-full-description"><textarea id="marketplace-listing-full-description" className="w-full" rows={6} value={form.full_description} onChange={(event) => setForm({ ...form, full_description: event.target.value })} /></Field>
              <Field label="Setup URL" id="marketplace-listing-setup-url" hint="GitHub sends the buyer here after installing your app."><input id="marketplace-listing-setup-url" type="url" className="w-full" required value={form.setup_url} onChange={(event) => setForm({ ...form, setup_url: event.target.value })} /></Field>
            </div>
          </Box>
          <Box className="mt-4" header={<b>Marketplace webhook</b>}>
            <div className="grid gap-4" style={{ padding: "1rem" }}>
              <Field label="Payload URL" id="marketplace-webhook-url" hint="Receives purchased, changed, and cancelled marketplace_purchase events."><input id="marketplace-webhook-url" type="url" className="w-full" required value={form.webhook_url} onChange={(event) => setForm({ ...form, webhook_url: event.target.value })} /></Field>
              <Field label="Secret" id="marketplace-webhook-secret"><input id="marketplace-webhook-secret" type="password" className="w-full" value={form.webhook_secret} onChange={(event) => setForm({ ...form, webhook_secret: event.target.value })} /></Field>
              <div className="grid gap-3 sm:grid-cols-2"><Field label="Content type" id="marketplace-webhook-content"><select id="marketplace-webhook-content" className="w-full" value={form.webhook_content_type} onChange={(event) => setForm({ ...form, webhook_content_type: event.target.value as "json" | "form" })}><option value="json">application/json</option><option value="form">application/x-www-form-urlencoded</option></select></Field><label className="flex items-center gap-2" style={{ marginTop: "1.6rem", fontSize: ".82rem" }}><input type="checkbox" checked={form.webhook_active} onChange={(event) => setForm({ ...form, webhook_active: event.target.checked })} /> Active</label></div>
            </div>
          </Box>
          {save.error && <div className="mt-4"><ErrorBanner>{String(save.error)}</ErrorBanner></div>}
          <div className="mt-4 flex items-center justify-end gap-3">
            <label className="flex items-center gap-2" style={{ fontSize: ".82rem" }}><input type="checkbox" checked={form.published} onChange={(event) => setForm({ ...form, published: event.target.checked })} /> Publish listing</label>
            <Button variant="primary" type="submit" disabled={save.isPending}>{listing ? "Save listing" : "Create draft listing"}</Button>
          </div>
        </form>
        <aside>
          <Box header={<div className="flex w-full items-center justify-between"><b>Pricing plans</b>{listing && <Button size="sm" onClick={() => setShowPlan((value) => !value)}><PlusIcon size={14} /> Add plan</Button>}</div>}>
            {listing?.plans.length ? <div>{listing.plans.map((plan) => <div key={plan.id} className="marketplace-publisher-plan"><div><b>{plan.name}</b><p>{plan.description}</p></div><StateLabel state={plan.state === "published" ? "open" : "draft"}>{plan.state}</StateLabel></div>)}</div> : <div style={{ padding: "1rem", color: "var(--color-fg-muted)", fontSize: ".8rem" }}>{listing ? "Add at least one published plan before publishing this listing." : "Save the draft listing before adding pricing plans."}</div>}
          </Box>
          {listing && showPlan && <PlanForm publisher={publisher} onDone={() => setShowPlan(false)} />}
          <Box className="mt-4" header={<b>Publication checklist</b>}>
            <ul className="marketplace-checklist"><li className={form.name && form.description ? "done" : ""}>Complete listing description</li><li className={form.setup_url ? "done" : ""}>Setup URL configured</li><li className={form.webhook_url && form.webhook_active ? "done" : ""}>Marketplace webhook active</li><li className={listing?.plans.some((plan) => plan.state === "published") ? "done" : ""}>Published pricing plan</li></ul>
          </Box>
        </aside>
      </div>
    </div>
  );
}

function Field({ label, id, hint, children }: { label: string; id: string; hint?: string; children: ReactNode }) {
  return <div><FormLabel id={id}>{label}</FormLabel>{children}{hint && <p className="mt-1" style={{ color: "var(--color-fg-muted)", fontSize: ".7rem" }}>{hint}</p>}</div>;
}

function PlanForm({ publisher, onDone }: { publisher: string; onDone: () => void }) {
  const client = useQueryClient();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [model, setModel] = useState<"FREE" | "FLAT_RATE" | "PER_UNIT">("FREE");
  const [monthly, setMonthly] = useState(0);
  const [yearly, setYearly] = useState(0);
  const [trial, setTrial] = useState(false);
  const [unitName, setUnitName] = useState("");
  const mutation = useMutation({
    mutationFn: () => createMarketplacePlanSettings(publisher, { name, description, price_model: model, monthly_price_in_cents: monthly, yearly_price_in_cents: yearly, has_free_trial: trial, unit_name: unitName, state: "published", bullets: [] }),
    onSuccess: async () => { await client.invalidateQueries({ queryKey: ["marketplace"] }); onDone(); },
  });
  const submit = (event: FormEvent) => { event.preventDefault(); mutation.mutate(); };
  return <Box className="mt-4" header={<b>New pricing plan</b>}><form onSubmit={submit} className="grid gap-3" style={{ padding: "1rem" }}><Field label="Name" id="marketplace-plan-name"><input id="marketplace-plan-name" className="w-full" required value={name} onChange={(event) => setName(event.target.value)} /></Field><Field label="Description" id="marketplace-plan-description"><input id="marketplace-plan-description" className="w-full" value={description} onChange={(event) => setDescription(event.target.value)} /></Field><Field label="Pricing model" id="marketplace-plan-model"><select id="marketplace-plan-model" className="w-full" value={model} onChange={(event) => setModel(event.target.value as typeof model)}><option value="FREE">Free</option><option value="FLAT_RATE">Flat rate</option><option value="PER_UNIT">Per unit</option></select></Field>{model !== "FREE" && <div className="grid grid-cols-2 gap-2"><Field label="Monthly (cents)" id="marketplace-plan-monthly"><input id="marketplace-plan-monthly" className="w-full" type="number" min={0} value={monthly} onChange={(event) => setMonthly(Number(event.target.value))} /></Field><Field label="Yearly (cents)" id="marketplace-plan-yearly"><input id="marketplace-plan-yearly" className="w-full" type="number" min={0} value={yearly} onChange={(event) => setYearly(Number(event.target.value))} /></Field></div>}{model === "PER_UNIT" && <Field label="Unit name" id="marketplace-plan-unit"><input id="marketplace-plan-unit" className="w-full" required value={unitName} onChange={(event) => setUnitName(event.target.value)} /></Field>}<label className="flex items-center gap-2" style={{ fontSize: ".78rem" }}><input type="checkbox" checked={trial} onChange={(event) => setTrial(event.target.checked)} disabled={model === "FREE"} /> Offer a 14-day free trial</label>{mutation.error && <ErrorBanner>{String(mutation.error)}</ErrorBanner>}<div className="flex justify-end gap-2"><Button type="button" onClick={onDone}>Cancel</Button><Button type="submit" variant="primary" disabled={mutation.isPending}>Publish plan</Button></div></form></Box>;
}
