import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { DashboardPage } from "../pages/DashboardPage.js";

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
        <DashboardPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const overviewData = {
  components_up: 3,
  components_down: 0,
  components_total: 3,
  total_containers: 12,
  backends: 2,
  components: [
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
      health: "up",
      uptime: 1800,
    },
    {
      name: "sim-aws",
      type: "simulator",
      addr: "http://localhost:4566",
      health: "up",
      uptime: 600,
    },
  ],
};

describe("DashboardPage", () => {
  it("renders the system overview heading", async () => {
    mockFetch.mockResolvedValue(jsonResponse(overviewData));
    renderPage();
    await waitFor(() => {
      // Editorial design renders the title in a serif <h2>; case-
      // insensitive match keeps the test resilient to copy tweaks.
      expect(screen.getByRole("heading", { name: /system overview/i })).toBeInTheDocument();
    });
  });

  it("renders KPI cards with correct values", async () => {
    mockFetch.mockResolvedValue(jsonResponse(overviewData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("3")).toBeInTheDocument(); // components up
      expect(screen.getByText("12")).toBeInTheDocument(); // total containers
    });
  });

  it("renders component health cards", async () => {
    mockFetch.mockResolvedValue(jsonResponse(overviewData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("memory")).toBeInTheDocument();
      expect(screen.getByText("ecs")).toBeInTheDocument();
      expect(screen.getByText("sim-aws")).toBeInTheDocument();
    });
  });

  it("shows status badges for healthy components", async () => {
    mockFetch.mockResolvedValue(jsonResponse(overviewData));
    renderPage();
    await waitFor(() => {
      const okBadges = screen.getAllByText("ok");
      expect(okBadges.length).toBe(3);
    });
  });
});
