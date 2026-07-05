// Property/fuzz-style hardening for the api.ts envelope parsers and the
// base64 content decoder. Every parser must either produce a correct value
// or throw a clear Error — never silently return a wrong-but-plausible value,
// and never throw a non-Error that a component could not catch as a query
// failure. The malformed/adversarial payloads here stand in for a
// contract-breaking or hostile server.
import { describe, it, expect, vi, afterEach } from "vitest";
import {
  parseLinkNext,
  parseLinkLast,
  fetchActionsWorkflows,
  fetchRunJobs,
  fetchRepoIssuesPage,
  fetchUserReposPage,
  fetchSecrets,
  fetchEnvironments,
  fetchEnvironmentsDetail,
  fetchEnvBranchPolicies,
  fetchEnvProtectionRules,
  fetchCopilotSeats,
  fetchCopilotSpaces,
  fetchCombinedStatus,
  fetchPRRequestedReviewers,
  searchRepositories,
  fetchDeploymentsPage,
  clearToken,
} from "../api.js";
import { decodeContentsBase64 } from "../utils/workflowDispatch.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

afterEach(() => {
  mockFetch.mockReset();
  clearToken();
});

function jsonResponse(data: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

/** A Response whose body is not valid JSON (truncated / non-JSON). */
function rawResponse(body: string, status = 200) {
  return new Response(body, {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// ─── Link-header parsers ─────────────────────────────────────────────────

describe("parseLinkNext / parseLinkLast — adversarial Link headers", () => {
  it("extracts next + last from a well-formed multi-rel header", () => {
    const link =
      `</api/v3/x?page=2&per_page=30>; rel="next", ` +
      `</api/v3/x?page=9&per_page=30>; rel="last"`;
    expect(parseLinkNext(link)).toBe("/api/v3/x?page=2&per_page=30");
    expect(parseLinkLast(link)).toBe(9);
  });

  it("returns null (never throws) for null / empty / garbage headers", () => {
    const garbage = [
      null,
      "",
      "   ",
      "not a link header at all",
      "<>; rel=",
      '<>; rel="next"', // empty url between brackets — [^>]+ requires >=1 char
      "<no-rel-here>",
      '<u>; rel="prev", <v>; rel="first"',
      '<u>; rel="nextish"', // must be exactly next
      "<".repeat(5000), // long, unterminated — must not hang or throw
      ">".repeat(5000),
      'page=2>; rel="last"'.repeat(200),
    ];
    for (const g of garbage) {
      const next = parseLinkNext(g);
      const last = parseLinkLast(g);
      expect(next === null || typeof next === "string").toBe(true);
      expect(last === null || (typeof last === "number" && Number.isFinite(last))).toBe(true);
    }
    // The exactly-empty-url case must be null, not "".
    expect(parseLinkNext('<>; rel="next"')).toBeNull();
    expect(parseLinkNext(null)).toBeNull();
    expect(parseLinkLast("<u>; rel=\"prev\"")).toBeNull();
  });

  it("parseLinkLast ignores a negative or non-numeric page and returns null", () => {
    expect(parseLinkLast('</x?page=-5>; rel="last"')).toBeNull();
    expect(parseLinkLast('</x?page=abc>; rel="last"')).toBeNull();
    // rel=last present but no page param at all
    expect(parseLinkLast('</x?per_page=30>; rel="last"')).toBeNull();
  });

  it("parseLinkLast reads a very large page number as a finite number", () => {
    const n = parseLinkLast('</x?page=999999999>; rel="last"');
    expect(n).toBe(999999999);
    // Huge (beyond Number range) still parses to a finite (non-NaN) number.
    const big = parseLinkLast('</x?page=999999999999999999999999>; rel="last"');
    expect(typeof big).toBe("number");
    expect(Number.isNaN(big)).toBe(false);
  });

  it("random fuzz: never hangs, never throws, result types are sound", () => {
    const chars = '<>;,="?& pagerelnextlastprevfirst0123456789/api';
    let seed = 1234567;
    const rand = () => {
      seed = (seed * 1103515245 + 12345) & 0x7fffffff;
      return seed / 0x7fffffff;
    };
    for (let i = 0; i < 2000; i++) {
      const len = Math.floor(rand() * 120);
      let s = "";
      for (let j = 0; j < len; j++) s += chars[Math.floor(rand() * chars.length)];
      const next = parseLinkNext(s);
      const last = parseLinkLast(s);
      expect(next === null || typeof next === "string").toBe(true);
      expect(last === null || (typeof last === "number" && Number.isFinite(last))).toBe(true);
    }
  });
});

// ─── ghFetchEnvelope — {total_count, <key>: [...]} ───────────────────────

describe("ghFetchEnvelope — malformed envelopes throw, valid ones parse", () => {
  it("parses a well-formed envelope and reads total_count + next", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse(
        { total_count: 2, workflows: [{ id: 1 }, { id: 2 }] },
        200,
        { Link: '</api/v3/x?page=2>; rel="next"' },
      ),
    );
    const page = await fetchActionsWorkflows("a", "b");
    expect(page.items).toHaveLength(2);
    expect(page.totalCount).toBe(2);
    expect(page.nextUrl).toBe("/api/v3/x?page=2");
  });

  it("throws when the keyed array is missing", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ total_count: 0 }));
    await expect(fetchActionsWorkflows("a", "b")).rejects.toThrow(/missing "workflows" array/);
  });

  it("throws when the keyed member is the wrong type (object, string, null)", async () => {
    for (const bad of [{ workflows: {} }, { workflows: "x" }, { workflows: null }, { workflows: 5 }]) {
      mockFetch.mockResolvedValue(jsonResponse(bad));
      await expect(fetchActionsWorkflows("a", "b")).rejects.toThrow(/missing "workflows" array/);
    }
  });

  it("throws an Error (catchable as a query failure) on truncated JSON", async () => {
    mockFetch.mockResolvedValue(rawResponse('{"total_count": 1, "jobs": [')); // truncated
    await expect(fetchRunJobs("a", "b", 1)).rejects.toBeInstanceOf(Error);
  });

  it("surfaces an ApiError (with status) on a non-2xx response", async () => {
    mockFetch.mockResolvedValue(new Response("nope", { status: 500 }));
    await expect(fetchRunJobs("a", "b", 1)).rejects.toMatchObject({ status: 500 });
  });

  it("does not confuse a deeply nested wrong shape for the array", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({ total_count: 1, data: { workflows: [{ id: 1 }] } }),
    );
    await expect(fetchActionsWorkflows("a", "b")).rejects.toThrow(/missing "workflows" array/);
  });
});

// ─── ghFetchPage — bare JSON array + Link header ─────────────────────────

describe("ghFetchPage — a non-array body is a contract break, not empty", () => {
  it("parses a bare array and its Link header", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse([{ id: 1 }], 200, { Link: '</x?page=3>; rel="last"' }),
    );
    const page = await fetchRepoIssuesPage("a", "b", "open");
    expect(page.items).toHaveLength(1);
    expect(page.lastPage).toBe(3);
  });

  it("throws when the body is an object instead of an array", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ items: [{ id: 1 }] }));
    await expect(fetchRepoIssuesPage("a", "b", "open")).rejects.toThrow(/expected a JSON array/);
  });

  it("throws for string / number / null bodies", async () => {
    for (const bad of ["oops", 42, null]) {
      mockFetch.mockResolvedValue(jsonResponse(bad));
      await expect(fetchUserReposPage()).rejects.toThrow(/expected a JSON array/);
    }
  });

  it("throws an Error on truncated JSON", async () => {
    mockFetch.mockResolvedValue(rawResponse("[{ bad"));
    await expect(fetchDeploymentsPage("a", "b")).rejects.toBeInstanceOf(Error);
  });
});

// ─── Strict list unwrappers (no `?? []`) ─────────────────────────────────

describe("strict list unwrappers throw on a missing/mistyped array", () => {
  const cases: Array<{
    name: string;
    call: () => Promise<unknown>;
    good: unknown;
    bad: unknown;
    err: RegExp;
  }> = [
    {
      name: "fetchSecrets",
      call: () => fetchSecrets("a", "b"),
      good: { total_count: 1, secrets: [{ name: "S" }] },
      bad: { total_count: 1 },
      err: /missing "secrets" array/,
    },
    {
      name: "fetchEnvironments",
      call: () => fetchEnvironments("a", "b"),
      good: { environments: [{ name: "prod" }] },
      bad: {},
      err: /missing "environments" array/,
    },
    {
      name: "fetchEnvironmentsDetail",
      call: () => fetchEnvironmentsDetail("a", "b"),
      good: { environments: [{ name: "prod" }] },
      bad: { environments: {} },
      err: /missing "environments" array/,
    },
    {
      name: "fetchEnvBranchPolicies",
      call: () => fetchEnvBranchPolicies("a", "b", "prod"),
      good: { total_count: 0, branch_policies: [] },
      bad: { total_count: 0 },
      err: /missing "branch_policies" array/,
    },
    {
      name: "fetchEnvProtectionRules",
      call: () => fetchEnvProtectionRules("a", "b", "prod"),
      good: { total_count: 0, custom_deployment_protection_rules: [] },
      bad: { total_count: 0 },
      err: /missing "custom_deployment_protection_rules" array/,
    },
    {
      name: "fetchCopilotSeats",
      call: () => fetchCopilotSeats("org"),
      good: { total_seats: 1, seats: [{ assignee: { login: "x" } }] },
      bad: { total_seats: 1 },
      err: /missing "seats" array/,
    },
    {
      name: "fetchCopilotSpaces",
      call: () => fetchCopilotSpaces("org"),
      good: { spaces: [{ id: 1 }] },
      bad: { spaces: "nope" },
      err: /missing "spaces" array/,
    },
    {
      name: "fetchCombinedStatus",
      call: () => fetchCombinedStatus("a", "b", "sha"),
      good: { state: "success", statuses: [] },
      bad: { state: "success" },
      err: /missing "statuses" array/,
    },
    {
      name: "fetchPRRequestedReviewers",
      call: () => fetchPRRequestedReviewers("a", "b", 1),
      good: { users: [], teams: [] },
      bad: { users: [] },
      err: /missing "users"\/"teams" arrays/,
    },
    {
      name: "searchRepositories",
      call: () => searchRepositories("q"),
      good: { total_count: 0, incomplete_results: false, items: [] },
      bad: { total_count: 0, incomplete_results: false },
      err: /missing "items" array/,
    },
  ];

  for (const c of cases) {
    it(`${c.name} parses a valid envelope`, async () => {
      mockFetch.mockResolvedValue(jsonResponse(c.good));
      await expect(c.call()).resolves.toBeDefined();
    });
    it(`${c.name} rejects a missing/mistyped array (never silent empty)`, async () => {
      mockFetch.mockResolvedValue(jsonResponse(c.bad));
      await expect(c.call()).rejects.toThrow(c.err);
    });
  }
});

// ─── base64 content decoder ──────────────────────────────────────────────

describe("decodeContentsBase64 — never silently returns wrong text", () => {
  it("decodes valid base64 (with GitHub's embedded newlines) to UTF-8", () => {
    // "héllo 🌍" as UTF-8 base64, chunked with a newline like GitHub sends.
    const b64 = btoa(
      Array.from(new TextEncoder().encode("héllo 🌍"), (b) => String.fromCharCode(b)).join(""),
    );
    const chunked = b64.slice(0, 4) + "\n" + b64.slice(4);
    expect(decodeContentsBase64(chunked)).toBe("héllo 🌍");
  });

  it("decodes empty content to empty string", () => {
    expect(decodeContentsBase64("")).toBe("");
  });

  it("throws on structurally invalid base64 rather than returning garbage", () => {
    // A single stray '!' is not a base64 alphabet char → atob throws.
    expect(() => decodeContentsBase64("not_base64_!@#$")).toThrow();
  });

  it("round-trips arbitrary ASCII payloads (property check)", () => {
    let seed = 99;
    const rand = () => {
      seed = (seed * 1103515245 + 12345) & 0x7fffffff;
      return seed / 0x7fffffff;
    };
    for (let i = 0; i < 300; i++) {
      const len = Math.floor(rand() * 64);
      let s = "";
      for (let j = 0; j < len; j++) s += String.fromCharCode(32 + Math.floor(rand() * 94));
      const b64 = btoa(s);
      expect(decodeContentsBase64(b64)).toBe(s);
    }
  });
});
