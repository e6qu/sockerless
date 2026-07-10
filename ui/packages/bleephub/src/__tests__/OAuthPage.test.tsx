import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
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
  vi.restoreAllMocks();
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

describe("OAuthPage", () => {
  it("renders OAuth flow controls without reading internal OAuth state", () => {
    renderPage();
    expect(screen.getAllByText(/OAuth flow controls/i).length).toBeGreaterThan(0);
    expect(screen.queryByText(/active device codes/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/active authorization codes/i)).not.toBeInTheDocument();
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("starts device flow through the GitHub device-code endpoint", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ device_code: "device-123", user_code: "ABCD-EFGH" }));
    renderPage();
    fireEvent.change(screen.getByLabelText("Client identifier"), { target: { value: "Iv1.client" } });
    fireEvent.click(screen.getByRole("button", { name: "Device flow" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });
    const [url, opts] = mockFetch.mock.calls[0];
    expect(url).toBe("/login/device/code");
    expect(opts).toMatchObject({ method: "POST" });
    const body = new URLSearchParams(String(opts.body));
    expect(body.get("client_id")).toBe("Iv1.client");
    expect(body.get("scope")).toBe("repo read:org");
    expect(screen.getByText(/ABCD-EFGH/)).toBeInTheDocument();
  });

  it("polls the shared OAuth access-token endpoint for a device token", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ access_token: "gho_token", token_type: "bearer", scope: "repo" }));
    renderPage();
    fireEvent.change(screen.getByLabelText("Client identifier"), { target: { value: "Iv1.client" } });
    fireEvent.change(screen.getByLabelText("Device code"), { target: { value: "device-123" } });
    fireEvent.click(screen.getByRole("button", { name: "Poll device token" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });
    const [url, opts] = mockFetch.mock.calls[0];
    expect(url).toBe("/login/oauth/access_token");
    expect(opts).toMatchObject({ method: "POST" });
    expect(opts.headers).toMatchObject({ Accept: "application/json" });
    const body = new URLSearchParams(String(opts.body));
    expect(body.get("client_id")).toBe("Iv1.client");
    expect(body.get("device_code")).toBe("device-123");
  });

  it("opens the GitHub OAuth authorize endpoint for web flow", () => {
    const openSpy = vi.spyOn(window, "open").mockReturnValue(null);
    renderPage();
    fireEvent.change(screen.getByLabelText("Client identifier"), { target: { value: "Iv1.client" } });
    fireEvent.click(screen.getByRole("button", { name: "Web flow" }));

    expect(openSpy).toHaveBeenCalledWith(
      "/login/oauth/authorize?client_id=Iv1.client&redirect_uri=http%3A%2F%2Flocalhost%3A8080%2Fcallback&scope=repo%20read%3Aorg&state=STATE-1",
      "_blank",
      "noopener",
    );
  });
});
