import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { WorkflowDetailPage } from "../pages/WorkflowDetailPage.js";

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
      <MemoryRouter initialEntries={["/ui/workflows/wf-1"]}>
        <Routes>
          <Route path="/ui/workflows/:id" element={<WorkflowDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const workflowData = {
  id: "wf-1",
  name: "CI Build",
  runId: 42,
  status: "completed",
  result: "success",
  createdAt: "2026-01-01T00:00:00Z",
  eventName: "push",
  repoFullName: "admin/test",
  jobs: {
    build: { key: "build", jobId: "j1", displayName: "Build", status: "completed", result: "success", needs: [] },
    test: { key: "test", jobId: "j2", displayName: "Test", status: "completed", result: "success", needs: ["build"] },
  },
};

describe("WorkflowDetailPage", () => {
  it("renders workflow name and details", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/logs")) return Promise.resolve(jsonResponse({}));
      return Promise.resolve(jsonResponse(workflowData));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("CI Build")).toBeInTheDocument();
      expect(screen.getByText("Run #42")).toBeInTheDocument();
    });
  });

  it("renders job table with both jobs", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/logs")) return Promise.resolve(jsonResponse({}));
      return Promise.resolve(jsonResponse(workflowData));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Jobs (2)")).toBeInTheDocument();
      expect(screen.getByText("Build")).toBeInTheDocument();
      expect(screen.getByText("Test")).toBeInTheDocument();
    });
  });
});
