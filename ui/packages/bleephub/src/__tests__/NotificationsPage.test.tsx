import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { NotificationsPage } from "../pages/NotificationsPage.js";

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
      <BrowserRouter>
        <NotificationsPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const thread = {
  id: "t1",
  repository: { full_name: "admin/repo" },
  subject: { title: "Issue title", url: "/api/v3/repos/admin/repo/issues/1", latest_comment_url: "", type: "Issue" },
  reason: "subscribed",
  unread: true,
  updated_at: "2026-01-01T00:00:00Z",
  last_read_at: null,
  subscription_url: "/api/v3/notifications/threads/t1/subscription",
  url: "/api/v3/notifications/threads/t1",
};

function mockEndpoints() {
  mockFetch.mockImplementation((url: string, init?: RequestInit) => {
    if (url === "/api/v3/notifications") return Promise.resolve(jsonResponse([thread]));
    if (url === "/api/v3/notifications/threads/t1/subscription") {
      if (init?.method === "DELETE") return Promise.resolve(new Response(null, { status: 204 }));
      return Promise.resolve(
        jsonResponse({
          subscribed: true,
          ignored: false,
          reason: "subscribed",
          created_at: "2026-01-01T00:00:00Z",
          url: "/api/v3/notifications/threads/t1/subscription",
          thread_url: "/api/v3/notifications/threads/t1/subscription",
        }),
      );
    }
    if (url === "/api/v3/notifications/threads/t1" && init?.method === "PATCH") {
      return Promise.resolve(new Response(null, { status: 205 }));
    }
    return Promise.resolve(jsonResponse({}));
  });
}

describe("NotificationsPage", () => {
  it("renders unread notifications", async () => {
    mockEndpoints();
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Notifications")).toBeInTheDocument();
      expect(screen.getByText("Issue title")).toBeInTheDocument();
    });
  });

  it("switches to all notifications", async () => {
    mockEndpoints();
    renderPage();
    await waitFor(() => expect(screen.getByText("Issue title")).toBeInTheDocument());
    fireEvent.click(screen.getByText("All"));
    await waitFor(() => {
      expect(screen.getByText("All")).toBeInTheDocument();
      expect(screen.getByText("Issue title")).toBeInTheDocument();
    });
  });

  it("marks a thread read", async () => {
    mockEndpoints();
    renderPage();
    await waitFor(() => expect(screen.getByText("Mark read")).toBeInTheDocument());
    fireEvent.click(screen.getByText("Mark read"));
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/v3/notifications/threads/t1",
        expect.objectContaining({ method: "PATCH" }),
      );
    });
  });

  it("opens subscription dialog", async () => {
    mockEndpoints();
    renderPage();
    await waitFor(() => expect(screen.getByText("Subscription")).toBeInTheDocument());
    fireEvent.click(screen.getByText("Subscription"));
    await waitFor(() => {
      expect(screen.getByText("Thread subscription")).toBeInTheDocument();
      expect(screen.getByText("Subscribed:")).toBeInTheDocument();
    });
  });
});
