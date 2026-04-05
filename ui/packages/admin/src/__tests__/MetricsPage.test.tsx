import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { MetricsPage } from "../pages/MetricsPage.js";

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
        <MetricsPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const componentsData = [
  {
    name: "memory",
    type: "backend",
    addr: "http://localhost:9100",
    health: "up",
    uptime: 3600,
  },
  {
    name: "ecs",
    type: "backend",
    addr: "http://localhost:9102",
    health: "down",
    uptime: 0,
  },
];

describe("MetricsPage", () => {
  it("renders the Metrics heading", async () => {
    mockFetch.mockResolvedValue(jsonResponse(componentsData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Metrics")).toBeInTheDocument();
    });
  });

  it("renders a panel for each component", async () => {
    mockFetch.mockResolvedValue(jsonResponse(componentsData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("memory")).toBeInTheDocument();
      expect(screen.getByText("ecs")).toBeInTheDocument();
    });
  });

  it("renders empty state when no components", async () => {
    mockFetch.mockResolvedValue(jsonResponse([]));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("No components found.")).toBeInTheDocument();
    });
  });
});
