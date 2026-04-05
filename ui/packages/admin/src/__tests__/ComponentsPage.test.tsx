import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { ComponentsPage } from "../pages/ComponentsPage.js";

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
        <ComponentsPage />
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

describe("ComponentsPage", () => {
  it("renders the components heading", async () => {
    mockFetch.mockResolvedValue(jsonResponse(componentsData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Components")).toBeInTheDocument();
    });
  });

  it("renders component names in the table", async () => {
    mockFetch.mockResolvedValue(jsonResponse(componentsData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("memory")).toBeInTheDocument();
      expect(screen.getByText("ecs")).toBeInTheDocument();
    });
  });

  it("renders health status badges", async () => {
    mockFetch.mockResolvedValue(jsonResponse(componentsData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("ok")).toBeInTheDocument();
      expect(screen.getByText("error")).toBeInTheDocument();
    });
  });
});
