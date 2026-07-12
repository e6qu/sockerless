import { describe, it, expect, vi, afterEach } from "vitest";
import {
  fetchOAuthApps,
  createApp,
  fetchSecrets,
  fetchEnvironments,
  fetchRepoIssuesPage,
  fetchRepoCommits,
  fetchPRDetail,
  createIssue,
  parseLinkNext,
  dispatchWorkflow,
  setToken,
  clearToken,
  fetchUserCodespaces,
  fetchRepoCodespaces,
  fetchCodespaceMachines,
  createUserCodespace,
  createRepoCodespace,
  startCodespace,
  stopCodespace,
  deleteCodespace,
  fetchNotifications,
  markThreadRead,
  getThreadSubscription,
  setThreadSubscription,
  deleteThreadSubscription,
  fetchGistCommits,
  fetchGistForks,
  forkGist,
  starGist,
  unstarGist,
  isGistStarred,
  fetchPublicGists,
  fetchStarredGists,
  fetchRepos,
  fetchUsers,
  createUser,
  updateUser,
  deleteUser,
  fetchOrgs,
  createOrg,
  updateOrg,
  deleteOrg,
  fetchTeams,
  createTeam,
  updateTeam,
  deleteTeam,
  fetchAuditLog,
  fetchAuditLogOrgs,
  buildAuditLogPhrase,
  fetchEnterpriseSlug,
} from "../api.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  mockFetch.mockReset();
  clearToken();
});

describe("api wire-shape normalization", () => {
  // The server emits snake_case (client_id/callback_url/created_at), while the
  // user interface reads camelCase. fetchOAuthApps bridges that boundary.
  it("fetchOAuthApps maps snake_case wire fields to camelCase", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse([
        {
          client_id: "Iv1.abc123",
          name: "my-oauth",
          description: "d",
          url: "https://example.test",
          callback_url: "https://example.test/cb",
          owner_id: 1,
          created_at: "2026-01-01T00:00:00Z",
        },
      ]),
    );
    const apps = await fetchOAuthApps();
    expect(apps[0].clientId).toBe("Iv1.abc123");
    expect(apps[0].callbackUrl).toBe("https://example.test/cb");
    expect(apps[0].createdAt).toBe("2026-01-01T00:00:00Z");
  });

  it("createApp uses the GitHub App Manifest flow", async () => {
    setToken("admintoken");
    const manifestRedirect = new Response("", { status: 200 });
    Object.defineProperty(manifestRedirect, "url", { value: "http://localhost/ui/apps?code=manifest-code" });
    mockFetch
      .mockResolvedValueOnce(manifestRedirect)
      .mockResolvedValueOnce(
        jsonResponse({
          client_id: "Iv1.created",
          pem: "-----BEGIN RSA PRIVATE KEY-----\nkey\n-----END RSA PRIVATE KEY-----",
          client_secret: "secret",
          webhook_secret: "hook",
        }),
      );

    const created = await createApp({
      name: "Manifest App",
      description: "Created through the manifest flow",
      permissions: { contents: "read" },
      events: ["push"],
    });

    expect(mockFetch.mock.calls[0][0]).toBe("/settings/apps/new");
    const firstOptions = mockFetch.mock.calls[0][1] as RequestInit;
    expect(firstOptions.method).toBe("POST");
    expect(firstOptions.redirect).toBeUndefined();
    expect(firstOptions.headers).toMatchObject({
      Authorization: "Bearer admintoken",
      "Content-Type": "application/x-www-form-urlencoded",
    });
    const manifest = JSON.parse(
      new URLSearchParams(firstOptions.body as string).get("manifest") || "{}",
    );
    expect(manifest.name).toBe("Manifest App");
    expect(manifest.default_permissions).toEqual({ contents: "read" });
    expect(manifest.default_events).toEqual(["push"]);

    expect(mockFetch.mock.calls[1][0]).toBe("/api/v3/app-manifests/manifest-code/conversions");
    expect(mockFetch.mock.calls[1][1]).toMatchObject({
      method: "POST",
      headers: { Authorization: "Bearer admintoken" },
    });
    expect(created.clientId).toBe("Iv1.created");
    expect(created.client_secret).toBe("secret");
  });

  // BUG-1596: server returns the GitHub list envelope, not a bare array.
  it("fetchSecrets unwraps the {secrets:[…]} envelope", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({ total_count: 1, secrets: [{ name: "TOKEN", created_at: "x", updated_at: "y" }] }),
    );
    const secrets = await fetchSecrets("admin", "repo");
    expect(Array.isArray(secrets)).toBe(true);
    expect(secrets[0].name).toBe("TOKEN");
  });

  it("fetchEnvironments unwraps the {environments:[…]} envelope", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({ total_count: 1, environments: [{ name: "prod", node_id: "n", url: "u" }] }),
    );
    const envs = await fetchEnvironments("admin", "repo");
    expect(envs[0].name).toBe("prod");
  });

  it("fetchEnterpriseSlug requires the runtime enterprise coordinate", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ status: "ok", service: "bleephub" }));
    await expect(fetchEnterpriseSlug()).rejects.toThrow(
      "/health response did not include enterprise_slug",
    );
  });
});

describe("repository API helpers", () => {
  it("fetchRepoCommits reads the user-interface commit adapter", async () => {
    mockFetch.mockResolvedValue(jsonResponse([]));

    await expect(fetchRepoCommits("admin", "empty")).resolves.toEqual([]);
    expect(mockFetch.mock.calls[0][0]).toBe("/ui-data/repos/admin/empty/commits");
  });

  it("fetchRepoCommits still fails on adapter errors", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ message: "Git object unavailable" }, 500));

    await expect(fetchRepoCommits("admin", "blocked")).rejects.toMatchObject({ status: 500 });
  });
});

describe("Link-header pagination", () => {
  it("parseLinkNext extracts the rel=next URL", () => {
    expect(
      parseLinkNext(
        `</api/v3/repos/a/b/issues?page=2&per_page=50>; rel="next", </api/v3/repos/a/b/issues?page=3&per_page=50>; rel="last"`,
      ),
    ).toBe("/api/v3/repos/a/b/issues?page=2&per_page=50");
  });

  it("parseLinkNext returns null without a next page", () => {
    expect(parseLinkNext(null)).toBeNull();
    expect(parseLinkNext(`</api/v3/x?page=1>; rel="prev"`)).toBeNull();
  });

  it("fetchRepoIssuesPage surfaces items plus the next-page URL", async () => {
    mockFetch.mockResolvedValue(
      new Response(JSON.stringify([{ id: 1 }]), {
        status: 200,
        headers: {
          "Content-Type": "application/json",
          Link: `</api/v3/repos/a/b/issues?page=2&per_page=50&state=open>; rel="next"`,
        },
      }),
    );
    const page = await fetchRepoIssuesPage("a", "b", "open");
    expect(page.items).toHaveLength(1);
    expect(page.nextUrl).toBe("/api/v3/repos/a/b/issues?page=2&per_page=50&state=open");
  });

  it("fetchRepoIssuesPage follows an explicit page URL when given", async () => {
    mockFetch.mockResolvedValue(jsonResponse([]));
    await fetchRepoIssuesPage("a", "b", "open", "/api/v3/repos/a/b/issues?page=2");
    expect(mockFetch.mock.calls[0][0]).toBe("/api/v3/repos/a/b/issues?page=2");
  });

  it("fetchRepos lists repositories through the GitHub REST user repository endpoint", async () => {
    mockFetch
      .mockResolvedValueOnce(
        new Response(JSON.stringify([{ id: 1, full_name: "admin/one" }]), {
          status: 200,
          headers: {
            "Content-Type": "application/json",
            Link: `</api/v3/user/repos?page=2&per_page=100>; rel="next"`,
          },
        }),
      )
      .mockResolvedValueOnce(jsonResponse([{ id: 2, full_name: "admin/two" }]));

    const repos = await fetchRepos();

    expect(repos.map((r) => r.full_name)).toEqual(["admin/one", "admin/two"]);
    expect(mockFetch.mock.calls.map((c) => c[0])).toEqual([
      "/api/v3/user/repos?per_page=100",
      "/api/v3/user/repos?page=2&per_page=100",
    ]);
  });
});

describe("single-resource fetches", () => {
  it("fetchPRDetail hits the single-PR endpoint", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ id: 1, number: 7 }));
    await fetchPRDetail("a", "b", 7);
    expect(mockFetch.mock.calls[0][0]).toBe("/api/v3/repos/a/b/pulls/7");
  });
});

describe("mutation error surfaces", () => {
  it("createIssue includes the response body in its thrown error", async () => {
    mockFetch.mockResolvedValue(
      new Response(JSON.stringify({ message: "Validation Failed" }), { status: 422 }),
    );
    await expect(createIssue("a", "b", { title: "t" })).rejects.toThrow(
      /createIssue 422: .*Validation Failed/,
    );
  });
});

describe("api auth headers", () => {
  // BUG-1592: dispatchWorkflow must carry the Authorization header like
  // every other mutating call.
  it("dispatchWorkflow sends the Authorization header", async () => {
    setToken("ghp_testtoken");
    mockFetch.mockResolvedValue(new Response(null, { status: 204 }));
    await dispatchWorkflow("admin/repo", 1, { ref: "main" });
    expect(mockFetch).toHaveBeenCalledTimes(1);
    const [, opts] = mockFetch.mock.calls[0];
    expect((opts.headers as Record<string, string>).Authorization).toBe("Bearer ghp_testtoken");
  });
});

// ─── GitHub Codespaces Representational State Transfer ──────────────────

describe("Codespaces application programming interface helpers", () => {
  const machine = {
    name: "basicLinux32",
    display_name: "Basic Linux",
    operating_system: "linux",
    storage_in_bytes: 34359738368,
    memory_in_bytes: 4294967296,
    cpus: 2,
    prebuild_availability: "none",
  };

  const codespace = {
    id: 1,
    name: "crimson-spoon-abc123",
    display_name: "my codespace",
    environment_id: "abc",
    owner: { login: "admin", type: "User" },
    billable_owner: { login: "admin", type: "User" },
    repository: { id: 10, full_name: "admin/test", name: "test", owner: { login: "admin", type: "User" } },
    machine,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    last_used_at: "2026-01-01T00:00:00Z",
    state: "Available",
    url: "/api/v3/user/codespaces/crimson-spoon-abc123",
    html_url: "/ui/codespaces/crimson-spoon-abc123",
    web_url: "http://x",
    billing_url: "http://x/billing",
    git_status: { ahead: 0, behind: 0, has_uncommitted_changes: false, ref: "main" },
    devcontainer_path: ".devcontainer/devcontainer.json",
    image: "mcr.microsoft.com/devcontainers/base",
    retention_period_minutes: 10080,
  };

  it("fetchUserCodespaces unwraps the codespaces envelope", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ total_count: 1, codespaces: [codespace] }));
    const page = await fetchUserCodespaces();
    expect(page.items).toHaveLength(1);
    expect(page.items[0].name).toBe("crimson-spoon-abc123");
  });

  it("fetchRepoCodespaces hits the repo-scoped endpoint", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ total_count: 1, codespaces: [codespace] }));
    await fetchRepoCodespaces("admin", "test");
    expect(mockFetch.mock.calls[0][0]).toBe("/api/v3/repos/admin/test/codespaces");
  });

  it("fetchCodespaceMachines unwraps the machines envelope", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ total_count: 1, machines: [machine] }));
    const page = await fetchCodespaceMachines("admin", "test");
    expect(page.items).toHaveLength(1);
    expect(page.items[0].name).toBe("basicLinux32");
  });

  it("createUserCodespace sends repository_id and display_name", async () => {
    mockFetch.mockResolvedValue(jsonResponse(codespace, 201));
    await createUserCodespace({ repository_id: 10, machine: "basicLinux32", display_name: "New space" });
    const [url, opts] = mockFetch.mock.calls[0];
    expect(url).toBe("/api/v3/user/codespaces");
    expect(opts.method).toBe("POST");
    const body = JSON.parse(opts.body as string);
    expect(body.repository_id).toBe(10);
    expect(body.display_name).toBe("New space");
    expect(body.machine).toBe("basicLinux32");
  });

  it("createRepoCodespace hits the repo-scoped endpoint", async () => {
    mockFetch.mockResolvedValue(jsonResponse(codespace, 201));
    await createRepoCodespace("admin", "test", { machine: "basicLinux32" });
    expect(mockFetch.mock.calls[0][0]).toBe("/api/v3/repos/admin/test/codespaces");
  });

  it("startCodespace POSTs to the start subresource", async () => {
    mockFetch.mockResolvedValue(jsonResponse(codespace));
    await startCodespace("crimson-spoon-abc123");
    const [url, opts] = mockFetch.mock.calls[0];
    expect(url).toBe("/api/v3/user/codespaces/crimson-spoon-abc123/start");
    expect(opts.method).toBe("POST");
  });

  it("stopCodespace POSTs to the stop subresource", async () => {
    mockFetch.mockResolvedValue(jsonResponse(codespace));
    await stopCodespace("crimson-spoon-abc123");
    const [url, opts] = mockFetch.mock.calls[0];
    expect(url).toBe("/api/v3/user/codespaces/crimson-spoon-abc123/stop");
    expect(opts.method).toBe("POST");
  });

  it("deleteCodespace DELETEs the named codespace", async () => {
    mockFetch.mockResolvedValue(new Response(null, { status: 204 }));
    await deleteCodespace("crimson-spoon-abc123");
    const [url, opts] = mockFetch.mock.calls[0];
    expect(url).toBe("/api/v3/user/codespaces/crimson-spoon-abc123");
    expect(opts.method).toBe("DELETE");
  });
});

// ─── Notifications Representational State Transfer ──────────────────────

describe("Notifications application programming interface helpers", () => {
  const thread = {
    id: "t1",
    repository: { full_name: "admin/repo" },
    subject: { title: "Issue title", url: "/api/v3/repos/admin/repo/issues/1", latest_comment_url: "", type: "Issue" },
    reason: "subscribed",
    unread: true,
    updated_at: "2026-01-01T00:00:00Z",
    last_read_at: null,
    subscription_url: "/api/v3/notifications/threads/t1/subscription",
    url: "/api/v3/notifications/threads/t1",
  };

  const subscription = {
    subscribed: true,
    ignored: false,
    reason: "subscribed",
    created_at: "2026-01-01T00:00:00Z",
    url: "/api/v3/notifications/threads/t1/subscription",
    thread_url: "/api/v3/notifications/threads/t1/subscription",
  };

  it("fetchNotifications lists threads", async () => {
    mockFetch.mockResolvedValue(jsonResponse([thread]));
    const threads = await fetchNotifications();
    expect(mockFetch.mock.calls[0][0]).toBe("/api/v3/notifications");
    expect(threads).toHaveLength(1);
    expect(threads[0].id).toBe("t1");
  });

  it("markThreadRead PATCHes the thread", async () => {
    mockFetch.mockResolvedValue(new Response(null, { status: 205 }));
    await markThreadRead("t1");
    const [url, opts] = mockFetch.mock.calls[0];
    expect(url).toBe("/api/v3/notifications/threads/t1");
    expect(opts.method).toBe("PATCH");
  });

  it("getThreadSubscription returns subscription JSON", async () => {
    mockFetch.mockResolvedValue(jsonResponse(subscription));
    const sub = await getThreadSubscription("t1");
    expect(sub?.subscribed).toBe(true);
  });

  it("getThreadSubscription returns null on 404", async () => {
    mockFetch.mockResolvedValue(new Response(null, { status: 404 }));
    const sub = await getThreadSubscription("t1");
    expect(sub).toBeNull();
  });

  it("setThreadSubscription PUTs subscription state", async () => {
    mockFetch.mockResolvedValue(jsonResponse(subscription));
    await setThreadSubscription("t1", true);
    const [url, opts] = mockFetch.mock.calls[0];
    expect(url).toBe("/api/v3/notifications/threads/t1/subscription");
    expect(opts.method).toBe("PUT");
    expect(JSON.parse(opts.body as string)).toEqual({ subscribed: true, ignored: false });
  });

  it("deleteThreadSubscription DELETEs the subscription", async () => {
    mockFetch.mockResolvedValue(new Response(null, { status: 204 }));
    await deleteThreadSubscription("t1");
    const [url, opts] = mockFetch.mock.calls[0];
    expect(url).toBe("/api/v3/notifications/threads/t1/subscription");
    expect(opts.method).toBe("DELETE");
  });
});

// ─── Organizations and Teams GitHub REST helpers ────────────────────────

describe("Organization and team application programming interface helpers", () => {
  it("fetchOrgs lists organizations through GitHub REST and hydrates full organization records", async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse([{ id: 10, login: "acme", avatar_url: "", description: "short" }]))
      .mockResolvedValueOnce(jsonResponse({ id: 10, login: "acme", name: "Acme", description: "full", created_at: "2026-01-01T00:00:00Z" }));

    const orgs = await fetchOrgs();

    expect(orgs[0].name).toBe("Acme");
    expect(mockFetch.mock.calls.map((c) => c[0])).toEqual([
      "/api/v3/organizations?per_page=100",
      "/api/v3/orgs/acme",
    ]);
    expect(mockFetch.mock.calls.map((c) => c[0])).not.toContain("/internal/orgs");
  });

  it("createOrg uses the GitHub Enterprise Server admin organization endpoint", async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse({ id: 1, login: "admin", type: "User", site_admin: true, created_at: "2026-01-01T00:00:00Z" }))
      .mockResolvedValueOnce(jsonResponse({ id: 10, login: "acme", name: "Acme", created_at: "2026-01-01T00:00:00Z" }, 201))
      .mockResolvedValueOnce(jsonResponse({ id: 10, login: "acme", name: "Acme", description: "Real org", created_at: "2026-01-01T00:00:00Z" }));

    await createOrg({ login: "acme", name: "Acme", description: "Real org" });

    const calls = mockFetch.mock.calls;
    expect(calls[0][0]).toBe("/api/v3/user");
    expect(calls[1][0]).toBe("/api/v3/admin/organizations");
    expect(calls[1][1]).toMatchObject({ method: "POST" });
    expect(JSON.parse(calls[1][1].body as string)).toEqual({
      login: "acme",
      admin: "admin",
      profile_name: "Acme",
    });
    expect(calls[2][0]).toBe("/api/v3/orgs/acme");
    expect(calls[2][1]).toMatchObject({ method: "PATCH" });
    expect(calls.map((c) => c[0])).not.toContain("/internal/orgs");
  });

  it("updateOrg and deleteOrg use organization login on GitHub REST routes", async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse({ id: 10, login: "acme", name: "Acme", created_at: "2026-01-01T00:00:00Z" }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));

    await updateOrg("acme", { name: "Acme" });
    await deleteOrg("acme");

    expect(mockFetch.mock.calls[0][0]).toBe("/api/v3/orgs/acme");
    expect(mockFetch.mock.calls[0][1]).toMatchObject({ method: "PATCH" });
    expect(mockFetch.mock.calls[1][0]).toBe("/api/v3/orgs/acme");
    expect(mockFetch.mock.calls[1][1]).toMatchObject({ method: "DELETE" });
  });

  it("team helpers use authenticated-user and organization team REST routes", async () => {
    const team = {
      id: 20,
      slug: "core",
      name: "Core",
      description: "platform",
      privacy: "closed",
      organization: { id: 10, login: "acme" },
      created_at: "2026-01-01T00:00:00Z",
    };
    mockFetch
      .mockResolvedValueOnce(jsonResponse([team]))
      .mockResolvedValueOnce(jsonResponse(team, 201))
      .mockResolvedValueOnce(jsonResponse({ ...team, name: "Core Platform" }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));

    await fetchTeams();
    await createTeam({ org: "acme", name: "Core", description: "platform", privacy: "closed" });
    await updateTeam("acme", "core", { name: "Core Platform" });
    await deleteTeam("acme", "core");

    const calls = mockFetch.mock.calls;
    expect(calls[0][0]).toBe("/api/v3/user/teams?per_page=100");
    expect(calls[1][0]).toBe("/api/v3/orgs/acme/teams");
    expect(calls[1][1]).toMatchObject({ method: "POST" });
    expect(calls[2][0]).toBe("/api/v3/orgs/acme/teams/core");
    expect(calls[2][1]).toMatchObject({ method: "PATCH" });
    expect(calls[3][0]).toBe("/api/v3/orgs/acme/teams/core");
    expect(calls[3][1]).toMatchObject({ method: "DELETE" });
    expect(calls.map((c) => c[0])).not.toContain("/internal/teams");
  });
});

// ─── GitHub Enterprise Server user administration helpers ───────────────

describe("GitHub Enterprise Server user administration application programming interface helpers", () => {
  it("fetchUsers lists users through GitHub REST and hydrates full user records", async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse([{ id: 3, login: "dev", type: "User", site_admin: false }]))
      .mockResolvedValueOnce(jsonResponse({ id: 3, login: "dev", type: "User", site_admin: false, created_at: "2026-01-01T00:00:00Z" }));

    const users = await fetchUsers();

    expect(users[0].created_at).toBe("2026-01-01T00:00:00Z");
    expect(mockFetch.mock.calls.map((c) => c[0])).toEqual([
      "/api/v3/users?per_page=100",
      "/api/v3/users/dev",
    ]);
    expect(mockFetch.mock.calls.map((c) => c[0])).not.toContain("/internal/users");
  });

  it("createUser uses GitHub Enterprise Server admin user and site-admin routes", async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse({ id: 3, login: "dev", type: "User", site_admin: false }, 201))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(jsonResponse({ id: 3, login: "dev", type: "User", site_admin: true, created_at: "2026-01-01T00:00:00Z" }));

    await createUser({ login: "dev", email: "dev@example.com", site_admin: true });

    const calls = mockFetch.mock.calls;
    expect(calls[0][0]).toBe("/api/v3/admin/users");
    expect(calls[0][1]).toMatchObject({ method: "POST" });
    expect(JSON.parse(calls[0][1].body as string)).toEqual({ login: "dev", email: "dev@example.com" });
    expect(calls[1][0]).toBe("/api/v3/users/dev/site_admin");
    expect(calls[1][1]).toMatchObject({ method: "PUT" });
    expect(calls[2][0]).toBe("/api/v3/users/dev");
    expect(calls.map((c) => c[0])).not.toContain("/internal/users");
  });

  it("updateUser and deleteUser use GitHub Enterprise Server account routes", async () => {
    mockFetch
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(jsonResponse({ id: 3, login: "dev", type: "User", site_admin: false, created_at: "2026-01-01T00:00:00Z" }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));

    await updateUser("dev", { site_admin: false });
    await deleteUser("dev");

    expect(mockFetch.mock.calls[0][0]).toBe("/api/v3/users/dev/site_admin");
    expect(mockFetch.mock.calls[0][1]).toMatchObject({ method: "DELETE" });
    expect(mockFetch.mock.calls[1][0]).toBe("/api/v3/users/dev");
    expect(mockFetch.mock.calls[2][0]).toBe("/api/v3/admin/users/dev");
    expect(mockFetch.mock.calls[2][1]).toMatchObject({ method: "DELETE" });
  });
});

// ─── GitHub Enterprise Server organization audit log helpers ────────────

describe("GitHub Enterprise Server organization audit log application programming interface helpers", () => {
  it("fetchAuditLogOrgs lists organizations through the authenticated-user organizations route", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse([
        { id: 2, login: "zeta", name: "Zeta", description: "", created_at: "2026-01-01T00:00:00Z" },
        { id: 1, login: "acme", name: "Acme", description: "", created_at: "2026-01-01T00:00:00Z" },
      ]),
    );

    const orgs = await fetchAuditLogOrgs();

    expect(orgs.map((o) => o.login)).toEqual(["acme", "zeta"]);
    expect(mockFetch.mock.calls[0][0]).toBe("/api/v3/user/orgs?per_page=100");
  });

  it("fetchAuditLog uses the GitHub Enterprise Server organization audit-log route", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse([
        {
          _document_id: "123",
          "@timestamp": "2026-01-01T00:00:00Z",
          action: "repo.create",
          actor: "admin",
          org: "acme",
          data: { repo: "acme/demo" },
          version: "1.1",
        },
      ]),
    );

    const events = await fetchAuditLog({ org: "acme", phrase: "repo.create admin", order: "desc" });

    expect(events[0]).toMatchObject({
      id: 123,
      actor_login: "admin",
      action: "repo.create",
      entity_type: "acme",
      entity_id: "acme/demo",
      created_at: "2026-01-01T00:00:00Z",
    });
    const url = String(mockFetch.mock.calls[0][0]);
    expect(url).toContain("/api/v3/orgs/acme/audit-log?");
    expect(url).toContain("include=all");
    expect(url).toContain("per_page=100");
    expect(url).toContain("order=desc");
    expect(url).toContain("phrase=repo.create+admin");
    expect(url).not.toContain("/internal/audit-log");
  });

  it("buildAuditLogPhrase combines typed filters into GitHub's phrase query", () => {
    expect(buildAuditLogPhrase({ actor: "admin", action: "repo.create", text: "demo" })).toBe("admin repo.create demo");
  });
});

// ─── Gists Representational State Transfer ──────────────────────────────

describe("Gists application programming interface helpers", () => {
  const gist = {
    id: "g1",
    description: "hello",
    public: true,
    owner: { login: "admin", type: "User" },
    files: {},
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };

  const commit = {
    url: "/api/v3/gists/g1/abc123",
    version: "abc123",
    user: { login: "admin", type: "User" },
    change_status: { additions: 1, deletions: 0, total: 1 },
    committed_at: "2026-01-01T00:00:00Z",
  };

  it("fetchPublicGists hits the public endpoint", async () => {
    mockFetch.mockResolvedValue(jsonResponse([gist]));
    const gists = await fetchPublicGists();
    expect(mockFetch.mock.calls[0][0]).toBe("/api/v3/gists/public");
    expect(gists).toHaveLength(1);
  });

  it("fetchStarredGists hits the starred endpoint", async () => {
    mockFetch.mockResolvedValue(jsonResponse([gist]));
    const gists = await fetchStarredGists();
    expect(mockFetch.mock.calls[0][0]).toBe("/api/v3/gists/starred");
    expect(gists).toHaveLength(1);
  });

  it("fetchGistCommits lists commits", async () => {
    mockFetch.mockResolvedValue(jsonResponse([commit]));
    const commits = await fetchGistCommits("g1");
    expect(mockFetch.mock.calls[0][0]).toBe("/api/v3/gists/g1/commits");
    expect(commits[0].version).toBe("abc123");
  });

  it("fetchGistForks lists forks", async () => {
    mockFetch.mockResolvedValue(jsonResponse([gist]));
    const forks = await fetchGistForks("g1");
    expect(mockFetch.mock.calls[0][0]).toBe("/api/v3/gists/g1/forks");
    expect(forks).toHaveLength(1);
  });

  it("forkGist POSTs to forks", async () => {
    mockFetch.mockResolvedValue(jsonResponse(gist, 201));
    await forkGist("g1");
    const [url, opts] = mockFetch.mock.calls[0];
    expect(url).toBe("/api/v3/gists/g1/forks");
    expect(opts.method).toBe("POST");
  });

  it("isGistStarred returns true on 204", async () => {
    mockFetch.mockResolvedValue(new Response(null, { status: 204 }));
    expect(await isGistStarred("g1")).toBe(true);
  });

  it("isGistStarred returns false on 404", async () => {
    mockFetch.mockResolvedValue(new Response(null, { status: 404 }));
    expect(await isGistStarred("g1")).toBe(false);
  });

  it("starGist PUTs the star endpoint", async () => {
    mockFetch.mockResolvedValue(new Response(null, { status: 204 }));
    await starGist("g1");
    const [url, opts] = mockFetch.mock.calls[0];
    expect(url).toBe("/api/v3/gists/g1/star");
    expect(opts.method).toBe("PUT");
  });

  it("unstarGist DELETEs the star endpoint", async () => {
    mockFetch.mockResolvedValue(new Response(null, { status: 204 }));
    await unstarGist("g1");
    const [url, opts] = mockFetch.mock.calls[0];
    expect(url).toBe("/api/v3/gists/g1/star");
    expect(opts.method).toBe("DELETE");
  });
});
