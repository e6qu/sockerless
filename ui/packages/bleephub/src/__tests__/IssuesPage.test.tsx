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
          <Route path="/ui/repos/:owner/:repo/labels" element={<IssuesPage view="labels" />} />
          <Route path="/ui/repos/:owner/:repo/milestones" element={<IssuesPage view="milestones" />} />
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
      if (u.includes("/issues/7/reactions")) return Promise.resolve(jsonResponse([]));
      if (u.endsWith("/api/v3/user")) return Promise.resolve(jsonResponse({ login: "admin" }));
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

describe("IssuesPage list filter bar", () => {
  function issueWith(number: number, title: string, overrides: Record<string, unknown>) {
    return { ...issue(number, title), ...overrides };
  }

  it("shows the Open/Closed count header and filters by label via the dropdown", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/pulls")) return Promise.resolve(jsonResponse([]));
      if (u.includes("state=closed")) return Promise.resolve(jsonResponse([]));
      if (u.includes("/issues?")) {
        return Promise.resolve(
          jsonResponse([
            issueWith(1, "bug issue", { labels: [{ name: "bug", color: "d73a4a" }] }),
            issueWith(2, "plain issue", { labels: [] }),
          ]),
        );
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/issues");

    await waitFor(() => expect(screen.getByText("bug issue")).toBeInTheDocument());
    // Count header renders Open and Closed toggles.
    expect(screen.getByText(/Open$/)).toBeInTheDocument();
    expect(screen.getByText(/Closed$/)).toBeInTheDocument();

    // Selecting the Label filter narrows the list client-side.
    fireEvent.change(screen.getByLabelText("Label"), { target: { value: "bug" } });
    await waitFor(() => {
      expect(screen.queryByText("plain issue")).not.toBeInTheDocument();
    });
    expect(screen.getByText("bug issue")).toBeInTheDocument();
  });

  it("switches to closed via the count header, refetching with state=closed", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/pulls")) return Promise.resolve(jsonResponse([]));
      if (u.includes("state=closed")) {
        return Promise.resolve(jsonResponse([issueWith(3, "done issue", { state: "closed" })]));
      }
      if (u.includes("/issues?")) return Promise.resolve(jsonResponse([issue(1, "open issue")]));
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/issues");
    await waitFor(() => expect(screen.getByText("open issue")).toBeInTheDocument());

    fireEvent.click(screen.getByText(/Closed$/));
    await waitFor(() => expect(screen.getByText("done issue")).toBeInTheDocument());
    const calls = mockFetch.mock.calls.map((c) => c[0].toString());
    expect(calls.some((u) => u.includes("/issues?state=closed"))).toBe(true);
  });
});

describe("IssuesPage detail sidebar", () => {
  it("renders the two-column layout with the metadata sidebar", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/issues/7/comments")) return Promise.resolve(jsonResponse([]));
      if (u.includes("/issues/7/reactions")) return Promise.resolve(jsonResponse([]));
      if (u.endsWith("/api/v3/user")) return Promise.resolve(jsonResponse({ login: "admin" }));
      if (u.includes("/issues/7")) {
        return Promise.resolve(
          jsonResponse({ ...issue(7, "Sidebar issue"), assignees: [{ login: "carol" }] }),
        );
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/issues/7");
    await waitFor(() => expect(screen.getByText("Sidebar issue")).toBeInTheDocument());
    // Sidebar sections present (Assignees + Development are sidebar-only labels;
    // Projects/Labels also name repo tabs, so assert the distinctive ones).
    expect(screen.getByText("Assignees")).toBeInTheDocument();
    expect(screen.getByText("Development")).toBeInTheDocument();
    // The assignee login shows in the sidebar.
    expect(screen.getByText("carol")).toBeInTheDocument();
  });
});

const bugLabel = { id: 1, name: "bug", color: "d73a4a", description: "Broken", default: false };

function milestone(number: number, title: string, state = "open") {
  return {
    id: number,
    number,
    title,
    description: "",
    state,
    creator: { login: "admin", avatar_url: "" },
    open_issues: 1,
    closed_issues: 3,
    due_on: null,
    closed_at: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

describe("IssuesPage labels view", () => {
  it("lists repo labels with descriptions", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/labels")) return Promise.resolve(jsonResponse([bugLabel]));
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/labels");
    await waitFor(() => {
      expect(screen.getByText("bug")).toBeInTheDocument();
    });
    expect(screen.getByText("Broken")).toBeInTheDocument();
  });

  it("shows an empty state when the repo has no labels", async () => {
    mockFetch.mockImplementation(() => Promise.resolve(jsonResponse([])));
    renderAt("/ui/repos/admin/test/labels");
    await waitFor(() => {
      expect(screen.getByText(/no labels yet/i)).toBeInTheDocument();
    });
  });

  it("creates a label through the dialog", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = url.toString();
      if (u.includes("/labels") && init?.method === "POST") {
        return Promise.resolve(jsonResponse(bugLabel, 201));
      }
      if (u.includes("/labels")) return Promise.resolve(jsonResponse([]));
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/labels");
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /new label/i })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /new label/i }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "bug" } });
    fireEvent.click(screen.getByRole("button", { name: /create label/i }));
    await waitFor(() => {
      const post = mockFetch.mock.calls.find(
        (c) => c[0].toString().includes("/api/v3/repos/admin/test/labels") && c[1]?.method === "POST",
      );
      expect(post).toBeTruthy();
      expect(JSON.parse(String(post![1]!.body))).toMatchObject({ name: "bug" });
    });
  });
});

describe("IssuesPage milestones view", () => {
  it("lists milestones with progress and supports closing", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = url.toString();
      if (u.includes("/milestones/1") && init?.method === "PATCH") {
        return Promise.resolve(jsonResponse(milestone(1, "v1.0", "closed")));
      }
      if (u.includes("/milestones?")) return Promise.resolve(jsonResponse([milestone(1, "v1.0")]));
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/milestones");
    await waitFor(() => {
      expect(screen.getByText("v1.0")).toBeInTheDocument();
    });
    expect(screen.getByText(/75% complete · 1 open · 3 closed/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "close" }));
    await waitFor(() => {
      const patch = mockFetch.mock.calls.find(
        (c) => c[0].toString().includes("/milestones/1") && c[1]?.method === "PATCH",
      );
      expect(patch).toBeTruthy();
      expect(JSON.parse(String(patch![1]!.body))).toMatchObject({ state: "closed" });
    });
  });
});

describe("IssuesPage detail triage", () => {
  const epicType = {
    id: 5,
    node_id: "IT_kwDO00000005",
    name: "Epic",
    description: "Coordinated work",
    color: "purple",
    is_enabled: true,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };

  function mockDetailEndpoints() {
    mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = url.toString();
      if (u.endsWith("/api/v3/repos/admin/test")) {
        return Promise.resolve(
          jsonResponse({ owner: { login: "admin", type: "Organization" } }),
        );
      }
      if (u.includes("/issues/7") && init?.method === "PATCH") {
        return Promise.resolve(
          jsonResponse({ ...issue(7, "Triaged"), milestone: milestone(2, "v2.0"), issue_type: epicType }),
        );
      }
      if (u.includes("/issues/7/labels") && init?.method === "POST") {
        return Promise.resolve(jsonResponse([bugLabel]));
      }
      if (u.includes("/issues/7/comments")) return Promise.resolve(jsonResponse([]));
      if (u.includes("/issues/7/reactions")) return Promise.resolve(jsonResponse([]));
      if (u.endsWith("/api/v3/user")) return Promise.resolve(jsonResponse({ login: "admin" }));
      if (u.includes("/issues/7")) return Promise.resolve(jsonResponse(issue(7, "Triaged")));
      if (u.includes("/milestones?")) {
        return Promise.resolve(jsonResponse([milestone(2, "v2.0")]));
      }
      if (u.includes("/api/v3/repos/admin/test/labels")) {
        return Promise.resolve(jsonResponse([bugLabel]));
      }
      if (u.includes("/api/v3/orgs/admin/issue-types")) {
        return Promise.resolve(jsonResponse([epicType]));
      }
      return Promise.resolve(jsonResponse([]));
    });
  }

  it("adds a label from the repo label list", async () => {
    mockDetailEndpoints();
    renderAt("/ui/repos/admin/test/issues/7");
    await waitFor(() => {
      expect(screen.getByLabelText("Add label")).toBeInTheDocument();
    });
    fireEvent.change(screen.getByLabelText("Add label"), { target: { value: "bug" } });
    await waitFor(() => {
      const post = mockFetch.mock.calls.find(
        (c) => c[0].toString().includes("/issues/7/labels") && c[1]?.method === "POST",
      );
      expect(post).toBeTruthy();
      expect(JSON.parse(String(post![1]!.body))).toEqual({ labels: ["bug"] });
    });
  });

  it("sets the milestone via PATCH", async () => {
    mockDetailEndpoints();
    renderAt("/ui/repos/admin/test/issues/7");
    await waitFor(() => {
      expect(screen.getByRole("option", { name: "v2.0" })).toBeInTheDocument();
    });
    fireEvent.change(screen.getByLabelText("Set milestone"), { target: { value: "2" } });
    await waitFor(() => {
      const patch = mockFetch.mock.calls.find(
        (c) => c[0].toString().endsWith("/issues/7") && c[1]?.method === "PATCH",
      );
      expect(patch).toBeTruthy();
      expect(JSON.parse(String(patch![1]!.body))).toEqual({ milestone: 2 });
    });
  });

  it("sets the organization issue type via PATCH", async () => {
    mockDetailEndpoints();
    renderAt("/ui/repos/admin/test/issues/7");
    await waitFor(() => {
      expect(screen.getByRole("option", { name: "Epic" })).toBeInTheDocument();
    });
    fireEvent.change(screen.getByLabelText("Set issue type"), { target: { value: "5" } });
    await waitFor(() => {
      const patch = mockFetch.mock.calls.find(
        (c) => c[0].toString().endsWith("/issues/7") && c[1]?.method === "PATCH",
      );
      expect(patch).toBeTruthy();
      expect(JSON.parse(String(patch![1]!.body))).toEqual({ issue_type_id: 5 });
    });
  });

  it("hides issue type controls and skips the org issue-type endpoint for user-owned repositories", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/issues/7/comments")) return Promise.resolve(jsonResponse([]));
      if (u.includes("/issues/7/reactions")) return Promise.resolve(jsonResponse([]));
      if (u.endsWith("/api/v3/user")) return Promise.resolve(jsonResponse({ login: "admin" }));
      if (u.endsWith("/api/v3/repos/admin/test")) {
        return Promise.resolve(jsonResponse({ owner: { login: "admin", type: "User" } }));
      }
      if (u.includes("/issues/7")) return Promise.resolve(jsonResponse(issue(7, "Triaged")));
      if (u.includes("/milestones?")) return Promise.resolve(jsonResponse([]));
      if (u.includes("/api/v3/repos/admin/test/labels")) return Promise.resolve(jsonResponse([]));
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/issues/7");
    await waitFor(() => expect(screen.getByText("Triaged")).toBeInTheDocument());
    expect(screen.queryByLabelText("Set issue type")).not.toBeInTheDocument();
    expect(
      mockFetch.mock.calls.some((c) => c[0].toString().includes("/api/v3/orgs/admin/issue-types")),
    ).toBe(false);
  });
});
