import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { ProcessesPage } from "../pages/ProcessesPage.js";

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
        <ProcessesPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const processData = [
  {
    name: "sim-aws",
    binary: "simulator-aws",
    status: "running",
    pid: 1234,
    addr: ":4566",
    started_at: "2025-01-01T00:00:00Z",
    exit_code: 0,
    type: "simulator",
  },
  {
    name: "ecs",
    binary: "sockerless-backend-ecs",
    status: "stopped",
    pid: 0,
    addr: ":9100",
    started_at: "",
    exit_code: 0,
    type: "backend",
  },
];

describe("ProcessesPage", () => {
  it("renders the processes heading", async () => {
    mockFetch.mockResolvedValue(jsonResponse(processData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Processes")).toBeInTheDocument();
    });
  });

  it("renders process names as links", async () => {
    mockFetch.mockResolvedValue(jsonResponse(processData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("sim-aws")).toBeInTheDocument();
      expect(screen.getByText("ecs")).toBeInTheDocument();
    });
  });

  it("shows Start button for stopped processes", async () => {
    mockFetch.mockResolvedValue(jsonResponse(processData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Start")).toBeInTheDocument();
    });
  });

  it("shows Stop button for running processes", async () => {
    mockFetch.mockResolvedValue(jsonResponse(processData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Stop")).toBeInTheDocument();
    });
  });
});
