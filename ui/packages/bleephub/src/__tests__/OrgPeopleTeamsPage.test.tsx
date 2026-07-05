import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { OrgPeoplePage } from "../pages/OrgPeoplePage.js";
import { OrgTeamsPage } from "../pages/OrgTeamsPage.js";

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

function renderPeople(org = "acme") {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/ui/orgs/${org}/people`]}>
        <Routes>
          <Route path="/ui/orgs/:org/people" element={<OrgPeoplePage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderTeams(org = "acme") {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/ui/orgs/${org}/teams`]}>
        <Routes>
          <Route path="/ui/orgs/:org/teams" element={<OrgTeamsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("OrgPeoplePage", () => {
  it("lists members from /api/v3/orgs/{org}/members and links to profiles", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse([{ id: 2, login: "dev", avatar_url: "", type: "User", site_admin: false }]),
    );
    renderPeople();
    await waitFor(() => {
      expect(screen.getByText("dev")).toBeInTheDocument();
    });
    const link = screen.getByRole("link", { name: "dev" });
    expect(link).toHaveAttribute("href", "/ui/dev");
    expect(mockFetch).toHaveBeenCalledWith("/api/v3/orgs/acme/members", expect.anything());
  });

  it("surfaces a members load error", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ message: "Not Found" }, 404));
    renderPeople("missing");
    await waitFor(() => {
      expect(screen.getByText(/failed to load members/i)).toBeInTheDocument();
    });
  });
});

describe("OrgTeamsPage", () => {
  it("lists teams from /api/v3/orgs/{org}/teams", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse([
        { id: 1, slug: "core", name: "Core", description: "core team", privacy: "closed", permission: "push", html_url: "http://x", parent: null },
      ]),
    );
    renderTeams();
    await waitFor(() => {
      expect(screen.getByText("Core")).toBeInTheDocument();
    });
    expect(screen.getByText("@core")).toBeInTheDocument();
    expect(mockFetch).toHaveBeenCalledWith("/api/v3/orgs/acme/teams", expect.anything());
  });

  it("shows an honest empty state when the org has no teams", async () => {
    mockFetch.mockResolvedValue(jsonResponse([]));
    renderTeams();
    await waitFor(() => {
      expect(screen.getByText("No teams")).toBeInTheDocument();
    });
  });
});
