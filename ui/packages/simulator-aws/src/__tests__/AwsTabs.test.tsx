import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { AwsTabs } from "../console/AwsTabs.js";

/**
 * AwsTabs is the one genuinely new interactive shared component this pass
 * adds — the detail pages' resource data always requires a live cloud read,
 * so it isn't reachable through the per-package Playwright suite (no
 * identity provider there). Its ARIA structure and keyboard contract are
 * proven directly here instead, against inert tab content. This package has
 * no jest-dom matchers installed, so visibility and focus are read straight
 * off the DOM the way the rest of this suite already does.
 */

afterEach(cleanup);

function renderTabs() {
  return render(
    <AwsTabs
      ariaLabel="Test detail"
      tabs={[
        { id: "a", label: "First", content: <p data-testid="panel-a">First content</p> },
        { id: "b", label: "Second", content: <p data-testid="panel-b">Second content</p> },
        { id: "c", label: "Third", content: <p data-testid="panel-c">Third content</p> },
      ]}
    />,
  );
}

/** Only the active tab's panel renders its children (see AwsTabs.tsx: an
 * inactive panel's content is never mounted, so its own text can never be
 * announced twice) — every panel still exists in the DOM, `hidden`, with its
 * `aria-controls`/id pairing intact, which is what this reads directly
 * rather than depending on content that may not be there. */
function panelFor(tabLabel: string): HTMLElement {
  const tab = screen.getByRole("tab", { name: tabLabel });
  const panelId = tab.getAttribute("aria-controls");
  const panel = panelId ? document.getElementById(panelId) : null;
  if (!panel) throw new Error(`no panel found for tab "${tabLabel}"`);
  return panel;
}

describe("AwsTabs", () => {
  it("exposes a tablist with one tab per entry and marks the first as selected by default", () => {
    renderTabs();
    expect(screen.getByRole("tablist", { name: "Test detail" })).toBeTruthy();
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((tab) => tab.textContent)).toEqual(["First", "Second", "Third"]);
    expect(tabs[0].getAttribute("aria-selected")).toBe("true");
    expect(tabs[1].getAttribute("aria-selected")).toBe("false");
    expect(tabs[2].getAttribute("aria-selected")).toBe("false");
  });

  it("shows only the active panel's content, each panel labelled by its own tab", () => {
    renderTabs();
    expect(panelFor("First").hasAttribute("hidden")).toBe(false);
    expect(panelFor("Second").hasAttribute("hidden")).toBe(true);
    expect(panelFor("Third").hasAttribute("hidden")).toBe(true);
    expect(screen.getByTestId("panel-a").textContent).toBe("First content");
    expect(screen.queryByTestId("panel-b")).toBeNull();
    expect(screen.queryByTestId("panel-c")).toBeNull();
    const firstTab = screen.getByRole("tab", { name: "First" });
    expect(panelFor("First").getAttribute("aria-labelledby")).toBe(firstTab.id);
  });

  it("switches the active tab and panel on click, mounting only the newly active content", () => {
    renderTabs();
    fireEvent.click(screen.getByRole("tab", { name: "Second" }));
    expect(screen.getByRole("tab", { name: "Second" }).getAttribute("aria-selected")).toBe("true");
    expect(panelFor("Second").hasAttribute("hidden")).toBe(false);
    expect(panelFor("First").hasAttribute("hidden")).toBe(true);
    expect(screen.getByTestId("panel-b").textContent).toBe("Second content");
    expect(screen.queryByTestId("panel-a")).toBeNull();
  });

  it("keeps only the active tab in the page's Tab order (roving tabindex)", () => {
    renderTabs();
    const [first, second, third] = screen.getAllByRole("tab");
    expect(first.tabIndex).toBe(0);
    expect(second.tabIndex).toBe(-1);
    expect(third.tabIndex).toBe(-1);
    fireEvent.click(second);
    expect(first.tabIndex).toBe(-1);
    expect(second.tabIndex).toBe(0);
  });

  it("moves focus and selection with ArrowRight/ArrowLeft, wrapping at the ends", () => {
    renderTabs();
    const [first, second, third] = screen.getAllByRole("tab");
    first.focus();
    fireEvent.keyDown(first, { key: "ArrowRight" });
    expect(document.activeElement).toBe(second);
    expect(second.getAttribute("aria-selected")).toBe("true");

    fireEvent.keyDown(second, { key: "ArrowRight" });
    expect(document.activeElement).toBe(third);

    // Wraps forward from the last tab back to the first.
    fireEvent.keyDown(third, { key: "ArrowRight" });
    expect(document.activeElement).toBe(first);

    // Wraps backward from the first tab to the last.
    fireEvent.keyDown(first, { key: "ArrowLeft" });
    expect(document.activeElement).toBe(third);
  });

  it("jumps to the first and last tab with Home and End", () => {
    renderTabs();
    const [first, second, third] = screen.getAllByRole("tab");
    second.focus();
    fireEvent.keyDown(second, { key: "End" });
    expect(document.activeElement).toBe(third);
    fireEvent.keyDown(third, { key: "Home" });
    expect(document.activeElement).toBe(first);
  });
});
