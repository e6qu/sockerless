import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { DeploymentsPage } from "../pages/DeploymentsPage.js";

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
      <MemoryRouter initialEntries={["/ui/repos/admin/deploy-repo/deployments"]}>
        <Routes>
          <Route path="/ui/repos/:owner/:repo/deployments" element={<DeploymentsPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const deployment = {
  id: 7,
  node_id: "DE_kgDO00000007",
  sha: "abc123",
  ref: "main",
  task: "deploy",
  environment: "production",
  original_environment: "production",
  description: "",
  creator: { login: "admin" },
  payload: {},
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
  transient_environment: false,
  production_environment: true,
  statuses_url: "/api/v3/repos/admin/deploy-repo/deployments/7/statuses",
};

const status = {
  id: 3,
  node_id: "DS_kgDO00000003",
  state: "success",
  creator: { login: "admin" },
  description: "shipped",
  environment: "production",
  target_url: "",
  log_url: "",
  environment_url: "https://prod.example.test",
  created_at: "2026-06-01T00:05:00Z",
  updated_at: "2026-06-01T00:05:00Z",
};

const environment = {
  id: 11,
  node_id: "EN_kgDO00000011",
  name: "production",
  url: "/api/v3/repos/admin/deploy-repo/environments/production",
  html_url: "/admin/deploy-repo/deployments/activity_log?environments_filter=production",
  created_at: "2026-05-01T00:00:00Z",
  updated_at: "2026-05-01T00:00:00Z",
  deployment_branch_policy: { protected_branches: false, custom_branch_policies: true },
  protection_rules: [
    { id: 111, node_id: "GA_kwDO00000111", type: "wait_timer", wait_timer: 30 },
    {
      id: 112,
      node_id: "GA_kwDO00000112",
      type: "required_reviewers",
      reviewers: [{ type: "User", reviewer: { login: "octocat" } }],
    },
  ],
};

describe("DeploymentsPage", () => {
  it("lists deployments and expands the statuses timeline", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.startsWith("/api/v3/repos/admin/deploy-repo/deployments?")) {
        return Promise.resolve(jsonResponse([deployment]));
      }
      if (url === "/api/v3/repos/admin/deploy-repo/deployments/7/statuses") {
        return Promise.resolve(jsonResponse([status]));
      }
      return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
    });
    renderPage();

    await waitFor(() => expect(screen.getByText("#7")).toBeInTheDocument());
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/v3/repos/admin/deploy-repo/deployments?per_page=30",
      expect.anything(),
    );

    fireEvent.click(screen.getByText("#7"));
    // "success" appears both in the timeline entry and the create-status select.
    await waitFor(() => expect(screen.getAllByText("success").length).toBeGreaterThan(0));
    expect(screen.getByText(/shipped/)).toBeInTheDocument();
  });

  it("creates a deployment status via POST .../statuses", async () => {
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      if (url.startsWith("/api/v3/repos/admin/deploy-repo/deployments?")) {
        return Promise.resolve(jsonResponse([deployment]));
      }
      if (url === "/api/v3/repos/admin/deploy-repo/deployments/7/statuses") {
        if (init?.method === "POST") {
          return Promise.resolve(jsonResponse({ ...status, id: 4, state: "queued" }, 201));
        }
        return Promise.resolve(jsonResponse([status]));
      }
      return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
    });
    renderPage();

    await waitFor(() => screen.getByText("#7"));
    fireEvent.click(screen.getByText("#7"));
    await waitFor(() => screen.getByLabelText("New status state"));

    fireEvent.change(screen.getByLabelText("New status state"), { target: { value: "queued" } });
    fireEvent.click(screen.getByRole("button", { name: /create status/i }));

    await waitFor(() => {
      const postCall = mockFetch.mock.calls.find(
        (call) =>
          call[0] === "/api/v3/repos/admin/deploy-repo/deployments/7/statuses" &&
          call[1]?.method === "POST",
      );
      expect(postCall).toBeDefined();
      expect(JSON.parse(postCall![1].body as string)).toEqual({ state: "queued" });
    });
  });

  it("shows environments with protection rules and branch policies", async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url === "/api/v3/repos/admin/deploy-repo/environments") {
        return Promise.resolve(jsonResponse({ total_count: 1, environments: [environment] }));
      }
      if (url.endsWith("/environments/production/deployment-branch-policies")) {
        return Promise.resolve(
          jsonResponse({
            total_count: 1,
            branch_policies: [
              { id: 21, node_id: "DBP_kwDO00000021", name: "release/*", type: "branch" },
            ],
          }),
        );
      }
      if (url.endsWith("/environments/production/deployment_protection_rules")) {
        return Promise.resolve(
          jsonResponse({ total_count: 0, custom_deployment_protection_rules: [] }),
        );
      }
      if (url.startsWith("/api/v3/repos/admin/deploy-repo/deployments?")) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
    });
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Environments" }));
    await waitFor(() => expect(screen.getByText("production")).toBeInTheDocument());
    expect(screen.getByText(/2 protection rules/)).toBeInTheDocument();
    expect(screen.getByText(/custom branch policies/)).toBeInTheDocument();

    fireEvent.click(screen.getByText("production"));
    await waitFor(() => expect(screen.getByText(/Wait timer: 30 minutes/)).toBeInTheDocument());
    expect(screen.getByText(/octocat/)).toBeInTheDocument();
    // The branch-policy list resolves from its own fetch after the expand.
    await waitFor(() => expect(screen.getByText(/release\/\*/)).toBeInTheDocument());
  });

  it("approves pending deployments for a waiting run", async () => {
    const run = {
      id: 42,
      name: "Deploy",
      run_number: 9,
      run_attempt: 1,
      event: "push",
      status: "waiting",
      conclusion: null,
      head_branch: "main",
      head_sha: "abc123",
      path: ".github/workflows/deploy.yml",
      workflow_id: 1,
      created_at: "2026-06-01T00:00:00Z",
      updated_at: "2026-06-01T00:00:00Z",
      actor: { login: "admin" },
    };
    mockFetch.mockImplementation((url: string, init?: RequestInit) => {
      if (url.startsWith("/api/v3/repos/admin/deploy-repo/actions/runs?")) {
        return Promise.resolve(jsonResponse({ total_count: 1, workflow_runs: [run] }));
      }
      if (url === "/api/v3/repos/admin/deploy-repo/actions/runs/42/pending_deployments") {
        if (init?.method === "POST") {
          return Promise.resolve(jsonResponse([]));
        }
        return Promise.resolve(
          jsonResponse([
            {
              environment: { id: 11, name: "production" },
              wait_timer: 0,
              wait_timer_started_at: "2026-06-01T00:00:00Z",
              current_user_can_approve: true,
              reviewers: [],
            },
          ]),
        );
      }
      if (url.startsWith("/api/v3/repos/admin/deploy-repo/deployments?")) {
        return Promise.resolve(jsonResponse([]));
      }
      return Promise.resolve(jsonResponse({ message: "Not Found" }, 404));
    });
    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Pending approvals" }));
    await waitFor(() => expect(screen.getByText(/Waiting to deploy to/)).toBeInTheDocument());
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining("/actions/runs?per_page=30&status=waiting"),
      expect.anything(),
    );

    fireEvent.click(screen.getByRole("button", { name: /approve and deploy/i }));
    await waitFor(() => {
      const postCall = mockFetch.mock.calls.find(
        (call) =>
          call[0] === "/api/v3/repos/admin/deploy-repo/actions/runs/42/pending_deployments" &&
          call[1]?.method === "POST",
      );
      expect(postCall).toBeDefined();
      expect(JSON.parse(postCall![1].body as string)).toEqual({
        environment_ids: [11],
        state: "approved",
        comment: "",
      });
    });
  });
});
