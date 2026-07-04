import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { CopilotPage } from "../pages/CopilotPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

afterEach(() => {
  cleanup();
  mockFetch.mockReset();
});

function renderAt(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/ui/orgs/:org/copilot" element={<CopilotPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const billing = {
  seat_breakdown: {
    total: 3,
    added_this_cycle: 1,
    pending_cancellation: 0,
    pending_invitation: 0,
    active_this_cycle: 0,
    inactive_this_cycle: 3,
  },
  public_code_suggestions: "allow",
  ide_chat: "enabled",
  platform_chat: "enabled",
  cli: "enabled",
  seat_management_setting: "assign_selected",
  plan_type: "business",
};

const seat = {
  assignee: { id: 2, login: "dev1", avatar_url: "", type: "User", site_admin: false },
  assigning_team: null,
  pending_cancellation_date: null,
  last_activity_at: null,
  last_activity_editor: null,
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
  plan_type: "business",
};

function mockCopilotEndpoints(overrides: Record<string, () => Response> = {}) {
  mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
    const u = url.toString();
    for (const [needle, make] of Object.entries(overrides)) {
      if (u.includes(needle)) return Promise.resolve(make());
    }
    if (u.includes("/copilot/billing/selected_users") && init?.method === "POST") {
      return Promise.resolve(jsonResponse({ seats_created: 1 }, 201));
    }
    if (u.includes("/copilot/billing/seats")) {
      return Promise.resolve(jsonResponse({ total_seats: 1, seats: [seat] }));
    }
    if (u.includes("/copilot/billing")) return Promise.resolve(jsonResponse(billing));
    if (u.includes("/copilot-spaces")) return Promise.resolve(jsonResponse({ spaces: [] }));
    return Promise.resolve(jsonResponse([]));
  });
}

describe("CopilotPage", () => {
  it("renders billing breakdown, seats, and an honest empty Spaces state", async () => {
    mockCopilotEndpoints();
    renderAt("/ui/orgs/acme/copilot");

    await waitFor(() => {
      expect(screen.getByText("@dev1")).toBeInTheDocument();
    });
    expect(screen.getByText("Total seats")).toBeInTheDocument();
    expect(screen.getByText("business")).toBeInTheDocument();
    expect(screen.getByText("assign_selected")).toBeInTheDocument();
    expect(screen.getByText(/no copilot spaces/i)).toBeInTheDocument();
  });

  it("assigns seats for comma-separated usernames", async () => {
    mockCopilotEndpoints();
    renderAt("/ui/orgs/acme/copilot");
    await waitFor(() => {
      expect(screen.getByLabelText("Usernames to assign Copilot seats")).toBeInTheDocument();
    });
    fireEvent.change(screen.getByLabelText("Usernames to assign Copilot seats"), {
      target: { value: "dev2, dev3" },
    });
    fireEvent.click(screen.getByRole("button", { name: /assign seats/i }));
    await waitFor(() => {
      const post = mockFetch.mock.calls.find(
        (c) =>
          c[0].toString().includes("/copilot/billing/selected_users") && c[1]?.method === "POST",
      );
      expect(post).toBeTruthy();
      expect(JSON.parse(String(post![1]!.body))).toEqual({
        selected_usernames: ["dev2", "dev3"],
      });
    });
  });

  it("cancels a seat via DELETE selected_users", async () => {
    mockCopilotEndpoints({
      "/copilot/billing/selected_users": () => new Response(null, { status: 204 }),
    });
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    renderAt("/ui/orgs/acme/copilot");
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /cancel seat/i })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /cancel seat/i }));
    await waitFor(() => {
      const del = mockFetch.mock.calls.find(
        (c) =>
          c[0].toString().includes("/copilot/billing/selected_users") && c[1]?.method === "DELETE",
      );
      expect(del).toBeTruthy();
      expect(JSON.parse(String(del![1]!.body))).toEqual({ selected_usernames: ["dev1"] });
    });
    confirmSpy.mockRestore();
  });

  it("creates a Copilot Space through the dialog", async () => {
    mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = url.toString();
      if (u.endsWith("/copilot-spaces") && init?.method === "POST") {
        return Promise.resolve(
          jsonResponse(
            {
              id: 2,
              number: 5,
              name: "onboarding",
              description: null,
              general_instructions: null,
              base_role: "reader",
              owner: { login: "acme" },
              creator: null,
              created_at: "2026-07-01T00:00:00Z",
              updated_at: "2026-07-01T00:00:00Z",
            },
            201,
          ),
        );
      }
      if (u.includes("/copilot-spaces")) return Promise.resolve(jsonResponse({ spaces: [] }));
      if (u.includes("/copilot/billing/seats")) {
        return Promise.resolve(jsonResponse({ total_seats: 0, seats: [] }));
      }
      if (u.includes("/copilot/billing")) return Promise.resolve(jsonResponse(billing));
      return Promise.resolve(jsonResponse([]));
    });
    renderAt("/ui/orgs/acme/copilot");
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /new space/i })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /new space/i }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "onboarding" } });
    fireEvent.change(screen.getByLabelText("Base role for organization members"), {
      target: { value: "reader" },
    });
    fireEvent.click(screen.getByRole("button", { name: /create space/i }));
    await waitFor(() => {
      const post = mockFetch.mock.calls.find(
        (c) => c[0].toString().endsWith("/copilot-spaces") && c[1]?.method === "POST",
      );
      expect(post).toBeTruthy();
      expect(JSON.parse(String(post![1]!.body))).toMatchObject({
        name: "onboarding",
        base_role: "reader",
      });
    });
  });

  const detailSpace = {
    id: 1,
    number: 4,
    name: "onboarding",
    description: "New-hire context",
    general_instructions: null,
    base_role: "reader",
    owner: { login: "acme" },
    creator: null,
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-02T00:00:00Z",
  };

  function mockSpaceDetailEndpoints() {
    mockFetch.mockImplementation((url: RequestInfo | URL, init?: RequestInit) => {
      const u = url.toString();
      if (u.includes("/copilot-spaces/4/collaborators") && init?.method === "POST") {
        return Promise.resolve(
          jsonResponse(
            { actor_type: "User", role: "reader", id: 3, login: "dev2", avatar_url: "" },
            201,
          ),
        );
      }
      if (u.includes("/copilot-spaces/4/collaborators")) {
        return Promise.resolve(
          jsonResponse({
            collaborators: [
              { actor_type: "User", role: "writer", id: 2, login: "dev1", avatar_url: "" },
            ],
          }),
        );
      }
      if (u.includes("/copilot-spaces/4/resources") && init?.method === "POST") {
        return Promise.resolve(
          jsonResponse(
            {
              id: 2,
              resource_type: "repository",
              metadata: { repository_id: 42 },
              created_at: "2026-07-01T00:00:00Z",
              updated_at: "2026-07-01T00:00:00Z",
            },
            201,
          ),
        );
      }
      if (u.includes("/copilot-spaces/4/resources")) {
        return Promise.resolve(
          jsonResponse({
            resources: [
              {
                id: 1,
                resource_type: "free_text",
                metadata: { text: "Deploy checklist" },
                created_at: "2026-06-01T00:00:00Z",
                updated_at: "2026-06-01T00:00:00Z",
              },
            ],
          }),
        );
      }
      if (u.includes("/copilot-spaces/4") && init?.method === "DELETE") {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      if (u.includes("/copilot-spaces")) {
        return Promise.resolve(jsonResponse({ spaces: [detailSpace] }));
      }
      if (u.includes("/copilot/billing/seats")) {
        return Promise.resolve(jsonResponse({ total_seats: 0, seats: [] }));
      }
      if (u.includes("/copilot/billing")) return Promise.resolve(jsonResponse(billing));
      return Promise.resolve(jsonResponse([]));
    });
  }

  it("manages collaborators and resources in the space detail", async () => {
    mockSpaceDetailEndpoints();
    renderAt("/ui/orgs/acme/copilot");
    await waitFor(() => {
      expect(screen.getByText("onboarding")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByText("onboarding"));
    await waitFor(() => {
      expect(screen.getByText("@dev1")).toBeInTheDocument();
    });
    expect(screen.getByText("Deploy checklist")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Collaborator username or team slug"), {
      target: { value: "dev2" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add" }));
    await waitFor(() => {
      const post = mockFetch.mock.calls.find(
        (c) =>
          c[0].toString().includes("/copilot-spaces/4/collaborators") && c[1]?.method === "POST",
      );
      expect(post).toBeTruthy();
      expect(JSON.parse(String(post![1]!.body))).toEqual({
        actor_type: "User",
        actor_identifier: "dev2",
        role: "reader",
      });
    });

    fireEvent.change(screen.getByLabelText("Resource repository ID"), { target: { value: "42" } });
    fireEvent.click(screen.getByRole("button", { name: "Attach" }));
    await waitFor(() => {
      const post = mockFetch.mock.calls.find(
        (c) => c[0].toString().includes("/copilot-spaces/4/resources") && c[1]?.method === "POST",
      );
      expect(post).toBeTruthy();
      expect(JSON.parse(String(post![1]!.body))).toEqual({
        resource_type: "repository",
        metadata: { repository_id: 42 },
      });
    });
  });

  it("deletes a space after confirmation", async () => {
    mockSpaceDetailEndpoints();
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    renderAt("/ui/orgs/acme/copilot");
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "delete" })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: "delete" }));
    await waitFor(() => {
      const del = mockFetch.mock.calls.find(
        (c) => c[0].toString().includes("/copilot-spaces/4") && c[1]?.method === "DELETE",
      );
      expect(del).toBeTruthy();
    });
    confirmSpy.mockRestore();
  });

  it("lists Copilot Spaces when present and surfaces billing errors", async () => {
    mockCopilotEndpoints({
      "/copilot/billing/seats": () => jsonResponse({ total_seats: 0, seats: [] }),
      "/copilot/billing": () => jsonResponse({ message: "forbidden" }, 403),
      "/copilot-spaces": () =>
        jsonResponse({
          spaces: [
            {
              id: 1,
              number: 4,
              name: "onboarding",
              description: "New-hire context",
              general_instructions: null,
              base_role: "reader",
              owner: { login: "acme" },
              creator: { id: 1, login: "admin", avatar_url: "", type: "User", site_admin: true },
              created_at: "2026-06-01T00:00:00Z",
              updated_at: "2026-06-02T00:00:00Z",
            },
          ],
        }),
    });
    renderAt("/ui/orgs/acme/copilot");

    await waitFor(() => {
      expect(screen.getByText(/failed to load copilot billing/i)).toBeInTheDocument();
    });
    expect(screen.getByText("onboarding")).toBeInTheDocument();
    expect(screen.getByText("New-hire context")).toBeInTheDocument();
    expect(screen.getByText(/base role: reader · created by @admin/)).toBeInTheDocument();
  });
});
