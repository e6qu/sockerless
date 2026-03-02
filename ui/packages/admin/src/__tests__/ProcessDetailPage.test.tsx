import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { ProcessDetailPage } from "../pages/ProcessDetailPage.js";

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
      <MemoryRouter initialEntries={["/ui/processes/sim-aws"]}>
        <Routes>
          <Route path="/ui/processes/:name" element={<ProcessDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const processData = [
  { name: "sim-aws", binary: "simulator-aws", status: "running", pid: 1234, addr: ":4566", started_at: "2025-01-01T00:00:00Z", exit_code: 0, type: "simulator" },
];

const logData = ["Starting simulator...", "Listening on :4566"];

describe("ProcessDetailPage", () => {
  it("renders process name and status", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/logs")) return Promise.resolve(jsonResponse(logData));
      return Promise.resolve(jsonResponse(processData));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("sim-aws")).toBeInTheDocument();
      expect(screen.getByText("running")).toBeInTheDocument();
    });
  });

  it("renders info cards", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/logs")) return Promise.resolve(jsonResponse(logData));
      return Promise.resolve(jsonResponse(processData));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Binary")).toBeInTheDocument();
      expect(screen.getByText("simulator-aws")).toBeInTheDocument();
      expect(screen.getByText("PID")).toBeInTheDocument();
    });
  });

  it("renders stop button for running process", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/logs")) return Promise.resolve(jsonResponse(logData));
      return Promise.resolve(jsonResponse(processData));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Stop")).toBeInTheDocument();
    });
  });
});
