import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { RepoDetailPage } from "../pages/RepoDetailPage.js";

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
      <MemoryRouter initialEntries={["/ui/repos/admin/test"]}>
        <Routes>
          <Route path="/ui/repos/:owner/:repo" element={<RepoDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const repoData = {
  id: 1,
  name: "test",
  full_name: "admin/test",
  description: "a repo",
  homepage: "https://example.com",
  default_branch: "main",
  visibility: "public",
  private: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-02T00:00:00Z",
  stargazers_count: 5,
  subscribers_count: 2,
  forks_count: 1,
};

const topicsData = { names: ["cli", "tooling"] };

const releasesData = [
  {
    id: 1,
    tag_name: "v1.0.0",
    name: "First release",
    body: "",
    draft: false,
    prerelease: false,
    created_at: "2026-02-01T00:00:00Z",
    published_at: "2026-02-01T00:00:00Z",
    html_url: "http://x/admin/test/releases/tag/v1.0.0",
  },
  {
    id: 2,
    tag_name: "v1.1.0",
    name: "Draft release",
    body: "",
    draft: true,
    prerelease: false,
    created_at: "2026-03-01T00:00:00Z",
    published_at: null,
    html_url: "http://x/admin/test/releases/tag/v1.1.0",
  },
];

const branchesData = [{ name: "main", commit: { sha: "abc" } }];
const commitsData = [
  {
    sha: "abc123",
    commit: {
      message: "Initial commit",
      author: { name: "Admin", email: "a@b", date: "2026-01-01T00:00:00Z" },
    },
  },
];
const contentsData = [
  { name: "README.md", path: "README.md", sha: "r1", type: "file", size: 14 },
  { name: "src", path: "src", sha: "d1", type: "dir" },
];
const readmeData = {
  name: "README.md",
  path: "README.md",
  sha: "r1",
  type: "file",
  encoding: "base64",
  content: "IyB0ZXN0CgpuZXh0cmEgZGV0YWls",
};

function routedFetch(url: RequestInfo | URL): Promise<Response> {
  const u = url.toString();
  if (u.includes("/releases")) return Promise.resolve(jsonResponse(releasesData));
  if (u.endsWith("/topics")) return Promise.resolve(jsonResponse(topicsData));
  if (u.endsWith("/packages")) return Promise.resolve(jsonResponse([]));
  if (u.endsWith("/repos/admin/test")) return Promise.resolve(jsonResponse(repoData));
  if (u.endsWith("/branches")) return Promise.resolve(jsonResponse(branchesData));
  if (u.endsWith("/commits")) return Promise.resolve(jsonResponse(commitsData));
  if (u.includes("/readme")) return Promise.resolve(jsonResponse(readmeData));
  if (u.includes("/contents/")) return Promise.resolve(jsonResponse(contentsData));
  return Promise.resolve(jsonResponse([]));
}

describe("RepoDetailPage releases", () => {
  it("renders a draft release as 'draft', not a 1970 date", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await screen.findByText("a repo");
    fireEvent.click(screen.getByRole("button", { name: "Releases" }));
    await waitFor(() => {
      expect(screen.getByText("Draft release")).toBeInTheDocument();
    });
    expect(screen.getByText("draft")).toBeInTheDocument();
    // the published release still shows its real date
    expect(
      screen.getByText(`published ${new Date("2026-02-01T00:00:00Z").toLocaleDateString()}`),
    ).toBeInTheDocument();
    // no zero-time rendering anywhere
    expect(screen.queryByText(/1970/)).not.toBeInTheDocument();
  });
});

describe("RepoDetailPage code", () => {
  it("renders the file tree and README for a non-empty repo", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await screen.findByText("a repo");

    await waitFor(() => {
      // README.md appears both as a file row and the README panel header.
      expect(screen.getAllByText("README.md").length).toBeGreaterThan(0);
      expect(screen.getByText("src")).toBeInTheDocument();
    });
    // "test" appears in the repo breadcrumb and the rendered README <h1>.
    expect(screen.getAllByText("test").length).toBeGreaterThan(0);
  });

  it("shows only supported empty-repository transport setup", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.endsWith("/repos/admin/test")) return Promise.resolve(jsonResponse(repoData));
      if (u.endsWith("/branches")) return Promise.resolve(jsonResponse([]));
      if (u.endsWith("/commits")) return Promise.resolve(jsonResponse([]));
      return Promise.resolve(jsonResponse([]));
    });
    renderPage();
    await screen.findByText("a repo");

    await waitFor(() => {
      expect(screen.getByText("This repository is empty")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "HTTPS" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "SSH" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "GitHub CLI" })).toBeInTheDocument();
  });

  it("renders the latest-commit banner above the file table", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/Initial commit/)).toBeInTheDocument();
    });
    // short sha + commit count
    expect(screen.getByText("abc123".slice(0, 7))).toBeInTheDocument();
    expect(screen.getByText(/1 commit\b/)).toBeInTheDocument();
  });

  it("exposes a Code clone dropdown with the HTTPS clone URL and a copy button", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await screen.findAllByText("README.md");

    // The repo sub-tab strip also has a "Code" button; the clone dropdown is
    // the one carrying aria-expanded (matched via the `expanded` filter).
    const codeButton = screen.getByRole("button", { name: "Code", expanded: false });
    fireEvent.click(codeButton);

    const field = screen.getByLabelText("HTTPS clone URL") as HTMLInputElement;
    expect(field.value).toMatch(/\/admin\/test\.git$/);
    expect(screen.getByRole("button", { name: "Copy clone URL" })).toBeInTheDocument();
  });

  it("drives Watch, Fork, and Star through the public GitHub repository APIs", async () => {
    let starred = false;
    let subscribed = false;
    const calls: Array<{ method: string; url: string }> = [];
    mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = url.toString();
      const method = init?.method ?? "GET";
      calls.push({ method, url: u });
      if (u === "/api/v3/user") {
        return Promise.resolve(jsonResponse({ id: 9, login: "octocat", type: "User", site_admin: false }));
      }
      if (u.startsWith("/api/v3/user/orgs")) {
        return Promise.resolve(jsonResponse([{ id: 10, login: "acme" }]));
      }
      if (u.endsWith("/ui-data/repos/admin/test/viewer")) {
        return Promise.resolve(jsonResponse({ starred, subscribed }));
      }
      if (u.endsWith("/user/starred/admin/test")) {
        if (method === "PUT") {
          starred = true;
          return Promise.resolve(new Response(null, { status: 204 }));
        }
        return Promise.resolve(new Response(null, { status: starred ? 204 : 404 }));
      }
      if (u.endsWith("/repos/admin/test/subscription")) {
        if (method === "PUT") subscribed = true;
        return Promise.resolve(jsonResponse({
          subscribed,
          ignored: false,
          reason: null,
          created_at: "2026-01-01T00:00:00Z",
          url: u,
          repository_url: "/api/v3/repos/admin/test",
        }));
      }
      if (u.endsWith("/repos/admin/test/forks") && method === "POST") {
        return Promise.resolve(jsonResponse({ ...repoData, id: 2, full_name: "octocat/test", owner: { login: "octocat" } }, 202));
      }
      return routedFetch(url);
    });
    renderPage();

    const actions = await screen.findByLabelText("Repository actions");
    fireEvent.click(within(actions).getByRole("button", { name: /Watch/ }));
    await waitFor(() => expect(calls).toContainEqual({ method: "PUT", url: "/api/v3/repos/admin/test/subscription" }));

    fireEvent.click(within(actions).getByRole("button", { name: /Star/ }));
    await waitFor(() => expect(calls).toContainEqual({ method: "PUT", url: "/api/v3/user/starred/admin/test" }));
    await waitFor(() => expect(within(actions).getByRole("button", { name: /Unstar/ })).toBeInTheDocument());

    fireEvent.click(within(actions).getByRole("button", { name: /Fork/ }));
    const forkDialog = await screen.findByRole("dialog", { name: "Create a new fork" });
    await within(forkDialog).findByRole("option", { name: "acme" });
    fireEvent.change(within(forkDialog).getByLabelText("Owner"), { target: { value: "acme" } });
    fireEvent.click(within(forkDialog).getByRole("button", { name: "Create fork" }));
    await waitFor(() => expect(calls).toContainEqual({ method: "POST", url: "/api/v3/repos/admin/test/forks" }));
    expect(calls.some(({ url }) => url.startsWith("/internal/"))).toBe(false);
  });

  it("groups administrative resources under the repository More menu", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await screen.findAllByText("README.md");

    expect(screen.getByRole("navigation", { name: "Repository content" })).toBeInTheDocument();
    expect(screen.getByText("Repository administration")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "All repository settings" })).toHaveAttribute(
      "href",
      "/ui/repos/admin/test/settings",
    );
  });
});

describe("RepoDetailPage About sidebar", () => {
  it("renders description, website, topics, releases, packages and social counts", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await screen.findAllByText("README.md");

    // description + website live in the sidebar
    const about = screen.getByRole("complementary", { name: "About" });
    expect(within(about).getByText("a repo")).toBeInTheDocument();
    expect(within(about).getByText("example.com")).toBeInTheDocument();
    // topics as pill chips
    expect(within(about).getByText("cli")).toBeInTheDocument();
    expect(within(about).getByText("tooling")).toBeInTheDocument();
    // latest release + Latest badge
    expect(within(about).getByText("Latest")).toBeInTheDocument();
    expect(within(about).getByText("No packages published")).toBeInTheDocument();
    // social counts moved into the sidebar
    expect(within(about).getByText(/5 stars/)).toBeInTheDocument();
    expect(within(about).getByText(/2 watchers/)).toBeInTheDocument();
    expect(within(about).getByText(/1 fork/)).toBeInTheDocument();
  });

  it("styles README headings via the markdown-body class", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    const { container } = renderPage();
    await waitFor(() => {
      // README markdown "# test" renders as a real <h1> inside .markdown-body
      const heading = container.querySelector(".markdown-body h1");
      expect(heading).not.toBeNull();
      expect(heading?.textContent).toBe("test");
    });
  });
});
