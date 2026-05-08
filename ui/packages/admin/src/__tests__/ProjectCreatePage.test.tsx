import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router";
import { ProjectCreatePage } from "../pages/ProjectCreatePage.js";

const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

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
      <BrowserRouter>
        <ProjectCreatePage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

describe("ProjectCreatePage", () => {
  it("renders the heading", () => {
    renderPage();
    expect(screen.getByRole("heading", { name: /create project/i })).toBeInTheDocument();
  });

  it("renders step indicator", () => {
    renderPage();
    expect(screen.getAllByText(/cloud/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/backend/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/configure/i).length).toBeGreaterThan(0);
  });

  it("renders cloud selection cards", () => {
    renderPage();
    expect(screen.getByText("AWS")).toBeInTheDocument();
    expect(screen.getByText("GCP")).toBeInTheDocument();
    expect(screen.getByText("Azure")).toBeInTheDocument();
  });

  it("shows cloud descriptions", () => {
    renderPage();
    expect(
      screen.getByText(/ECS \/ Lambda \+ AWS simulator/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Cloud Run \/ GCF \+ GCP simulator/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/ACA \/ AZF \+ Azure simulator/i)).toBeInTheDocument();
  });
});
