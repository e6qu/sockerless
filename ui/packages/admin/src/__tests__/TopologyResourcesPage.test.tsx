import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import {
  TopologyResourcesPage,
  __test,
} from "../pages/TopologyResourcesPage.js";
import type { RollupResource, RollupResponse } from "../api.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown) {
  return new Response(JSON.stringify(data), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  mockFetch.mockReset();
});

afterEach(() => {
  cleanup();
});

function renderPage() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/ui/topology/resources"]}>
        <Routes>
          <Route
            path="/ui/topology/resources"
            element={<TopologyResourcesPage />}
          />
        </Routes>
      </MemoryRouter>,
    </QueryClientProvider>,
  );
}

const sampleRollup: RollupResponse = {
  sources: [
    {
      project: "p1",
      instance: "be-ecs",
      cloud: "aws",
      backend: "ecs",
      port: 3375,
      ok: true,
      resource_count: 2,
    },
    {
      project: "p1",
      instance: "be-lambda",
      cloud: "aws",
      backend: "lambda",
      port: 3376,
      ok: false,
      error: "connect refused",
      resource_count: 0,
    },
  ],
  resources: [
    {
      project: "p1",
      instance: "be-ecs",
      cloud: "aws",
      backend: "ecs",
      port: 3375,
      resource_type: "task",
      resource_id: "task/abc",
      cleaned_up: false,
      status: "RUNNING",
    },
    {
      project: "p1",
      instance: "be-ecs",
      cloud: "aws",
      backend: "ecs",
      port: 3375,
      resource_type: "task",
      resource_id: "task/def",
      cleaned_up: true,
      status: "STOPPED",
    },
  ],
};

describe("groupResources", () => {
  const r: RollupResource[] = [
    {
      project: "p",
      instance: "a",
      cloud: "aws",
      backend: "ecs",
      port: 1,
      resource_type: "task",
      resource_id: "1",
      cleaned_up: false,
    },
    {
      project: "p",
      instance: "a",
      cloud: "aws",
      backend: "ecs",
      port: 1,
      resource_type: "log-group",
      resource_id: "lg-1",
      cleaned_up: false,
    },
    {
      project: "p",
      instance: "b",
      cloud: "gcp",
      backend: "cloudrun",
      port: 2,
      resource_type: "task",
      resource_id: "2",
      cleaned_up: false,
    },
  ];
  it("groups by instance", () => {
    const g = __test.groupResources(r, "instance");
    expect(g.length).toBe(2);
    expect(g[0]!.entries.length + g[1]!.entries.length).toBe(3);
  });
  it("groups by cloud", () => {
    const g = __test.groupResources(r, "cloud");
    const keys = g.map((x) => x.key).sort();
    expect(keys).toEqual(["aws", "gcp"]);
  });
  it("groups by service", () => {
    const g = __test.groupResources(r, "service");
    const keys = g.map((x) => x.key).sort();
    expect(keys).toEqual(["log-group", "task"]);
  });
  it("flat returns single group", () => {
    const g = __test.groupResources(r, "flat");
    expect(g.length).toBe(1);
    expect(g[0]!.entries.length).toBe(3);
  });
  it("empty input → empty result", () => {
    expect(__test.groupResources([], "instance")).toEqual([]);
    expect(__test.groupResources([], "flat")).toEqual([]);
  });
});

describe("TopologyResourcesPage", () => {
  it("renders summary and resource rows", async () => {
    mockFetch.mockResolvedValue(jsonResponse(sampleRollup));
    renderPage();
    await waitFor(() => {
      // 2 resources from 1/2 backend(s)
      expect(screen.getByText(/2 resources from 1\/2 backends/)).toBeInTheDocument();
    });
    // Resource ids visible
    expect(screen.getByText("task/abc")).toBeInTheDocument();
    expect(screen.getByText("task/def")).toBeInTheDocument();
  });

  it("surfaces failed sources banner", async () => {
    mockFetch.mockResolvedValue(jsonResponse(sampleRollup));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/1 backend unreachable/)).toBeInTheDocument();
      expect(screen.getByText(/connect refused/)).toBeInTheDocument();
    });
  });

  it("switches grouping when a chip is clicked", async () => {
    mockFetch.mockResolvedValue(jsonResponse(sampleRollup));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("p1 / be-ecs (ecs)")).toBeInTheDocument();
    });
    const cloudBtn = screen.getByRole("button", { name: /by cloud/i });
    fireEvent.click(cloudBtn);
    // After regrouping, the instance-style group label is gone; the
    // cloud-style group label "aws" appears.
    await waitFor(() => {
      expect(screen.queryByText("p1 / be-ecs (ecs)")).not.toBeInTheDocument();
    });
    expect(screen.getAllByText(/aws/i).length).toBeGreaterThan(0);
  });

  it("refetches with active=true by default", async () => {
    mockFetch.mockResolvedValue(jsonResponse(sampleRollup));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText(/active only/i)).toBeInTheDocument();
    });
    const url = String(mockFetch.mock.calls[0]?.[0] ?? "");
    expect(url).toContain("/api/v1/topology/resources");
    expect(url).toContain("active=true");
  });
});
