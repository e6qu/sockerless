import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
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

// Phase 131 — UI now has two tabs:
// - Workflows (files) — backed by GET /internal/workflow_files
// - Runs           — backed by GET /internal/workflows
const workflowFilesData = [
  {
    id: 1234,
    name: "CI Build",
    path: ".github/workflows/ci.yml",
    state: "active",
    repoFullName: "admin/test",
    source: "submitted",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  },
];

const workflowRunsData = [
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
    },
  },
];

// Route the mocked fetch by URL so the two tabs see distinct payloads.
function routedFetch(url: RequestInfo | URL): Promise<Response> {
  const u = typeof url === "string" ? url : url.toString();
  if (u.includes("/internal/workflow_files")) return Promise.resolve(jsonResponse(workflowFilesData));
  if (u.includes("/internal/workflows")) return Promise.resolve(jsonResponse(workflowRunsData));
  return Promise.resolve(jsonResponse([]));
}

describe("WorkflowsPage", () => {
  it("renders both tabs", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Workflows (files)")).toBeInTheDocument();
      expect(screen.getByText("Runs")).toBeInTheDocument();
    });
  });

  it("Workflows tab renders the file listing with a Run-workflow action", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(".github/workflows/ci.yml")).toBeInTheDocument();
      expect(screen.getByText("Run workflow")).toBeInTheDocument();
    });
  });

  it("Runs tab shows the run-level workflows", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await waitFor(() => {
      // Default tab is Workflows (files); switch to Runs.
      fireEvent.click(screen.getByText("Runs"));
    });
    await waitFor(() => {
      expect(screen.getByText("running")).toBeInTheDocument();
      expect(screen.getByText("push")).toBeInTheDocument();
    });
  });

  it("opens the dispatch dialog when Run workflow is clicked", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Run workflow")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByText("Run workflow"));
    await waitFor(() => {
      // Dialog title.
      expect(screen.getByRole("heading", { name: "Run workflow" })).toBeInTheDocument();
      expect(screen.getByLabelText("Ref")).toBeInTheDocument();
      expect(screen.getByLabelText("Inputs (JSON)")).toBeInTheDocument();
    });
  });
});
