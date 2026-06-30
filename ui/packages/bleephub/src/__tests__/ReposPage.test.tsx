import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { ReposPage } from "../pages/ReposPage.js";

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

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <ReposPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const reposData = [
  {
    id: 1,
    name: "test",
    full_name: "admin/test",
    description: "a repo",
    homepage: null,
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
  },
];

describe("ReposPage", () => {
  it("renders the repo list from /api/v3/user/repos", async () => {
    mockFetch.mockResolvedValue(jsonResponse(reposData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("admin/test")).toBeInTheDocument();
    });
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/v3/user/repos?per_page=30",
      expect.anything(),
    );
  });

  it("shows an error state instead of spinning when the list fails", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ message: "boom" }, 500));
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
      expect(screen.getByText(/failed to load repositories/i)).toBeInTheDocument();
    });
    expect(screen.queryByText(/loading repos/i)).not.toBeInTheDocument();
  });

  it("opens the create dialog and submits POST /api/v3/user/repos", async () => {
    mockFetch.mockResolvedValue(jsonResponse(reposData, 200, { Link: '' }));
    renderPage();
    await waitFor(() => screen.getByText("admin/test"));

    fireEvent.click(screen.getByRole("button", { name: /new repository/i }));
    await waitFor(() =>
      expect(screen.getByRole("heading", { name: /create a new repository/i })).toBeInTheDocument(),
    );

    fireEvent.change(screen.getByLabelText(/repository name/i), { target: { value: "new-repo" } });
    fireEvent.change(screen.getByLabelText(/description/i), { target: { value: "My new repo" } });

    mockFetch.mockResolvedValueOnce(
      jsonResponse({
        id: 2,
        name: "new-repo",
        full_name: "admin/new-repo",
        description: "My new repo",
        homepage: null,
        default_branch: "main",
        visibility: "public",
        private: false,
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
        pushed_at: "2026-01-01T00:00:00Z",
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
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: /create repository/i }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenLastCalledWith(
        "/api/v3/user/repos",
        expect.objectContaining({
          method: "POST",
          body: expect.stringContaining("new-repo"),
        }),
      );
    });
  });
});
