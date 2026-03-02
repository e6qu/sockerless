import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { RunnersPage } from "./RunnersPage.js";

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

const runnersData = [
  { id: 1, description: "docker-runner", active: true, tag_list: ["docker", "linux"] },
  { id: 2, description: "shell-runner", active: false, tag_list: ["shell"] },
];

describe("RunnersPage", () => {
  it("renders the runners heading", async () => {
    mockFetch.mockResolvedValue(jsonResponse(runnersData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Runners (2)")).toBeInTheDocument();
    });
  });

  it("renders runner table", async () => {
    mockFetch.mockResolvedValue(jsonResponse(runnersData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("docker-runner")).toBeInTheDocument();
      expect(screen.getByText("shell-runner")).toBeInTheDocument();
    });
  });

  it("renders tags", async () => {
    mockFetch.mockResolvedValue(jsonResponse(runnersData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("docker, linux")).toBeInTheDocument();
    });
  });
});
