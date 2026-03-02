import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { ProjectsPage } from "../pages/ProjectsPage.js";

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
        <ProjectsPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const projectData = [
  {
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
  },
  {
    name: "test-gcp",
    cloud: "gcp",
    backend: "cloudrun",
    log_level: "debug",
    sim_port: 5000,
    backend_port: 9101,
    frontend_port: 2376,
    frontend_mgmt_port: 9201,
    created_at: "2025-01-01T00:00:00Z",
    status: "stopped",
    sim_status: "stopped",
    backend_status: "stopped",
    frontend_status: "stopped",
  },
];

describe("ProjectsPage", () => {
  it("renders the projects heading", async () => {
    mockFetch.mockResolvedValue(jsonResponse(projectData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Projects")).toBeInTheDocument();
    });
  });

  it("renders project cards with names", async () => {
    mockFetch.mockResolvedValue(jsonResponse(projectData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("test-aws")).toBeInTheDocument();
      expect(screen.getByText("test-gcp")).toBeInTheDocument();
    });
  });

  it("shows Start button for stopped projects", async () => {
    mockFetch.mockResolvedValue(jsonResponse(projectData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Start")).toBeInTheDocument();
    });
  });

  it("shows Stop button for running projects", async () => {
    mockFetch.mockResolvedValue(jsonResponse(projectData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Stop")).toBeInTheDocument();
    });
  });

  it("shows empty state when no projects", async () => {
    mockFetch.mockResolvedValue(jsonResponse([]));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/No projects configured/)).toBeInTheDocument();
    });
  });

  it("shows New Project button", async () => {
    mockFetch.mockResolvedValue(jsonResponse([]));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("New Project")).toBeInTheDocument();
    });
  });
});
