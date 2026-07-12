import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { CodeScanningPage } from "../pages/CodeScanningPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

const repository = {
  id: 41,
  name: "security",
  full_name: "admin/security",
  private: false,
  visibility: "public",
  default_branch: "trunk",
  owner: { login: "admin", type: "User" },
  has_issues: true,
  has_projects: true,
  has_wiki: true,
};
const headSHA = "6bbf4f18bc0046a7f4280f789f05c39dfe29fdb7";
const database = {
  id: 7,
  name: "go-database",
  language: "go",
  uploader: { login: "codeql[bot]", type: "Bot" },
  content_type: "application/zip",
  size: 4096,
  created_at: "2026-07-12T10:00:00Z",
  updated_at: "2026-07-12T11:00:00Z",
  url: "/api/v3/repos/admin/security/code-scanning/codeql/databases/go",
  commit_oid: headSHA,
};

function json(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), { status, headers: { "Content-Type": "application/json" } });
}

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

describe("CodeScanningPage", () => {
  it("uses the real default-branch commit and manages CodeQL databases", async () => {
    let sarifRequest: Record<string, string> | undefined;
    mockFetch.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url === "/api/v3/repos/admin/security") return json(repository);
      if (url.endsWith("/branches/trunk")) return json({ name: "trunk", commit: { sha: headSHA } });
      if (url.endsWith("/code-scanning/codeql/databases")) return json([database]);
      if (init?.method === "DELETE" && url.endsWith("/code-scanning/codeql/databases/go")) return new Response(null, { status: 204 });
      if (init?.method === "POST" && url.endsWith("/code-scanning/sarifs")) {
        sarifRequest = JSON.parse(String(init.body)) as Record<string, string>;
        return json({ id: "sarif-1", url: "/sarif-1" }, 202);
      }
      if (url.endsWith("/code-scanning/sarifs/sarif-1")) {
        return json({ processing_status: "complete", analyses_url: "/analyses", errors: null });
      }
      if (url.includes("/code-scanning/alerts") || url.includes("/code-scanning/analyses")) return json([]);
      if (url.includes("/ui-data/repos/admin/security/viewer")) return json({ starred: false, subscribed: false });
      if (url.includes("/issues") || url.includes("/pulls") || url.endsWith("/branches")) return json([]);
      return json([]);
    });

    const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/ui/repos/admin/security/security/code-scanning"]}>
          <Routes>
            <Route path="/ui/repos/:owner/:repo/security/code-scanning" element={<CodeScanningPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("heading", { name: "Code scanning" })).toBeVisible();
    expect(await screen.findByText("go-database")).toBeVisible();
    expect(screen.getByText("trunk")).toBeVisible();
    expect(screen.getByText(headSHA.slice(0, 7))).toBeVisible();
    expect(screen.getByText("4.0 KB", { exact: false })).toBeVisible();

    const file = new File([`{"version":"2.1.0","note":"café"}`], "results.sarif", { type: "application/sarif+json" });
    fireEvent.change(screen.getByLabelText("SARIF file"), { target: { files: [file] } });
    const uploadButton = screen.getByRole("button", { name: "Upload SARIF" });
    await waitFor(() => expect(uploadButton).toBeEnabled());
    fireEvent.click(uploadButton);

    await waitFor(() => expect(sarifRequest).toBeDefined());
    expect(sarifRequest?.commit_sha).toBe(headSHA);
    expect(sarifRequest?.ref).toBe("refs/heads/trunk");
    expect(new TextDecoder().decode(Uint8Array.from(atob(sarifRequest!.sarif), (char) => char.charCodeAt(0)))).toContain("café");
    expect(sarifRequest?.commit_sha).not.toMatch(/^0+$/);

    fireEvent.click(screen.getByRole("button", { name: "Delete go CodeQL database" }));
    await waitFor(() => expect(mockFetch).toHaveBeenCalledWith(
      "/api/v3/repos/admin/security/code-scanning/codeql/databases/go",
      expect.objectContaining({ method: "DELETE" }),
    ));
  });
});
