import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { DiscussionsPage } from "../pages/DiscussionsPage.js";

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
          <Route path="/ui/repos/:owner/:repo/discussions" element={<DiscussionsPage />} />
          <Route path="/ui/repos/:owner/:repo/discussions/:number" element={<DiscussionsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const category = { id: "DGC_kgDO00000001", name: "General", emoji: ":speech_balloon:", description: "", isAnswerable: false };

function discussion(number: number, title: string) {
  return {
    id: `D_kgDO${String(number).padStart(8, "0")}`,
    number,
    title,
    bodyText: "body",
    author: { login: "admin", avatarUrl: "" },
    category,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    comments: { totalCount: 0 },
  };
}

describe("DiscussionsPage list", () => {
  it("renders the discussion list", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = url.toString();
      if (u.includes("/repos/admin/test")) {
        return Promise.resolve(jsonResponse({ id: 1, node_id: "R_kgDO00000001" }));
      }
      if (u.includes("/api/graphql")) {
        const body = JSON.parse((init?.body as string) ?? "{}");
        if (body.query.includes("discussionCategories")) {
          return Promise.resolve(jsonResponse({ data: { repository: { discussionCategories: { nodes: [category] } } } }));
        }
        return Promise.resolve(
          jsonResponse({
            data: {
              repository: {
                discussions: {
                  nodes: [discussion(1, "First discussion")],
                  totalCount: 1,
                  pageInfo: { hasNextPage: false, endCursor: null },
                },
              },
            },
          }),
        );
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/discussions");
    await waitFor(() => {
      expect(screen.getByText("First discussion")).toBeInTheDocument();
    });
  });

  it("shows category filters", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = url.toString();
      if (u.includes("/repos/admin/test")) {
        return Promise.resolve(jsonResponse({ id: 1, node_id: "R_kgDO00000001" }));
      }
      if (u.includes("/api/graphql")) {
        const body = JSON.parse((init?.body as string) ?? "{}");
        if (body.query.includes("discussionCategories")) {
          return Promise.resolve(jsonResponse({ data: { repository: { discussionCategories: { nodes: [category] } } } }));
        }
        return Promise.resolve(
          jsonResponse({ data: { repository: { discussions: { nodes: [], totalCount: 0, pageInfo: { hasNextPage: false, endCursor: null } } } } }),
        );
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/discussions");
    await waitFor(() => {
      expect(screen.getByText(/General/i)).toBeInTheDocument();
    });
  });
});

describe("DiscussionsPage detail", () => {
  it("renders the discussion when found", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = url.toString();
      if (u.includes("/api/graphql")) {
        const body = JSON.parse((init?.body as string) ?? "{}");
        if (body.query.includes("discussionCategories")) {
          return Promise.resolve(jsonResponse({ data: { repository: { discussionCategories: { nodes: [category] } } } }));
        }
        return Promise.resolve(
          jsonResponse({
            data: {
              repository: {
                discussion: {
                  ...discussion(7, "A real discussion"),
                  body: "details",
                  bodyHTML: "<p>details</p>",
                  comments: { nodes: [], totalCount: 0 },
                },
              },
            },
          }),
        );
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/discussions/7");
    await waitFor(() => {
      expect(screen.getByText("A real discussion")).toBeInTheDocument();
    });
  });

  it("shows a not-found state for a missing discussion", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = url.toString();
      if (u.includes("/api/graphql")) {
        const body = JSON.parse((init?.body as string) ?? "{}");
        if (body.query.includes("discussionCategories")) {
          return Promise.resolve(jsonResponse({ data: { repository: { discussionCategories: { nodes: [category] } } } }));
        }
        return Promise.resolve(
          jsonResponse({
            data: { repository: { discussion: null } },
            errors: [{ message: "Could not resolve to a Discussion with the number of 999.", type: "NOT_FOUND" }],
          }),
        );
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/repos/admin/test/discussions/999");
    await waitFor(() => {
      expect(screen.getByText(/failed to load discussion #999/i)).toBeInTheDocument();
    });
  });
});
