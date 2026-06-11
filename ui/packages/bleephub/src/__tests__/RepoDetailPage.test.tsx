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

function routedFetch(url: RequestInfo | URL): Promise<Response> {
  const u = url.toString();
  if (u.includes("/releases")) return Promise.resolve(jsonResponse(releasesData));
  if (u.endsWith("/repos/admin/test")) return Promise.resolve(jsonResponse(repoData));
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
