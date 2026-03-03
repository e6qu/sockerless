import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { ProjectDetailPage } from "../pages/ProjectDetailPage.js";

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

const projectData = {
  name: "test-aws",
  cloud: "aws",
  backend: "ecs",
  log_level: "info",
  sim_port: 4566,
  backend_port: 9100,
  frontend_port: 2375,
  frontend_mgmt_port: 9200,
  created_at: "2025-01-01T00:00:00Z",
  status: "running",
  sim_status: "running",
  backend_status: "running",
  frontend_status: "running",
};

const connectionData = {
  docker_host: "tcp://localhost:2375",
  env_export: "export DOCKER_HOST=tcp://localhost:2375",
  podman_connection: "podman system connection add test-aws tcp://localhost:2375",
  simulator_addr: "http://localhost:4566",
  backend_addr: "http://localhost:9100",
  frontend_addr: "http://localhost:2375",
  frontend_mgmt_addr: "http://localhost:9200",
};

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/ui/projects/test-aws"]}>
        <Routes>
          <Route path="/ui/projects/:name" element={<ProjectDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("ProjectDetailPage", () => {
  it("renders project name and status", async () => {
    mockFetch.mockImplementation(() => Promise.resolve(jsonResponse(projectData)));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("test-aws")).toBeInTheDocument();
    });
  });

  it("renders component cards", async () => {
    mockFetch.mockImplementation(() => Promise.resolve(jsonResponse(projectData)));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Simulator")).toBeInTheDocument();
      expect(screen.getByText("Frontend")).toBeInTheDocument();
      // "Backend" appears as both MetricsCard title and component label
      expect(screen.getAllByText("Backend")).toHaveLength(2);
    });
  });

  it("renders connection info", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (typeof url === "string" && url.includes("/connection")) {
        return Promise.resolve(jsonResponse(connectionData));
      }
      return Promise.resolve(jsonResponse(projectData));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Connection Info")).toBeInTheDocument();
    });
  });

  it("renders View Logs link", async () => {
    mockFetch.mockImplementation(() => Promise.resolve(jsonResponse(projectData)));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("View Logs")).toBeInTheDocument();
    });
  });

  it("renders Delete button", async () => {
    mockFetch.mockImplementation(() => Promise.resolve(jsonResponse(projectData)));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Delete")).toBeInTheDocument();
    });
  });
});
