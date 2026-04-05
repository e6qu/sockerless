import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { ResourcesPage } from "../pages/ResourcesPage.js";

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
        <ResourcesPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const resourcesData = [
  {
    containerId: "abc123",
    backend: "ecs",
    resourceType: "task",
    resourceId: "task-1",
    instanceId: "i-123",
    createdAt: "2026-01-01T00:00:00Z",
    cleanedUp: false,
    status: "active",
  },
  {
    containerId: "def456",
    backend: "lambda",
    resourceType: "function",
    resourceId: "fn-1",
    instanceId: "i-456",
    createdAt: "2026-01-01T01:00:00Z",
    cleanedUp: true,
    status: "cleaned",
  },
];

describe("ResourcesPage", () => {
  it("renders the Cloud Resources heading with count", async () => {
    mockFetch.mockResolvedValue(jsonResponse(resourcesData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Cloud Resources (2)")).toBeInTheDocument();
    });
  });

  it("renders resource IDs in the table", async () => {
    mockFetch.mockResolvedValue(jsonResponse(resourcesData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("task-1")).toBeInTheDocument();
      expect(screen.getByText("fn-1")).toBeInTheDocument();
    });
  });

  it("renders empty state when no resources", async () => {
    mockFetch.mockResolvedValue(jsonResponse([]));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("No resources found.")).toBeInTheDocument();
    });
  });
});
