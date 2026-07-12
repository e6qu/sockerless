import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { ClassroomPage } from "../pages/ClassroomPage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

function jsonResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), { status, headers: { "Content-Type": "application/json" } });
}

afterEach(() => { cleanup(); mockFetch.mockReset(); });

function renderAt(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={[path]}><Routes>
    <Route path="/ui/classrooms" element={<ClassroomPage />} />
    <Route path="/ui/classrooms/:classroomId" element={<ClassroomPage />} />
    <Route path="/ui/classrooms/accept/:inviteCode" element={<ClassroomPage />} />
    <Route path="/ui/repos/:owner/:repo" element={<div>Repository opened</div>} />
  </Routes></MemoryRouter></QueryClientProvider>);
}

const classroom = {
  id: 41,
  name: "Systems Programming",
  archived: false,
  url: "/classrooms/41-systems-programming",
  organization: { id: 7, login: "octo-school", name: "Octo School", avatar_url: "" },
  roster: [{ id: 8, login: "mona", avatar_url: "", roster_identifier: "mona@example.edu" }],
  assignments: [{ id: 51, title: "Processes", type: "individual", slug: "processes", invite_link: "http://x/a/code", invitations_enabled: true, public_repo: false, accepted: 3, submitted: 2, passing: 1, deadline: null, autograding_tests: [{ name: "Tests", command: "go test ./...", points: 10 }] }],
};

describe("ClassroomPage", () => {
  it("renders the retained Classroom dashboard and real coursework counters", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ classrooms: [classroom], organizations: [classroom.organization] }));
    renderAt("/ui/classrooms");
    expect(await screen.findByText("GitHub Classroom, kept alive.")).toBeInTheDocument();
    expect(screen.getByText("Systems Programming")).toBeInTheDocument();
    expect(screen.getByText((_text, element) => element?.tagName === "SPAN" && element.textContent === "1 assignments")).toBeInTheDocument();
    expect(screen.getByText((_text, element) => element?.tagName === "SPAN" && element.textContent === "1 students")).toBeInTheDocument();
  });

  it("renders assignment organization, roster, autograding, and status detail", async () => {
    mockFetch.mockResolvedValue(jsonResponse({ classrooms: [classroom], organizations: [classroom.organization] }));
    renderAt("/ui/classrooms/41");
    expect(await screen.findByRole("heading", { name: "Systems Programming" })).toBeInTheDocument();
    expect(screen.getByText("Processes")).toBeInTheDocument();
    expect(screen.getByText("3 accepted")).toBeInTheDocument();
    expect(screen.getByText("1 passing")).toBeInTheDocument();
    expect(screen.getByText("10 autograding points")).toBeInTheDocument();
  });

  it("accepts an invite and hands off to the generated repository", async () => {
    mockFetch.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") return Promise.resolve(jsonResponse({ id: 1, repository: { full_name: "octo-school/processes-mona", html_url: "/octo-school/processes-mona" } }, 201));
      return Promise.resolve(jsonResponse({ ...classroom.assignments[0], starter_code_repository: { full_name: "octo-school/processes-starter" } }));
    });
    renderAt("/ui/classrooms/accept/code");
    expect(await screen.findByText("Processes")).toBeInTheDocument();
    screen.getByRole("button", { name: "Accept this assignment" }).click();
    await waitFor(() => expect(screen.getByText("Repository opened")).toBeInTheDocument());
  });
});
