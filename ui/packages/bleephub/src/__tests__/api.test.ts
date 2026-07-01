import { describe, it, expect, vi, afterEach } from "vitest";
import {
  fetchOAuthApps,
  fetchSecrets,
  fetchEnvironments,
  fetchRepoIssuesPage,
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
  // BUG-1597: server emits snake_case (client_id/callback_url/created_at);
  // the UI reads camelCase. fetchOAuthApps must bridge that.
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

// ─── GitHub Codespaces REST ─────────────────────────────────────────────

describe("Codespaces API helpers", () => {
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
