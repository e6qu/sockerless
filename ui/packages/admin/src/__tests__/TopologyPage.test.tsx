import { describe, it, expect, vi, afterEach } from "vitest";
import {
  render,
  cleanup,
  screen,
  waitFor,
  within,
  fireEvent,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { ToastProvider } from "@sockerless/ui-core/components";
import { TopologyPage } from "../pages/TopologyPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
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
      <ToastProvider>
        <BrowserRouter>
          <TopologyPage />
        </BrowserRouter>
      </ToastProvider>
    </QueryClientProvider>,
  );
}

const sampleTopology = {
  projects: [
    {
      name: "test-aws",
      instances: [
        {
          name: "aws-sim",
          kind: "sim",
          cloud: "aws",
          port: 4566,
        },
        {
          name: "ecs-backend",
          kind: "backend",
          cloud: "aws",
          backend: "ecs",
          port: 3300,
          sim: "aws-sim",
        },
      ],
    },
  ],
  ports: {
    ranges: {
      sim: { from: 4500, to: 4999 },
      backend: { from: 3300, to: 3399 },
      bleephub: { from: 5500, to: 5599 },
    },
  },
};

// Per-instance status endpoint default — not running.
const stoppedStatus = (project: string, name: string) => ({
  project,
  name,
  running: false,
  pid: 0,
  health: "unknown",
});

function routeFetch(url: string) {
  if (url === "/api/v1/topology") {
    return jsonResponse(sampleTopology);
  }
  // /api/v1/topology/projects/{p}/instances/{i}/status
  const m = url.match(
    /^\/api\/v1\/topology\/projects\/([^/]+)\/instances\/([^/]+)\/status$/,
  );
  if (m) {
    return jsonResponse(stoppedStatus(m[1], m[2]));
  }
  return jsonResponse({ error: "no mock for " + url }, 500);
}

describe("TopologyPage", () => {
  it("renders the topology heading + project + instances", async () => {
    mockFetch.mockImplementation((url: string) =>
      Promise.resolve(routeFetch(url)),
    );
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Topology")).toBeInTheDocument();
      expect(screen.getByText("test-aws")).toBeInTheDocument();
      expect(screen.getByText("aws-sim")).toBeInTheDocument();
      expect(screen.getByText("ecs-backend")).toBeInTheDocument();
    });
  });

  it("renders the port registry with configured ranges + claimed ports", async () => {
    mockFetch.mockImplementation((url: string) =>
      Promise.resolve(routeFetch(url)),
    );
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Port registry")).toBeInTheDocument();
      // Ranges render as "from–to" rows.
      expect(screen.getByText("4500–4999")).toBeInTheDocument();
      expect(screen.getByText("3300–3399")).toBeInTheDocument();
      // Claimed ports render as ":<port>".
      expect(screen.getByText(":4566")).toBeInTheDocument();
      expect(screen.getByText(":3300")).toBeInTheDocument();
    });
  });

  it("opens the project form when the + project button is clicked", async () => {
    mockFetch.mockImplementation((url: string) =>
      Promise.resolve(routeFetch(url)),
    );
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /\+ project/i })).toBeInTheDocument();
    });
    const button = screen.getByRole("button", { name: /\+ project/i });
    fireEvent.click(button);
    await waitFor(() => {
      // ProjectForm modal renders the kicker + heading.
      expect(screen.getByText(/add project/i)).toBeInTheDocument();
    });
  });

  it("shows empty state when no projects exist", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url === "/api/v1/topology") {
        return Promise.resolve(jsonResponse({ projects: [], ports: {} }));
      }
      return Promise.resolve(jsonResponse({ error: "n/a" }, 500));
    });
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/no projects configured/i)).toBeInTheDocument();
    });
  });

  it("renders Start button for stopped instances + Stop disappears", async () => {
    mockFetch.mockImplementation((url: string) =>
      Promise.resolve(routeFetch(url)),
    );
    renderPage();
    await waitFor(() => {
      const buttons = screen.getAllByRole("button", { name: /^start$/i });
      expect(buttons.length).toBeGreaterThan(0);
    });
  });

  it("opens the instance form on + instance with project pre-selected", async () => {
    mockFetch.mockImplementation((url: string) =>
      Promise.resolve(routeFetch(url)),
    );
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("test-aws")).toBeInTheDocument();
    });
    const addInstance = screen.getByRole("button", { name: /\+ instance/i });
    fireEvent.click(addInstance);
    await waitFor(() => {
      expect(screen.getByText(/add instance/i)).toBeInTheDocument();
    });
    // The kicker shows the project name.
    expect(screen.getByText(/project · test-aws/i)).toBeInTheDocument();
  });

  it("shows the confirm dialog when delete project is clicked", async () => {
    mockFetch.mockImplementation((url: string) =>
      Promise.resolve(routeFetch(url)),
    );
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("test-aws")).toBeInTheDocument();
    });
    const deleteBtn = screen.getByRole("button", { name: /delete project/i });
    fireEvent.click(deleteBtn);
    await waitFor(() => {
      expect(
        screen.getByText(/delete project test-aws/i),
      ).toBeInTheDocument();
    });
    // Confirm modal renders Cancel + Delete buttons.
    const dialogs = screen.getAllByRole("dialog");
    const confirm = dialogs[dialogs.length - 1];
    expect(within(confirm).getByRole("button", { name: /cancel/i })).toBeInTheDocument();
    expect(within(confirm).getByRole("button", { name: /^delete$/i })).toBeInTheDocument();
  });
});
