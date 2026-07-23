import { test, expect } from "@playwright/test";

const SERVICES = [
  { path: "/ui/ecs", nav: "Elastic Container Service", title: "Tasks", columns: ["Task ARN", "Last status"] },
  { path: "/ui/lambda", nav: "Lambda", title: "Functions", columns: ["Function name", "State"] },
  { path: "/ui/ecr", nav: "Elastic Container Registry", title: "Repositories", columns: ["Repository name", "URI"] },
  { path: "/ui/s3", nav: "Simple Storage Service", title: "Buckets", columns: ["Name", "Creation date"] },
  { path: "/ui/logs", nav: "CloudWatch Logs", title: "Log groups", columns: ["Log group", "Retention"] },
];

test.describe("AWS console shell", () => {
  test("presents the console header, breadcrumbs and grouped service navigation", async ({ page }) => {
    await page.goto("/ui/");
    expect((await page.request.get("/ui/favicon.svg")).status()).toBe(200);
    await expect(page.locator(".aws-header")).toBeVisible();
    await expect(page.getByRole("navigation", { name: "Breadcrumbs" })).toBeVisible();
    const nav = page.getByRole("navigation", { name: "Service" });
    for (const group of ["Dashboard", "Compute", "Storage and registry", "Management"]) {
      await expect(nav.getByText(group, { exact: true })).toBeVisible();
    }
  });

  // The header used to be a fixed 40px tall. Anything taller than that — the
  // account control in particular — spilled below it, and the breadcrumb bar
  // painted over the overflow. A control covered by an opaque sibling still
  // reports as visible, so the failure surfaced only as clicks landing on the
  // breadcrumbs instead of the sign-out control. Assert containment, which is
  // the property that actually broke.
  test("contains every header control within the header itself", async ({ page }) => {
    await page.goto("/ui/");
    const header = page.locator(".aws-header");
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
    const toggle = page.locator(".aws-theme-toggle");
    const headerBox = await page.locator(".aws-header").boundingBox();
    const toggleBox = await toggle.boundingBox();
    expect(toggleBox!.x).toBeGreaterThan(headerBox!.x + headerBox!.width / 2);

    const isDark = () => page.evaluate(() => document.documentElement.classList.contains("dark"));
    const before = await isDark();
    await toggle.click();
    expect(await isDark()).toBe(!before);
    await toggle.click();
    expect(await isDark()).toBe(before);
  });

  test("keeps the AWS S3 root protocol surface separate from /ui/", async ({ page }) => {
    const response = await page.goto("/");
    expect(page.url()).toBe("http://localhost:19310/");
    expect(response?.status()).toBe(200);
    await expect(page.locator("body")).toContainText("ListAllMyBucketsResult");
  });

  test("exposes a skip link ahead of the console content", async ({ page }) => {
    await page.goto("/ui/");
    await page.keyboard.press("Tab");
    await expect(page.locator(".sl-skip-link")).toBeFocused();
    await expect(page.locator("#main-content")).toHaveCount(1);
  });
});

test.describe("Overview", () => {
  test("states the Region alongside the resource counts", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
    await expect(page.getByText("eu-west-1").first()).toBeVisible();
  });
});

for (const service of SERVICES) {
  test.describe(service.nav, () => {
    test("renders its heading and table columns", async ({ page }) => {
      await page.goto(service.path);
      await expect(page.getByRole("heading", { name: service.title })).toBeVisible();
      for (const column of service.columns) {
        await expect(page.getByRole("columnheader", { name: column })).toBeVisible();
      }
    });
  });
}

test.describe("Navigation", () => {
  test("reaches every service from the side navigation and updates the breadcrumb", async ({ page }) => {
    await page.goto("/ui/");
    const crumbs = page.getByRole("navigation", { name: "Breadcrumbs" });
    for (const service of SERVICES) {
      await page.getByRole("link", { name: service.nav, exact: true }).click();
      await expect(page).toHaveURL(new RegExp(`${service.path}$`));
      await expect(page.getByRole("heading", { name: service.title })).toBeVisible();
      await expect(crumbs).toContainText(service.nav);
    }
    await page.getByRole("link", { name: "Overview", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  });
});

test.describe("The console reads the real AWS APIs", () => {
  // Seeds resources through the real AWS APIs the console reads, so the
  // assertions prove live resources render rather than a fixture. The simulator
  // accepts the calls unsigned; the console signs them, which the relying-party
  // suite exercises with a live identity.
  test("lists an ECR repository created through the real API", async ({ page }) => {
    const name = `console-repo-${Date.now()}`;
    const created = await page.request.post("/", {
      headers: { "content-type": "application/x-amz-json-1.1", "x-amz-target": "AmazonEC2ContainerRegistry_V20150921.CreateRepository" },
      data: { repositoryName: name },
    });
    expect(created.ok(), `creating repository: HTTP ${created.status()}`).toBeTruthy();

    await page.goto("/ui/ecr");
    await expect(page.getByRole("cell", { name, exact: true })).toBeVisible();
  });

  test("lists an S3 bucket created through the real API", async ({ page }) => {
    const name = `console-bucket-${Date.now()}`;
    const created = await page.request.put(`/${name}`);
    expect(created.ok(), `creating bucket: HTTP ${created.status()}`).toBeTruthy();

    await page.goto("/ui/s3");
    await expect(page.getByRole("cell", { name, exact: true })).toBeVisible();
  });

  test("lists a CloudWatch log group created through the real API", async ({ page }) => {
    const name = `console-log-group-${Date.now()}`;
    const created = await page.request.post("/", {
      headers: { "content-type": "application/x-amz-json-1.1", "x-amz-target": "Logs_20140328.CreateLogGroup" },
      data: { logGroupName: name },
    });
    expect(created.ok(), `creating log group: HTTP ${created.status()}`).toBeTruthy();

    await page.goto("/ui/logs");
    await expect(page.getByRole("cell", { name, exact: true })).toBeVisible();
  });
});
