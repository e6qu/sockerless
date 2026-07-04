import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { OrgHooksPage } from "../pages/OrgHooksPage.js";

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
      <MemoryRouter initialEntries={["/ui/orgs/acme/hooks"]}>
        <Routes>
          <Route path="/ui/orgs/:org/hooks" element={<OrgHooksPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const orgHook = {
  id: 3,
  type: "Organization",
  name: "web",
  active: true,
  events: ["push", "issues"],
  config: { url: "https://ci.example.test/org-hook", content_type: "json", insecure_ssl: "0" },
  created_at: "2026-05-01T00:00:00Z",
  updated_at: "2026-05-01T00:00:00Z",
  url: "/api/v3/orgs/acme/hooks/3",
  ping_url: "/api/v3/orgs/acme/hooks/3/pings",
  deliveries_url: "/api/v3/orgs/acme/hooks/3/deliveries",
};

describe("OrgHooksPage", () => {
  it("lists organization webhooks with a deliveries link", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.startsWith("/api/v3/orgs/acme/hooks?")) {
        return Promise.resolve(jsonResponse([orgHook]));
      }
      return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
    });
    renderPage();

    await waitFor(() => expect(screen.getByText("#3")).toBeInTheDocument());
    expect(screen.getByText(/https:\/\/ci\.example\.test\/org-hook/)).toBeInTheDocument();
    expect(screen.getByText(/events: push, issues/)).toBeInTheDocument();
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/v3/orgs/acme/hooks?per_page=30",
      expect.anything(),
    );

    const link = screen.getByRole("link", { name: /deliveries/i });
    expect(link).toHaveAttribute("href", "/ui/orgs/acme/hooks/3/deliveries");
  });

  it("shows an honest empty state when the org has no webhooks", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.startsWith("/api/v3/orgs/acme/hooks?")) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
    });
    renderPage();

    await waitFor(() =>
      expect(screen.getByText("No organization webhooks")).toBeInTheDocument(),
    );
  });

  it("surfaces list errors instead of swallowing them", async () => {
    mockFetch.mockImplementation(() =>
      Promise.resolve(jsonResponse({ message: "boom" }, 500)),
    );
    renderPage();

    await waitFor(() =>
      expect(screen.getByText(/failed to load organization webhooks/i)).toBeInTheDocument(),
    );
  });
});
