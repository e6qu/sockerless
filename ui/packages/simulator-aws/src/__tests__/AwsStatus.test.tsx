import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AwsStatus } from "../console/AwsConsole.js";

/**
 * Status is matched on whole words, not substrings. "Unavailable" contains
 * "available" and "Inactive" contains "active", so a substring test reports a
 * green success tick for a failure — which is worse than showing nothing,
 * because an operator reads the tick and stops looking.
 */
describe("AwsStatus", () => {
  function kindOf(status: string): string {
    const { container } = render(<AwsStatus status={status} />);
    const element = container.querySelector("span[class*='aws-status-']");
    return element?.className.match(/aws-status-(\w+)/)?.[1] ?? "";
  }

  it("reports failure states as failures even when a success word is a substring", () => {
    expect(kindOf("Unavailable")).toBe("error");
    expect(kindOf("Inactive")).toBe("error");
    expect(kindOf("Deactivated")).toBe("error");
    expect(kindOf("STOPPED")).toBe("error");
  });

  it("reports genuine success states as successes", () => {
    expect(kindOf("Running")).toBe("success");
    expect(kindOf("Available")).toBe("success");
    expect(kindOf("Active")).toBe("success");
  });

  it("reports transitional states as in progress", () => {
    expect(kindOf("Pending")).toBe("warning");
    expect(kindOf("Provisioning")).toBe("warning");
  });

  it("carries a shape so the state does not depend on colour alone", () => {
    const { container } = render(<AwsStatus status="Unavailable" />);
    const status = container.querySelector(".aws-status-error");
    expect(status?.textContent).toContain("Unavailable");
    // The icon is a distinct shape per state — a screen reader ignores it
    // (aria-hidden) and reads the word, while a sighted user reads the shape.
    const icon = status?.querySelector("svg[aria-hidden]");
    expect(icon).not.toBeNull();
    // The error and success shapes differ, so state is legible without colour.
    const success = render(<AwsStatus status="Running" />).container.querySelector(
      ".aws-status-success svg[aria-hidden] path",
    );
    const error = icon?.querySelector("path");
    expect(success?.getAttribute("d")).not.toBe(error?.getAttribute("d"));
  });
});
