import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { PipelineDetailPage } from "./PipelineDetailPage.js";

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
      <MemoryRouter initialEntries={["/ui/pipelines/1"]}>
        <Routes>
          <Route path="/ui/pipelines/:id" element={<PipelineDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const pipelineData = {
  id: 1,
  project_id: 1,
  project_name: "my-project",
  status: "success",
  result: "success",
  ref: "main",
  sha: "abc12345",
  stages: ["build", "test"],
  jobs: {
    compile: { id: 1, name: "compile", stage: "build", status: "success", result: "success", allow_failure: false },
    unit: { id: 2, name: "unit", stage: "test", status: "success", result: "success", allow_failure: false },
  },
  created_at: "2026-01-01T00:00:00Z",
};

const logsData = {
  compile: ["Building...", "Done."],
  unit: ["Running tests...", "All passed."],
};

function mockAllEndpoints() {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes("/logs")) return Promise.resolve(jsonResponse(logsData));
    if (url.includes("/internal/pipelines/")) return Promise.resolve(jsonResponse(pipelineData));
    return Promise.resolve(jsonResponse({}));
  });
}

describe("PipelineDetailPage", () => {
  it("renders pipeline header", async () => {
    mockAllEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Pipeline #1")).toBeInTheDocument();
      expect(screen.getByText("my-project")).toBeInTheDocument();
    });
  });

  it("renders stage grouping", async () => {
    mockAllEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Stages")).toBeInTheDocument();
      expect(screen.getByText("build")).toBeInTheDocument();
      expect(screen.getByText("test")).toBeInTheDocument();
    });
  });

  it("renders log viewer", async () => {
    mockAllEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Logs")).toBeInTheDocument();
      expect(screen.getAllByText("compile").length).toBeGreaterThanOrEqual(2);
    });
  });
});
