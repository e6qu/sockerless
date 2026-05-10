import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, act } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { InstanceLogsPage } from "../pages/InstanceLogsPage.js";

// jsdom does not implement EventSource — minimal fake good enough for
// unit tests. Tracks the URL it was opened with + lets the test push
// `data: <line>\n\n` events into the consumer.
class FakeEventSource {
  static instances: FakeEventSource[] = [];
  url: string;
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  closed = false;
  listeners = new Map<string, EventListener>();
  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
    queueMicrotask(() => this.onopen?.(new Event("open")));
  }
  addEventListener(name: string, fn: EventListener) {
    this.listeners.set(name, fn);
  }
  removeEventListener(name: string) {
    this.listeners.delete(name);
  }
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
});

afterEach(() => {
  cleanup();
});

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route
          path="/ui/topology/:project/:instance/logs"
          element={<InstanceLogsPage />}
        />
      </Routes>
    </MemoryRouter>,
  );
}

describe("InstanceLogsPage", () => {
  it("opens a follow=1 SSE connection to the right URL", async () => {
    renderAt("/ui/topology/proj/sim-aws/logs");
    await waitFor(() => {
      expect(FakeEventSource.instances.length).toBe(1);
    });
    const url = FakeEventSource.instances[0]!.url;
    expect(url).toContain("/api/v1/topology/projects/proj/instances/sim-aws/logs");
    expect(url).toContain("follow=1");
    expect(url).toContain("lines=200");
  });

  it("renders streamed lines into the LogViewer", async () => {
    renderAt("/ui/topology/proj/sim-aws/logs");
    await waitFor(() => {
      expect(FakeEventSource.instances.length).toBe(1);
    });
    const es = FakeEventSource.instances[0]!;
    act(() => {
      es.emit("hello");
      es.emit("world");
    });
    await waitFor(() => {
      expect(screen.getByText("hello")).toBeInTheDocument();
      expect(screen.getByText("world")).toBeInTheDocument();
    });
  });

  it("pause closes the stream; resume reopens it", async () => {
    renderAt("/ui/topology/proj/sim-aws/logs");
    await waitFor(() => {
      expect(FakeEventSource.instances.length).toBe(1);
    });
    const first = FakeEventSource.instances[0]!;
    const pauseBtn = screen.getByRole("button", { name: /pause/i });

    act(() => {
      pauseBtn.click();
    });
    await waitFor(() => {
      expect(first.closed).toBe(true);
    });

    const resumeBtn = screen.getByRole("button", { name: /resume/i });
    act(() => {
      resumeBtn.click();
    });
    await waitFor(() => {
      expect(FakeEventSource.instances.length).toBe(2);
    });
  });

  it("clear empties the buffer", async () => {
    renderAt("/ui/topology/proj/sim-aws/logs");
    await waitFor(() => {
      expect(FakeEventSource.instances.length).toBe(1);
    });
    const es = FakeEventSource.instances[0]!;
    act(() => {
      es.emit("first-line");
    });
    await waitFor(() => {
      expect(screen.getByText("first-line")).toBeInTheDocument();
    });
    const clearBtn = screen.getByRole("button", { name: /clear/i });
    act(() => {
      clearBtn.click();
    });
    await waitFor(() => {
      expect(screen.queryByText("first-line")).not.toBeInTheDocument();
    });
  });
});
