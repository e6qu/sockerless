import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { RepoSocialPage, type RepoSocialKind } from "../pages/RepoSocialPage.js";

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

function renderPage(kind: RepoSocialKind) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/ui/repos/admin/social-repo/${kind}`]}>
        <Routes>
          <Route path={`/ui/repos/:owner/:repo/${kind}`} element={<RepoSocialPage kind={kind} />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function routeSocial(target: string, targetResponse: Response) {
  mockFetch.mockImplementation((url: RequestInfo | URL) => {
    const path = String(url);
    if (path === "/ui-data/repos/admin/social-repo/viewer") {
      return Promise.resolve(jsonResponse({ starred: false, subscribed: false }));
    }
    if (path === "/api/v3/repos/admin/social-repo") {
      return Promise.resolve(jsonResponse({ stargazers_count: 0, subscribers_count: 0, forks_count: 0 }));
    }
    if (path.startsWith(target)) return Promise.resolve(targetResponse.clone());
    return Promise.resolve(jsonResponse([]));
  });
}

describe("RepoSocialPage", () => {
  it("lists stargazers from GET /stargazers", async () => {
    routeSocial(
      "/api/v3/repos/admin/social-repo/stargazers",
      jsonResponse([
        { id: 1, login: "star-a", avatar_url: "", type: "User", site_admin: false },
        { id: 2, login: "star-b", avatar_url: "", type: "User", site_admin: false },
      ]),
    );
    renderPage("stargazers");
    await waitFor(() => {
      expect(screen.getByText("star-a")).toBeInTheDocument();
    });
    expect(screen.getByText("star-b")).toBeInTheDocument();
    expect(
      mockFetch.mock.calls.some((c) =>
        String(c[0]).startsWith("/api/v3/repos/admin/social-repo/stargazers"),
      ),
    ).toBe(true);
  });

  it("shows an honest empty state for watchers", async () => {
    routeSocial("/api/v3/repos/admin/social-repo/subscribers", jsonResponse([]));
    renderPage("watchers");
    await waitFor(() => {
      expect(screen.getByText("No watchers yet")).toBeInTheDocument();
    });
    expect(
      mockFetch.mock.calls.some((c) =>
        String(c[0]).startsWith("/api/v3/repos/admin/social-repo/subscribers"),
      ),
    ).toBe(true);
  });

  it("lists forks linking to the forked repositories", async () => {
    routeSocial(
      "/api/v3/repos/admin/social-repo/forks",
      jsonResponse([
        {
          id: 5,
          full_name: "carol/social-repo",
          updated_at: "2026-06-01T00:00:00Z",
        },
      ]),
    );
    renderPage("forks");
    await waitFor(() => {
      expect(screen.getByText("carol/social-repo")).toBeInTheDocument();
    });
    expect(screen.getByText("carol/social-repo").closest("a")).toHaveAttribute(
      "href",
      "/ui/repos/carol/social-repo",
    );
  });

  it("surfaces list failures", async () => {
    routeSocial(
      "/api/v3/repos/admin/social-repo/stargazers",
      jsonResponse({ message: "boom" }, 500),
    );
    renderPage("stargazers");
    await waitFor(() => {
      expect(screen.getByText("Failed to load stargazers")).toBeInTheDocument();
    });
  });
});
