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
    expect(screen.getByText("New Project")).toBeInTheDocument();
  });

  it("renders step indicator", () => {
    renderPage();
    expect(screen.getByText("1. Cloud")).toBeInTheDocument();
    expect(screen.getByText("2. Backend")).toBeInTheDocument();
    expect(screen.getByText("3. Configure")).toBeInTheDocument();
  });

  it("renders cloud selection cards", () => {
    renderPage();
    expect(screen.getByText("AWS")).toBeInTheDocument();
    expect(screen.getByText("GCP")).toBeInTheDocument();
    expect(screen.getByText("Azure")).toBeInTheDocument();
  });

  it("shows cloud descriptions", () => {
    renderPage();
    expect(screen.getByText("ECS / Lambda + AWS Simulator")).toBeInTheDocument();
    expect(screen.getByText("Cloud Run / GCF + GCP Simulator")).toBeInTheDocument();
    expect(screen.getByText("ACA / AZF + Azure Simulator")).toBeInTheDocument();
  });
});
