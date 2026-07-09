import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { RunnersPage } from "../pages/RunnersPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown) {
  return new Response(JSON.stringify(data), {
    status: 200,
    headers: { "Content-Type": "application/json" },
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
      <BrowserRouter>
        <RunnersPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const sessionsData = [
  {
    sessionId: "sess-1",
    ownerName: "runner1",
    agent: {
      id: 1,
      name: "my-runner",
      version: "2.320.0",
      enabled: true,
      status: "online",
      osDescription: "Linux",
      labels: [{ id: 1, name: "self-hosted", type: "system" }],
      ephemeral: false,
      createdOn: "2026-01-01T00:00:00Z",
    },
    pendingMessages: 2,
  },
];

const reposData = [
  {
    id: 1,
    name: "test",
    full_name: "admin/test",
    description: "",
    default_branch: "main",
    visibility: "public",
    private: false,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
];

const runnersData = {
  total_count: 1,
  runners: [
    {
      id: 7,
      name: "gh-runner-7",
      os: "linux",
      status: "online",
      busy: true,
      labels: [
        { id: 1, name: "self-hosted", type: "read-only" },
        { id: 2, name: "linux", type: "read-only" },
        { id: 3, name: "gpu", type: "custom" },
      ],
    },
  ],
};

/** URL-routed mock — each call gets a fresh Response (bodies are single-read). */
function installMocks() {
  mockFetch.mockImplementation((url: RequestInfo | URL) => {
    const u = url.toString();
    if (u.includes("/internal/sessions")) return Promise.resolve(jsonResponse(sessionsData));
    if (u === "/api/v3/user/repos?per_page=100") return Promise.resolve(jsonResponse(reposData));
    if (u.includes("/actions/runners")) return Promise.resolve(jsonResponse(runnersData));
    return Promise.resolve(jsonResponse([]));
  });
}

describe("RunnersPage", () => {
  it("renders the runners heading", async () => {
    installMocks();
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /connected runners/i })).toBeInTheDocument();
    });
  });

  it("renders agent table with agent name", async () => {
    installMocks();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("my-runner")).toBeInTheDocument();
    });
  });

  it("shows pending messages count", async () => {
    installMocks();
    renderPage();
    await waitFor(() => {
      expect(screen.getAllByText(/pending messages/i).length).toBeGreaterThan(0);
      expect(screen.getByText("2")).toBeInTheDocument();
    });
  });

  it("lists registered runners from the GitHub Representational State Transfer endpoint with label pills and busy state", async () => {
    installMocks();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("gh-runner-7")).toBeInTheDocument();
    });
    // "self-hosted" also appears in the sessions table's label column —
    // assert the registry-only label ("gpu") uniquely and the rest loosely.
    expect(screen.getAllByText("self-hosted").length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText("gpu")).toBeInTheDocument();
    expect(screen.getByText("busy")).toBeInTheDocument();
    expect(screen.getByLabelText("online")).toBeInTheDocument();
    const calls = mockFetch.mock.calls.map((c) => c[0].toString());
    expect(calls).toContain("/api/v3/user/repos?per_page=100");
    expect(calls).toContain("/api/v3/repos/admin/test/actions/runners");
    expect(calls).not.toContain("/internal/repos");
  });
});
