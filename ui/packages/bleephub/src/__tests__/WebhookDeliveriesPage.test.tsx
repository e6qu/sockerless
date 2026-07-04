import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { WebhookDeliveriesPage } from "../pages/WebhookDeliveriesPage.js";

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

function renderRepoScoped() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/ui/repos/admin/hook-repo/hooks/5/deliveries"]}>
        <Routes>
          <Route
            path="/ui/repos/:owner/:repo/hooks/:hookId/deliveries"
            element={<WebhookDeliveriesPage />}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderOrgScoped() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/ui/orgs/acme/hooks/3/deliveries"]}>
        <Routes>
          <Route path="/ui/orgs/:org/hooks/:hookId/deliveries" element={<WebhookDeliveriesPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const deliverySummary = {
  id: 101,
  guid: "9a8b7c6d-0000-0000-0000-000000000000",
  delivered_at: "2026-06-02T10:00:00Z",
  redelivery: false,
  duration: 0.27,
  status: "OK",
  status_code: 200,
  event: "push",
  action: null,
  installation_id: null,
  repository_id: 1,
  throttled_at: null,
};

const deliveryDetail = {
  ...deliverySummary,
  url: "https://ci.example.test/webhook",
  request: {
    headers: { "Content-Type": "application/json", "X-GitHub-Event": "push" },
    payload: { ref: "refs/heads/main", repository: { full_name: "admin/hook-repo" } },
  },
  response: { headers: { Server: "ci" }, payload: '{"ok":true}' },
};

describe("WebhookDeliveriesPage", () => {
  it("lists repo hook deliveries with status badges", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.startsWith("/api/v3/repos/admin/hook-repo/hooks/5/deliveries?")) {
        return Promise.resolve(
          jsonResponse([
            deliverySummary,
            { ...deliverySummary, id: 102, status: "502 Bad Gateway", status_code: 502 },
          ]),
        );
      }
      return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
    });
    renderRepoScoped();

    await waitFor(() => expect(screen.getByText("OK")).toBeInTheDocument());
    expect(screen.getByText("502 Bad Gateway")).toBeInTheDocument();
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/v3/repos/admin/hook-repo/hooks/5/deliveries?per_page=30",
      expect.anything(),
    );
  });

  it("expands a delivery to the request/response payloads and redelivers", async () => {
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      if (url.startsWith("/api/v3/repos/admin/hook-repo/hooks/5/deliveries?")) {
        return Promise.resolve(jsonResponse([deliverySummary]));
      }
      if (url === "/api/v3/repos/admin/hook-repo/hooks/5/deliveries/101") {
        return Promise.resolve(jsonResponse(deliveryDetail));
      }
      if (
        url === "/api/v3/repos/admin/hook-repo/hooks/5/deliveries/101/attempts" &&
        init?.method === "POST"
      ) {
        return Promise.resolve(new Response(null, { status: 202 }));
      }
      return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
    });
    renderRepoScoped();

    await waitFor(() => screen.getByText(deliverySummary.guid));
    fireEvent.click(screen.getByText(deliverySummary.guid));

    // Payload is pretty-printed JSON (multi-line, indented).
    await waitFor(() =>
      expect(screen.getByText(/"ref": "refs\/heads\/main"/)).toBeInTheDocument(),
    );
    expect(screen.getByText(/X-GitHub-Event: push/)).toBeInTheDocument();
    expect(screen.getByText(/"ok": true/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /redeliver/i }));
    await waitFor(() => {
      const postCall = mockFetch.mock.calls.find(
        (call) =>
          call[0] === "/api/v3/repos/admin/hook-repo/hooks/5/deliveries/101/attempts" &&
          call[1]?.method === "POST",
      );
      expect(postCall).toBeDefined();
    });
    expect(await screen.findByText(/redelivery queued/i)).toBeInTheDocument();
  });

  it("uses the org hook endpoints for org-scoped routes", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.startsWith("/api/v3/orgs/acme/hooks/3/deliveries?")) {
        return Promise.resolve(jsonResponse([deliverySummary]));
      }
      return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
    });
    renderOrgScoped();

    await waitFor(() => expect(screen.getByText(deliverySummary.guid)).toBeInTheDocument());
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/v3/orgs/acme/hooks/3/deliveries?per_page=30",
      expect.anything(),
    );
  });
});
