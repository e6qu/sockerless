import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { MarketplacePage } from "../pages/MarketplacePage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), { status, headers: { "Content-Type": "application/json" } });
}

const plans = [
  { id: 11, number: 11, name: "Community", description: "Open source", monthly_price_in_cents: 0, yearly_price_in_cents: 0, price_model: "FREE", has_free_trial: false, unit_name: null, state: "published", bullets: ["Public repositories"], url: "/plans/11", accounts_url: "/plans/11/accounts" },
  { id: 12, number: 12, name: "Team", description: "Private projects", monthly_price_in_cents: 1200, yearly_price_in_cents: 12000, price_model: "FLAT_RATE", has_free_trial: true, unit_name: null, state: "published", bullets: ["Private repositories"], url: "/plans/12", accounts_url: "/plans/12/accounts" },
] as const;
const listing = { slug: "spark-app", name: "Spark App", description: "A bright developer workflow", full_description: "Automate reviews and ship with confidence.", setup_url: "https://example.test/setup", installation_url: null, github_app_id: 7, oauth_app_client_id: null, webhook_url: "https://example.test/hook", webhook_content_type: "json", webhook_active: true, published: true, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z", plans };
const account = { id: 2, login: "mona", type: "User", avatar_url: "" };

function renderAt(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={[path]}><Routes><Route path="/ui/marketplace" element={<MarketplacePage />} /><Route path="/ui/marketplace/:slug" element={<MarketplacePage />} /></Routes></MemoryRouter></QueryClientProvider>);
}

afterEach(() => { cleanup(); mockFetch.mockReset(); });

describe("MarketplacePage", () => {
  it("renders a GitHub-organized saturated Marketplace directory", async () => {
    mockFetch.mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/subscriptions")) return Promise.resolve(jsonResponse([]));
      return Promise.resolve(jsonResponse([listing]));
    });
    renderAt("/ui/marketplace");
    expect(await screen.findByRole("heading", { name: "Build more. Ship brighter." })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Apps for every workflow" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Spark App/ })).toHaveAttribute("href", "/ui/marketplace/spark-app");
    expect(document.querySelector(".marketplace-hero")).toBeInTheDocument();
  });

  it("completes a real plan purchase and exposes the setup handoff", async () => {
    mockFetch.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (init?.method === "POST") return Promise.resolve(jsonResponse({ id: account.id, login: account.login, type: account.type, account_login: account.login, listing, setup_url: "https://example.test/setup?installation_id=41&marketplace_listing_plan_id=12", marketplace_pending_change: null, marketplace_purchase: { billing_cycle: "monthly", next_billing_date: "2026-02-01T00:00:00Z", is_installed: true, unit_count: 0, on_free_trial: true, free_trial_ends_on: "2026-01-15T00:00:00Z", updated_at: "2026-01-01T00:00:00Z", plan: plans[1] } }, 201));
      if (url.endsWith("/accounts")) return Promise.resolve(jsonResponse([account]));
      if (url.endsWith("/subscriptions")) return Promise.resolve(jsonResponse([]));
      return Promise.resolve(jsonResponse(listing));
    });
    renderAt("/ui/marketplace/spark-app");
    expect(await screen.findByRole("heading", { name: "Spark App" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Team Private projects/ }));
    fireEvent.click(screen.getByRole("checkbox", { name: /14-day free trial/ }));
    fireEvent.click(screen.getByRole("button", { name: "Complete order and begin installation" }));
    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith("/ui-data/marketplace/listings/spark-app/purchase", expect.objectContaining({ method: "POST" })));
    expect(await screen.findByRole("link", { name: /Continue to Spark App setup/ })).toHaveAttribute("href", expect.stringContaining("installation_id=41"));
  });
});
