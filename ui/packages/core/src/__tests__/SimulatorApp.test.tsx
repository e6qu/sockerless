import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Route } from "react-router";
import { SimulatorApp } from "../components/SimulatorApp.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

function jsonResponse(data: unknown) {
  return new Response(JSON.stringify(data), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function renderApp(title: string, navItems: { label: string; to: string }[]) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <SimulatorApp title={title} navItems={navItems}>
        <Route path="/ui/" element={<div>overview</div>} />
        <Route path="/ui/tasks" element={<div>tasks</div>} />
      </SimulatorApp>
    </QueryClientProvider>,
  );
}

describe("SimulatorApp", () => {
  it("renders the title in the sidebar", () => {
    mockFetch.mockResolvedValue(jsonResponse({ status: "ok", provider: "aws" }));
    const { container } = renderApp("AWS Simulator", [
      { label: "Overview", to: "/ui/" },
      { label: "Tasks", to: "/ui/tasks" },
    ]);
    expect(container.textContent).toContain("AWS Simulator");
  });

  it("renders the provided nav items", () => {
    mockFetch.mockResolvedValue(jsonResponse({ status: "ok", provider: "gcp" }));
    const { container } = renderApp("GCP Simulator", [
      { label: "Overview", to: "/ui/" },
      { label: "Jobs", to: "/ui/jobs" },
      { label: "Functions", to: "/ui/functions" },
    ]);
    const links = container.querySelectorAll("a");
    const labels = Array.from(links).map((l) => l.textContent);
    expect(labels).toContain("Overview");
    expect(labels).toContain("Jobs");
    expect(labels).toContain("Functions");
  });
});
