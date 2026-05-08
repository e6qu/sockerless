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

const metricsData = {
  workflow_submissions: 15,
  job_dispatches: 30,
  job_completions: { success: 25, failure: 5 },
  active_workflows: 1,
  active_sessions: 2,
  uptime_seconds: 7200,
  goroutines: 50,
  heap_alloc_mb: 18.3,
};

const statusData = {
  active_workflows: 1,
  jobs_by_status: { completed: 20, running: 3, pending: 5 },
  connected_runners: 2,
  uptime_seconds: 7200,
};

function mockEndpoints() {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/internal/status")) return Promise.resolve(jsonResponse(statusData));
    if (url.includes("/internal/metrics")) return Promise.resolve(jsonResponse(metricsData));
    return Promise.resolve(jsonResponse({}));
  });
}

describe("MetricsPage", () => {
  it("renders the metrics heading", async () => {
    mockEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /runtime/i })).toBeInTheDocument();
    });
  });

  it("renders metrics cards", async () => {
    mockEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/workflow submissions/i)).toBeInTheDocument();
      expect(screen.getByText("15")).toBeInTheDocument();
      expect(screen.getByText(/job dispatches/i)).toBeInTheDocument();
      expect(screen.getByText("30")).toBeInTheDocument();
      // Goroutines + heap moved into the PageHeading meta line in the
      // redesign — they appear as part of the meta string, not as
      // standalone MetricsCard titles.
      expect(screen.getByText(/50 goroutines/i)).toBeInTheDocument();
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
});
