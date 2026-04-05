import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { ComponentDetailPage } from "../pages/ComponentDetailPage.js";

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

const componentsData = [
  {
    name: "memory",
    type: "backend",
    addr: "http://localhost:9100",
    health: "up",
    uptime: 3600,
  },
];

const statusData = {
  name: "memory",
  type: "backend",
  health: "up",
  address: "http://localhost:9100",
  uptime: 3600,
  containers: 5,
};
const metricsData = { requests: 100, errors: 2 };
const providerData = {
  provider: "aws",
  mode: "simulator",
  region: "us-east-1",
  endpoint: "http://localhost:4566",
};

function renderPage(componentName: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/ui/components/${componentName}`]}>
        <Routes>
          <Route
            path="/ui/components/:name"
            element={<ComponentDetailPage />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("ComponentDetailPage", () => {
  it("renders component name and type", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (
        url.includes("/components") &&
        !url.includes("/status") &&
        !url.includes("/metrics") &&
        !url.includes("/provider")
      ) {
        return Promise.resolve(jsonResponse(componentsData));
      }
      if (url.includes("/status"))
        return Promise.resolve(jsonResponse(statusData));
      if (url.includes("/metrics"))
        return Promise.resolve(jsonResponse(metricsData));
      if (url.includes("/provider"))
        return Promise.resolve(jsonResponse(providerData));
      return Promise.resolve(jsonResponse({}));
    });
    renderPage("memory");
    await waitFor(() => {
      expect(screen.getByText("memory")).toBeInTheDocument();
      expect(screen.getByText("backend")).toBeInTheDocument();
    });
  });

  it("shows not-found message for unknown component", async () => {
    mockFetch.mockResolvedValue(jsonResponse(componentsData));
    renderPage("nonexistent");
    await waitFor(() => {
      expect(screen.getByText(/not found/)).toBeInTheDocument();
    });
  });

  it("renders the reload button for backend components", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes("/status"))
        return Promise.resolve(jsonResponse(statusData));
      if (url.includes("/metrics"))
        return Promise.resolve(jsonResponse(metricsData));
      if (url.includes("/provider"))
        return Promise.resolve(jsonResponse(providerData));
      return Promise.resolve(jsonResponse(componentsData));
    });
    renderPage("memory");
    await waitFor(() => {
      expect(screen.getByText("Reload")).toBeInTheDocument();
    });
  });
});
