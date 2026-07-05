import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route, MemoryRouter } from "react-router";
import { OrgReposPage } from "../pages/OrgReposPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown, status = 200, headers?: Record<string, string>) {
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
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/ui/orgs/${org}/repos`]}>
        <Routes>
          <Route path="/ui/orgs/:org/repos" element={<OrgReposPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const orgRepo = {
  id: 1,
  name: "org-repo",
  full_name: "acme/org-repo",
  description: "org repo",
  homepage: null,
  default_branch: "main",
  visibility: "public",
  private: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-02T00:00:00Z",
  pushed_at: "2026-01-02T00:00:00Z",
  size: 0,
  owner: { login: "acme", type: "Organization" },
  organization: { login: "acme", type: "Organization" },
  license: null,
  has_issues: true,
  has_projects: false,
  has_wiki: false,
  has_pull_requests: true,
  is_template: false,
  archived: false,
  web_commit_signoff_required: false,
  allow_squash_merge: true,
  allow_merge_commit: true,
  allow_rebase_merge: true,
  allow_auto_merge: false,
  allow_update_branch: false,
  delete_branch_on_merge: false,
  use_squash_pr_title_as_default: false,
  squash_merge_commit_title: "COMMIT_OR_PR_TITLE",
  squash_merge_commit_message: "COMMIT_MESSAGES",
  merge_commit_title: "PR_TITLE",
  merge_commit_message: "PR_BODY",
  pull_request_creation_policy: "open",
};

describe("OrgReposPage", () => {
  it("renders the org repo list from /api/v3/orgs/{org}/repos", async () => {
    mockFetch.mockResolvedValue(jsonResponse([orgRepo], 200, { Link: '' }));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("acme/org-repo")).toBeInTheDocument();
    });
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/v3/orgs/acme/repos?per_page=30",
      expect.anything(),
    );
    // The org repos page now carries the shared OrgHeader tab bar so the
    // org chrome is uniform with the other org sub-pages.
    expect(screen.getByRole("navigation", { name: /organization/i })).toBeInTheDocument();
  });

  it("shows an error state when the org does not exist", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ message: "Not Found" }, 404));
    renderPage("missing");
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
      expect(screen.getByText(/failed to load/i)).toBeInTheDocument();
    });
  });
});
