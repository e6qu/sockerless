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
  components_down: 1,
  components_total: 4,
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
    {
      name: "bleephub",
      type: "coordinator",
      addr: "http://localhost:5555",
      health: "down",
      uptime: 0,
    },
  ],
};

describe("DashboardPage", () => {
  it("renders the system overview heading", async () => {
    mockFetch.mockResolvedValue(jsonResponse(overviewData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("System Overview")).toBeInTheDocument();
    });
  });

  it("renders KPI cards with correct values", async () => {
    mockFetch.mockResolvedValue(jsonResponse(overviewData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("3")).toBeInTheDocument(); // components up
      expect(screen.getByText("1")).toBeInTheDocument(); // components down
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
      expect(screen.getByText("bleephub")).toBeInTheDocument();
    });
  });

  it("shows status badges for healthy and unhealthy components", async () => {
    mockFetch.mockResolvedValue(jsonResponse(overviewData));
    renderPage();
    await waitFor(() => {
      const okBadges = screen.getAllByText("ok");
      const errorBadges = screen.getAllByText("error");
      expect(okBadges.length).toBe(3);
      expect(errorBadges.length).toBe(1);
    });
  });
});
