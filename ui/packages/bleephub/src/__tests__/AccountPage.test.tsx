import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { AccountPage } from "../pages/AccountPage.js";

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

function installFetchRoutes(overrides: Record<string, () => Response> = {}) {
  mockFetch.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    const key = `${method} ${url}`;
    if (overrides[key]) return Promise.resolve(overrides[key]());
    if (key === "GET /api/v3/user/keys")
      return Promise.resolve(
        jsonResponse([
          {
            id: 3,
            key: "ssh-ed25519 AAAA...",
            title: "laptop",
            verified: true,
            created_at: "2026-05-01T00:00:00Z",
            read_only: false,
          },
        ]),
      );
    if (key === "GET /api/v3/user/gpg_keys") return Promise.resolve(jsonResponse([]));
    if (key === "GET /api/v3/user/ssh_signing_keys") return Promise.resolve(jsonResponse([]));
    if (key === "GET /api/v3/user/emails")
      return Promise.resolve(
        jsonResponse([
          { email: "admin@example.com", primary: true, verified: true, visibility: "private" },
          { email: "alt@example.com", primary: false, verified: false, visibility: null },
        ]),
      );
    if (key === "GET /api/v3/user/blocks")
      return Promise.resolve(jsonResponse([{ login: "spammer" }]));
    return Promise.resolve(jsonResponse({ message: `unexpected ${key}` }, 500));
  });
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/ui/account"]}>
        <AccountPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("AccountPage", () => {
  it("lists SSH keys from GET /user/keys", async () => {
    installFetchRoutes();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("laptop")).toBeInTheDocument();
    });
    expect(screen.getByText(/verified · added/)).toBeInTheDocument();
  });

  it("renders a left settings sub-nav and marks the active item", async () => {
    installFetchRoutes();
    renderPage();
    await waitFor(() => screen.getByText("laptop"));

    const nav = screen.getByRole("navigation", { name: "Settings" });
    expect(nav).toBeInTheDocument();
    // Section headings from the vertical sub-nav are present.
    expect(screen.getByText("Access")).toBeInTheDocument();
    expect(screen.getByText("Moderation")).toBeInTheDocument();

    // The default SSH keys item is the current page; switching updates it.
    const sshItem = screen.getByRole("button", { name: "SSH keys" });
    expect(sshItem).toHaveAttribute("aria-current", "page");

    fireEvent.click(screen.getByRole("button", { name: "Emails" }));
    await waitFor(() => screen.getByText("admin@example.com"));
    expect(screen.getByRole("button", { name: "Emails" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "SSH keys" })).not.toHaveAttribute("aria-current");
  });

  it("adds an SSH key via POST /user/keys", async () => {
    installFetchRoutes({
      "POST /api/v3/user/keys": () =>
        jsonResponse(
          {
            id: 4,
            key: "ssh-rsa BBBB...",
            title: "desktop",
            verified: true,
            created_at: "2026-06-01T00:00:00Z",
            read_only: false,
          },
          201,
        ),
    });
    renderPage();
    await waitFor(() => screen.getByText("laptop"));

    fireEvent.change(document.getElementById("user-ssh-keys-title")!, {
      target: { value: "desktop" },
    });
    fireEvent.change(document.getElementById("user-ssh-keys-key")!, {
      target: { value: "ssh-rsa BBBB..." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add SSH key" }));

    await waitFor(() => {
      const post = mockFetch.mock.calls.find((c) => c[1]?.method === "POST");
      expect(post).toBeDefined();
      expect(String(post![0])).toBe("/api/v3/user/keys");
      expect(post![1].body).toContain("ssh-rsa BBBB...");
    });
  });

  it("creates a repository-scoped fine-grained token and shows its credential once", async () => {
    const dashboard = {
      tokens: [],
      resource_owners: [{ login: "admin", type: "User" }],
      repositories: { admin: [{ id: 7, name: "release", private: true }] },
      pending_requests: [],
    };
    installFetchRoutes({
      "GET /settings/personal-access-tokens": () => jsonResponse(dashboard),
      "POST /settings/personal-access-tokens": () => jsonResponse({
        id: 11, name: "Release automation", resource_owner: "admin",
        repository_selection: "all", repository_ids: [], permissions: { repository: { contents: "read" } },
        created_at: "2026-07-12T00:00:00Z", expires_at: null, status: "active",
        token: "github_pat_once_only",
      }, 201),
    });
    renderPage();
    await waitFor(() => screen.getByText("laptop"));
    fireEvent.click(screen.getByRole("button", { name: "Personal access tokens" }));
    expect(await screen.findByText("Fine-grained personal access tokens")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Token name"), { target: { value: "Release automation" } });
    fireEvent.click(screen.getByRole("button", { name: "Generate token" }));
    expect(await screen.findByText("github_pat_once_only")).toBeInTheDocument();
    const post = mockFetch.mock.calls.find((call) => call[1]?.method === "POST" && String(call[0]) === "/settings/personal-access-tokens");
    expect(post).toBeDefined();
    expect(post![1].body).toContain('"resource_owner":"admin"');
    expect(post![1].body).toContain('"contents":"read"');
  });

  it("deletes an SSH key via DELETE /user/keys/{id}", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    installFetchRoutes({
      "DELETE /api/v3/user/keys/3": () => new Response(null, { status: 204 }),
    });
    renderPage();
    await waitFor(() => screen.getByText("laptop"));

    fireEvent.click(screen.getByRole("button", { name: "delete" }));
    await waitFor(() => {
      const del = mockFetch.mock.calls.find((c) => c[1]?.method === "DELETE");
      expect(del).toBeDefined();
      expect(String(del![0])).toBe("/api/v3/user/keys/3");
    });
  });

  it("shows emails with visibility and toggles the primary visibility", async () => {
    installFetchRoutes({
      "PATCH /api/v3/user/email/visibility": () =>
        jsonResponse([
          { email: "admin@example.com", primary: true, verified: true, visibility: "public" },
        ]),
    });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Emails" }));
    await waitFor(() => {
      expect(screen.getByText("admin@example.com")).toBeInTheDocument();
    });
    expect(screen.getByText(/primary · verified · visibility: private/)).toBeInTheDocument();
    expect(screen.getByText(/unverified · visibility unset/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Make public" }));
    await waitFor(() => {
      const patch = mockFetch.mock.calls.find((c) => c[1]?.method === "PATCH");
      expect(patch).toBeDefined();
      expect(String(patch![0])).toBe("/api/v3/user/email/visibility");
      expect(patch![1].body).toContain('"visibility":"public"');
    });
  });

  it("adds and removes email addresses", async () => {
    installFetchRoutes({
      "POST /api/v3/user/emails": () =>
        jsonResponse(
          [{ email: "new@example.com", primary: false, verified: false, visibility: null }],
          201,
        ),
    });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Emails" }));
    await waitFor(() => screen.getByText("admin@example.com"));

    fireEvent.change(screen.getByLabelText("New email address"), {
      target: { value: "new@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    await waitFor(() => {
      const post = mockFetch.mock.calls.find((c) => c[1]?.method === "POST");
      expect(post).toBeDefined();
      expect(String(post![0])).toBe("/api/v3/user/emails");
      expect(post![1].body).toContain("new@example.com");
    });
  });

  it("lists and unblocks blocked users", async () => {
    installFetchRoutes({
      "DELETE /api/v3/user/blocks/spammer": () => new Response(null, { status: 204 }),
    });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Blocked users" }));
    await waitFor(() => {
      expect(screen.getByText("spammer")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "unblock" }));
    await waitFor(() => {
      const del = mockFetch.mock.calls.find((c) => c[1]?.method === "DELETE");
      expect(del).toBeDefined();
      expect(String(del![0])).toBe("/api/v3/user/blocks/spammer");
    });
  });

  it("blocks a user via PUT /user/blocks/{username}", async () => {
    installFetchRoutes({
      "PUT /api/v3/user/blocks/troll": () => new Response(null, { status: 204 }),
    });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Blocked users" }));
    await waitFor(() => screen.getByLabelText("Username to block"));

    fireEvent.change(screen.getByLabelText("Username to block"), { target: { value: "troll" } });
    fireEvent.click(screen.getByRole("button", { name: "Block" }));
    await waitFor(() => {
      const put = mockFetch.mock.calls.find((c) => c[1]?.method === "PUT");
      expect(put).toBeDefined();
      expect(String(put![0])).toBe("/api/v3/user/blocks/troll");
    });
  });
});
