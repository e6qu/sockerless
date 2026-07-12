import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { MarketplacePublisherPage } from "../pages/MarketplacePublisherPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

const listing = { slug: "publisher-app", name: "Publisher App", description: "A listed app", full_description: "Complete publisher description", setup_url: "https://example.test/setup", installation_url: null, github_app_id: 8, oauth_app_client_id: null, webhook_url: "https://example.test/hook", webhook_content_type: "json", webhook_active: true, published: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z", plans: [{ id: 21, number: 21, name: "Community", description: "Free", monthly_price_in_cents: 0, yearly_price_in_cents: 0, price_model: "FREE", has_free_trial: false, unit_name: null, state: "published", bullets: [], url: "/plans/21", accounts_url: "/plans/21/accounts" }] };

function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, statusText: status === 404 ? "Not Found" : "OK", headers: { "Content-Type": "application/json" } });
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={["/ui/apps/publisher-app/marketplace"]}><Routes><Route path="/ui/apps/:publisher/marketplace" element={<MarketplacePublisherPage />} /></Routes></MemoryRouter></QueryClientProvider>);
}

afterEach(() => { cleanup(); mockFetch.mockReset(); });

describe("MarketplacePublisherPage", () => {
  it("renders durable listing, plan, webhook, and publication organization", async () => {
    mockFetch.mockResolvedValue(response({ listing }));
    renderPage();
    expect(await screen.findByRole("heading", { name: "Manage Marketplace listing" })).toBeInTheDocument();
    expect(screen.getByText("Community")).toBeInTheDocument();
    expect(screen.getByLabelText("Payload URL")).toHaveValue("https://example.test/hook");
    expect(screen.getByText("Publication checklist")).toBeInTheDocument();
  });

  it("creates a listing draft through the publisher settings route", async () => {
    mockFetch.mockImplementation((_input: RequestInfo | URL, init?: RequestInit) => init?.method === "PUT" ? Promise.resolve(response(listing, 201)) : Promise.resolve(response({ listing: null })));
    renderPage();
    expect(await screen.findByRole("heading", { name: "Create Marketplace listing" })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Listing name"), { target: { value: "Publisher App" } });
    fireEvent.change(screen.getByLabelText("Short description"), { target: { value: "A listed app" } });
    fireEvent.change(screen.getByLabelText("Setup URL"), { target: { value: "https://example.test/setup" } });
    fireEvent.change(screen.getByLabelText("Payload URL"), { target: { value: "https://example.test/hook" } });
    fireEvent.click(screen.getByRole("button", { name: "Create draft listing" }));
    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith("/settings/apps/publisher-app/marketplace", expect.objectContaining({ method: "PUT" })));
  });
});
