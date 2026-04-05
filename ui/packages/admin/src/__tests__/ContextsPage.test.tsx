import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { ContextsPage } from "../pages/ContextsPage.js";

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
        <ContextsPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const contextsData = [
  {
    name: "default",
    active: true,
    backend: "ecs",
    backend_addr: "http://localhost:9100",
    frontend_addr: "http://localhost:2375",
  },
  {
    name: "staging",
    active: false,
    backend: "lambda",
    backend_addr: "http://localhost:9200",
  },
];

describe("ContextsPage", () => {
  it("renders the CLI Contexts heading", async () => {
    mockFetch.mockResolvedValue(jsonResponse(contextsData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("CLI Contexts")).toBeInTheDocument();
    });
  });

  it("renders context names in the table", async () => {
    mockFetch.mockResolvedValue(jsonResponse(contextsData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("default")).toBeInTheDocument();
      expect(screen.getByText("staging")).toBeInTheDocument();
    });
  });

  it("renders empty state when no contexts", async () => {
    mockFetch.mockResolvedValue(jsonResponse([]));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("No contexts found.")).toBeInTheDocument();
    });
  });
});
