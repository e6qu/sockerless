import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { IssuesPage } from "../pages/IssuesPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

function renderAt(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/ui/repos/:owner/:repo/issues" element={<IssuesPage />} />
          <Route path="/ui/repos/:owner/:repo/issues/:number" element={<IssuesPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function issue(number: number, title: string) {
  return {
    id: number,
    number,
    title,
    body: "body",
    state: "open",
    user: { login: "admin", avatar_url: "" },
    labels: [],
    assignees: [],
    comments: 0,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    closed_at: null,
  };
}

describe("IssuesPage detail", () => {
  it("shows a not-found state (not a spinner) for a missing issue", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/issues/999")) {
        return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/issues/999");
    await waitFor(() => {
      expect(screen.getByText(/issue #999 not found/i)).toBeInTheDocument();
    });
    expect(screen.queryByText(/loading issue/i)).not.toBeInTheDocument();
  });

  it("shows an error state when the issue fetch fails", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/issues/7")) {
        return Promise.resolve(jsonResponse({ message: "boom" }, 500));
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/issues/7");
    await waitFor(() => {
      expect(screen.getByText(/failed to load issue #7/i)).toBeInTheDocument();
    });
  });

  it("renders the issue when found", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/issues/7/comments")) return Promise.resolve(jsonResponse([]));
      if (u.includes("/issues/7")) return Promise.resolve(jsonResponse(issue(7, "A real issue")));
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/issues/7");
    await waitFor(() => {
      expect(screen.getByText("A real issue")).toBeInTheDocument();
    });
  });
});

describe("IssuesPage list pagination", () => {
  it("shows Load more when the server advertises a next page, and appends it", async () => {
    const page2Url = "/api/v3/repos/admin/test/issues?state=open&per_page=50&page=2";
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/pulls")) return Promise.resolve(jsonResponse([]));
      if (u.includes("page=2")) {
        return Promise.resolve(jsonResponse([issue(3, "third issue")]));
      }
      if (u.includes("/issues?")) {
        return Promise.resolve(
          jsonResponse([issue(1, "first issue"), issue(2, "second issue")], 200, {
            Link: `<${page2Url}>; rel="next"`,
          }),
        );
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/issues");

    await waitFor(() => {
      expect(screen.getByText("first issue")).toBeInTheDocument();
    });
    const loadMore = screen.getByRole("button", { name: /load more/i });
    fireEvent.click(loadMore);
    await waitFor(() => {
      expect(screen.getByText("third issue")).toBeInTheDocument();
    });
    // page 2 was fetched via the Link rel="next" URL the server advertised
    const calls = mockFetch.mock.calls.map((c) => c[0].toString());
    expect(calls).toContain(page2Url);
  });

  it("renders an honest N+ badge when the open count is truncated by paging", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/pulls")) return Promise.resolve(jsonResponse([]));
      if (u.includes("/issues?")) {
        return Promise.resolve(
          jsonResponse([issue(1, "first issue"), issue(2, "second issue")], 200, {
            Link: `</api/v3/repos/admin/test/issues?state=open&per_page=50&page=2>; rel="next"`,
          }),
        );
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/issues");
    await waitFor(() => {
      expect(screen.getByText("2+")).toBeInTheDocument();
    });
  });
});
