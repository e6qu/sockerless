import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { RepoSettingsPage } from "../pages/RepoSettingsPage.js";

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
      <MemoryRouter initialEntries={["/ui/repos/admin/settings-repo/settings"]}>
        <Routes>
          <Route path="/ui/repos/:owner/:repo/settings" element={<RepoSettingsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const repo = {
  id: 1,
  name: "settings-repo",
  full_name: "admin/settings-repo",
  description: "before",
  homepage: "https://before.test",
  default_branch: "main",
  visibility: "public",
  private: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-02T00:00:00Z",
  pushed_at: "2026-01-02T00:00:00Z",
  size: 0,
  owner: { login: "admin", type: "User" },
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

describe("RepoSettingsPage", () => {
  it("loads repo details and renders the settings form", async () => {
    mockFetch.mockResolvedValue(jsonResponse(repo));
    renderPage();
    await waitFor(() => {
      expect(screen.getByDisplayValue("before")).toBeInTheDocument();
    });
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/v3/repos/admin/settings-repo",
      expect.anything(),
    );
  });

  it("renders a left settings sub-nav with grouped sections", async () => {
    mockFetch.mockResolvedValue(jsonResponse(repo));
    renderPage();
    await waitFor(() => screen.getByDisplayValue("before"));

    const nav = screen.getByRole("navigation", { name: "Settings" });
    expect(nav).toBeInTheDocument();
    expect(screen.getByText("Access")).toBeInTheDocument();
    expect(screen.getByText("Code and automation")).toBeInTheDocument();
    expect(screen.getByText("Danger zone")).toBeInTheDocument();
    // General is the default-active item.
    expect(screen.getByRole("button", { name: "General" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "Collaborators" })).not.toHaveAttribute("aria-current");
  });

  it("submits PATCH /api/v3/repos/{owner}/{repo} on save", async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse(repo)) // fetchRepoDetail
      .mockResolvedValueOnce(jsonResponse([])) // issues count
      .mockResolvedValueOnce(jsonResponse([])) // prs count
      .mockResolvedValueOnce(jsonResponse({ ...repo, description: "after" })); // PATCH
    renderPage();
    await waitFor(() => screen.getByDisplayValue("before"));

    fireEvent.change(screen.getByDisplayValue("before"), { target: { value: "after" } });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));

    await waitFor(() => {
      const patchCall = mockFetch.mock.calls.find((call) => call[1]?.method === "PATCH");
      expect(patchCall).toBeDefined();
      expect(patchCall![0]).toBe("/api/v3/repos/admin/settings-repo");
      expect(patchCall![1].body).toContain("after");
    });
  });
});
