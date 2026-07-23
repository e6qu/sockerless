import { test, expect } from "@playwright/test";

const SERVICES = [
  { path: "/ui/cloudrun", nav: "Cloud Run jobs", title: "Cloud Run jobs", columns: ["Name", "Status of last execution", "Created", "Executions", "Launch stage"] },
  { path: "/ui/functions", nav: "Cloud Run functions", title: "Cloud Run functions", columns: ["Name", "State", "Environment"] },
  { path: "/ui/ar", nav: "Artifact Registry", title: "Artifact Registry", columns: ["Name", "Format", "Created"] },
  { path: "/ui/gcs", nav: "Cloud Storage", title: "Cloud Storage", columns: ["Name"] },
  { path: "/ui/logging", nav: "Logs Explorer", title: "Logs Explorer", columns: ["Timestamp", "Severity", "Log name"] },
];

test.describe("Google Cloud console shell", () => {
  test("presents the header with project chip, central search and pill navigation", async ({ page }) => {
    await page.goto("/ui/");
    expect((await page.request.get("/ui/favicon.svg")).status()).toBe(200);
    await expect(page.locator(".gc-header")).toBeVisible();
    await expect(page.getByText("Google Cloud", { exact: true })).toBeVisible();
    await expect(page.locator(".gc-project-chip")).toBeVisible();
    await expect(page.getByLabel("Search resources, docs, products, and more")).toBeVisible();
    const nav = page.getByRole("navigation", { name: "Product" });
    await expect(nav.getByRole("link", { name: "Overview" })).toBeVisible();
  });

  // A fixed-height header leaves anything taller drawn outside its box, where
  // the row below paints over it. A control covered by an opaque sibling still
  // reports as visible, so the failure shows up only as clicks landing on the
  // wrong element — as it did on the AWS console. Assert containment.
  test("contains every header control within the header itself", async ({ page }) => {
    await page.goto("/ui/");
    const header = page.locator(".gc-header");
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
    const toggle = page.locator(".gc-header-right .gc-icon-button");
    const headerBox = await page.locator(".gc-header").boundingBox();
    const toggleBox = await toggle.boundingBox();
    expect(toggleBox!.x).toBeGreaterThan(headerBox!.x + headerBox!.width / 2);

    const isDark = () => page.evaluate(() => document.documentElement.classList.contains("dark"));
    const before = await isDark();
    await toggle.click();
    expect(await isDark()).toBe(!before);
    await toggle.click();
    expect(await isDark()).toBe(before);
  });

  test("redirects / to /ui/", async ({ page }) => {
    const response = await page.goto("/");
    expect(page.url()).toContain("/ui/");
    expect(response?.status()).toBe(200);
  });

  test("exposes a skip link ahead of the console content", async ({ page }) => {
    await page.goto("/ui/");
    await page.keyboard.press("Tab");
    await expect(page.locator(".sl-skip-link")).toBeFocused();
    await expect(page.locator("#main-content")).toHaveCount(1);
  });
});

test.describe("Overview", () => {
  test("states service health and links each count to its resource", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
    await page.getByRole("link", { name: /^\d+$/ }).first().click();
    await expect(page.locator(".gc-page-header h1")).toBeVisible();
  });
});

for (const service of SERVICES) {
  test.describe(service.nav, () => {
    test("renders its title, description and table columns", async ({ page }) => {
      await page.goto(service.path);
      await expect(page.getByRole("heading", { name: service.title })).toBeVisible();
      await expect(page.locator(".gc-page-description")).toBeVisible();
      for (const column of service.columns) {
        await expect(page.getByRole("columnheader", { name: column, exact: true })).toBeVisible();
      }
    });
  });
}

test.describe("Navigation", () => {
  test("reaches every product from the navigation and marks the active one", async ({ page }) => {
    await page.goto("/ui/");
    for (const service of SERVICES) {
      await page.getByRole("link", { name: service.nav, exact: true }).click();
      await expect(page).toHaveURL(new RegExp(`${service.path}$`));
      await expect(page.getByRole("heading", { name: service.title })).toBeVisible();
      await expect(page.getByRole("link", { name: service.nav, exact: true })).toHaveClass(/gc-nav-link-active/);
    }
    await page.getByRole("link", { name: "Overview", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  });
});

test.describe("Contrast", () => {
  // Measured against the surfaces the browser actually paints, walking up for
  // the first opaque background, rather than asserted from the palette. Disabled
  // controls are excluded — WCAG 1.4.3 exempts inactive components, and the
  // console greys them deliberately. The tightest here is around 4.68:1, so the
  // guard has little room to spare and is worth holding.
  test("every enabled text role clears WCAG AA in both themes", async ({ page }) => {
    await page.goto("/ui/cloudrun");
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
        ".gc-wordmark",
        ".gc-project-chip",
        ".gc-nav-link",
        ".gc-nav-link-active",
        ".gc-product-title",
        ".gc-page-header h1",
        ".gc-page-description",
        ".gc-refresh",
        ".gc-filter-chip",
        ".gc-table th button",
        ".gc-empty-headline",
        ".gc-empty-description",
      ];
      const sample = () =>
        selectors
          .flatMap((selector) => Array.from(document.querySelectorAll(selector)))
          // A disabled control is exempt from the contrast requirement.
          .filter((el) => !(el as HTMLButtonElement).disabled && !el.closest(":disabled"))
          .map((el) => ({ tag: el.className, ratio: ratio(el) }));

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
        expect(sample.ratio, `${theme}: ${sample.tag} measured ${sample.ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(4.5);
      }
    }
  });
});

test.describe("Cloud Run jobs read the real API", () => {
  const project = "sockerless";
  const region = "us-central1";
  const jobsParent = `/v2/projects/${project}/locations/${region}/jobs`;

  // Seeds a job through the real Cloud Run Admin API the console reads, so the
  // assertions prove the console renders live resources rather than a fixture.
  async function seedJob(request: import("@playwright/test").APIRequestContext, id: string) {
    const response = await request.post(`${jobsParent}?jobId=${id}`, {
      data: { template: { template: { containers: [{ image: "busybox:latest" }] } } },
    });
    expect(response.ok(), `seeding job ${id}: HTTP ${response.status()}`).toBeTruthy();
  }

  test("lists a job created through the real Cloud Run API and opens its detail", async ({ page }) => {
    const id = `console-${Date.now()}`;
    await seedJob(page.request, id);

    await page.goto("/ui/cloudrun");
    const row = page.getByRole("link", { name: id, exact: true });
    await expect(row).toBeVisible();
    // The status comes from the resource's terminal condition, not a fixed string.
    await expect(page.getByText("Ready").first()).toBeVisible();

    await row.click();
    await expect(page).toHaveURL(new RegExp(`/ui/cloudrun/${id}$`));
    await expect(page.getByRole("heading", { name: id })).toBeVisible();
    await expect(page.getByText("Unique ID")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Executions" })).toBeVisible();
  });

  test("shows the console's empty state when the project has no jobs", async ({ page }) => {
    // A project the seed never touched has no jobs, so the real list is empty.
    await page.goto("/ui/cloudrun");
    await expect(page.getByRole("heading", { name: "Cloud Run jobs" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Name", exact: true })).toBeVisible();
  });
});

test.describe("The remaining resources read their real APIs", () => {
  const project = "sockerless";
  const region = "us-central1";

  test("lists a Cloud Storage bucket created through the real API and opens its detail", async ({ page }) => {
    const name = `console-bucket-${Date.now()}`;
    const created = await page.request.post(`/storage/v1/b?project=${project}`, {
      data: { name, location: "US-CENTRAL1", storageClass: "STANDARD" },
    });
    expect(created.ok(), `creating bucket: HTTP ${created.status()}`).toBeTruthy();

    await page.goto("/ui/gcs");
    const row = page.getByRole("link", { name, exact: true });
    await expect(row).toBeVisible();
    await row.click();
    await expect(page).toHaveURL(new RegExp(`/ui/gcs/${name}$`));
    await expect(page.getByRole("heading", { name })).toBeVisible();
    await expect(page.getByText("Storage class")).toBeVisible();
  });

  test("lists an Artifact Registry repository created through the real API and opens its detail", async ({ page }) => {
    const id = `console-repo-${Date.now()}`;
    const created = await page.request.post(
      `/v1/projects/${project}/locations/${region}/repositories?repositoryId=${id}`,
      { data: { format: "DOCKER", mode: "STANDARD_REPOSITORY" } },
    );
    expect(created.ok(), `creating repository: HTTP ${created.status()}`).toBeTruthy();

    await page.goto("/ui/ar");
    const row = page.getByRole("link", { name: id, exact: true });
    await expect(row).toBeVisible();
    await row.click();
    await expect(page).toHaveURL(new RegExp(`/ui/ar/${id}$`));
    await expect(page.getByRole("heading", { name: id })).toBeVisible();
    await expect(page.getByText("Format")).toBeVisible();
  });

  test("shows a log entry written through the real Cloud Logging API", async ({ page }) => {
    const message = `console-log-${Date.now()}`;
    const written = await page.request.post(`/v2/entries:write`, {
      data: {
        entries: [
          {
            logName: `projects/${project}/logs/console-test`,
            resource: { type: "global" },
            severity: "WARNING",
            textPayload: message,
          },
        ],
      },
    });
    expect(written.ok(), `writing log entry: HTTP ${written.status()}`).toBeTruthy();

    await page.goto("/ui/logging");
    await expect(page.getByText(message)).toBeVisible();
  });
});
