// Error-path hardening for the pages added across the recent feature PRs.
// Each page is rendered against a fetch layer that fails (HTTP 500), rejects
// (network error), or returns a malformed shape, and must degrade to a
// visible InlineError/ErrorBanner — never a blank screen, never a thrown
// render (which would blank the whole app), never fabricated data. A
// render that throws makes render()/act() reject and fails the test; a
// blank screen makes the findBy* time out and fails the test.
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import type { ReactElement } from "react";

import { InsightsPage } from "../pages/InsightsPage.js";
import { PullsPage } from "../pages/PullsPage.js";
import { DeploymentsPage } from "../pages/DeploymentsPage.js";
import { WebhookDeliveriesPage } from "../pages/WebhookDeliveriesPage.js";
import { SearchPage } from "../pages/SearchPage.js";
import { AccountPage } from "../pages/AccountPage.js";
import { OrgGovernancePage } from "../pages/OrgGovernancePage.js";
import { EnterprisePage } from "../pages/EnterprisePage.js";
import { CopilotPage } from "../pages/CopilotPage.js";
import { RepoSocialPage } from "../pages/RepoSocialPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

function jsonResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function renderAt(routePath: string, entry: string, element: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[entry]}>
        <Routes>
          <Route path={routePath} element={element} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/** Every page under test, with the route + a URL that drives its primary fetch. */
const PAGES: Array<{ name: string; routePath: string; entry: string; element: ReactElement }> = [
  {
    name: "InsightsPage",
    routePath: "/ui/repos/:owner/:repo/insights",
    entry: "/ui/repos/admin/r/insights",
    element: <InsightsPage />,
  },
  {
    name: "PullsPage (PR detail / reviews)",
    routePath: "/ui/repos/:owner/:repo/pulls/:number",
    entry: "/ui/repos/admin/r/pulls/1",
    element: <PullsPage />,
  },
  {
    name: "PullsPage (list)",
    routePath: "/ui/repos/:owner/:repo/pulls",
    entry: "/ui/repos/admin/r/pulls",
    element: <PullsPage />,
  },
  {
    name: "DeploymentsPage",
    routePath: "/ui/repos/:owner/:repo/deployments",
    entry: "/ui/repos/admin/r/deployments",
    element: <DeploymentsPage />,
  },
  {
    name: "WebhookDeliveriesPage",
    routePath: "/ui/repos/:owner/:repo/hooks/:hookId/deliveries",
    entry: "/ui/repos/admin/r/hooks/3/deliveries",
    element: <WebhookDeliveriesPage />,
  },
  {
    name: "SearchPage",
    routePath: "/ui/search",
    entry: "/ui/search?q=hello&type=repositories",
    element: <SearchPage />,
  },
  {
    name: "AccountPage",
    routePath: "/ui/account",
    entry: "/ui/account",
    element: <AccountPage />,
  },
  {
    name: "OrgGovernancePage",
    routePath: "/ui/orgs/:org/governance",
    entry: "/ui/orgs/acme/governance",
    element: <OrgGovernancePage />,
  },
  {
    name: "EnterprisePage",
    routePath: "/ui/admin/enterprise",
    entry: "/ui/admin/enterprise",
    element: <EnterprisePage />,
  },
  {
    name: "CopilotPage",
    routePath: "/ui/orgs/:org/copilot",
    entry: "/ui/orgs/acme/copilot",
    element: <CopilotPage />,
  },
  {
    name: "RepoSocialPage (stargazers)",
    routePath: "/ui/repos/:owner/:repo/stargazers",
    entry: "/ui/repos/admin/r/stargazers",
    element: <RepoSocialPage kind="stargazers" />,
  },
];

// Any InlineError/ErrorBanner surface the pages under test render.
const ERROR_TEXT = /Failed to (load|resolve)|Search failed|Invalid webhook route/i;

describe("page error paths — Hypertext Transfer Protocol 500 degrades to a visible error, never a crash", () => {
  for (const p of PAGES) {
    it(`${p.name} renders an error surface when the application programming interface returns 500`, async () => {
      mockFetch.mockResolvedValue(jsonResponse({ message: "Internal Server Error" }, 500));
      renderAt(p.routePath, p.entry, p.element);
      const errs = await screen.findAllByText(ERROR_TEXT, undefined, { timeout: 3000 });
      expect(errs.length).toBeGreaterThan(0);
    });
  }
});

describe("page error paths — network rejection degrades to a visible error", () => {
  for (const p of PAGES) {
    it(`${p.name} renders an error surface when fetch rejects`, async () => {
      mockFetch.mockRejectedValue(new TypeError("network down"));
      renderAt(p.routePath, p.entry, p.element);
      const errs = await screen.findAllByText(ERROR_TEXT, undefined, { timeout: 3000 });
      expect(errs.length).toBeGreaterThan(0);
    });
  }
});

describe("page error paths — malformed 200 shapes surface as an error, not fabricated data", () => {
  it("CopilotPage: a seats envelope missing its array is an error, not zero seats", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (String(url).includes("/copilot/billing/seats")) {
        // 200 but the "seats" array is absent — the strict parser must throw.
        return Promise.resolve(jsonResponse({ total_seats: 5 }));
      }
      if (String(url).includes("/copilot/billing")) {
        return Promise.resolve(jsonResponse({ seat_breakdown: { total: 5 }, seat_management_setting: "assign_selected" }));
      }
      return Promise.resolve(jsonResponse({ spaces: [] }));
    });
    renderAt("/ui/orgs/:org/copilot", "/ui/orgs/acme/copilot", <CopilotPage />);
    expect(
      (await screen.findAllByText(/Failed to load Copilot seats/i, undefined, { timeout: 3000 })).length,
    ).toBeGreaterThan(0);
  });

  it("DeploymentsPage: a non-array deployments body is an error, not an empty list", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (String(url).includes("/deployments?")) {
        // 200 but the paginated body is an object, not the expected JSON array.
        return Promise.resolve(jsonResponse({ items: [] }));
      }
      return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
    });
    renderAt(
      "/ui/repos/:owner/:repo/deployments",
      "/ui/repos/admin/r/deployments",
      <DeploymentsPage />,
    );
    expect(
      (await screen.findAllByText(/Failed to load deployments/i, undefined, { timeout: 3000 })).length,
    ).toBeGreaterThan(0);
  });
});
