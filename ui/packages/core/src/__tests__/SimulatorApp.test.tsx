import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, waitFor } from "@testing-library/react";
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
  window.history.pushState({}, "", "/ui/");
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
    mockFetch.mockResolvedValue(jsonResponse({ identityEndpoint: "", logoutEndpoint: "" }));
    const { container } = renderApp("AWS Simulator", [
      { label: "Overview", to: "/ui/" },
      { label: "Tasks", to: "/ui/tasks" },
    ]);
    expect(container.textContent).toContain("AWS Simulator");
  });

  it("renders the provided nav items", () => {
    mockFetch.mockResolvedValue(jsonResponse({ identityEndpoint: "", logoutEndpoint: "" }));
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

  it("shows the authenticated operator and local sign-out control", async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse({
        identityEndpoint: "/oauth2/userinfo",
        logoutEndpoint: "/oauth2/sign_out?rd=%2F",
      }))
      .mockResolvedValueOnce(jsonResponse({
        name: "Ada Lovelace",
        email: "ada@example.test",
      }));
    const { container } = renderApp("AWS Simulator", [{ label: "Overview", to: "/ui/" }]);

    await waitFor(() => expect(container.textContent).toContain("Ada Lovelace"));
    expect(container.textContent).toContain("ada@example.test");
    const signOut = container.querySelector<HTMLAnchorElement>('a[aria-label="Sign out Ada Lovelace"]');
    expect(signOut?.getAttribute("href")).toBe("/oauth2/sign_out?rd=%2F");
  });

  it("reports a configured identity endpoint failure", async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse({
        identityEndpoint: "/oauth2/userinfo",
        logoutEndpoint: "/oauth2/sign_out",
      }))
      .mockResolvedValueOnce(new Response("unauthorized", { status: 401 }));
    const { container } = renderApp("GCP Simulator", [{ label: "Overview", to: "/ui/" }]);

    await waitFor(() => expect(container.textContent).toContain("Signed-in identity is unavailable."));
  });
});
