import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { OAuthPage } from "../pages/OAuthPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown) {
  return new Response(JSON.stringify(data), {
    status: 200,
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
      <OAuthPage />
    </QueryClientProvider>,
  );
}

const oauthState = {
  deviceCodes: [
    {
      code: "abcdef0123",
      userCode: "BLEE-PHUB",
      scopes: "repo",
      userId: 1,
      expiresAt: "2026-01-01T00:15:00Z",
    },
  ],
  authCodes: [
    {
      code: "auth-1",
      clientId: "Iv1.test",
      redirectUri: "http://cb/",
      scopes: "repo",
      state: "S",
      userId: 1,
      createdAt: "2026-01-01T00:00:00Z",
      expiresAt: "2026-01-01T00:10:00Z",
    },
  ],
};

describe("OAuthPage", () => {
  it("renders the flow simulator + tables", async () => {
    mockFetch.mockResolvedValue(jsonResponse(oauthState));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("Flow simulator")).toBeInTheDocument();
      expect(screen.getByText(/Active device codes/)).toBeInTheDocument();
      expect(screen.getByText(/Active authorization codes/)).toBeInTheDocument();
    });
  });

  it("shows current device + auth codes", async () => {
    mockFetch.mockResolvedValue(jsonResponse(oauthState));
    renderPage();
    await waitFor(() => {
      expect(screen.getByText("BLEE-PHUB")).toBeInTheDocument();
      expect(screen.getByText("Iv1.test")).toBeInTheDocument();
    });
  });
});
