import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { LogViewer } from "../components/LogViewer.js";

afterEach(() => {
  cleanup();
});

describe("LogViewer", () => {
  it("renders log lines with line numbers", () => {
    render(<LogViewer lines={["hello", "world"]} />);
    expect(screen.getByText("1")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(screen.getByText("hello")).toBeInTheDocument();
    expect(screen.getByText("world")).toBeInTheDocument();
  });

  it("renders empty state when no lines", () => {
    render(<LogViewer lines={[]} />);
    expect(screen.getByText("No log output")).toBeInTheDocument();
  });

  it("strips ANSI codes and renders content", () => {
    render(<LogViewer lines={["\x1b[32mOK\x1b[0m"]} />);
    // The "OK" text should appear wrapped in a colored span
    const container = document.querySelector("pre");
    expect(container?.textContent).toContain("OK");
    // Should have an inline color style from the ANSI code
    const coloredSpan = document.querySelector('span[style*="color"]');
    expect(coloredSpan).not.toBeNull();
    expect(coloredSpan?.textContent).toBe("OK");
  });
});
