import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AppsPage } from "../pages/AppsPage.js";

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
      <AppsPage />
    </QueryClientProvider>,
  );
}

const apps = [
  {
    id: 1,
    slug: "ci-bot",
    name: "CI Bot",
    description: "Helper",
    ownerId: 1,
    createdAt: "2026-01-01T00:00:00Z",
  },
];

const installations = [
  {
    id: 100,
    appId: 1,
    appSlug: "ci-bot",
    targetType: "User",
    targetLogin: "octocat",
    repositorySelection: "all",
    createdAt: "2026-01-01T00:00:00Z",
  },
];

function routedFetch(url: RequestInfo | URL): Promise<Response> {
  const u = typeof url === "string" ? url : url.toString();
  if (u.includes("/internal/apps")) return Promise.resolve(jsonResponse(apps));
  if (u.includes("/internal/installations")) return Promise.resolve(jsonResponse(installations));
  return Promise.resolve(jsonResponse([]));
}

describe("AppsPage", () => {
  it("renders Apps and Installations tabs", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Apps" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Installations" })).toBeInTheDocument();
    });
  });

  it("Apps tab shows the app rows", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("ci-bot")).toBeInTheDocument();
      expect(screen.getByText("CI Bot")).toBeInTheDocument();
    });
  });

  it("Installations tab shows installation rows", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("ci-bot")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: "Installations" }));
    await waitFor(() => {
      expect(screen.getByText("octocat")).toBeInTheDocument();
    });
  });

  it("opens the Create App dialog", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL) => routedFetch(url));
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Create App" })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: "Create App" }));
    await waitFor(() => {
      expect(screen.getByLabelText("Name")).toBeInTheDocument();
      expect(screen.getByLabelText("Description")).toBeInTheDocument();
    });
  });
});
