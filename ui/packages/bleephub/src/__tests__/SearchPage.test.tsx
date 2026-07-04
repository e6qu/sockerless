import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { SearchPage } from "../pages/SearchPage.js";

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

function renderPage(initialEntry = "/ui/search") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route path="/ui/search" element={<SearchPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const repoItem = {
  id: 1,
  full_name: "admin/hit-repo",
  description: "matching repo",
  visibility: "public",
};

describe("SearchPage", () => {
  it("prompts for a query before searching", () => {
    renderPage();
    expect(screen.getByText("Search bleephub")).toBeInTheDocument();
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("searches repositories with the q parameter and shows the honest count", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({ total_count: 1, incomplete_results: false, items: [repoItem] }),
    );
    renderPage("/ui/search?q=hit&type=repositories");
    await waitFor(() => {
      expect(screen.getByText("admin/hit-repo")).toBeInTheDocument();
    });
    expect(screen.getByText(/1 repository/)).toBeInTheDocument();
    const url = String(mockFetch.mock.calls[0][0]);
    expect(url).toContain("/api/v3/search/repositories?");
    expect(url).toContain("q=hit");
    expect(url).toContain("per_page=30");
  });

  it("switches tabs and hits the matching search endpoint", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({ total_count: 0, incomplete_results: false, items: [] }),
    );
    renderPage("/ui/search?q=fix&type=issues");
    await waitFor(() => {
      expect(screen.getByText("No matching issues and pull requests")).toBeInTheDocument();
    });
    expect(String(mockFetch.mock.calls[0][0])).toContain("/api/v3/search/issues?");

    fireEvent.click(screen.getByRole("button", { name: "Commits" }));
    await waitFor(() => {
      const urls = mockFetch.mock.calls.map((c) => String(c[0]));
      expect(urls.some((u) => u.includes("/api/v3/search/commits?"))).toBe(true);
    });
  });

  it("marks pull requests distinctly in issue results", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({
        total_count: 2,
        incomplete_results: false,
        items: [
          {
            id: 10,
            number: 4,
            title: "A plain issue",
            state: "open",
            user: { login: "admin" },
            comments: 0,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-01-01T00:00:00Z",
            pull_request: null,
            repository: { full_name: "admin/a" },
          },
          {
            id: 11,
            number: 5,
            title: "A pull request",
            state: "open",
            user: { login: "admin" },
            comments: 2,
            created_at: "2026-01-02T00:00:00Z",
            updated_at: "2026-01-02T00:00:00Z",
            pull_request: { url: "http://x/pulls/5" },
            repository: { full_name: "admin/a" },
          },
        ],
      }),
    );
    renderPage("/ui/search?q=a&type=issues");
    await waitFor(() => {
      expect(screen.getByText("A plain issue")).toBeInTheDocument();
    });
    expect(screen.getByText(/issue · admin\/a#4/)).toBeInTheDocument();
    expect(screen.getByText(/pull request · admin\/a#5/)).toBeInTheDocument();
  });

  it("requires a repository for label search and resolves its id", async () => {
    mockFetch.mockImplementation((input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/api/v3/repos/admin/labelled") {
        return Promise.resolve(jsonResponse({ id: 77, full_name: "admin/labelled" }));
      }
      if (url.startsWith("/api/v3/search/labels?")) {
        return Promise.resolve(
          jsonResponse({
            total_count: 1,
            incomplete_results: false,
            items: [{ id: 1, name: "bug", color: "ff0000", default: true, description: "Bugs" }],
          }),
        );
      }
      return Promise.resolve(jsonResponse({ message: "unexpected" }, 500));
    });

    renderPage("/ui/search?q=bug&type=labels");
    expect(screen.getByText("Pick a repository")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Repository for label search"), {
      target: { value: "admin/labelled" },
    });
    fireEvent.click(screen.getByRole("button", { name: /search/i }));

    await waitFor(() => {
      expect(screen.getByText("bug")).toBeInTheDocument();
    });
    const labelCall = mockFetch.mock.calls
      .map((c) => String(c[0]))
      .find((u) => u.startsWith("/api/v3/search/labels?"));
    expect(labelCall).toContain("repository_id=77");
  });

  it("surfaces search failures", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ message: "boom" }, 500));
    renderPage("/ui/search?q=zzz&type=users");
    await waitFor(() => {
      expect(screen.getByText("Search failed")).toBeInTheDocument();
    });
  });
});
