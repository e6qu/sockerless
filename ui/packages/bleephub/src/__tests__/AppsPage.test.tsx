import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { AppsPage } from "../pages/AppsPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter><AppsPage /></MemoryRouter>
    </QueryClientProvider>,
  );
}

const apps = [
  {
    id: 1,
    node_id: "A_1",
    slug: "ci-bot",
    name: "CI Bot",
    client_id: "Iv1.example",
    description: "Helper",
    external_url: "https://example.test",
    html_url: "https://github.com/apps/ci-bot",
    permissions: {},
    events: [],
    installations_count: 1,
    owner: { id: 1, login: "octocat" },
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
];

const installations = [
  {
    id: 100,
    app_id: 1,
    app_slug: "ci-bot",
    target_type: "User",
    target_id: 1,
    account: { login: "octocat" },
    repository_selection: "all",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    suspended_at: null,
  },
];

function routedFetch(url: RequestInfo | URL): Promise<Response> {
  const u = typeof url === "string" ? url : url.toString();
  if (u === "/settings/apps") return Promise.resolve(jsonResponse(apps));
  if (u.includes("/api/v3/user/installations")) {
    return Promise.resolve(jsonResponse({ total_count: installations.length, installations }));
  }
  if (u === "/settings/oauth-apps") return Promise.resolve(jsonResponse([]));
  return Promise.resolve(jsonResponse([]));
}

describe("AppsPage", () => {
  it("renders GitHub Apps, Installations, and OAuth Apps tabs", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "GitHub Apps" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Installations" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "OAuth Apps" })).toBeInTheDocument();
    });
  });

  it("Apps tab shows the app rows", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("ci-bot")).toBeInTheDocument();
      expect(screen.getByText("CI Bot")).toBeInTheDocument();
    });
  });

  it("Installations tab shows installation rows", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("ci-bot")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: "Installations" }));
    await waitFor(() => {
      expect(screen.getByText("octocat")).toBeInTheDocument();
    });
  });

  it("opens the Create App dialog", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    // Header CTA on the default GitHub Apps tab is "New GitHub app".
    const cta = await screen.findByRole("button", { name: /new github app/i });
    fireEvent.click(cta);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /create github app/i })).toBeInTheDocument();
      expect(screen.getByLabelText(/^name$/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/description/i)).toBeInTheDocument();
    });
  });
});
