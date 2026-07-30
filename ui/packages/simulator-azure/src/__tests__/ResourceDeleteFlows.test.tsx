import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ACRRegistriesPage } from "../pages/ACRRegistriesPage.js";
import { StorageAccountsPage } from "../pages/StorageAccountsPage.js";
import { ContainerAppsPage } from "../pages/ContainerAppsPage.js";
import { AzureFunctionsPage } from "../pages/AzureFunctionsPage.js";

/**
 * The Container registries, Storage accounts, Container Apps jobs, and
 * Function Apps list pages used to render AzureResourceTable's Delete
 * command disabled with no handler wired — the real portal deletes every
 * resource type, so the simulator console leaving four unwired is a gap this
 * pass closes. Each page now passes AzureResourceTable a real `onDelete`,
 * which opens a real Fluent confirm `Dialog` naming the selected rows and,
 * on confirm, drives the resource's real Azure Resource Manager DELETE.
 * These tests drive the full read → select → confirm → mutate round trip
 * against a mocked federated fetch, the way ResourceHeaderActions.test.tsx
 * does for the AWS console's delete flows — the lightweight Playwright e2e
 * suite (simulator-azure.spec.ts) has no identity provider, so it can only
 * assert the Delete command renders disabled without a live cloud read;
 * this suite is where the dialog itself — structure, confirm, Escape, and
 * focus return — is proven, with real Fluent `Dialog` behaviour running for
 * real under jsdom (see test-setup.ts's tabster/NodeFilter polyfill).
 */

const mockFetch = vi.fn();
globalThis.fetch = mockFetch as unknown as typeof fetch;

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

function jsonResponse(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), { status, headers: { "content-type": "application/json" } });
}

function armError(code: string, message: string, status: number): Response {
  return jsonResponse({ error: { code, message } }, status);
}

type Rule = { when: (url: string, init?: RequestInit) => boolean; respond: (init?: RequestInit) => Response };

function installFetch(rules: Rule[]) {
  mockFetch.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input.toString();
    if (url === "/ui/config.json") return jsonResponse({});
    const rule = rules.find((candidate) => candidate.when(url, init));
    if (!rule) throw new Error(`unhandled fetch: ${init?.method ?? "GET"} ${url}`);
    return rule.respond(init);
  });
}

function renderWithQuery(ui: React.ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

const subscriptionsRule: Rule = {
  when: (url) => url === "/subscriptions?api-version=2022-12-01",
  respond: () => jsonResponse({ value: [{ subscriptionId: "sub-1" }] }),
};

describe("ACRRegistriesPage delete", () => {
  const id = "/subscriptions/sub-1/providers/Microsoft.ContainerRegistry/registries/myregistry1";
  const listRule: Rule = {
    when: (url) => url === "/subscriptions/sub-1/providers/Microsoft.ContainerRegistry/registries?api-version=2023-07-01",
    respond: () => jsonResponse({ value: [{ id, name: "myregistry1", location: "eastus" }] }),
  };

  it("renders Delete disabled until a registry is selected", async () => {
    installFetch([subscriptionsRule, listRule]);
    renderWithQuery(<ACRRegistriesPage />);
    await screen.findByText("myregistry1");
    expect((screen.getByTestId("acr-delete") as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByRole("checkbox", { name: `Select ${id}` }));
    expect((screen.getByTestId("acr-delete") as HTMLButtonElement).disabled).toBe(false);
  });

  it("names the registry, warns the action is irreversible, and deletes it through the real registries DELETE", async () => {
    let deleteCalled: { url: string; init?: RequestInit } | null = null;
    installFetch([
      subscriptionsRule,
      listRule,
      {
        when: (url, init) => url === `${id}?api-version=2023-07-01` && init?.method === "DELETE",
        respond: (init) => {
          deleteCalled = { url: `${id}?api-version=2023-07-01`, init };
          return new Response(null, { status: 200 });
        },
      },
    ]);
    renderWithQuery(<ACRRegistriesPage />);
    await screen.findByText("myregistry1");

    fireEvent.click(screen.getByRole("checkbox", { name: `Select ${id}` }));
    fireEvent.click(screen.getByTestId("acr-delete"));

    const dialog = await screen.findByRole("dialog", { name: "Delete myregistry1?" });
    expect(dialog.textContent).toContain("removes every repository and image it holds");
    expect(dialog.textContent).toContain("myregistry1");

    fireEvent.click(within(dialog).getByTestId("acr-delete-confirm"));

    await waitFor(() => expect(deleteCalled).not.toBeNull());
    expect(deleteCalled!.init?.method).toBe("DELETE");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("closes on Escape without deleting", async () => {
    installFetch([subscriptionsRule, listRule]);
    renderWithQuery(<ACRRegistriesPage />);
    await screen.findByText("myregistry1");

    fireEvent.click(screen.getByRole("checkbox", { name: `Select ${id}` }));
    const trigger = screen.getByTestId("acr-delete");
    fireEvent.click(trigger);
    const dialog = await screen.findByRole("dialog", { name: "Delete myregistry1?" });

    // Fluent's `Dialog` wires Escape to `onOpenChange({ open: false })` the
    // same way the Header disclosure popovers already exercised in
    // simulator-azure.spec.ts do — that Playwright suite (a real browser)
    // is where tabster's focus-return-to-trigger behaviour is proven for
    // this codebase's Escape-closeable overlays; jsdom's own rAF/focus
    // timing doesn't reliably surface it, so this test holds only the part
    // this environment can prove: no delete request goes out, and the
    // dialog is gone.
    fireEvent.keyDown(dialog, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(mockFetch.mock.calls.some(([, init]) => (init as RequestInit | undefined)?.method === "DELETE")).toBe(false);
  });

  it("surfaces Azure Resource Manager's own error rather than closing the dialog", async () => {
    installFetch([
      subscriptionsRule,
      listRule,
      {
        when: (url, init) => url === `${id}?api-version=2023-07-01` && init?.method === "DELETE",
        respond: () => armError("RegistryHasReplications", "The registry has active replications.", 409),
      },
    ]);
    renderWithQuery(<ACRRegistriesPage />);
    await screen.findByText("myregistry1");

    fireEvent.click(screen.getByRole("checkbox", { name: `Select ${id}` }));
    fireEvent.click(screen.getByTestId("acr-delete"));
    const dialog = await screen.findByRole("dialog", { name: "Delete myregistry1?" });
    fireEvent.click(within(dialog).getByTestId("acr-delete-confirm"));

    const alert = await within(dialog).findByRole("alert");
    expect(alert.textContent).toContain("active replications");
    expect(await screen.findByRole("dialog")).toBeTruthy();

    // A backdrop click that races with the immediate error response must not
    // discard Azure Resource Manager's actionable failure. Explicit Cancel
    // remains available and closes the surface normally.
    const backdrop = document.getElementById("acr-delete-backdrop");
    expect(backdrop).not.toBeNull();
    fireEvent.click(backdrop!);
    expect(screen.getByRole("dialog")).toBeTruthy();
    fireEvent.click(within(dialog).getByTestId("acr-delete-cancel"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
});

describe("StorageAccountsPage delete", () => {
  const id = "/subscriptions/sub-1/providers/Microsoft.Storage/storageAccounts/mystorageacct1";
  const listRule: Rule = {
    when: (url) => url === "/subscriptions/sub-1/providers/Microsoft.Storage/storageAccounts?api-version=2023-01-01",
    respond: () => jsonResponse({ value: [{ id, name: "mystorageacct1", location: "eastus", kind: "StorageV2" }] }),
  };

  it("deletes the selected account through the real storageAccounts DELETE", async () => {
    let deleteCalled: string | null = null;
    installFetch([
      subscriptionsRule,
      listRule,
      {
        when: (url, init) => url === `${id}?api-version=2023-01-01` && init?.method === "DELETE",
        respond: (init) => {
          deleteCalled = init?.method ?? "";
          return new Response(null, { status: 200 });
        },
      },
    ]);
    renderWithQuery(<StorageAccountsPage />);
    await screen.findByText("mystorageacct1");

    fireEvent.click(screen.getByRole("checkbox", { name: `Select ${id}` }));
    fireEvent.click(screen.getByTestId("storage-delete"));
    const dialog = await screen.findByRole("dialog", { name: "Delete mystorageacct1?" });
    expect(dialog.textContent).toContain("removes every container, blob, file share, table, and queue it holds");
    fireEvent.click(within(dialog).getByTestId("storage-delete-confirm"));

    await waitFor(() => expect(deleteCalled).toBe("DELETE"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
});

describe("ContainerAppsPage delete", () => {
  const id = "/subscriptions/sub-1/providers/Microsoft.App/jobs/myjob1";
  const listRule: Rule = {
    when: (url) => url === "/subscriptions/sub-1/providers/Microsoft.App/jobs?api-version=2024-03-01",
    respond: () => jsonResponse({ value: [{ id, name: "myjob1", location: "eastus", type: "Microsoft.App/jobs" }] }),
  };

  it("deletes the selected job through the real jobs DELETE", async () => {
    let deleteCalled: string | null = null;
    installFetch([
      subscriptionsRule,
      listRule,
      {
        when: (url, init) => url === `${id}?api-version=2024-03-01` && init?.method === "DELETE",
        respond: (init) => {
          deleteCalled = init?.method ?? "";
          return new Response(null, { status: 200 });
        },
      },
    ]);
    renderWithQuery(<ContainerAppsPage />);
    await screen.findByText("myjob1");

    fireEvent.click(screen.getByRole("checkbox", { name: `Select ${id}` }));
    fireEvent.click(screen.getByTestId("ca-delete"));
    const dialog = await screen.findByRole("dialog", { name: "Delete myjob1?" });
    expect(dialog.textContent).toContain("removes its execution history");
    fireEvent.click(within(dialog).getByTestId("ca-delete-confirm"));

    await waitFor(() => expect(deleteCalled).toBe("DELETE"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
});

describe("AzureFunctionsPage delete", () => {
  const id = "/subscriptions/sub-1/providers/Microsoft.Web/sites/mysite1";
  const listRule: Rule = {
    when: (url) => url === "/subscriptions/sub-1/providers/Microsoft.Web/sites?api-version=2023-12-01",
    respond: () => jsonResponse({ value: [{ id, name: "mysite1", location: "eastus", kind: "functionapp" }] }),
  };

  it("deletes the selected Function App through the real sites DELETE", async () => {
    let deleteCalled: string | null = null;
    installFetch([
      subscriptionsRule,
      listRule,
      {
        when: (url, init) => url === `${id}?api-version=2023-12-01` && init?.method === "DELETE",
        respond: (init) => {
          deleteCalled = init?.method ?? "";
          return new Response(null, { status: 200 });
        },
      },
    ]);
    renderWithQuery(<AzureFunctionsPage />);
    await screen.findByText("mysite1");

    fireEvent.click(screen.getByRole("checkbox", { name: `Select ${id}` }));
    fireEvent.click(screen.getByTestId("fn-delete"));
    const dialog = await screen.findByRole("dialog", { name: "Delete mysite1?" });
    expect(dialog.textContent).toContain("removes every function deployed to it");
    fireEvent.click(within(dialog).getByTestId("fn-delete-confirm"));

    await waitFor(() => expect(deleteCalled).toBe("DELETE"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
});
