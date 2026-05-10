import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  render,
  cleanup,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import {
  UnhealthyDiagnosticPanel,
  shouldRender,
} from "../components/UnhealthyDiagnosticPanel.js";
import type { InstanceStatus } from "../api.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => mockFetch.mockReset());
afterEach(() => cleanup());

const baseStatus: InstanceStatus = {
  project: "p",
  name: "sim-aws",
  running: true,
  pid: 1234,
  health: "ok",
};

function mockObservability(enabled: boolean, urls?: { logs?: string; traces?: string }) {
  // The panel fetches /api/v1/observability; route those calls to a
  // synthetic config so we can assert on the rendered links.
  const cfg = enabled
    ? {
        enabled: true,
        logs_dashboard: urls?.logs ?? "http://logs.local/",
        traces_dashboard: urls?.traces ?? "http://traces.local/",
        logs_service_param: "service.name",
        traces_service_param: "service",
      }
    : { enabled: false };
  // Default response covers /diagnostics; observability matched here.
  mockFetch.mockImplementation((url: string) => {
    if (String(url).includes("/api/v1/observability")) {
      return Promise.resolve(jsonResponse(cfg));
    }
    return Promise.resolve(
      jsonResponse({
        status: { ...baseStatus },
        log_lines: [],
        log_path: "",
      }),
    );
  });
}

function renderPanel(status: InstanceStatus) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <UnhealthyDiagnosticPanel
          project="p"
          instanceName="sim-aws"
          status={status}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("shouldRender", () => {
  it("false for healthy instance", () => {
    expect(shouldRender(baseStatus)).toBe(false);
  });
  it("true for unhealthy", () => {
    expect(shouldRender({ ...baseStatus, health: "unhealthy" })).toBe(true);
  });
  it("true for crashed_since_start", () => {
    expect(
      shouldRender({ ...baseStatus, running: false, crashed_since_start: true }),
    ).toBe(true);
  });
  it("true for process gone with pidfile", () => {
    expect(shouldRender({ ...baseStatus, running: false, pid: 5678 })).toBe(true);
  });
  it("false for cleanly stopped (PID=0)", () => {
    expect(shouldRender({ ...baseStatus, running: false, pid: 0 })).toBe(false);
  });
  it("false for undefined status", () => {
    expect(shouldRender(undefined)).toBe(false);
  });
});

describe("UnhealthyDiagnosticPanel", () => {
  it("renders the failing-signal header + log lines", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({
        status: { ...baseStatus, health: "unhealthy", health_detail: "HTTP 503" },
        log_lines: ["error 1", "error 2"],
        log_path: "/path/sim-aws.log",
      }),
    );
    renderPanel({ ...baseStatus, health: "unhealthy", health_detail: "HTTP 503" });
    await waitFor(() => {
      expect(screen.getByText(/health probe unhealthy/i)).toBeInTheDocument();
      expect(screen.getByText("error 1")).toBeInTheDocument();
      expect(screen.getByText("error 2")).toBeInTheDocument();
    });
    expect(screen.getByText(/HTTP 503/)).toBeInTheDocument();
  });

  it("crashed_since_start surfaces exit info", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({
        status: {
          ...baseStatus,
          running: false,
          crashed_since_start: true,
          exit: { code: 137, at: "2026-05-10T12:00:00Z" },
        },
        log_lines: ["fatal: oom"],
        log_path: "/path/sim-aws.log",
      }),
    );
    renderPanel({
      ...baseStatus,
      running: false,
      crashed_since_start: true,
      exit: { code: 137, at: "2026-05-10T12:00:00Z" },
    });
    await waitFor(() => {
      expect(screen.getByText(/process exited unexpectedly/i)).toBeInTheDocument();
    });
    expect(screen.getByText(/exit 137/)).toBeInTheDocument();
    expect(screen.getByText(/2026-05-10T12:00:00Z/)).toBeInTheDocument();
  });

  it("renders deep links to logs + console", async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({
        status: { ...baseStatus, health: "unhealthy" },
        log_lines: [],
        log_path: "",
      }),
    );
    renderPanel({ ...baseStatus, health: "unhealthy" });
    await waitFor(() => {
      const fullLogs = screen.getByText(/full logs/i);
      expect(fullLogs.getAttribute("href")).toBe(
        "/ui/topology/p/sim-aws/logs",
      );
      const console = screen.getByText(/console/i);
      expect(console.getAttribute("href")).toBe("/ui/topology/p/console");
    });
  });

  it("error path surfaces the message", async () => {
    mockFetch.mockResolvedValue(new Response("oh no", { status: 500 }));
    renderPanel({ ...baseStatus, health: "unhealthy" });
    await waitFor(() => {
      expect(screen.getByText(/Admin API error 500/)).toBeInTheDocument();
    });
  });

  it("renders VictoriaLogs / Jaeger deep links when observability is enabled", async () => {
    mockObservability(true);
    renderPanel({ ...baseStatus, health: "unhealthy" });
    await waitFor(() => {
      const logs = screen.getByText(/VictoriaLogs ↗/);
      const traces = screen.getByText(/Jaeger ↗/);
      const logsHref = logs.getAttribute("href") ?? "";
      const tracesHref = traces.getAttribute("href") ?? "";
      expect(logsHref).toContain("service.name=sim-aws");
      expect(tracesHref).toContain("service=sim-aws");
    });
  });

  it("hides deep links when observability is disabled", async () => {
    mockObservability(false);
    renderPanel({ ...baseStatus, health: "unhealthy" });
    await waitFor(() => {
      // Diagnostic fetch resolves; verify the panel rendered normally
      // (full logs link is always there) and the OTel chips are not.
      expect(screen.getByText(/full logs/)).toBeInTheDocument();
    });
    expect(screen.queryByText(/VictoriaLogs ↗/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Jaeger ↗/)).not.toBeInTheDocument();
  });
});
