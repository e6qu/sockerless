import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { WorkflowsPage } from "../pages/WorkflowsPage.js";

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
        <WorkflowsPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const workflowsData = [
  {
    id: "wf-1",
    name: "CI Build",
    runId: 1,
    status: "running",
    result: "",
    createdAt: "2026-01-01T00:00:00Z",
    eventName: "push",
    repoFullName: "admin/test",
    jobs: {
      build: { key: "build", jobId: "j1", displayName: "Build", status: "running", result: "" },
      test: { key: "test", jobId: "j2", displayName: "Test", status: "pending", result: "" },
    },
  },
];

describe("WorkflowsPage", () => {
  it("renders the workflows heading", async () => {
    mockFetch.mockResolvedValue(jsonResponse(workflowsData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Workflows (1)")).toBeInTheDocument();
    });
  });

  it("renders DataTable columns", async () => {
    mockFetch.mockResolvedValue(jsonResponse(workflowsData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Name")).toBeInTheDocument();
      expect(screen.getByText("Status")).toBeInTheDocument();
      expect(screen.getByText("Result")).toBeInTheDocument();
      expect(screen.getByText("Event")).toBeInTheDocument();
    });
  });

  it("renders workflow status badges", async () => {
    mockFetch.mockResolvedValue(jsonResponse(workflowsData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("running")).toBeInTheDocument();
    });
  });
});
