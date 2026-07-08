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
  fireEvent.change(screen.getByLabelText(/admin token/i), { target: { value: token } });
  fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
}

describe("LoginPage", () => {
  it("verifies against /internal/status and signs in on success", async () => {
    mockFetch.mockResolvedValue(new Response(JSON.stringify({}), { status: 200 }));
    submitToken("ghp_validpat");
    await waitFor(() => {
      expect(window.location.href).toBe("/ui/");
    });
    // verification went to the operator surface the dashboard actually uses
    const [url, opts] = mockFetch.mock.calls[0];
    expect(url.toString()).toBe("/internal/status");
    expect((opts.headers as Record<string, string>).Authorization).toBe("Bearer ghp_validpat");
    expect(getToken()).toBe("ghp_validpat");
  });

  it("rejects a gho_ token at login with a personal-access-token or admin-token message", async () => {
    // /internal/* only accepts personal access tokens. An OAuth token gets a
    // 401 here even though /api/v3/user would have accepted it.
    mockFetch.mockResolvedValue(
      new Response(JSON.stringify({ message: "Requires authentication" }), { status: 401 }),
    );
    submitToken("gho_oauthtoken");
    await waitFor(() => {
      expect(screen.getByText(/personal access token or the admin token/i)).toBeInTheDocument();
    });
    expect(window.location.href).toBe("");
    expect(getToken()).toBeNull();
  });
});
