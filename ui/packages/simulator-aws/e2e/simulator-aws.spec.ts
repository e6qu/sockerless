import { test, expect } from "@playwright/test";

const SERVICES = [
  { path: "/ui/ecs", nav: "Elastic Container Service", title: "Tasks", columns: ["Task ARN", "Last status"] },
  { path: "/ui/lambda", nav: "Lambda", title: "Functions", columns: ["Function name", "State"] },
  { path: "/ui/ecr", nav: "Elastic Container Registry", title: "Repositories", columns: ["Repository name", "URI"] },
  { path: "/ui/s3", nav: "Simple Storage Service", title: "Buckets", columns: ["Name", "Creation date"] },
  { path: "/ui/logs", nav: "CloudWatch Logs", title: "Log groups", columns: ["Log group", "Retention"] },
  { path: "/ui/organizations", nav: "AWS Organizations", title: "AWS accounts", columns: ["Account name", "Account ID", "Email"] },
  { path: "/ui/iam", nav: "Identity and Access Management", title: "Users", columns: ["User name", "ARN"] },
];

test.describe("AWS console shell", () => {
  test("presents the console header, breadcrumbs and grouped service navigation", async ({ page }) => {
    await page.goto("/ui/");
    expect((await page.request.get("/ui/favicon.svg")).status()).toBe(200);
    await expect(page.locator(".aws-header")).toBeVisible();
    await expect(page.getByRole("navigation", { name: "Breadcrumbs" })).toBeVisible();
    const nav = page.getByRole("navigation", { name: "Service" });
    for (const group of [
      "Dashboard",
      "Compute",
      "Storage and registry",
      "Management & Governance",
      "Security, identity, and compliance",
    ]) {
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
    const toggle = page.getByRole("button", { name: /Switch to (light|dark) theme/ });
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

// These pin the ground-truth values read from the live Cloudscape Design
// System (cloudscape.design) — the console's font, action blue, rounded
// containers, and header/table icon controls — so a regression away from the
// AWS look fails here rather than being judged by eye.
test.describe("Cloudscape visual fidelity", () => {
  test("renders console text in Open Sans, Cloudscape's console font", async ({ page }) => {
    await page.goto("/ui/");
    // The shared shell sets the document body to its own display face; the AWS
    // console reasserts Open Sans on its own root, so read that element.
    const family = await page.locator(".aws").evaluate((node) => getComputedStyle(node).fontFamily);
    expect(family).toContain("Open Sans");
  });

  test("uses Cloudscape's action blue for inactive links and dark text for the active one", async ({ page }) => {
    // Pin both treatments on a page with a known active route: the current
    // service reads as dark text, every other service link as the action blue.
    // The colour lives on the .aws-sidenav-link span, not the wrapping anchor.
    await page.goto("/ui/lambda");
    const nav = page.getByRole("navigation", { name: "Service" });
    const colorOf = (name: string) =>
      nav.locator(".aws-sidenav-link", { hasText: name }).first().evaluate((node) => getComputedStyle(node).color);
    // #0972d3, Cloudscape's action blue, read from the design system.
    expect(await colorOf("Elastic Container Service")).toBe("rgb(9, 114, 211)");
    // #0f141a, Cloudscape's primary text, marks the current page.
    expect(await colorOf("Lambda")).toBe("rgb(15, 20, 26)");
  });

  test("rounds containers the way the current Cloudscape theme does", async ({ page }) => {
    await page.goto("/ui/ecs");
    const radius = await page
      .locator(".aws-container")
      .first()
      .evaluate((node) => getComputedStyle(node).borderTopLeftRadius);
    expect(radius).toBe("16px");
  });

  test("carries the global search field and header tool icons", async ({ page }) => {
    await page.goto("/ui/");
    const header = page.locator(".aws-header");
    await expect(header.getByRole("searchbox", { name: "Search" })).toBeVisible();
    for (const label of ["Notifications", "Settings", "Support"]) {
      const button = header.getByRole("button", { name: label });
      await expect(button).toBeVisible();
      await expect(button.locator("svg")).toHaveCount(1);
    }
  });

  test("gives the table a search-prefixed filter and a refresh control", async ({ page }) => {
    await page.goto("/ui/ecs");
    const filter = page.locator(".aws-table-filter");
    await expect(filter.locator("svg")).toHaveCount(1);
    await expect(filter.getByRole("searchbox")).toBeVisible();
    await expect(page.locator(".aws-table-tools").getByRole("button", { name: "Refresh" })).toBeVisible();
  });
});

test.describe("Overview", () => {
  test("states the Region alongside the resource counts", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
    await expect(page.getByText("us-east-1").first()).toBeVisible();
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

// The Users page's credential-minting affordances are part of the shell — the
// header actions and the create dialog render before (and regardless of) any
// cloud read, so they are assertable here without an identity provider. The
// authenticated mint→configure→signed-read loop belongs to the Shauth
// relying-party suite.
test.describe("Identity and Access Management credential minting", () => {
  test("offers Create user and opens the create dialog", async ({ page }) => {
    await page.goto("/ui/iam");
    const create = page.getByTestId("iam-create-user");
    await expect(create).toBeVisible();
    await expect(page.getByTestId("iam-delete-user")).toBeDisabled();
    await create.click();
    const dialog = page.getByRole("dialog", { name: "Create user" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId("iam-user-name-input")).toBeVisible();
    // An empty or invalid user name must not be submittable.
    await expect(dialog.getByTestId("iam-create-user-submit")).toBeDisabled();
    await dialog.getByTestId("iam-user-name-input").fill("cli-operator");
    await expect(dialog.getByTestId("iam-create-user-submit")).toBeEnabled();
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });

  test("routes to a user's Security credentials page with the access-key table and Create access key", async ({
    page,
  }) => {
    await page.goto("/ui/iam/users/cli-operator");
    await expect(page.getByRole("heading", { name: "cli-operator" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Access keys" })).toBeVisible();
    for (const column of ["Access key ID", "Status", "Created"]) {
      await expect(page.getByRole("columnheader", { name: column })).toBeVisible();
    }
    await expect(page.getByTestId("iam-create-access-key")).toBeVisible();
    const crumbs = page.getByRole("navigation", { name: "Breadcrumbs" });
    await expect(crumbs).toContainText("Identity and Access Management");
    await expect(crumbs).toContainText("cli-operator");
  });
});

// The Organizations page's account-management affordances are part of the
// shell — the header actions and the Add an AWS account dialog render before
// (and regardless of) any cloud read. The authenticated
// CreateAccount→DescribeCreateAccountStatus→ListAccounts loop belongs to the
// Shauth relying-party suite.
test.describe("AWS Organizations account management", () => {
  test("offers Add an AWS account and opens the add dialog", async ({ page }) => {
    await page.goto("/ui/organizations");
    await expect(page.getByTestId("org-accounts-table")).toBeVisible();
    await expect(page.getByTestId("org-remove-account")).toBeDisabled();
    await expect(page.getByTestId("org-close-account")).toBeDisabled();
    const add = page.getByTestId("org-add-account");
    await expect(add).toBeVisible();
    await add.click();
    const dialog = page.getByRole("dialog", { name: "Add an AWS account" });
    await expect(dialog).toBeVisible();
    // Neither field alone is enough: CreateAccount requires the account name
    // and the owner email, so the submit stays disabled until both are valid.
    await expect(dialog.getByTestId("org-add-account-submit")).toBeDisabled();
    await dialog.getByTestId("org-account-name-input").fill("Sandbox");
    await expect(dialog.getByTestId("org-add-account-submit")).toBeDisabled();
    await dialog.getByTestId("org-account-email-input").fill("sandbox@sim.invalid");
    await expect(dialog.getByTestId("org-add-account-submit")).toBeEnabled();
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });

  test("routes to an account's detail page with the Organizations breadcrumb", async ({ page }) => {
    await page.goto("/ui/organizations/accounts/123456789012");
    await expect(page.getByRole("heading", { name: "123456789012" })).toBeVisible();
    await expect(page.getByTestId("org-remove-account")).toBeVisible();
    await expect(page.getByTestId("org-close-account")).toBeVisible();
    const crumbs = page.getByRole("navigation", { name: "Breadcrumbs" });
    await expect(crumbs).toContainText("AWS Organizations");
    await expect(crumbs).toContainText("123456789012");
  });
});

// The Amazon ECS, AWS Lambda, Amazon ECR, Amazon S3, and CloudWatch Logs
// pages used to render AwsTable's default "View details" and "Delete" header
// actions — enabled, but with no handler wired (BUG-2637). Each page now
// passes its own `actions`, so those inert defaults never render; the real
// action (Stop for ECS tasks, Delete elsewhere) takes their place, disabled
// until a row is selected. Like the IAM and Organizations header-action
// checks above, this is assertable without an identity provider — reading
// zero rows still renders the header controls. The authenticated
// select→confirm→mutate loop belongs to the Shauth relying-party suite.
test.describe("Resource header actions", () => {
  const cases = [
    { path: "/ui/ecs", testId: "ecs-stop-task", label: "Stop" },
    { path: "/ui/lambda", testId: "lambda-delete-function", label: "Delete" },
    { path: "/ui/ecr", testId: "ecr-delete-repo", label: "Delete" },
    { path: "/ui/s3", testId: "s3-delete-bucket", label: "Delete" },
    { path: "/ui/logs", testId: "logs-delete-log-group", label: "Delete" },
  ];

  for (const { path, testId, label } of cases) {
    test(`${path} wires a real, initially-disabled ${label} action and renders no inert default`, async ({ page }) => {
      await page.goto(path);
      const action = page.getByTestId(testId);
      await expect(action).toBeVisible();
      await expect(action).toHaveText(label);
      await expect(action).toBeDisabled();
      // The AwsTable default header actions this replaced.
      await expect(page.getByRole("button", { name: "View details" })).toHaveCount(0);
      if (label !== "Delete") {
        await expect(page.getByRole("button", { name: "Delete", exact: true })).toHaveCount(0);
      }
    });
  }
});

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

// The console's authenticated reads of the real AWS APIs are proven in the
// Shauth relying-party suite (ui/e2e/shauth-rps.mjs): with a live operator
// identity the console federates into credentials and reads the real APIs. This
// lightweight per-package suite has no identity provider, so the console reaches
// the simulator unauthenticated and the now-enforcing simulator rejects those
// reads exactly as real AWS would — data-render assertions therefore belong to
// the authenticated RPS path, not here. This suite covers the shell, the
// navigation, and the visual language, which do not depend on cloud reads.
