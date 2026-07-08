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
      <MemoryRouter initialEntries={["/ui/workflows/admin~test~42"]}>
        <Routes>
          <Route path="/ui/workflows/:id" element={<WorkflowDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const workflowData = {
  id: 42,
  name: "CI Build",
  run_number: 42,
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
};

const jobsData = [
  {
    id: 101,
    run_id: 42,
    name: "Build",
    status: "completed",
    conclusion: "success",
    started_at: "2026-01-01T00:00:01Z",
    completed_at: "2026-01-01T00:00:02Z",
    steps: [],
    labels: ["self-hosted"],
    run_attempt: 1,
  },
  {
    id: 102,
    run_id: 42,
    name: "Test",
    status: "completed",
    conclusion: "success",
    started_at: "2026-01-01T00:00:03Z",
    completed_at: "2026-01-01T00:00:04Z",
    steps: [],
    labels: ["self-hosted"],
    run_attempt: 1,
  },
];

describe("WorkflowDetailPage", () => {
  it("renders workflow name and details", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/logs")) return Promise.resolve(new Response("", { status: 200 }));
      if (url.includes("/jobs")) return Promise.resolve(jsonResponse({ total_count: 2, jobs: jobsData }));
      return Promise.resolve(jsonResponse(workflowData));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /CI Build/i })).toBeInTheDocument();
      expect(screen.getByText("#42")).toBeInTheDocument();
    });
  });

  it("renders job table with both jobs", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/logs")) return Promise.resolve(new Response("", { status: 200 }));
      if (url.includes("/jobs")) return Promise.resolve(jsonResponse({ total_count: 2, jobs: jobsData }));
      return Promise.resolve(jsonResponse(workflowData));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Jobs (2)")).toBeInTheDocument();
      expect(screen.getByText("Build")).toBeInTheDocument();
      expect(screen.getByText("Test")).toBeInTheDocument();
    });
  });

  it("renders per-job log output when logs are present", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/actions/jobs/101/logs")) return Promise.resolve(new Response("Run actions/checkout@v4\nBuild succeeded in 4s", { status: 200 }));
      if (url.includes("/actions/jobs/102/logs")) return Promise.resolve(new Response("Run go test ./...\nok\tall packages", { status: 200 }));
      if (url.includes("/jobs")) return Promise.resolve(jsonResponse({ total_count: 2, jobs: jobsData }));
      return Promise.resolve(jsonResponse(workflowData));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Logs")).toBeInTheDocument();
      // LogViewer renders each line; assert one unique line per job.
      expect(screen.getByText("Build succeeded in 4s")).toBeInTheDocument();
      expect(screen.getByText("Run go test ./...")).toBeInTheDocument();
    });
  });
});
