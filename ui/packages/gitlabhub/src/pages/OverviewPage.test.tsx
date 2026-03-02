import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { OverviewPage } from "./OverviewPage.js";

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

const healthData = { status: "ok", service: "gitlabhub" };
const metricsData = {
  pipeline_submissions: 10,
  job_dispatches: 25,
  job_completions: { success: 20, failure: 5 },
  active_pipelines: 2,
  registered_runners: 3,
  uptime_seconds: 3600,
  goroutines: 42,
  heap_alloc_mb: 12.5,
};
const pipelinesData = [
  {
    id: 1,
    project_id: 1,
    project_name: "my-project",
    status: "success",
    result: "success",
    ref: "main",
    sha: "abc123",
    stages: ["build", "test"],
    jobs: { build: { id: 1, name: "build", stage: "build", status: "success" } },
    created_at: "2026-01-01T00:00:00Z",
  },
];

function mockAllEndpoints() {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/health")) return Promise.resolve(jsonResponse(healthData));
    if (url.includes("/internal/metrics")) return Promise.resolve(jsonResponse(metricsData));
    if (url.includes("/internal/pipelines")) return Promise.resolve(jsonResponse(pipelinesData));
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
      expect(screen.getByText("Active Pipelines")).toBeInTheDocument();
      expect(screen.getByText("2")).toBeInTheDocument();
      expect(screen.getByText("Registered Runners")).toBeInTheDocument();
      expect(screen.getByText("3")).toBeInTheDocument();
      expect(screen.getByText("Submissions")).toBeInTheDocument();
      expect(screen.getByText("10")).toBeInTheDocument();
    });
  });

  it("renders recent pipelines table", async () => {
    mockAllEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Recent Pipelines")).toBeInTheDocument();
      expect(screen.getByText("my-project")).toBeInTheDocument();
    });
  });
});
