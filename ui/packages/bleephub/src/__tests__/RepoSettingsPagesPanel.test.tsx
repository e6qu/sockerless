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
      <MemoryRouter initialEntries={["/ui/repos/admin/pages-repo/settings"]}>
        <Routes>
          <Route path="/ui/repos/:owner/:repo/settings" element={<RepoSettingsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const repo = {
  id: 1,
  name: "pages-repo",
  full_name: "admin/pages-repo",
  description: "",
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
};

const pagesSite = {
  cname: "www.example.test",
  url: "/api/v3/repos/admin/pages-repo/pages",
  html_url: "https://admin.github.io/pages-repo",
  status: "built",
  source: { branch: "main", path: "/" },
  public: true,
  custom_404: false,
  protected_domain_state: null,
  build_type: "legacy",
  https_enforced: true,
};

const pagesBuild = {
  url: "/api/v3/repos/admin/pages-repo/pages/builds/9",
  status: "built",
  pusher: { login: "admin", id: 1, type: "User" },
  commit: "abcdef1234567890",
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:01:00Z",
  duration: 60,
  error: { message: null },
};

const pagesHealth = {
  domain: {
    host: "www.example.test",
    uri: "http://www.example.test/",
    nameservers: "default",
    dns_resolves: true,
    is_valid_domain: true,
    is_apex_domain: false,
    is_pages_domain: false,
    is_valid: true,
    reason: null,
    enforces_https: true,
  },
  alt_domain: null,
};

function mockPagesRoutes(overrides: Record<string, () => Response> = {}) {
  mockFetch.mockImplementation((url: string, init?: RequestInit) => {
    const key = `${init?.method ?? "GET"} ${url}`;
    if (overrides[key]) return Promise.resolve(overrides[key]());
    if (key === "GET /api/v3/repos/admin/pages-repo") return Promise.resolve(jsonResponse(repo));
    if (key === "GET /api/v3/repos/admin/pages-repo/topics")
      return Promise.resolve(jsonResponse({ names: [] }));
    if (key === "GET /api/v3/repos/admin/pages-repo/pages")
      return Promise.resolve(jsonResponse(pagesSite));
    if (key === "GET /api/v3/repos/admin/pages-repo/pages/builds")
      return Promise.resolve(jsonResponse([pagesBuild]));
    if (key === "GET /api/v3/repos/admin/pages-repo/pages/health")
      return Promise.resolve(jsonResponse(pagesHealth));
    return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
  });
}

async function openPagesTab() {
  renderPage();
  fireEvent.click(await screen.findByRole("button", { name: "Pages" }));
}

describe("RepoSettingsPage Pages panel", () => {
  it("shows site status, builds, and domain health", async () => {
    mockPagesRoutes();
    await openPagesTab();

    // "built" appears both as the site status and the build row's pill.
    await waitFor(() => expect(screen.getAllByText("built").length).toBeGreaterThan(0));
    expect(screen.getByText("https://admin.github.io/pages-repo")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText(/abcdef1/)).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText("healthy")).toBeInTheDocument());
    expect(screen.getByText(/DNS resolves: yes/)).toBeInTheDocument();
  });

  it("requests a build via POST /pages/builds", async () => {
    mockPagesRoutes({
      "POST /api/v3/repos/admin/pages-repo/pages/builds": () =>
        jsonResponse({ status: "queued", url: "/api/v3/repos/admin/pages-repo/pages/builds/10" }, 201),
    });
    await openPagesTab();

    await waitFor(() => screen.getByRole("button", { name: /request build/i }));
    fireEvent.click(screen.getByRole("button", { name: /request build/i }));

    await waitFor(() => {
      const postCall = mockFetch.mock.calls.find(
        (call) =>
          call[0] === "/api/v3/repos/admin/pages-repo/pages/builds" && call[1]?.method === "POST",
      );
      expect(postCall).toBeDefined();
    });
  });

  it("saves cname + https_enforced via PUT /pages", async () => {
    mockPagesRoutes({
      "PUT /api/v3/repos/admin/pages-repo/pages": () => new Response(null, { status: 204 }),
    });
    await openPagesTab();

    const cnameInput = await screen.findByLabelText(/custom domain/i);
    fireEvent.change(cnameInput, { target: { value: "pages.example.test" } });
    fireEvent.click(screen.getByRole("button", { name: /save pages settings/i }));

    await waitFor(() => {
      const putCall = mockFetch.mock.calls.find(
        (call) => call[0] === "/api/v3/repos/admin/pages-repo/pages" && call[1]?.method === "PUT",
      );
      expect(putCall).toBeDefined();
      expect(JSON.parse(putCall![1].body as string)).toEqual({
        cname: "pages.example.test",
        https_enforced: true,
      });
    });
    expect(await screen.findByText(/pages settings saved/i)).toBeInTheDocument();
  });

  it("offers the enable form when Pages is not configured and POSTs /pages", async () => {
    mockPagesRoutes({
      "GET /api/v3/repos/admin/pages-repo/pages": () =>
        jsonResponse({ message: "Not Found" }, 404),
      "POST /api/v3/repos/admin/pages-repo/pages": () => jsonResponse(pagesSite, 201),
    });
    await openPagesTab();

    await waitFor(() =>
      expect(screen.getByText(/pages is not enabled/i)).toBeInTheDocument(),
    );
    fireEvent.change(screen.getByLabelText(/source branch/i), { target: { value: "main" } });
    fireEvent.click(screen.getByRole("button", { name: /enable pages/i }));

    await waitFor(() => {
      const postCall = mockFetch.mock.calls.find(
        (call) => call[0] === "/api/v3/repos/admin/pages-repo/pages" && call[1]?.method === "POST",
      );
      expect(postCall).toBeDefined();
      expect(JSON.parse(postCall![1].body as string)).toEqual({
        build_type: "legacy",
        source: { branch: "main", path: "/" },
      });
    });
  });

  it("disables Pages via DELETE /pages after confirmation", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    mockPagesRoutes({
      "DELETE /api/v3/repos/admin/pages-repo/pages": () => new Response(null, { status: 204 }),
    });
    await openPagesTab();

    fireEvent.click(await screen.findByRole("button", { name: /disable pages/i }));

    await waitFor(() => {
      const deleteCall = mockFetch.mock.calls.find(
        (call) =>
          call[0] === "/api/v3/repos/admin/pages-repo/pages" && call[1]?.method === "DELETE",
      );
      expect(deleteCall).toBeDefined();
    });
  });

  it("looks up a Pages deployment status by id", async () => {
    mockPagesRoutes({
      "GET /api/v3/repos/admin/pages-repo/pages/deployments/12": () =>
        jsonResponse({ status: "succeed" }),
    });
    await openPagesTab();

    const input = await screen.findByLabelText(/deployment id/i);
    fireEvent.change(input, { target: { value: "12" } });
    fireEvent.click(screen.getByRole("button", { name: /check status/i }));

    await waitFor(() => expect(screen.getByText("succeed")).toBeInTheDocument());
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/v3/repos/admin/pages-repo/pages/deployments/12",
      expect.anything(),
    );
  });
});
