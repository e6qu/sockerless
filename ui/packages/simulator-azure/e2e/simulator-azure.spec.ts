import { test, expect } from "@playwright/test";

const SERVICES = [
  { path: "/ui/container-apps", menu: "Container Apps", columns: ["Name", "Resource group", "Type"] },
  { path: "/ui/functions", menu: "Function Apps", columns: ["Name", "Resource group", "App kind"] },
  { path: "/ui/acr", menu: "Container registries", columns: ["Name", "Login server"] },
  { path: "/ui/storage", menu: "Storage accounts", columns: ["Name", "Kind"] },
  { path: "/ui/monitor", menu: "Logs", columns: ["Time generated", "Source", "Message"] },
];

test.describe("Azure portal shell", () => {
  test("presents the portal header, breadcrumb, resource title and command bar", async ({ page }) => {
    await page.goto("/ui/");
    expect((await page.request.get("/ui/favicon.svg")).status()).toBe(200);
    await expect(page.locator(".az-header")).toBeVisible();
    await expect(page.getByRole("navigation", { name: "Breadcrumbs" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Simulator" })).toBeVisible();
    await expect(page.getByRole("toolbar", { name: "Commands" })).toBeVisible();
    await expect(page.getByLabel("Search resources, services, and docs")).toBeVisible();
  });

  // A fixed-height header leaves anything taller drawn outside its box, where
  // the bar below paints over it. A control covered by an opaque sibling still
  // reports as visible, so the failure shows up only as clicks landing on the
  // wrong element — as it did on the AWS console. Assert containment.
  test("contains every header control within the header itself", async ({ page }) => {
    await page.goto("/ui/");
    const header = page.locator(".az-header");
    const headerBox = await header.boundingBox();
    expect(headerBox).not.toBeNull();
    const controls = header.locator(":scope > * > *");
    const count = await controls.count();
    expect(count).toBeGreaterThan(0);
    for (let index = 0; index < count; index += 1) {
      const control = controls.nth(index);
      const box = await control.boundingBox();
      if (!box) continue;
      const label = await control.evaluate((node) => node.outerHTML.slice(0, 80));
      expect(box.y, `header control escaped the header: ${label}`).toBeGreaterThanOrEqual(headerBox!.y - 0.5);
      expect(box.y + box.height, `header control escaped the header: ${label}`).toBeLessThanOrEqual(
        headerBox!.y + headerBox!.height + 0.5,
      );
    }
  });

  test("puts the theme control in the top right and switches both ways", async ({ page }) => {
    await page.goto("/ui/");
    const toggle = page.locator(".az-icon-button");
    const headerBox = await page.locator(".az-header").boundingBox();
    const toggleBox = await toggle.boundingBox();
    expect(toggleBox!.x).toBeGreaterThan(headerBox!.x + headerBox!.width / 2);

    const isDark = () => page.evaluate(() => document.documentElement.classList.contains("dark"));
    const before = await isDark();
    await toggle.click();
    expect(await isDark()).toBe(!before);
    await toggle.click();
    expect(await isDark()).toBe(before);
  });

  test("keeps the Azure API root separate from /ui/", async ({ page }) => {
    const response = await page.goto("/");
    expect(page.url()).toBe("http://localhost:19330/");
    expect(response?.status()).toBe(404);
  });

  test("exposes a skip link ahead of the portal content", async ({ page }) => {
    await page.goto("/ui/");
    await page.keyboard.press("Tab");
    await expect(page.locator(".sl-skip-link")).toBeFocused();
    await expect(page.locator("#main-content")).toHaveCount(1);
  });
});

test.describe("Service menu", () => {
  test("groups services and collapses a group without losing the others", async ({ page }) => {
    await page.goto("/ui/");
    const menu = page.getByRole("navigation", { name: "Service" });
    for (const group of ["Compute", "Storage and registry", "Monitoring"]) {
      await expect(menu.getByRole("button", { name: group })).toBeVisible();
    }
    await expect(menu.getByRole("link", { name: "Container Apps" })).toBeVisible();
    await menu.getByRole("button", { name: "Compute" }).click();
    await expect(menu.getByRole("link", { name: "Container Apps" })).toHaveCount(0);
    await expect(menu.getByRole("link", { name: "Storage accounts" })).toBeVisible();
    await menu.getByRole("button", { name: "Compute" }).click();
    await expect(menu.getByRole("link", { name: "Container Apps" })).toBeVisible();
  });

  test("narrows to what a search matches, opening a collapsed group to show it", async ({ page }) => {
    await page.goto("/ui/");
    const menu = page.getByRole("navigation", { name: "Service" });
    await menu.getByRole("button", { name: "Compute" }).click();
    await menu.getByLabel("Search the service menu").fill("registries");
    await expect(menu.getByRole("link", { name: "Container registries" })).toBeVisible();
    await expect(menu.getByRole("link", { name: "Storage accounts" })).toHaveCount(0);
  });
});

test.describe("Overview", () => {
  test("leads with Essentials and links each count to its resource", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("region", { name: "Essentials" })).toBeVisible();
    await page.getByRole("link", { name: /^\d+$/ }).first().click();
    await expect(page.getByRole("toolbar", { name: "Commands" })).toBeVisible();
  });
});

for (const service of SERVICES) {
  test.describe(service.menu, () => {
    test("renders Essentials and its table columns", async ({ page }) => {
      await page.goto(service.path);
      await expect(page.getByRole("region", { name: "Essentials" })).toBeVisible();
      for (const column of service.columns) {
        await expect(page.getByRole("columnheader", { name: column, exact: true })).toBeVisible();
      }
    });
  });
}

test.describe("Navigation", () => {
  test("reaches every service from the menu and updates the breadcrumb", async ({ page }) => {
    await page.goto("/ui/");
    const crumbs = page.getByRole("navigation", { name: "Breadcrumbs" });
    for (const service of SERVICES) {
      await page.getByRole("link", { name: service.menu, exact: true }).click();
      await expect(page).toHaveURL(new RegExp(`${service.path}$`));
      await expect(crumbs).toContainText(service.menu);
    }
    await page.getByRole("link", { name: "Overview", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Simulator" })).toBeVisible();
  });
});

test.describe("Contrast", () => {
  // Measured against the surfaces the browser actually paints, walking up for
  // the first opaque background, rather than asserted from the palette. The
  // header sits at 4.53:1 — white on the portal's own header blue — so this
  // has little room to spare and is worth holding.
  test("every text role clears WCAG AA in both themes", async ({ page }) => {
    await page.goto("/ui/container-apps");
    const results = await page.evaluate(() => {
      const parse = (c: string) => {
        const m = (c.match(/[\d.]+/g) ?? []).map(Number);
        return [m[0] ?? 0, m[1] ?? 0, m[2] ?? 0, m[3] ?? 1] as const;
      };
      const lin = (v: number) => {
        const n = v / 255;
        return n <= 0.04045 ? n / 12.92 : Math.pow((n + 0.055) / 1.055, 2.4);
      };
      const lum = (c: readonly number[]) => 0.2126 * lin(c[0]) + 0.7152 * lin(c[1]) + 0.0722 * lin(c[2]);
      const opaqueBehind = (el: Element) => {
        let node: Element | null = el;
        while (node && node !== document.documentElement) {
          const c = parse(getComputedStyle(node).backgroundColor);
          if (c[3] > 0) return c;
          node = node.parentElement;
        }
        return [255, 255, 255, 1] as const;
      };
      const ratio = (el: Element) => {
        const a = lum(parse(getComputedStyle(el).color));
        const b = lum(opaqueBehind(el));
        return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
      };
      const selectors = [
        ".az-wordmark",
        ".az-breadcrumbs a",
        ".az-resource-title h1",
        ".az-resource-title p",
        ".az-command:not(:disabled)",
        ".az-service-link",
        ".az-service-link-active",
        ".az-service-group-toggle",
        ".az-essentials-pair dt",
        ".az-essentials-pair dd",
        ".az-table th button",
        ".az-empty strong",
        ".az-empty p",
      ];
      const sample = () =>
        selectors
          .map((selector) => {
            const el = document.querySelector(selector);
            return el ? { selector, ratio: ratio(el) } : null;
          })
          .filter((entry): entry is { selector: string; ratio: number } => entry !== null);

      document.documentElement.classList.remove("dark");
      const light = sample();
      document.documentElement.classList.add("dark");
      const dark = sample();
      document.documentElement.classList.remove("dark");
      return { light, dark };
    });

    for (const [theme, samples] of Object.entries(results)) {
      expect(samples.length).toBeGreaterThan(10);
      for (const sample of samples) {
        expect(sample.ratio, `${theme}: ${sample.selector} measured ${sample.ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(4.5);
      }
    }
  });
});
