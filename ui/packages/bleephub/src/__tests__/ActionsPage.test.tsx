import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { ActionsPage } from "../pages/ActionsPage.js";
import { parseWorkflowDispatch } from "../utils/workflowDispatch.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
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
      <MemoryRouter initialEntries={["/ui/repos/admin/test/actions"]}>
        <Routes>
          <Route path="/ui/repos/:owner/:repo/actions" element={<ActionsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function run(id: number, name: string, overrides: Record<string, unknown> = {}) {
  return {
    id,
    name,
    run_number: id,
    run_attempt: 1,
    event: "push",
    status: "completed",
    conclusion: "success",
    head_branch: "main",
    head_sha: "abc1234567890",
    path: ".github/workflows/ci.yml",
    workflow_id: 10,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    actor: { login: "admin" },
    ...overrides,
  };
}

const workflowsData = {
  total_count: 2,
  workflows: [
    {
      id: 10,
      name: "CI",
      path: ".github/workflows/ci.yml",
      state: "active",
      badge_url: "http://localhost/admin/test/actions/workflows/ci.yml/badge.svg",
    },
    {
      id: 11,
      name: "Deploy",
      path: ".github/workflows/deploy.yml",
      state: "disabled_manually",
      badge_url: "http://localhost/admin/test/actions/workflows/deploy.yml/badge.svg",
    },
  ],
};

const ciYaml = `name: CI
on:
  workflow_dispatch:
    inputs:
      env_name:
        description: Environment name
        required: true
        default: staging
      log_level:
        type: choice
        options:
          - debug
          - info
        default: info
      dry_run:
        type: boolean
        default: true
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`;

function contentsResponse(yaml: string) {
  return {
    name: "ci.yml",
    path: ".github/workflows/ci.yml",
    sha: "deadbeef",
    type: "file",
    encoding: "base64",
    content: Buffer.from(yaml, "utf-8").toString("base64"),
  };
}

/** URL-routed mocks for the actions surface; returns nothing extra for counts. */
function installMocks({ yaml = ciYaml } = {}) {
  mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
    const u = url.toString();
    const method = init?.method ?? "GET";
    if (u.includes("/issues") || u.includes("/pulls")) return Promise.resolve(jsonResponse([]));
    if (u.includes("/actions/workflows/10/runs")) {
      return Promise.resolve(
        jsonResponse({ total_count: 1, workflow_runs: [run(3, "CI", { event: "workflow_dispatch" })] }),
      );
    }
    if (u.includes("/actions/workflows/11/runs")) {
      return Promise.resolve(jsonResponse({ total_count: 0, workflow_runs: [] }));
    }
    if (method === "PUT" && (u.endsWith("/disable") || u.endsWith("/enable"))) {
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (method === "POST" && u.endsWith("/dispatches")) {
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (u.includes("/actions/workflows")) return Promise.resolve(jsonResponse(workflowsData));
    if (u.includes("/contents/")) return Promise.resolve(jsonResponse(contentsResponse(yaml)));
    if (u.includes("/actions/runs")) {
      return Promise.resolve(
        jsonResponse({
          total_count: 2,
          workflow_runs: [
            run(2, "CI", { status: "in_progress", conclusion: null }),
            run(1, "CI", { conclusion: "failure" }),
          ],
        }),
      );
    }
    return Promise.resolve(jsonResponse([]));
  });
}

describe("ActionsPage runs list", () => {
  it("renders runs with status icons, branch, event and total count", async () => {
    installMocks();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("2 workflow runs")).toBeInTheDocument();
    });
    expect(screen.getAllByText("CI").length).toBeGreaterThanOrEqual(2);
    expect(screen.getByTitle("in progress")).toBeInTheDocument();
    expect(screen.getByTitle("failure")).toBeInTheDocument();
    expect(screen.getAllByText("main").length).toBeGreaterThan(0);
    const calls = mockFetch.mock.calls.map((c) => c[0].toString());
    expect(calls.some((c) => c.startsWith("/api/v3/repos/admin/test/actions/runs?"))).toBe(true);
  });

  it("applies the status filter as a query param", async () => {
    installMocks();
    renderPage();
    await waitFor(() => expect(screen.getByText("2 workflow runs")).toBeInTheDocument());
    fireEvent.change(screen.getByLabelText("Status"), { target: { value: "completed" } });
    await waitFor(() => {
      const calls = mockFetch.mock.calls.map((c) => c[0].toString());
      expect(calls.some((c) => c.includes("/actions/runs?") && c.includes("status=completed"))).toBe(true);
    });
  });

  it("filters runs by workflow via the sidebar (workflows/{id}/runs)", async () => {
    installMocks();
    renderPage();
    await waitFor(() => expect(screen.getByRole("button", { name: "CI" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "CI" }));
    await waitFor(() => {
      const calls = mockFetch.mock.calls.map((c) => c[0].toString());
      expect(calls.some((c) => c.includes("/actions/workflows/10/runs"))).toBe(true);
    });
  });
});

describe("ActionsPage workflow dispatch", () => {
  it("builds the dispatch form from parsed YAML inputs and POSTs ref + inputs", async () => {
    installMocks();
    renderPage();
    await waitFor(() => expect(screen.getByRole("button", { name: "CI" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "CI" }));

    // The Run workflow button appears once the YAML's workflow_dispatch is parsed.
    const runBtn = await screen.findByRole("button", { name: "Run workflow" });
    fireEvent.click(runBtn);

    // Form fields from on.workflow_dispatch.inputs, defaults prefilled.
    const envInput = await screen.findByLabelText(/environment name \*/i);
    expect(envInput).toHaveValue("staging");
    const choice = screen.getByLabelText("log_level") as HTMLSelectElement;
    expect(choice.value).toBe("info");
    expect(screen.getByRole("option", { name: "debug" })).toBeInTheDocument();
    const checkbox = screen.getByRole("checkbox") as HTMLInputElement;
    expect(checkbox.checked).toBe(true);

    fireEvent.change(envInput, { target: { value: "production" } });
    fireEvent.change(choice, { target: { value: "debug" } });
    fireEvent.click(checkbox);

    // Two "Run workflow" buttons exist now (header + modal submit) — the
    // modal's submit is the last in document order.
    const submitButtons = screen.getAllByRole("button", { name: /^run workflow$/i });
    fireEvent.click(submitButtons[submitButtons.length - 1]);
    await waitFor(() => {
      const dispatchCall = mockFetch.mock.calls.find(
        (c) => c[0].toString().endsWith("/actions/workflows/10/dispatches") && c[1]?.method === "POST",
      );
      expect(dispatchCall).toBeTruthy();
      const body = JSON.parse(dispatchCall![1]!.body as string);
      expect(body).toEqual({
        ref: "main",
        inputs: { env_name: "production", log_level: "debug", dry_run: "false" },
      });
    });
  });

  it("offers no Run workflow button when the YAML has no workflow_dispatch trigger", async () => {
    installMocks({ yaml: "name: CI\non: push\njobs: {}\n" });
    renderPage();
    await waitFor(() => expect(screen.getByRole("button", { name: "CI" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "CI" }));
    // Wait for the contents fetch to settle, then assert absence.
    await waitFor(() => {
      const calls = mockFetch.mock.calls.map((c) => c[0].toString());
      expect(calls.some((c) => c.includes("/contents/"))).toBe(true);
    });
    expect(screen.queryByRole("button", { name: "Run workflow" })).not.toBeInTheDocument();
  });
});

describe("ActionsPage enable/disable", () => {
  it("disables an active workflow via PUT .../disable", async () => {
    installMocks();
    renderPage();
    await waitFor(() => expect(screen.getByRole("button", { name: "CI" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "CI" }));
    fireEvent.click(await screen.findByRole("button", { name: /workflow options/i }));
    fireEvent.click(await screen.findByRole("menuitem", { name: /disable workflow/i }));
    await waitFor(() => {
      const call = mockFetch.mock.calls.find(
        (c) => c[0].toString().endsWith("/actions/workflows/10/disable") && c[1]?.method === "PUT",
      );
      expect(call).toBeTruthy();
    });
  });

  it("shows the disabled note and enables via PUT .../enable", async () => {
    installMocks();
    renderPage();
    await waitFor(() => expect(screen.getByRole("button", { name: "Deploy" })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Deploy" }));
    expect(await screen.findByText(/this workflow was disabled/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /workflow options/i }));
    fireEvent.click(await screen.findByRole("menuitem", { name: /enable workflow/i }));
    await waitFor(() => {
      const call = mockFetch.mock.calls.find(
        (c) => c[0].toString().endsWith("/actions/workflows/11/enable") && c[1]?.method === "PUT",
      );
      expect(call).toBeTruthy();
    });
  });
});

describe("parseWorkflowDispatch", () => {
  it("detects scalar, list and map trigger shapes", () => {
    expect(parseWorkflowDispatch("on: workflow_dispatch\n").hasDispatch).toBe(true);
    expect(parseWorkflowDispatch("on: push\n").hasDispatch).toBe(false);
    expect(parseWorkflowDispatch("on: [push, workflow_dispatch]\n").hasDispatch).toBe(true);
    expect(parseWorkflowDispatch("on:\n  push:\n").hasDispatch).toBe(false);
    expect(parseWorkflowDispatch("on:\n  workflow_dispatch:\n").hasDispatch).toBe(true);
  });

  it("normalizes input definitions", () => {
    const spec = parseWorkflowDispatch(ciYaml);
    expect(spec.hasDispatch).toBe(true);
    expect(spec.inputs.env_name).toEqual({
      description: "Environment name",
      required: true,
      default: "staging",
    });
    expect(spec.inputs.log_level.options).toEqual(["debug", "info"]);
    expect(spec.inputs.dry_run.type).toBe("boolean");
    expect(spec.inputs.dry_run.default).toBe(true);
  });

  it("treats unparsable YAML as no dispatch trigger", () => {
    expect(parseWorkflowDispatch(":::: not yaml {").hasDispatch).toBe(false);
  });
});
