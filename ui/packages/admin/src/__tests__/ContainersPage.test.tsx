import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { ContainersPage } from "../pages/ContainersPage.js";

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
        <ContainersPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const containersData = [
  {
    id: "abc123def456",
    name: "web-1",
    image: "nginx:latest",
    state: "running",
    created: "2026-01-01T00:00:00Z",
    backend: "ecs",
  },
  {
    id: "xyz789ghi012",
    name: "api-1",
    image: "node:18",
    state: "exited",
    created: "2026-01-01T01:00:00Z",
    backend: "lambda",
  },
];

describe("ContainersPage", () => {
  it("renders the containers heading with count", async () => {
    mockFetch.mockResolvedValue(jsonResponse(containersData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Containers (2)")).toBeInTheDocument();
    });
  });

  it("renders container names in the table", async () => {
    mockFetch.mockResolvedValue(jsonResponse(containersData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("web-1")).toBeInTheDocument();
      expect(screen.getByText("api-1")).toBeInTheDocument();
    });
  });

  it("renders empty state when no containers", async () => {
    mockFetch.mockResolvedValue(jsonResponse([]));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("No containers found.")).toBeInTheDocument();
    });
  });
});
