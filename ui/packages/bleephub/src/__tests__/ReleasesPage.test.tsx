import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { ReleasesPage } from "../pages/ReleasesPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

const asset = { id: 9, name: "artifact.txt", label: "Linux artifact", size: 24, download_count: 0 };
const release = {
  id: 1, tag_name: "v1.0.0", target_commitish: "main", name: "First release", body: "notes",
  draft: false, prerelease: false, created_at: "2026-07-12T00:00:00Z", published_at: "2026-07-12T00:00:00Z",
  author: { login: "admin" }, assets: [asset], upload_url: "", html_url: "", url: "",
};
const repo = {
  id: 1, name: "release", full_name: "admin/release", private: false, visibility: "public", default_branch: "main",
  owner: { login: "admin", type: "User" }, has_issues: true, has_projects: true, has_wiki: true,
};

function response(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), { status, headers: { "Content-Type": "application/json" } });
}

afterEach(() => { cleanup(); mockFetch.mockReset(); });

describe("ReleasesPage", () => {
  it("returns from editing with the updated release and its assets intact", async () => {
    mockFetch.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (init?.method === "PATCH" && url.endsWith("/releases/1")) return Promise.resolve(response({ ...release, name: "Updated release" }));
      if (url.endsWith("/releases/1")) return Promise.resolve(response(release));
      if (url === "/api/v3/repos/admin/release") return Promise.resolve(response(repo));
      if (url.includes("/ui-data/repos/admin/release/viewer")) return Promise.resolve(response({ starred: false, subscribed: false }));
      if (url.includes("/issues") || url.includes("/pulls") || url.includes("/branches")) return Promise.resolve(response([]));
      return Promise.resolve(response([]));
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><MemoryRouter initialEntries={["/ui/repos/admin/release/releases/1"]}><Routes><Route path="/ui/repos/:owner/:repo/releases/:releaseId" element={<ReleasesPage />} /></Routes></MemoryRouter></QueryClientProvider>);

    expect(await screen.findByRole("button", { name: "Delete artifact.txt" })).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    fireEvent.change(screen.getByLabelText("Release title"), { target: { value: "Updated release" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(screen.getByRole("heading", { name: "Updated release" })).toBeVisible());
    expect(screen.getByRole("button", { name: "Delete artifact.txt" })).toBeVisible();
  });
});
