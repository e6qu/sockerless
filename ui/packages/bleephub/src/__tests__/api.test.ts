import { describe, it, expect, vi, afterEach } from "vitest";
import {
  fetchOAuthApps,
  fetchSecrets,
  fetchEnvironments,
  dispatchWorkflow,
  setToken,
  clearToken,
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
