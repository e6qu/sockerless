import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { OverviewPage } from "../pages/OverviewPage.js";

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
        <OverviewPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const healthData = { status: "ok", service: "bleephub" };
const metricsData = {
  workflow_submissions: 10,
  job_dispatches: 25,
  job_completions: { success: 20, failure: 5 },
  active_workflows: 2,
  active_sessions: 3,
  uptime_seconds: 3600,
  goroutines: 42,
  heap_alloc_mb: 12.5,
};
const workflowsData = [
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
];
const reposData = [{ id: 1, name: "test", full_name: "admin/test", default_branch: "main", owner: { login: "admin", type: "User" } }];
const jobsData = [{
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
}];

function mockAllEndpoints() {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/health")) return Promise.resolve(jsonResponse(healthData));
    if (url.includes("/internal/metrics")) return Promise.resolve(jsonResponse(metricsData));
    if (url.includes("/api/v3/user/repos")) return Promise.resolve(jsonResponse(reposData));
    if (url.includes("/actions/runs/1/jobs")) return Promise.resolve(jsonResponse({ total_count: 1, jobs: jobsData }));
    if (url.includes("/actions/runs")) return Promise.resolve(jsonResponse({ total_count: 1, workflow_runs: workflowsData }));
    return Promise.resolve(jsonResponse({}));
  });
}

describe("OverviewPage", () => {
  it("renders the overview heading", async () => {
    mockAllEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /system status/i })).toBeInTheDocument();
    });
  });

  it("renders health badge", async () => {
    mockAllEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("ok")).toBeInTheDocument();
    });
  });

  it("renders metrics cards", async () => {
    mockAllEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/active workflows/i)).toBeInTheDocument();
      expect(screen.getByText("2")).toBeInTheDocument();
      expect(screen.getByText(/connected runners/i)).toBeInTheDocument();
      expect(screen.getByText("3")).toBeInTheDocument();
      expect(screen.getByText("Submissions")).toBeInTheDocument();
      expect(screen.getByText("10")).toBeInTheDocument();
    });
  });

  it("renders recent workflows table", async () => {
    mockAllEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/recent workflows/i)).toBeInTheDocument();
      expect(screen.getByText("CI Build")).toBeInTheDocument();
    });
  });
});
