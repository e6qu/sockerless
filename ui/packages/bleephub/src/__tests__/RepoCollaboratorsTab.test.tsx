import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { RepoSettingsPage } from "../pages/RepoSettingsPage.js";

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

const repo = {
  id: 1,
  name: "collab-repo",
  full_name: "admin/collab-repo",
  description: "",
  homepage: null,
  default_branch: "main",
  visibility: "private",
  private: true,
  owner: { login: "admin", type: "User" },
  has_issues: true,
  has_projects: false,
  has_wiki: false,
  has_pull_requests: true,
  allow_squash_merge: true,
  allow_merge_commit: true,
  allow_rebase_merge: true,
  allow_auto_merge: false,
  delete_branch_on_merge: false,
};

const collaborators = [
  {
    id: 1,
    login: "admin",
    type: "User",
    role_name: "admin",
    permissions: { pull: true, push: true, admin: true },
  },
  {
    id: 2,
    login: "carol",
    type: "User",
    role_name: "write",
    permissions: { pull: true, push: true, admin: false },
  },
];

const invitation = {
  id: 9,
  invitee: { login: "dave" },
  inviter: { login: "admin" },
  permissions: "read",
  created_at: "2026-06-01T00:00:00Z",
  expired: false,
};

function installFetchRoutes(overrides: Record<string, () => Response> = {}) {
  mockFetch.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    const key = `${method} ${url}`;
    if (overrides[key]) return Promise.resolve(overrides[key]());
    if (key === "GET /api/v3/repos/admin/collab-repo") return Promise.resolve(jsonResponse(repo));
    if (key === "GET /api/v3/repos/admin/collab-repo/collaborators")
      return Promise.resolve(jsonResponse(collaborators));
    if (key === "GET /api/v3/repos/admin/collab-repo/invitations")
      return Promise.resolve(jsonResponse([invitation]));
    if (url.includes("/issues?") || url.includes("/pulls?"))
      return Promise.resolve(jsonResponse([]));
    return Promise.resolve(jsonResponse({ message: `unexpected ${key}` }, 500));
  });
}

async function renderCollaboratorsTab() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/ui/repos/admin/collab-repo/settings"]}>
        <Routes>
          <Route path="/ui/repos/:owner/:repo/settings" element={<RepoSettingsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await waitFor(() => screen.getByRole("button", { name: "Collaborators" }));
  fireEvent.click(screen.getByRole("button", { name: "Collaborators" }));
}

describe("RepoSettingsPage collaborators tab", () => {
  it("lists collaborators with roles and pending invitations", async () => {
    installFetchRoutes();
    await renderCollaboratorsTab();
    await waitFor(() => {
      expect(screen.getByText("carol")).toBeInTheDocument();
    });
    expect(screen.getByText("write")).toBeInTheDocument();
    expect(screen.getByText("dave")).toBeInTheDocument();
    expect(screen.getByText(/read · invited by admin/)).toBeInTheDocument();
  });

  it("invites a user by username with the selected role", async () => {
    installFetchRoutes({
      "PUT /api/v3/repos/admin/collab-repo/collaborators/erin": () =>
        jsonResponse(
          { ...invitation, id: 12, invitee: { login: "erin" }, permissions: "admin" },
          201,
        ),
    });
    await renderCollaboratorsTab();
    await waitFor(() => screen.getByLabelText("Username to invite"));

    fireEvent.change(screen.getByLabelText("Username to invite"), { target: { value: "erin" } });
    fireEvent.change(screen.getByLabelText("Role"), { target: { value: "admin" } });
    fireEvent.click(screen.getByRole("button", { name: "Invite" }));

    await waitFor(() => {
      expect(screen.getByText("Invitation sent to erin.")).toBeInTheDocument();
    });
    const putCall = mockFetch.mock.calls.find((c) => c[1]?.method === "PUT");
    expect(putCall).toBeDefined();
    expect(String(putCall![0])).toBe("/api/v3/repos/admin/collab-repo/collaborators/erin");
    expect(putCall![1].body).toContain('"permission":"admin"');
  });

  it("cancels a pending invitation", async () => {
    installFetchRoutes({
      "DELETE /api/v3/repos/admin/collab-repo/invitations/9": () =>
        new Response(null, { status: 204 }),
    });
    await renderCollaboratorsTab();
    await waitFor(() => screen.getByText("dave"));

    fireEvent.click(screen.getByRole("button", { name: "cancel" }));
    await waitFor(() => {
      const del = mockFetch.mock.calls.find((c) => c[1]?.method === "DELETE");
      expect(del).toBeDefined();
      expect(String(del![0])).toBe("/api/v3/repos/admin/collab-repo/invitations/9");
    });
  });

  it("removes a collaborator", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    installFetchRoutes({
      "DELETE /api/v3/repos/admin/collab-repo/collaborators/carol": () =>
        new Response(null, { status: 204 }),
    });
    await renderCollaboratorsTab();
    await waitFor(() => screen.getByText("carol"));

    fireEvent.click(screen.getByRole("button", { name: "remove" }));
    await waitFor(() => {
      const del = mockFetch.mock.calls.find(
        (c) => c[1]?.method === "DELETE" && String(c[0]).includes("/collaborators/"),
      );
      expect(del).toBeDefined();
      expect(String(del![0])).toBe("/api/v3/repos/admin/collab-repo/collaborators/carol");
    });
  });
});
