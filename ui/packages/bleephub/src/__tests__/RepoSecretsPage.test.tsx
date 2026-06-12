import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import sodium from "libsodium-wrappers";
import { RepoSecretsPage } from "../pages/RepoSecretsPage.js";

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
      <MemoryRouter initialEntries={["/ui/repos/admin/test/settings/secrets"]}>
        <Routes>
          <Route path="/ui/repos/:owner/:repo/settings/secrets" element={<RepoSecretsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const repoSecrets = {
  total_count: 1,
  secrets: [
    { name: "NPM_TOKEN", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-02T00:00:00Z" },
  ],
};

const repoVariables = {
  total_count: 1,
  variables: [
    {
      name: "NODE_ENV",
      value: "production",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-02T00:00:00Z",
    },
  ],
};

const orgSecrets = {
  total_count: 1,
  secrets: [
    {
      name: "ORG_TOKEN",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-02T00:00:00Z",
      visibility: "all",
    },
  ],
};

/**
 * Build mocks around a real X25519 keypair so the test can sealed-box
 * decrypt whatever the page uploads (libsodium runs fine under node).
 */
async function installMocks() {
  await sodium.ready;
  const keypair = sodium.crypto_box_keypair();
  const publicKey = {
    key_id: "568250167242549743",
    key: sodium.to_base64(keypair.publicKey, sodium.base64_variants.ORIGINAL),
  };
  mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
    const u = url.toString();
    const method = init?.method ?? "GET";
    if (u.includes("/issues") || u.includes("/pulls")) return Promise.resolve(jsonResponse([]));
    if (u.endsWith("/secrets/public-key")) return Promise.resolve(jsonResponse(publicKey));
    if (method === "PUT" && u.includes("/secrets/")) {
      return Promise.resolve(new Response(null, { status: 201 }));
    }
    if (method === "DELETE") return Promise.resolve(new Response(null, { status: 204 }));
    if (method === "POST" || method === "PATCH") {
      return Promise.resolve(new Response(null, { status: 201 }));
    }
    if (u.startsWith("/api/v3/orgs/admin/actions/secrets")) {
      return Promise.resolve(jsonResponse(orgSecrets));
    }
    if (u.startsWith("/api/v3/orgs/admin/actions/variables")) {
      return Promise.resolve(jsonResponse({ total_count: 0, variables: [] }));
    }
    if (u.includes("/actions/secrets")) return Promise.resolve(jsonResponse(repoSecrets));
    if (u.includes("/actions/variables")) return Promise.resolve(jsonResponse(repoVariables));
    return Promise.resolve(jsonResponse([]));
  });
  return keypair;
}

describe("RepoSecretsPage secrets", () => {
  it("lists secret names without values", async () => {
    await installMocks();
    renderPage();
    expect(await screen.findByText("NPM_TOKEN")).toBeInTheDocument();
    expect(screen.getByText(/updated 1\/2\/2026|updated 02\/01\/2026|updated/)).toBeInTheDocument();
  });

  it("sealed-box encrypts a new secret against the scope public key", async () => {
    const keypair = await installMocks();
    renderPage();
    await screen.findByText("NPM_TOKEN");

    fireEvent.click(screen.getByRole("button", { name: /new secret/i }));
    fireEvent.change(await screen.findByLabelText("Name"), { target: { value: "MY_SECRET" } });
    fireEvent.change(screen.getByLabelText("Value"), { target: { value: "super-plain-text" } });
    fireEvent.click(screen.getByRole("button", { name: /add secret/i }));

    await waitFor(() => {
      const call = mockFetch.mock.calls.find(
        (c) =>
          c[0].toString() === "/api/v3/repos/admin/test/actions/secrets/MY_SECRET" &&
          c[1]?.method === "PUT",
      );
      expect(call).toBeTruthy();
      const rawBody = call![1]!.body as string;
      // The plaintext must never appear on the wire.
      expect(rawBody).not.toContain("super-plain-text");
      const body = JSON.parse(rawBody);
      expect(body.key_id).toBe("568250167242549743");
      const opened = sodium.crypto_box_seal_open(
        sodium.from_base64(body.encrypted_value, sodium.base64_variants.ORIGINAL),
        keypair.publicKey,
        keypair.privateKey,
      );
      expect(sodium.to_string(opened)).toBe("super-plain-text");
    });
  });

  it("deletes a secret via DELETE", async () => {
    await installMocks();
    renderPage();
    await screen.findByText("NPM_TOKEN");
    fireEvent.click(screen.getAllByRole("button", { name: /^delete$/i })[0]);
    await waitFor(() => {
      const call = mockFetch.mock.calls.find(
        (c) =>
          c[0].toString() === "/api/v3/repos/admin/test/actions/secrets/NPM_TOKEN" &&
          c[1]?.method === "DELETE",
      );
      expect(call).toBeTruthy();
    });
  });
});

describe("RepoSecretsPage variables", () => {
  it("lists variables with their values", async () => {
    await installMocks();
    renderPage();
    expect(await screen.findByText("NODE_ENV")).toBeInTheDocument();
    expect(screen.getByText("production")).toBeInTheDocument();
  });

  it("creates a variable via POST {name, value}", async () => {
    await installMocks();
    renderPage();
    await screen.findByText("NODE_ENV");
    fireEvent.click(screen.getByRole("button", { name: /new variable/i }));
    fireEvent.change(await screen.findByLabelText("Name"), { target: { value: "REGION" } });
    fireEvent.change(screen.getByLabelText("Value"), { target: { value: "eu-west-1" } });
    fireEvent.click(screen.getByRole("button", { name: /add variable/i }));
    await waitFor(() => {
      const call = mockFetch.mock.calls.find(
        (c) =>
          c[0].toString() === "/api/v3/repos/admin/test/actions/variables" &&
          c[1]?.method === "POST",
      );
      expect(call).toBeTruthy();
      expect(JSON.parse(call![1]!.body as string)).toEqual({ name: "REGION", value: "eu-west-1" });
    });
  });

  it("edits a variable via PATCH .../variables/{name}", async () => {
    await installMocks();
    renderPage();
    await screen.findByText("NODE_ENV");
    fireEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    fireEvent.change(await screen.findByLabelText("Value"), { target: { value: "staging" } });
    fireEvent.click(screen.getByRole("button", { name: /update variable/i }));
    await waitFor(() => {
      const call = mockFetch.mock.calls.find(
        (c) =>
          c[0].toString() === "/api/v3/repos/admin/test/actions/variables/NODE_ENV" &&
          c[1]?.method === "PATCH",
      );
      expect(call).toBeTruthy();
      expect(JSON.parse(call![1]!.body as string)).toEqual({ name: "NODE_ENV", value: "staging" });
    });
  });

  it("deletes a variable via DELETE", async () => {
    await installMocks();
    renderPage();
    await screen.findByText("NODE_ENV");
    // Two delete buttons exist (secret + variable) — the variable one is second.
    fireEvent.click(screen.getAllByRole("button", { name: /^delete$/i })[1]);
    await waitFor(() => {
      const call = mockFetch.mock.calls.find(
        (c) =>
          c[0].toString() === "/api/v3/repos/admin/test/actions/variables/NODE_ENV" &&
          c[1]?.method === "DELETE",
      );
      expect(call).toBeTruthy();
    });
  });
});

describe("RepoSecretsPage org scope", () => {
  it("reads org secrets from /orgs/{org}/actions and sends visibility on create", async () => {
    const keypair = await installMocks();
    renderPage();
    await screen.findByText("NPM_TOKEN");
    fireEvent.click(screen.getByRole("button", { name: /organization \(admin\)/i }));
    expect(await screen.findByText("ORG_TOKEN")).toBeInTheDocument();
    expect(screen.getByText("all")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /new secret/i }));
    fireEvent.change(await screen.findByLabelText("Name"), { target: { value: "ORG_NEW" } });
    fireEvent.change(screen.getByLabelText("Value"), { target: { value: "org-value" } });
    fireEvent.change(screen.getByLabelText("Repository access"), { target: { value: "private" } });
    fireEvent.click(screen.getByRole("button", { name: /add secret/i }));

    await waitFor(() => {
      const call = mockFetch.mock.calls.find(
        (c) =>
          c[0].toString() === "/api/v3/orgs/admin/actions/secrets/ORG_NEW" &&
          c[1]?.method === "PUT",
      );
      expect(call).toBeTruthy();
      const body = JSON.parse(call![1]!.body as string);
      expect(body.visibility).toBe("private");
      const opened = sodium.crypto_box_seal_open(
        sodium.from_base64(body.encrypted_value, sodium.base64_variants.ORIGINAL),
        keypair.publicKey,
        keypair.privateKey,
      );
      expect(sodium.to_string(opened)).toBe("org-value");
    });
  });
});
