import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { ReposPage } from "../pages/ReposPage.js";

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
        <ReposPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const reposData = [
  {
    id: 1,
    name: "test",
    full_name: "admin/test",
    description: "a repo",
    default_branch: "main",
    visibility: "public",
    private: false,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-02T00:00:00Z",
  },
];

describe("ReposPage", () => {
  it("renders the repo list", async () => {
    mockFetch.mockResolvedValue(jsonResponse(reposData));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("admin/test")).toBeInTheDocument();
    });
  });

  it("shows an error state instead of spinning when /internal/repos fails", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ message: "boom" }, 500));
    renderPage();
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
      expect(screen.getByText(/failed to load repositories/i)).toBeInTheDocument();
    });
    expect(screen.queryByText(/loading repos/i)).not.toBeInTheDocument();
  });

  it("opens the create dialog and submits POST /api/v3/user/repos", async () => {
    mockFetch.mockResolvedValue(jsonResponse(reposData));
    renderPage();
    await waitFor(() => screen.getByText("admin/test"));

    fireEvent.click(screen.getByRole("button", { name: /new repository/i }));
    await waitFor(() =>
      expect(screen.getByRole("heading", { name: /create a new repository/i })).toBeInTheDocument(),
    );

    fireEvent.change(screen.getByLabelText(/repository name/i), { target: { value: "new-repo" } });
    fireEvent.change(screen.getByLabelText(/description/i), { target: { value: "My new repo" } });

    mockFetch.mockResolvedValueOnce(
      jsonResponse({
        id: 2,
        name: "new-repo",
        full_name: "admin/new-repo",
        description: "My new repo",
        default_branch: "main",
        visibility: "public",
        private: false,
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: /create repository/i }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenLastCalledWith(
        "/api/v3/user/repos",
        expect.objectContaining({
          method: "POST",
          body: expect.stringContaining("new-repo"),
        }),
      );
    });
  });
});
