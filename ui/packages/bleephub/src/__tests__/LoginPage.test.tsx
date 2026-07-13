import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { LoginPage } from "../pages/LoginPage.js";
import { clearToken, getToken } from "../api.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

// LoginPage navigates by assigning window.location.href; jsdom can't
// navigate, so swap in a writable stub and assert on it.
const originalLocation = window.location;
beforeEach(() => {
  const stub = { ...originalLocation, href: "" };
  Object.defineProperty(window, "location", { value: stub, writable: true, configurable: true });
});

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
  clearToken();
  Object.defineProperty(window, "location", {
    value: originalLocation,
    writable: true,
    configurable: true,
  });
});

function submitToken(token: string) {
  render(<LoginPage />);
  fireEvent.change(screen.getByLabelText(/access token/i), { target: { value: token } });
  fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
}

describe("LoginPage", () => {
  it("verifies against GitHub REST identity and signs in on success", async () => {
    mockFetch
      .mockResolvedValueOnce(new Response(JSON.stringify({ github: true }), { status: 200 }))
      .mockResolvedValue(new Response(JSON.stringify({ login: "octocat" }), { status: 200 }));
    submitToken("ghp_validpat");
    await waitFor(() => {
      expect(window.location.href).toBe("/ui/");
    });
    const [url, opts] = mockFetch.mock.calls.find(([url]) => url.toString() === "/api/v3/user")!;
    expect(url.toString()).toBe("/api/v3/user");
    expect((opts.headers as Record<string, string>).Authorization).toBe("Bearer ghp_validpat");
    expect(getToken()).toBe("ghp_validpat");
  });

  it("accepts an OAuth token when GitHub REST identity accepts it", async () => {
    mockFetch
      .mockResolvedValueOnce(new Response(JSON.stringify({ github: true }), { status: 200 }))
      .mockResolvedValue(new Response(JSON.stringify({ login: "octocat" }), { status: 200 }));
    submitToken("gho_oauthtoken");
    await waitFor(() => {
      expect(window.location.href).toBe("/ui/");
    });
    expect(mockFetch.mock.calls.some(([url]) => url.toString() === "/api/v3/user")).toBe(true);
    expect(getToken()).toBe("gho_oauthtoken");
  });

  it("rejects a token when GitHub REST identity rejects it", async () => {
    mockFetch
      .mockResolvedValueOnce(new Response(JSON.stringify({ github: true }), { status: 200 }))
      .mockResolvedValue(new Response(JSON.stringify({ message: "Requires authentication" }), { status: 401 }));
    submitToken("bad-token");
    await waitFor(() => {
      expect(screen.getByText(/GitHub REST user endpoint/i)).toBeInTheDocument();
    });
    expect(window.location.href).toBe("");
    expect(getToken()).toBeNull();
  });
});
