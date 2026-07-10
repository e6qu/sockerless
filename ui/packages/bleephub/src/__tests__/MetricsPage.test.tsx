import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { MetricsPage } from "../pages/MetricsPage.js";

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
        <MetricsPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const reposData = [{ id: 1, name: "test", full_name: "admin/test", default_branch: "main", owner: { login: "admin", type: "User" } }];
const workflowRunsData = [
  {
    id: 1,
    name: "CI Build",
    run_number: 1,
    run_attempt: 1,
    event: "push",
    status: "completed",
    conclusion: "success",
    head_branch: "main",
    head_sha: "abc",
    path: ".github/workflows/ci.yml",
    workflow_id: 1234,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    actor: { login: "admin" },
  },
  {
    id: 2,
    name: "Deploy",
    run_number: 2,
    run_attempt: 1,
    event: "workflow_dispatch",
    status: "in_progress",
    conclusion: null,
    head_branch: "main",
    head_sha: "def",
    path: ".github/workflows/deploy.yml",
    workflow_id: 1235,
    created_at: "2026-01-01T01:00:00Z",
    updated_at: "2026-01-01T01:00:00Z",
    actor: { login: "admin" },
  },
];
const jobsByRun: Record<string, unknown[]> = {
  "1": [
    {
      id: 101,
      run_id: 1,
      name: "build",
      status: "completed",
      conclusion: "success",
      started_at: "2026-01-01T00:00:01Z",
      completed_at: "2026-01-01T00:00:02Z",
      steps: [],
      labels: ["self-hosted"],
      run_attempt: 1,
    },
  ],
  "2": [
    {
      id: 201,
      run_id: 2,
      name: "deploy",
      status: "in_progress",
      conclusion: null,
      started_at: "2026-01-01T01:00:01Z",
      completed_at: null,
      steps: [],
      labels: ["self-hosted"],
      run_attempt: 1,
    },
  ],
};
const runnersData = [
  { id: 501, name: "runner-1", os: "linux", status: "online", busy: false, labels: [] },
  { id: 502, name: "runner-2", os: "linux", status: "offline", busy: false, labels: [] },
];

function mockEndpoints() {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/api/v3/user/repos")) return Promise.resolve(jsonResponse(reposData));
    const jobsMatch = url.match(/\/actions\/runs\/(\d+)\/jobs/);
    if (jobsMatch) {
      const jobs = jobsByRun[jobsMatch[1]] ?? [];
      return Promise.resolve(jsonResponse({ total_count: jobs.length, jobs }));
    }
    if (url.includes("/actions/runners")) return Promise.resolve(jsonResponse({ total_count: 2, runners: runnersData }));
    if (url.includes("/actions/runs")) return Promise.resolve(jsonResponse({ total_count: 2, workflow_runs: workflowRunsData }));
    return Promise.resolve(jsonResponse({}));
  });
}

describe("MetricsPage", () => {
  it("renders the metrics heading", async () => {
    mockEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /actions throughput/i })).toBeInTheDocument();
    });
  });

  it("renders metrics cards", async () => {
    mockEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getAllByText(/workflow runs/i).length).toBeGreaterThan(0);
      expect(screen.getAllByText("2").length).toBeGreaterThan(0);
      expect(screen.getByText(/job dispatches/i)).toBeInTheDocument();
      expect(screen.getAllByText(/connected runners/i).length).toBeGreaterThan(0);
      expect(screen.getByText(/2 workflow runs/i)).toBeInTheDocument();
    });
  });

  it("renders job completions breakdown", async () => {
    mockEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/job completions/i)).toBeInTheDocument();
    });
  });

  it("renders jobs by status section", async () => {
    mockEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/jobs by status/i)).toBeInTheDocument();
    });
  });

  it("does not call operator-only internal diagnostics endpoints", async () => {
    mockEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /actions throughput/i })).toBeInTheDocument();
    });
    const urls = mockFetch.mock.calls.map((call) => String(call[0]));
    expect(urls.some((url) => url.includes("/internal/metrics"))).toBe(false);
    expect(urls.some((url) => url.includes("/internal/status"))).toBe(false);
    expect(urls.some((url) => url.includes("/internal/storage"))).toBe(false);
  });
});
