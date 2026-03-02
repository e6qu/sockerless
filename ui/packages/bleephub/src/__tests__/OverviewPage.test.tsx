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
    id: "wf-1",
    name: "CI Build",
    runId: 1,
    status: "completed",
    result: "success",
    createdAt: "2026-01-01T00:00:00Z",
    eventName: "push",
    repoFullName: "admin/test",
    jobs: { build: { key: "build", jobId: "j1", displayName: "Build", status: "completed", result: "success" } },
  },
];

function mockAllEndpoints() {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/health")) return Promise.resolve(jsonResponse(healthData));
    if (url.includes("/internal/metrics")) return Promise.resolve(jsonResponse(metricsData));
    if (url.includes("/internal/workflows")) return Promise.resolve(jsonResponse(workflowsData));
    return Promise.resolve(jsonResponse({}));
  });
}

describe("OverviewPage", () => {
  it("renders the overview heading", async () => {
    mockAllEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Overview")).toBeInTheDocument();
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
      expect(screen.getByText("Active Workflows")).toBeInTheDocument();
      expect(screen.getByText("2")).toBeInTheDocument();
      expect(screen.getByText("Connected Runners")).toBeInTheDocument();
      expect(screen.getByText("3")).toBeInTheDocument();
      expect(screen.getByText("Submissions")).toBeInTheDocument();
      expect(screen.getByText("10")).toBeInTheDocument();
    });
  });

  it("renders recent workflows table", async () => {
    mockAllEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Recent Workflows")).toBeInTheDocument();
      expect(screen.getByText("CI Build")).toBeInTheDocument();
    });
  });
});
