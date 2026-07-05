import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { OrgOverviewPage } from "../pages/OrgOverviewPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

function renderPage(org = "acme") {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/ui/orgs/${org}`]}>
        <Routes>
          <Route path="/ui/orgs/:org" element={<OrgOverviewPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const orgProfile = {
  login: "acme",
  id: 1,
  avatar_url: "",
  description: "Acme Corp",
  name: "Acme",
  company: null,
  blog: "https://acme.example",
  location: "Cloud",
  email: "team@acme.example",
  twitter_username: null,
  public_repos: 2,
  followers: 0,
  following: 0,
  html_url: "http://x/acme",
  created_at: "2026-01-01T00:00:00Z",
};

const repo = {
  id: 1,
  name: "api",
  full_name: "acme/api",
  description: "the api",
  default_branch: "main",
  visibility: "public",
  private: false,
  updated_at: "2026-02-01T00:00:00Z",
};

describe("OrgOverviewPage", () => {
  it("renders the org profile, member count, and repository preview", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/repos")) return Promise.resolve(jsonResponse([repo], 200, { Link: "" }));
      if (u.includes("/members")) return Promise.resolve(jsonResponse([{ id: 2, login: "dev", avatar_url: "", type: "User", site_admin: false }]));
      return Promise.resolve(jsonResponse(orgProfile));
    });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Acme")).toBeInTheDocument();
    });
    expect(screen.getByText("Acme Corp")).toBeInTheDocument();
    expect(screen.getByText("1 member")).toBeInTheDocument();
    expect(screen.getByText("2 repositories")).toBeInTheDocument();
    expect(screen.getByText("api")).toBeInTheDocument();
    // OrgHeader is present (uniform org chrome).
    expect(screen.getByRole("navigation", { name: /organization/i })).toBeInTheDocument();
  });

  it("surfaces an org load error", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/repos")) return Promise.resolve(jsonResponse([], 200, { Link: "" }));
      if (u.includes("/members")) return Promise.resolve(jsonResponse([]));
      return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
    });
    renderPage("missing");
    await waitFor(() => {
      expect(screen.getByText(/failed to load organization/i)).toBeInTheDocument();
    });
  });
});
