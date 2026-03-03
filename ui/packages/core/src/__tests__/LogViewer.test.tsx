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

  it("escapes HTML in log lines to prevent XSS", () => {
    render(<LogViewer lines={['<script>alert("xss")</script>']} />);
    // The script tag should be escaped, not injected
    const pre = document.querySelector("pre");
    expect(pre?.innerHTML).toContain("&lt;script&gt;");
    expect(pre?.innerHTML).not.toContain("<script>");
  });

  it("handles reset+color combined sequence", () => {
    render(<LogViewer lines={["\x1b[0;32mGREEN\x1b[0m"]} />);
    const span = document.querySelector('span[style*="color"]');
    expect(span).not.toBeNull();
    expect(span?.textContent).toBe("GREEN");
  });

  it("closes unclosed spans at end of line", () => {
    render(<LogViewer lines={["\x1b[31mUNCLOSED"]} />);
    const container = document.querySelector("pre");
    // The innerHTML should have a closing </span> even without \x1b[0m
    const lineSpan = container?.querySelector('span[style*="color"]');
    expect(lineSpan).not.toBeNull();
    expect(lineSpan?.textContent).toBe("UNCLOSED");
    // Verify the span is properly closed (no dangling open tags)
    const html = lineSpan?.parentElement?.innerHTML ?? "";
    const opens = (html.match(/<span/g) || []).length;
    const closes = (html.match(/<\/span>/g) || []).length;
    expect(opens).toBe(closes);
  });

  it("handles multi-code ANSI sequences (bold+color)", () => {
    render(<LogViewer lines={["\x1b[1;32mBOLD GREEN\x1b[0m"]} />);
    const span = document.querySelector('span[style*="font-weight"]');
    expect(span).not.toBeNull();
    // Should have both bold and green in the style
    expect(span?.getAttribute("style")).toContain("font-weight:bold");
    expect(span?.getAttribute("style")).toContain("color:");
  });
});
