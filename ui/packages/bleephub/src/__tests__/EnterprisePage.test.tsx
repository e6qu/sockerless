import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { EnterprisePage } from "../pages/EnterprisePage.js";

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

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/ui/admin/enterprise"]}>
        <EnterprisePage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const platformTeam = {
  id: 1,
  name: "Platform",
  slug: "platform",
  description: "Cross-org platform crew",
  organization_selection_type: "all",
  group_id: null,
  notification_setting: "notifications_enabled",
  created_at: "2026-05-01T00:00:00Z",
  updated_at: "2026-05-01T00:00:00Z",
};

describe("EnterprisePage teams tab", () => {
  it("lists enterprise teams via the bleephub enterprise slug", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/api/v3/enterprises/bleephub/teams")) {
        return Promise.resolve(jsonResponse([platformTeam]));
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Platform")).toBeInTheDocument();
    });
    expect(screen.getByText("@platform")).toBeInTheDocument();
    expect(screen.getByText(/Cross-org platform crew · organizations: all/)).toBeInTheDocument();
    const listCall = mockFetch.mock.calls.find((c) =>
      c[0].toString().includes("/api/v3/enterprises/bleephub/teams"),
    );
    expect(listCall).toBeTruthy();
  });

  it("shows an empty state when no teams exist", async () => {
    mockFetch.mockImplementation(() => Promise.resolve(jsonResponse([])));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/no enterprise teams yet/i)).toBeInTheDocument();
    });
  });

  it("creates a team through the dialog", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = url.toString();
      if (u.endsWith("/enterprises/bleephub/teams") && init?.method === "POST") {
        return Promise.resolve(jsonResponse(platformTeam, 201));
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /new enterprise team/i })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /new enterprise team/i }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Platform" } });
    fireEvent.change(screen.getByLabelText("Organization selection"), { target: { value: "all" } });
    fireEvent.click(screen.getByRole("button", { name: /create team/i }));
    await waitFor(() => {
      const post = mockFetch.mock.calls.find(
        (c) => c[0].toString().endsWith("/enterprises/bleephub/teams") && c[1]?.method === "POST",
      );
      expect(post).toBeTruthy();
      expect(JSON.parse(String(post![1]!.body))).toMatchObject({
        name: "Platform",
        organization_selection_type: "all",
      });
    });
  });

  it("manages team memberships from the members dialog", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = url.toString();
      if (u.includes("/teams/platform/memberships/dev1") && init?.method === "PUT") {
        return Promise.resolve(
          jsonResponse({ id: 4, login: "dev1", avatar_url: "", type: "User", site_admin: false }, 201),
        );
      }
      if (u.includes("/teams/platform/memberships")) {
        return Promise.resolve(
          jsonResponse([{ id: 3, login: "existing", avatar_url: "", type: "User", site_admin: false }]),
        );
      }
      if (u.includes("/enterprises/bleephub/teams")) {
        return Promise.resolve(jsonResponse([platformTeam]));
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "members" })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: "members" }));
    await waitFor(() => {
      expect(screen.getByText("@existing")).toBeInTheDocument();
    });
    fireEvent.change(screen.getByLabelText("Username to add"), { target: { value: "dev1" } });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    await waitFor(() => {
      const put = mockFetch.mock.calls.find(
        (c) =>
          c[0].toString().includes("/teams/platform/memberships/dev1") && c[1]?.method === "PUT",
      );
      expect(put).toBeTruthy();
    });
  });
});

describe("EnterprisePage settings tab", () => {
  function mockSettings() {
    mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = url.toString();
      if (u.includes("/actions/cache/storage-limit") && init?.method === "PUT") {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      if (u.includes("/actions/cache/storage-limit")) {
        return Promise.resolve(jsonResponse({ max_cache_size_gb: 10 }));
      }
      if (u.includes("/dependabot/repository-access/default-level") && init?.method === "PUT") {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      if (u.includes("/dependabot/repository-access")) {
        return Promise.resolve(
          jsonResponse({
            default_level: "public",
            accessible_repositories: [
              { id: 7, full_name: "acme/private-lib", name: "private-lib" },
            ],
          }),
        );
      }
      return Promise.resolve(jsonResponse([]));
    });
  }

  it("shows the Actions cache limit and Dependabot access, and updates the limit", async () => {
    mockSettings();
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Settings" }));

    await waitFor(() => {
      expect(screen.getByText("10 GB")).toBeInTheDocument();
    });
    expect(screen.getByText("acme/private-lib")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("New cache size limit in GB"), {
      target: { value: "25" },
    });
    fireEvent.click(screen.getByRole("button", { name: /update limit/i }));
    await waitFor(() => {
      const put = mockFetch.mock.calls.find(
        (c) => c[0].toString().includes("/actions/cache/storage-limit") && c[1]?.method === "PUT",
      );
      expect(put).toBeTruthy();
      expect(JSON.parse(String(put![1]!.body))).toEqual({ max_cache_size_gb: 25 });
    });
  });

  it("surfaces settings load failures", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => {
      const u = url.toString();
      if (u.includes("/actions/cache/storage-limit")) {
        return Promise.resolve(jsonResponse({ message: "boom" }, 500));
      }
      if (u.includes("/dependabot/repository-access")) {
        return Promise.resolve(jsonResponse({ default_level: null, accessible_repositories: [] }));
      }
      return Promise.resolve(jsonResponse([]));
    });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Settings" }));
    await waitFor(() => {
      expect(screen.getByText(/failed to load actions cache limit/i)).toBeInTheDocument();
    });
    expect(
      screen.getByText(/no repositories granted private-repository dependabot access/i),
    ).toBeInTheDocument();
  });
});
