import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { InsightsPage } from "../pages/InsightsPage.js";

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

function renderAt(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/ui/repos/:owner/:repo/insights" element={<InsightsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const communityProfile = {
  health_percentage: 43,
  description: "a repo",
  documentation: null,
  files: {
    code_of_conduct: null,
    code_of_conduct_file: null,
    license: { key: "mit", name: "MIT License", spdx_id: "MIT" },
    contributing: null,
    readme: { url: "u", html_url: "h" },
    issue_template: null,
    pull_request_template: null,
  },
  updated_at: "2026-01-01T00:00:00Z",
};

function mockInsightsEndpoints(overrides: Record<string, () => Response> = {}) {
  mockFetch.mockImplementation((url: RequestInfo | URL) => {
    const u = url.toString();
    for (const [needle, make] of Object.entries(overrides)) {
      if (u.includes(needle)) return Promise.resolve(make());
    }
    if (u.includes("/community/profile")) return Promise.resolve(jsonResponse(communityProfile));
    if (u.includes("/contributors")) {
      return Promise.resolve(
        jsonResponse([
          { login: "admin", avatar_url: "", type: "User", contributions: 12 },
          { type: "Anonymous", name: "Ghost", email: "ghost@example.com", contributions: 2 },
        ]),
      );
    }
    if (u.includes("/stats/commit_activity")) {
      const weeks = Array.from({ length: 52 }, (_, i) => ({
        week: 1700000000 + i * 604800,
        days: [0, 0, 0, 0, 0, 0, 0],
        total: 0,
      }));
      weeks[51] = { week: weeks[51].week, days: [0, 3, 0, 1, 0, 0, 0], total: 4 };
      return Promise.resolve(jsonResponse(weeks));
    }
    if (u.includes("/traffic/views")) {
      return Promise.resolve(jsonResponse({ count: 0, uniques: 0, views: [] }));
    }
    if (u.includes("/traffic/clones")) {
      return Promise.resolve(
        jsonResponse({
          count: 5,
          uniques: 2,
          clones: [{ timestamp: "2026-07-01T00:00:00Z", count: 5, uniques: 2 }],
        }),
      );
    }
    if (u.includes("/traffic/popular/paths")) return Promise.resolve(jsonResponse([]));
    if (u.includes("/traffic/popular/referrers")) return Promise.resolve(jsonResponse([]));
    // useOpenCounts issue/PR badge fetches
    return Promise.resolve(jsonResponse([]));
  });
}

describe("InsightsPage", () => {
  it("renders contributors, community health, commit activity, and traffic", async () => {
    mockInsightsEndpoints();
    renderAt("/ui/repos/admin/test/insights");

    await waitFor(() => {
      expect(screen.getByText("@admin")).toBeInTheDocument();
    });
    expect(screen.getByText("12 commits")).toBeInTheDocument();
    // anonymous contributor rendered by name/email
    expect(screen.getByText(/Ghost <ghost@example.com>/)).toBeInTheDocument();
    // community health score
    expect(screen.getByText("43%")).toBeInTheDocument();
    // commit activity total
    expect(screen.getByText(/4 commits on the default branch/)).toBeInTheDocument();
    // clone traffic bucket list rendered, view traffic honestly empty
    expect(screen.getByText(/5 \(2 unique\)/)).toBeInTheDocument();
    expect(screen.getByText(/No views in the last 14 days/)).toBeInTheDocument();
    // popular content empty states
    expect(screen.getByText(/No path traffic recorded/)).toBeInTheDocument();
    expect(screen.getByText(/No referrer traffic recorded/)).toBeInTheDocument();
  });

  it("shows an honest empty state when contributors returns 204", async () => {
    mockInsightsEndpoints({
      "/contributors": () => new Response(null, { status: 204 }),
    });
    renderAt("/ui/repos/admin/test/insights");

    await waitFor(() => {
      expect(screen.getByText(/no contributors yet/i)).toBeInTheDocument();
    });
  });

  it("surfaces a section error when a fetch fails", async () => {
    mockInsightsEndpoints({
      "/community/profile": () => jsonResponse({ message: "boom" }, 500),
    });
    renderAt("/ui/repos/admin/test/insights");

    await waitFor(() => {
      expect(screen.getByText(/failed to load community profile/i)).toBeInTheDocument();
    });
    // other sections still render
    await waitFor(() => {
      expect(screen.getByText("@admin")).toBeInTheDocument();
    });
  });
});
