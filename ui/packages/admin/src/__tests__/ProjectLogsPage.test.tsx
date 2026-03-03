import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { ProjectLogsPage } from "../pages/ProjectLogsPage.js";

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
      <MemoryRouter initialEntries={["/ui/projects/test-aws/logs"]}>
        <Routes>
          <Route path="/ui/projects/:name/logs" element={<ProjectLogsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("ProjectLogsPage", () => {
  it("renders the logs heading", async () => {
    mockFetch.mockResolvedValue(jsonResponse([]));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Logs")).toBeInTheDocument();
    });
  });

  it("renders component selector buttons", async () => {
    mockFetch.mockResolvedValue(jsonResponse([]));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("All")).toBeInTheDocument();
      expect(screen.getByText("Simulator")).toBeInTheDocument();
      expect(screen.getByText("Backend")).toBeInTheDocument();
      expect(screen.getByText("Frontend")).toBeInTheDocument();
    });
  });

  it("renders back link", async () => {
    mockFetch.mockResolvedValue(jsonResponse([]));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Back to test-aws")).toBeInTheDocument();
    });
  });
});
