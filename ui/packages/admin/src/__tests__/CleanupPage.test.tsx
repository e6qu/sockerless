import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { CleanupPage } from "../pages/CleanupPage.js";

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
        <CleanupPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const scanResult = {
  items: [
    { category: "process", name: "stale.pid", description: "PID 12345 dead", size: 0, age: "2h" },
    { category: "tmp", name: "sockerless-abc", description: "Temp directory (1.5 MB)", size: 1572864, age: "3h" },
    { category: "container", name: "test-container", description: "Container abc on ecs (state: exited)", size: 0, age: "" },
  ],
  scanned_at: "2025-01-01T00:00:00Z",
};

describe("CleanupPage", () => {
  it("renders the scan button", () => {
    renderPage();
    expect(screen.getByText("Scan")).toBeInTheDocument();
  });

  it("renders scan results grouped by category", async () => {
    mockFetch.mockResolvedValue(jsonResponse(scanResult));
    renderPage();
    fireEvent.click(screen.getByText("Scan"));
    await waitFor(() => {
      expect(screen.getByText(/Orphaned Processes/)).toBeInTheDocument();
      expect(screen.getByText(/Stale Temp Files/)).toBeInTheDocument();
      expect(screen.getByText(/Stopped Containers/)).toBeInTheDocument();
    });
  });

  it("renders clean buttons per category", async () => {
    mockFetch.mockResolvedValue(jsonResponse(scanResult));
    renderPage();
    fireEvent.click(screen.getByText("Scan"));
    await waitFor(() => {
      const cleanButtons = screen.getAllByText("Clean");
      expect(cleanButtons.length).toBe(2); // process + tmp
      expect(screen.getByText("Prune")).toBeInTheDocument(); // containers
    });
  });

  it("shows empty state when no items found", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ items: [], scanned_at: "2025-01-01T00:00:00Z" }));
    renderPage();
    fireEvent.click(screen.getByText("Scan"));
    await waitFor(() => {
      expect(screen.getByText("No stale resources found.")).toBeInTheDocument();
    });
  });
});
