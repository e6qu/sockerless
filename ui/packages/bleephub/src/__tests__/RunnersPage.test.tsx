import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { RunnersPage } from "../pages/RunnersPage.js";

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
        <RunnersPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const sessionsData = [
  {
    sessionId: "sess-1",
    ownerName: "runner1",
    agent: {
      id: 1,
      name: "my-runner",
      version: "2.320.0",
      enabled: true,
      status: "online",
      osDescription: "Linux",
      labels: [{ id: 1, name: "self-hosted", type: "system" }],
      ephemeral: false,
      createdOn: "2026-01-01T00:00:00Z",
    },
    pendingMessages: 2,
  },
];

describe("RunnersPage", () => {
  it("renders the runners heading", async () => {
    mockFetch.mockResolvedValue(jsonResponse(sessionsData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Runners (1)")).toBeInTheDocument();
    });
  });

  it("renders agent table with agent name", async () => {
    mockFetch.mockResolvedValue(jsonResponse(sessionsData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("my-runner")).toBeInTheDocument();
    });
  });

  it("shows pending messages count", async () => {
    mockFetch.mockResolvedValue(jsonResponse(sessionsData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Pending Messages")).toBeInTheDocument();
      expect(screen.getByText("2")).toBeInTheDocument();
    });
  });
});
