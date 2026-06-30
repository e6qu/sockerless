import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
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
  default_branch: "main",
  visibility: "public",
  private: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-02T00:00:00Z",
};

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
  if (u.endsWith("/repos/admin/test")) return Promise.resolve(jsonResponse(repoData));
  if (u.endsWith("/branches")) return Promise.resolve(jsonResponse(branchesData));
  if (u.endsWith("/commits")) return Promise.resolve(jsonResponse(commitsData));
  if (u.endsWith("/readme")) return Promise.resolve(jsonResponse(readmeData));
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
      expect(screen.getByText("README.md")).toBeInTheDocument();
      expect(screen.getByText("src")).toBeInTheDocument();
    });
    expect(screen.getByText("test")).toBeInTheDocument();
  });

  it("shows GitHub-standard empty-repo setup tabs for an empty repo", async () => {
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
    expect(screen.getByRole("button", { name: "SSH" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "GitHub CLI" })).toBeInTheDocument();
  });
});
