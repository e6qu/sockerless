import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { MetricsPage } from "./MetricsPage.js";

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

const metricsData = {
  pipeline_submissions: 15,
  job_dispatches: 30,
  job_completions: { success: 25, failed: 5 },
  active_pipelines: 3,
  registered_runners: 4,
  uptime_seconds: 7200,
  goroutines: 50,
  heap_alloc_mb: 15.3,
};

const statusData = {
  active_pipelines: 3,
  jobs_by_status: { running: 2, pending: 1, success: 10 },
  registered_runners: 4,
  uptime_seconds: 7200,
};

function mockAllEndpoints() {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/internal/metrics")) return Promise.resolve(jsonResponse(metricsData));
    if (url.includes("/internal/status")) return Promise.resolve(jsonResponse(statusData));
    return Promise.resolve(jsonResponse({}));
  });
}

describe("MetricsPage", () => {
  it("renders metrics cards", async () => {
    mockAllEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Pipeline Submissions")).toBeInTheDocument();
      expect(screen.getByText("15")).toBeInTheDocument();
      expect(screen.getByText("Job Dispatches")).toBeInTheDocument();
      expect(screen.getByText("30")).toBeInTheDocument();
    });
  });

  it("renders job completions", async () => {
    mockAllEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Job Completions")).toBeInTheDocument();
      expect(screen.getByText("25")).toBeInTheDocument();
      expect(screen.getByText("failed")).toBeInTheDocument();
    });
  });

  it("renders jobs by status", async () => {
    mockAllEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Jobs by Status")).toBeInTheDocument();
      expect(screen.getByText("running")).toBeInTheDocument();
    });
  });
});
