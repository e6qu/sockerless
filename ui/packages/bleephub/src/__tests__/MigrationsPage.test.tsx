import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { MigrationsPage } from "../pages/MigrationsPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown) {
  return new Response(JSON.stringify(data), {
    status: 200,
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
      <BrowserRouter>
        <MigrationsPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const baseRepo = {
  id: 10,
  name: "repo",
  full_name: "admin/repo",
  description: "",
  homepage: null,
  default_branch: "main",
  visibility: "public",
  private: false,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
  pushed_at: null,
  size: 0,
  owner: { login: "admin", type: "User" },
  license: null,
  has_issues: true,
  has_projects: true,
  has_wiki: true,
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
  pull_request_creation_policy: "maintainer",
};

const userMigration = {
  id: 1,
  node_id: "M_kgDO00000001",
  guid: "guid-1",
  state: "exported",
  repositories: [baseRepo],
  lock_repositories: true,
  exclude_metadata: false,
  exclude_git_data: false,
  exclude_attachments: false,
  exclude_releases: false,
  exclude_owner_projects: false,
  org_metadata_only: false,
  url: "/api/v3/user/migrations/1",
  html_url: "/ui/migrations/1",
  archive_url: "/api/v3/user/migrations/1/archive",
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
  exported_at: "2024-01-01T00:00:00Z",
};

const orgMigration = {
  ...userMigration,
  id: 2,
  node_id: "M_kgDO00000002",
  guid: "guid-2",
};

function mockEndpoints() {
  mockFetch.mockImplementation((url: string) => {
    if (url === "/api/v3/user/migrations") return Promise.resolve(jsonResponse([userMigration]));
    if (url === "/api/v3/orgs/acme/migrations") return Promise.resolve(jsonResponse([orgMigration]));
    if (url === "/api/v3/user/repos?per_page=100") return Promise.resolve(jsonResponse([baseRepo]));
    return Promise.resolve(jsonResponse({}));
  });
}

describe("MigrationsPage", () => {
  it("renders user migrations", async () => {
    mockEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Migrations")).toBeInTheDocument();
      expect(screen.getByText(/1 rows/)).toBeInTheDocument();
    });
  });

  it("switches to organization migrations", async () => {
    mockEndpoints();
    renderPage();
    fireEvent.click(screen.getByText("Organization"));
    const input = screen.getByPlaceholderText("org-login");
    fireEvent.change(input, { target: { value: "acme" } });
    fireEvent.click(screen.getByText("Load"));
    await waitFor(() => {
      expect(input).toHaveValue("acme");
      expect(screen.getByText(/1 rows/)).toBeInTheDocument();
    });
  });

  it("loads migration repository choices from GitHub REST user repositories", async () => {
    mockEndpoints();
    renderPage();
    fireEvent.click(screen.getByText("New migration"));
    await waitFor(() => {
      expect(screen.getByRole("checkbox", { name: "admin/repo" })).toBeInTheDocument();
    });
    const calls = mockFetch.mock.calls.map((c) => c[0].toString());
    expect(calls).toContain("/api/v3/user/repos?per_page=100");
    expect(calls).not.toContain("/internal/repos");
  });
});
