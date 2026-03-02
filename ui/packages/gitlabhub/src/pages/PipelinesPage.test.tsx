import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { PipelinesPage } from "./PipelinesPage.js";

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
        <PipelinesPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const pipelinesData = [
  {
    id: 1,
    project_id: 1,
    project_name: "test-project",
    status: "running",
    result: "",
    ref: "main",
    sha: "abc123",
    stages: ["build", "test"],
    jobs: {
      build: { id: 1, name: "build", stage: "build", status: "running" },
      test: { id: 2, name: "test", stage: "test", status: "pending" },
    },
    created_at: "2026-01-01T00:00:00Z",
  },
];

describe("PipelinesPage", () => {
  it("renders the pipelines heading", async () => {
    mockFetch.mockResolvedValue(jsonResponse(pipelinesData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Pipelines (1)")).toBeInTheDocument();
    });
  });

  it("renders DataTable columns", async () => {
    mockFetch.mockResolvedValue(jsonResponse(pipelinesData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Project")).toBeInTheDocument();
      expect(screen.getByText("Status")).toBeInTheDocument();
      expect(screen.getByText("Ref")).toBeInTheDocument();
    });
  });

  it("renders status badges", async () => {
    mockFetch.mockResolvedValue(jsonResponse(pipelinesData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("running")).toBeInTheDocument();
    });
  });
});
