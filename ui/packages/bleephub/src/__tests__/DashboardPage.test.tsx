import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { DashboardPage } from "../pages/DashboardPage.js";

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

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/ui/"]}>
        <Routes>
          <Route path="/ui/" element={<DashboardPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const feedIssue = {
  id: 10,
  number: 7,
  title: "Flaky runner timeout",
  state: "open",
  comments: 2,
  updated_at: "2026-02-01T00:00:00Z",
  html_url: "http://x/acme/api/issues/7",
  repository: { full_name: "acme/api", name: "api", owner: { login: "acme" } },
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

describe("DashboardPage", () => {
  it("renders the top repositories rail and the activity feed", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/api/v3/user/repos")) return Promise.resolve(jsonResponse([repo], 200, { Link: "" }));
      if (u.includes("/api/v3/issues")) return Promise.resolve(jsonResponse([feedIssue]));
      if (u.includes("/api/v3/user")) return Promise.resolve(jsonResponse({ id: 1, login: "octocat", type: "User", site_admin: true, created_at: "2026-01-01T00:00:00Z" }));
      return Promise.resolve(jsonResponse([]));
    });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText("Flaky runner timeout")).toBeInTheDocument();
    });
    expect(screen.getByText("octocat")).toBeInTheDocument();
    // "acme/api" appears in both the top-repos rail and the feed row.
    expect(screen.getAllByText("acme/api").length).toBeGreaterThan(0);
    expect(screen.getByText("Recent activity")).toBeInTheDocument();
    // Quick links panel.
    expect(screen.getByText("System status")).toBeInTheDocument();
  });

  it("shows an honest empty state when there is no activity", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/api/v3/user/repos")) return Promise.resolve(jsonResponse([], 200, { Link: "" }));
      if (u.includes("/api/v3/issues")) return Promise.resolve(jsonResponse([]));
      if (u.includes("/api/v3/user")) return Promise.resolve(jsonResponse({ id: 1, login: "octocat", type: "User", site_admin: true, created_at: "2026-01-01T00:00:00Z" }));
      return Promise.resolve(jsonResponse([]));
    });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText(/no recent activity/i)).toBeInTheDocument();
    });
  });

  it("surfaces a feed error instead of swallowing it", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/api/v3/user/repos")) return Promise.resolve(jsonResponse([], 200, { Link: "" }));
      if (u.includes("/api/v3/issues")) return Promise.resolve(jsonResponse({ message: "boom" }, 500));
      if (u.includes("/api/v3/user")) return Promise.resolve(jsonResponse({ id: 1, login: "octocat", type: "User", site_admin: true, created_at: "2026-01-01T00:00:00Z" }));
      return Promise.resolve(jsonResponse([]));
    });
    renderPage();

    await waitFor(() => {
      expect(screen.getByText(/failed to load activity/i)).toBeInTheDocument();
    });
  });
});
