import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { PullsPage } from "../pages/PullsPage.js";

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
          <Route path="/ui/repos/:owner/:repo/pulls" element={<PullsPage />} />
          <Route path="/ui/repos/:owner/:repo/pulls/:number" element={<PullsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function pr(number: number, title: string, overrides: Record<string, unknown> = {}) {
  return {
    id: number,
    number,
    title,
    body: "body",
    state: "open",
    draft: false,
    user: { login: "admin", avatar_url: "" },
    head: { ref: "feature", sha: "abc" },
    base: { ref: "main", sha: "def" },
    labels: [],
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    merged_at: null,
    merged: false,
    ...overrides,
  };
}

const noChecks = { total_count: 0, check_runs: [] };

function checkRun(id: number, name: string, overrides: Record<string, unknown> = {}) {
  return {
    id,
    name,
    status: "completed",
    conclusion: "success",
    started_at: "2026-01-01T00:00:00Z",
    completed_at: "2026-01-01T00:00:42Z",
    details_url: "",
    app: { id: 1 },
    ...overrides,
  };
}

describe("PullsPage detail", () => {
  it("loads the PR via the single-PR endpoint, not by scanning the list", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/check-runs")) return Promise.resolve(jsonResponse(noChecks));
      if (u.includes("/pulls/77/") || u.includes("/issues/77/comments")) {
        return Promise.resolve(jsonResponse([]));
      }
      if (u.endsWith("/pulls/77")) return Promise.resolve(jsonResponse(pr(77, "Single fetch PR")));
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/pulls/77");
    await waitFor(() => {
      expect(screen.getByText("Single fetch PR")).toBeInTheDocument();
    });
    const calls = mockFetch.mock.calls.map((c) => c[0].toString());
    expect(calls).toContain("/api/v3/repos/admin/test/pulls/77");
  });

  it("shows a not-found state (not a spinner) for a missing PR", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.endsWith("/pulls/999")) {
        return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/pulls/999");
    await waitFor(() => {
      expect(screen.getByText(/pull request #999 not found/i)).toBeInTheDocument();
    });
    expect(screen.queryByText(/loading pr/i)).not.toBeInTheDocument();
  });

  it("shows an error state when the PR fetch fails", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.endsWith("/pulls/5")) {
        return Promise.resolve(jsonResponse({ message: "boom" }, 500));
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/pulls/5");
    await waitFor(() => {
      expect(screen.getByText(/failed to load pr #5/i)).toBeInTheDocument();
    });
  });
});

describe("PullsPage list pagination", () => {
  it("pages through via the Link header's rel=next URL", async () => {
    const page2Url = "/api/v3/repos/admin/test/pulls?state=open&per_page=50&page=2";
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/issues")) return Promise.resolve(jsonResponse([]));
      if (u.includes("page=2")) return Promise.resolve(jsonResponse([pr(3, "third pr")]));
      if (u.includes("/pulls?")) {
        return Promise.resolve(
          jsonResponse([pr(1, "first pr"), pr(2, "second pr")], 200, {
            Link: `<${page2Url}>; rel="next"`,
          }),
        );
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/pulls");
    await waitFor(() => {
      expect(screen.getByText("first pr")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /load more/i }));
    await waitFor(() => {
      expect(screen.getByText("third pr")).toBeInTheDocument();
    });
    const calls = mockFetch.mock.calls.map((c) => c[0].toString());
    expect(calls).toContain(page2Url);
  });
});

describe("PullsPage checks section", () => {
  function mockDetail(prData: unknown, checks: unknown) {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/commits/abc/check-runs")) return Promise.resolve(jsonResponse(checks));
      if (u.includes("/issues/9/comments")) return Promise.resolve(jsonResponse([]));
      if (u.endsWith("/pulls/9")) return Promise.resolve(jsonResponse(prData));
      return Promise.resolve(jsonResponse([]));
    });
  }

  it("shows the green all-passed summary with per-check rows", async () => {
    mockDetail(pr(9, "Checked PR"), {
      total_count: 2,
      check_runs: [checkRun(1, "build"), checkRun(2, "lint")],
    });
    renderAt("/ui/repos/admin/test/pulls/9");
    expect(await screen.findByText(/all checks have passed/i)).toBeInTheDocument();
    expect(screen.getByText("build")).toBeInTheDocument();
    expect(screen.getByText("lint")).toBeInTheDocument();
    // 42s duration from started/completed timestamps.
    expect(screen.getAllByText("42s").length).toBe(2);
  });

  it("shows the pending summary while a check is in progress", async () => {
    mockDetail(pr(9, "Checked PR"), {
      total_count: 2,
      check_runs: [
        checkRun(1, "build"),
        checkRun(2, "e2e", { status: "in_progress", conclusion: null, completed_at: null }),
      ],
    });
    renderAt("/ui/repos/admin/test/pulls/9");
    expect(await screen.findByText(/some checks haven't completed yet/i)).toBeInTheDocument();
  });

  it("shows the failure summary when a check concluded unsuccessfully", async () => {
    mockDetail(pr(9, "Checked PR"), {
      total_count: 2,
      check_runs: [checkRun(1, "build"), checkRun(2, "test", { conclusion: "failure" })],
    });
    renderAt("/ui/repos/admin/test/pulls/9");
    expect(await screen.findByText(/some checks were not successful/i)).toBeInTheDocument();
  });

  it("hides the checks box when the commit has no check runs", async () => {
    mockDetail(pr(9, "Checked PR"), noChecks);
    renderAt("/ui/repos/admin/test/pulls/9");
    await screen.findByText("Checked PR");
    expect(screen.queryByText(/all checks have passed/i)).not.toBeInTheDocument();
  });

  it("links a check to the run detail page when details_url points at a run", async () => {
    mockDetail(pr(9, "Checked PR"), {
      total_count: 1,
      check_runs: [
        checkRun(1, "build", {
          details_url: "http://bleephub.localhost/admin/test/actions/runs/42",
        }),
      ],
    });
    renderAt("/ui/repos/admin/test/pulls/9");
    const link = await screen.findByRole("link", { name: /build/i });
    expect(link).toHaveAttribute("href", "/ui/repos/admin/test/actions/runs/42");
  });

  it("disables merging and explains when mergeable_state is blocked", async () => {
    mockDetail(pr(9, "Blocked PR", { mergeable_state: "blocked" }), {
      total_count: 1,
      check_runs: [checkRun(1, "required-check", { status: "queued", conclusion: null })],
    });
    renderAt("/ui/repos/admin/test/pulls/9");
    expect(await screen.findByText(/merging is blocked — required checks must pass/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /merge pull request/i })).toBeDisabled();
  });
});
