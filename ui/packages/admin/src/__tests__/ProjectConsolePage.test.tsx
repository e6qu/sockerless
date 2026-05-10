import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  render,
  cleanup,
  screen,
  waitFor,
  act,
  fireEvent,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import {
  ProjectConsolePage,
  parseTimestamp,
} from "../pages/ProjectConsolePage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  url: string;
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  closed = false;
  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
    queueMicrotask(() => this.onopen?.(new Event("open")));
  }
  addEventListener() {}
  removeEventListener() {}
  emit(line: string) {
    this.onmessage?.(new MessageEvent("message", { data: line }));
  }
  close() {
    this.closed = true;
  }
}

beforeEach(() => {
  FakeEventSource.instances = [];
  // @ts-expect-error  override for jsdom
  globalThis.EventSource = FakeEventSource;
  mockFetch.mockReset();
});

afterEach(() => {
  cleanup();
});

const sampleTopology = {
  projects: [
    {
      name: "p",
      instances: [
        { name: "sim-aws", kind: "sim", port: 4500 },
        { name: "be-ecs", kind: "backend", port: 3375 },
      ],
    },
  ],
};

function renderPage() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/ui/topology/p/console"]}>
        <Routes>
          <Route
            path="/ui/topology/:project/console"
            element={<ProjectConsolePage />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("parseTimestamp", () => {
  it("extracts ISO-8601 prefix", () => {
    expect(parseTimestamp("2026-05-10T13:00:00Z hello")).toBeGreaterThan(0);
  });
  it("recognises JSON time field", () => {
    const ts = parseTimestamp(
      `{"time":"2026-05-10T13:00:00Z","msg":"hi"}`,
    );
    expect(ts).toBeGreaterThan(0);
  });
  it("returns 0 for unparseable lines", () => {
    expect(parseTimestamp("plain log line")).toBe(0);
  });
  it("handles JSON ts in seconds", () => {
    const ts = parseTimestamp(`{"ts":1700000000}`);
    expect(ts).toBe(1700000000 * 1000);
  });
  it("handles JSON ts in milliseconds", () => {
    const ts = parseTimestamp(`{"ts":1700000000000}`);
    expect(ts).toBe(1700000000000);
  });
});

describe("ProjectConsolePage", () => {
  it("renders combined timeline + API console headings", async () => {
    mockFetch.mockResolvedValue(jsonResponse(sampleTopology));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Combined timeline")).toBeInTheDocument();
      expect(screen.getByText("API console")).toBeInTheDocument();
    });
  });

  it("opens an SSE stream per instance", async () => {
    mockFetch.mockResolvedValue(jsonResponse(sampleTopology));
    renderPage();
    await waitFor(() => {
      expect(FakeEventSource.instances.length).toBe(2);
    });
    const urls = FakeEventSource.instances.map((es) => es.url);
    expect(urls.some((u) => u.includes("instances/sim-aws/logs"))).toBe(true);
    expect(urls.some((u) => u.includes("instances/be-ecs/logs"))).toBe(true);
  });

  it("renders streamed lines tagged with instance", async () => {
    mockFetch.mockResolvedValue(jsonResponse(sampleTopology));
    renderPage();
    await waitFor(() => {
      expect(FakeEventSource.instances.length).toBe(2);
    });
    const [aws, ecs] = FakeEventSource.instances;
    act(() => {
      aws!.emit("aws first");
      ecs!.emit("ecs first");
    });
    await waitFor(() => {
      expect(screen.getByText("aws first")).toBeInTheDocument();
      expect(screen.getByText("ecs first")).toBeInTheDocument();
    });
  });

  it("disabling an instance closes its stream", async () => {
    mockFetch.mockResolvedValue(jsonResponse(sampleTopology));
    renderPage();
    await waitFor(() => {
      expect(FakeEventSource.instances.length).toBe(2);
    });
    const awsStream = FakeEventSource.instances.find((es) =>
      es.url.includes("sim-aws"),
    )!;

    // Toggle button is the instance name text
    const btn = screen.getByRole("button", { name: "sim-aws" });
    act(() => {
      btn.click();
    });
    await waitFor(() => {
      expect(awsStream.closed).toBe(true);
    });
  });

  it("API console fires a proxy request", async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse(sampleTopology))
      .mockResolvedValueOnce(
        jsonResponse({
          status: 200,
          status_text: "200 OK",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ ok: true }),
          duration_ms: 5,
        }),
      );

    renderPage();
    await waitFor(() => {
      expect(screen.getByText("API console")).toBeInTheDocument();
    });

    const sendBtn = screen.getByRole("button", { name: /send/i });
    act(() => {
      sendBtn.click();
    });

    await waitFor(() => {
      const calls = mockFetch.mock.calls;
      const proxyCall = calls.find((c) => String(c[0]).includes("/proxy"));
      expect(proxyCall).toBeTruthy();
    });
    await waitFor(() => {
      expect(screen.getByText(/200 OK/)).toBeInTheDocument();
    });
  });

  it("defaults the path field to /v1/health", async () => {
    mockFetch.mockResolvedValue(jsonResponse(sampleTopology));
    renderPage();
    await waitFor(() => {
      const pathField = screen.getByPlaceholderText(
        "/v1/health",
      ) as HTMLInputElement;
      expect(pathField.value).toBe("/v1/health");
    });
  });

  it("disables body textarea for GET", async () => {
    mockFetch.mockResolvedValue(jsonResponse(sampleTopology));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("API console")).toBeInTheDocument();
    });
    const bodyArea = screen.getByPlaceholderText(
      /body disabled for GET\/HEAD/i,
    ) as HTMLTextAreaElement;
    expect(bodyArea.disabled).toBe(true);

    // Switch to POST → enabled
    const methodSelect = screen.getAllByRole(
      "combobox",
    )[1] as HTMLSelectElement;
    fireEvent.change(methodSelect, { target: { value: "POST" } });
    await waitFor(() => {
      expect(bodyArea.disabled).toBe(false);
    });
  });
});
