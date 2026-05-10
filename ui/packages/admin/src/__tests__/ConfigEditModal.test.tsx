import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  render,
  cleanup,
  screen,
  waitFor,
  fireEvent,
  act,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ConfigEditModal } from "../components/ConfigEditModal.js";
import { ToastProvider } from "@sockerless/ui-core/components";
import type { ConfigKeyMeta, TopologyInstance } from "../api.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  mockFetch.mockReset();
});

afterEach(() => {
  cleanup();
});

const metadata: ConfigKeyMeta[] = [
  { name: "SIM_LOG_LEVEL", hot_reloadable: true, doc: "Log level" },
  { name: "SIM_DATA_DIR", hot_reloadable: false, doc: "Data dir" },
];

const instance: TopologyInstance = {
  name: "sim-aws",
  kind: "sim",
  port: 4500,
  cloud: "aws",
  config: {
    SIM_LOG_LEVEL: "info",
    SIM_DATA_DIR: "/tmp/old",
  },
};

function renderModal(overrides: Partial<{
  onReload: ReturnType<typeof vi.fn>;
  onRestart: ReturnType<typeof vi.fn>;
}> = {}) {
  const onReload = overrides.onReload ?? vi.fn().mockResolvedValue(undefined);
  const onRestart = overrides.onRestart ?? vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return {
    onReload,
    onRestart,
    onClose,
    ...render(
      <QueryClientProvider client={qc}>
        <ToastProvider>
          <ConfigEditModal
            open
            onClose={onClose}
            project="proj"
            instance={instance}
            metadata={metadata}
            onReload={onReload}
            onRestart={onRestart}
          />
        </ToastProvider>
      </QueryClientProvider>,
    ),
  };
}

describe("ConfigEditModal", () => {
  it("renders rows with hot/restart badges", async () => {
    renderModal();
    await waitFor(() => {
      expect(screen.getByDisplayValue("SIM_LOG_LEVEL")).toBeInTheDocument();
      expect(screen.getByDisplayValue("SIM_DATA_DIR")).toBeInTheDocument();
    });
  });

  it("save → server classifies → UI offers Restart for restart-required change", async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse({
        project: "proj",
        instance: "sim-aws",
        hot_reloadable_changes: [],
        restart_required_changes: ["SIM_DATA_DIR"],
        config: { SIM_LOG_LEVEL: "info", SIM_DATA_DIR: "/tmp/new" },
      }),
    );
    renderModal();

    // Edit the SIM_DATA_DIR value field.
    const valueField = screen.getByDisplayValue("/tmp/old") as HTMLInputElement;
    fireEvent.change(valueField, { target: { value: "/tmp/new" } });

    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    await waitFor(() => {
      expect(screen.getByText(/Config saved/)).toBeInTheDocument();
    });
    expect(screen.getByText(/restart-required changes/i)).toBeInTheDocument();
    // Restart should be the primary, plus a Reload (partial) escape hatch.
    expect(screen.getByRole("button", { name: /^restart$/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /reload \(partial\)/i })).toBeInTheDocument();
  });

  it("save → only hot change → UI offers Reload only", async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse({
        project: "proj",
        instance: "sim-aws",
        hot_reloadable_changes: ["SIM_LOG_LEVEL"],
        restart_required_changes: [],
        config: { SIM_LOG_LEVEL: "debug", SIM_DATA_DIR: "/tmp/old" },
      }),
    );
    renderModal();

    const valueField = screen.getByDisplayValue("info") as HTMLInputElement;
    fireEvent.change(valueField, { target: { value: "debug" } });

    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    await waitFor(() => {
      expect(screen.getByText(/Config saved/)).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: /^reload$/i })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /^restart$/i }),
    ).not.toBeInTheDocument();
  });

  it("save → no diff → UI shows Close only", async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse({
        project: "proj",
        instance: "sim-aws",
        hot_reloadable_changes: [],
        restart_required_changes: [],
        config: instance.config,
      }),
    );
    renderModal();

    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    await waitFor(() => {
      expect(screen.getByText(/No changes detected/i)).toBeInTheDocument();
    });
    // The modal also has an aria-label="Close" X button in its header,
    // so match the text-only "Close" button via name="^Close$".
    const closeButtons = screen.getAllByRole("button", { name: /close/i });
    // X-button + the post-save Close primary = 2 expected; >2 indicates
    // a stray Reload/Restart leaking into this branch.
    expect(closeButtons.length).toBe(2);
    expect(screen.queryByRole("button", { name: /^reload$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^restart$/i })).not.toBeInTheDocument();
  });

  it("clicking Reload calls onReload + onClose", async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse({
        project: "proj",
        instance: "sim-aws",
        hot_reloadable_changes: ["SIM_LOG_LEVEL"],
        restart_required_changes: [],
        config: { SIM_LOG_LEVEL: "debug", SIM_DATA_DIR: "/tmp/old" },
      }),
    );
    const { onReload, onClose } = renderModal();

    const valueField = screen.getByDisplayValue("info") as HTMLInputElement;
    fireEvent.change(valueField, { target: { value: "debug" } });

    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /^reload$/i })).toBeInTheDocument(),
    );
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /^reload$/i }));
    });
    expect(onReload).toHaveBeenCalledWith("proj", "sim-aws");
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("save errors render the error message", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response("validate: bad", {
        status: 400,
        statusText: "Bad Request",
      }),
    );
    renderModal();

    const valueField = screen.getByDisplayValue("/tmp/old") as HTMLInputElement;
    fireEvent.change(valueField, { target: { value: "/tmp/new" } });

    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    // Error path: setError → inline error div + reportError → toast,
    // both contain the message. Either one shows the user, so just
    // assert at least one element matches.
    await waitFor(() => {
      const matches = screen.getAllByText(/Admin API error 400/);
      expect(matches.length).toBeGreaterThan(0);
    });
  });
});
