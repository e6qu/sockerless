import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { CodespacesPage } from "../pages/CodespacesPage.js";

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

function renderPage(path = "/ui/codespaces") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/ui/codespaces" element={<CodespacesPage />} />
          <Route path="/ui/repos/:owner/:repo/codespaces" element={<CodespacesPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const machine = {
  name: "basicLinux32",
  display_name: "Basic Linux",
  operating_system: "linux",
  storage_in_bytes: 34359738368,
  memory_in_bytes: 4294967296,
  cpus: 2,
  prebuild_availability: "none",
};

const codespace = {
  id: 1,
  name: "crimson-spoon-abc123",
  display_name: "my codespace",
  environment_id: "abc",
  owner: { login: "admin", type: "User" },
  billable_owner: { login: "admin", type: "User" },
  repository: { id: 10, full_name: "admin/test", name: "test", owner: { login: "admin", type: "User" } },
  machine,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  last_used_at: "2026-01-01T00:00:00Z",
  state: "Available",
  url: "/api/v3/user/codespaces/crimson-spoon-abc123",
  html_url: "/ui/codespaces/crimson-spoon-abc123",
  web_url: "http://x",
  billing_url: "http://x/billing",
  git_status: { ahead: 0, behind: 0, has_uncommitted_changes: false, ref: "main" },
  devcontainer_path: ".devcontainer/devcontainer.json",
  image: "mcr.microsoft.com/devcontainers/base",
  retention_period_minutes: 10080,
};

const baseRepo = {
  id: 10,
  name: "test",
  full_name: "admin/test",
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

function mockUserEndpoints() {
  mockFetch.mockImplementation((url: RequestInfo | URL) => {
    const u = url.toString();
    if (u === "/api/v3/user/codespaces") {
      return Promise.resolve(jsonResponse({ total_count: 1, codespaces: [codespace] }));
    }
    if (u === "/api/v3/user/repos?per_page=100") return Promise.resolve(jsonResponse([baseRepo]));
    return Promise.resolve(jsonResponse({}));
  });
}

function mockRepoEndpoints() {
  mockFetch.mockImplementation((url: RequestInfo | URL) => {
    const u = url.toString();
    if (u === "/api/v3/repos/admin/test/codespaces") {
      return Promise.resolve(jsonResponse({ total_count: 1, codespaces: [codespace] }));
    }
    if (u === "/api/v3/repos/admin/test/codespaces/machines") {
      return Promise.resolve(jsonResponse({ total_count: 1, machines: [machine] }));
    }
    return Promise.resolve(jsonResponse({}));
  });
}

describe("CodespacesPage", () => {
  it("renders user codespaces", async () => {
    mockUserEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("my codespace")).toBeInTheDocument();
    });
  });

  it("loads the create dialog repository selector from GitHub REST user repositories", async () => {
    mockUserEndpoints();
    renderPage();
    fireEvent.click(screen.getByText("New codespace"));
    await waitFor(() => {
      expect(screen.getByRole("option", { name: "admin/test" })).toBeInTheDocument();
    });
    const calls = mockFetch.mock.calls.map((c) => c[0].toString());
    expect(calls).toContain("/api/v3/user/repos?per_page=100");
    expect(calls).not.toContain("/internal/repos");
  });

  it("renders repo-scoped codespaces", async () => {
    mockRepoEndpoints();
    renderPage("/ui/repos/admin/test/codespaces");
    await waitFor(() => {
      expect(screen.getByText(/Codespaces for admin\/test/)).toBeInTheDocument();
      expect(screen.getByText("my codespace")).toBeInTheDocument();
    });
  });
});
