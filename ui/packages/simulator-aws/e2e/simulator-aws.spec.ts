import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

/** Forces the console into the requested theme regardless of what the OS
 * colour-scheme preference or a prior test left it in, so a theme-fidelity
 * assertion never passes by accident. */
/** The production build's CSS minifier shortens `#ffffff` to `#fff` (and
 * would do the same for any other 6-digit hex with repeated nibble pairs);
 * that is a lossless rewrite of the same colour, not a fidelity regression,
 * so token assertions compare through this expansion rather than pinning
 * whichever spelling the minifier happens to choose. */
function expandHex(color: string): string {
  const match = /^#([0-9a-f])([0-9a-f])([0-9a-f])$/i.exec(color);
  return match ? `#${match[1]}${match[1]}${match[2]}${match[2]}${match[3]}${match[3]}`.toLowerCase() : color.toLowerCase();
}

async function ensureTheme(page: Page, theme: "light" | "dark") {
  const isDark = await page.evaluate(() => document.documentElement.classList.contains("dark"));
  if ((theme === "dark") !== isDark) {
    await page.getByRole("button", { name: /Switch to (light|dark) theme/ }).click();
  }
  await expect
    .poll(() => page.evaluate(() => document.documentElement.classList.contains("dark")))
    .toBe(theme === "dark");
}

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
    // Grouped the way AWS's own "All services" menu groups them — Containers
    // and Storage are separate categories there, not the simulator's own
    // "Storage and registry" shorthand — since the nav now lists AWS's real
    // catalog, supported and not, rather than only the services sockerless
    // implements.
    for (const group of [
      "Dashboard",
      "Compute",
      "Containers",
      "Storage",
      "Database",
      "Networking & content delivery",
      "Application integration",
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
    // #006ce0 — color-text-link-default, read from
    // @cloudscape-design/design-tokens@3.0.104's current theme.
    expect(await colorOf("Elastic Container Service")).toBe("rgb(0, 108, 224)");
    // #0f141a — color-text-body-default, marks the current page.
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

// The real AWS console lists its full service catalog and is honest about
// what an account can and cannot use; this simulator does the same for what
// it does and does not implement. These tests pin that contract: a service
// AWS offers but sockerless has not built stays in the nav, reachable, and
// visibly and audibly marked as unsupported, rather than hidden or silently
// broken.
test.describe("Not-supported services", () => {
  test("marks an unimplemented AWS service with a visible pill and an accessible name that states it, and routes to an honest page", async ({
    page,
  }) => {
    await page.goto("/ui/");
    const nav = page.getByRole("navigation", { name: "Service" });
    const link = nav.getByRole("link", { name: "EC2, not supported in this simulator" });
    await expect(link).toBeVisible();
    await expect(link.getByText("Not supported")).toBeVisible();
    await link.click();
    await expect(page).toHaveURL(/\/ui\/not-supported\/ec2$/);
    await expect(page.getByRole("heading", { name: "EC2" })).toBeVisible();
    await expect(page.getByText("This service is not implemented by the Sockerless simulator.")).toBeVisible();
    await expect(page.getByRole("navigation", { name: "Breadcrumbs" })).toContainText("EC2");
  });

  test("names the service from the catalog when the not-supported page is reached directly by URL", async ({ page }) => {
    await page.goto("/ui/not-supported/dynamodb");
    await expect(page.getByRole("heading", { name: "DynamoDB" })).toBeVisible();
    await expect(page.getByLabel("DynamoDB is not supported in this simulator")).toBeVisible();
  });

  test("renders no not-supported pill on any service sockerless implements", async ({ page }) => {
    await page.goto("/ui/");
    const nav = page.getByRole("navigation", { name: "Service" });
    for (const service of SERVICES) {
      const link = nav.getByRole("link", { name: service.nav, exact: true });
      await expect(link.getByText("Not supported")).toHaveCount(0);
    }
  });
});

test.describe("Accessibility landmarks and keyboard operability", () => {
  test("exposes banner, service navigation, breadcrumb navigation, and main landmarks", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("banner")).toBeVisible();
    await expect(page.getByRole("navigation", { name: "Service" })).toBeVisible();
    await expect(page.getByRole("navigation", { name: "Breadcrumbs" })).toBeVisible();
    await expect(page.getByRole("main")).toBeVisible();
  });

  test("marks the current service with aria-current, and no other service, via react-router's NavLink", async ({
    page,
  }) => {
    await page.goto("/ui/lambda");
    const nav = page.getByRole("navigation", { name: "Service" });
    await expect(nav.getByRole("link", { name: "Lambda", exact: true })).toHaveAttribute("aria-current", "page");
    await expect(nav.getByRole("link", { name: "Elastic Container Service", exact: true })).not.toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  test("toggles the side navigation from the header's hamburger control, reflected in aria-expanded", async ({
    page,
  }) => {
    await page.goto("/ui/");
    const toggle = page.getByRole("button", { name: /(Open|Close) navigation/ });
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByRole("button", { name: "Close navigation" })).toBeVisible();
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(page.getByRole("button", { name: "Open navigation" })).toBeVisible();
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
  });

  test("carries a visible focus indicator that clears 3:1 in both themes", async ({ page }) => {
    await page.goto("/ui/");
    const link = page.getByRole("navigation", { name: "Service" }).getByRole("link", { name: "Overview" });
    await link.focus();
    const outline = await link.evaluate((el) => {
      const style = getComputedStyle(el);
      return { style: style.outlineStyle, width: style.outlineWidth };
    });
    expect(outline.style).not.toBe("none");
    expect(parseFloat(outline.width)).toBeGreaterThan(0);
  });

  test("moves focus into a dialog on open, honouring an autofocused field, and returns it to the trigger on Escape", async ({
    page,
  }) => {
    await page.goto("/ui/iam");
    const trigger = page.getByTestId("iam-create-user");
    await trigger.click();
    const dialog = page.getByRole("dialog", { name: "Create user" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId("iam-user-name-input")).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(trigger).toBeFocused();
  });

  test("traps Tab focus inside an open dialog, wrapping from the last control back to the first and vice versa", async ({
    page,
  }) => {
    await page.goto("/ui/iam");
    await page.getByTestId("iam-create-user").click();
    const dialog = page.getByRole("dialog", { name: "Create user" });
    const closeButton = dialog.getByRole("button", { name: "Close" });
    // "Create user" starts disabled (the name field is empty), so the browser
    // itself would never Tab to it — the trap's own notion of "last focusable
    // control" has to agree, and here that is Cancel, not the submit button.
    const cancelButton = dialog.getByRole("button", { name: "Cancel" });
    await expect(dialog.getByTestId("iam-create-user-submit")).toBeDisabled();
    await closeButton.focus();
    await page.keyboard.press("Shift+Tab");
    await expect(cancelButton).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(closeButton).toBeFocused();
  });
});

test.describe("Theme fidelity", () => {
  // These pin the ground-truth colour tokens read from
  // @cloudscape-design/design-tokens@3.0.104's current ("visual refresh")
  // theme (index-visual-refresh.json, `$value.light` / `$value.dark`) — see
  // tokens.css for the full derivation and citations.
  test("drives light-mode surface, text, and link colour from the published Cloudscape tokens", async ({ page }) => {
    await page.goto("/ui/ecs");
    await ensureTheme(page, "light");
    const tokens = await page.locator(".aws").evaluate((node) => {
      const cs = getComputedStyle(node);
      return {
        surface: cs.getPropertyValue("--aws-surface").trim(),
        fg: cs.getPropertyValue("--aws-fg").trim(),
        link: cs.getPropertyValue("--aws-link").trim(),
      };
    });
    expect(expandHex(tokens.surface)).toBe("#ffffff");
    expect(expandHex(tokens.fg)).toBe("#0f141a");
    expect(expandHex(tokens.link)).toBe("#006ce0");
  });

  // Cloudscape's current theme draws dark-mode content on one surface colour
  // instead of a lighter/darker gutter-vs-card contrast (see tokens.css); the
  // layout background and the container background are the same token.
  test("drives dark-mode surface, text, and link colour from the published Cloudscape tokens, with no separate page gutter", async ({
    page,
  }) => {
    await page.goto("/ui/ecs");
    await ensureTheme(page, "dark");
    const tokens = await page.locator(".aws").evaluate((node) => {
      const cs = getComputedStyle(node);
      return {
        surface: cs.getPropertyValue("--aws-surface").trim(),
        layout: cs.getPropertyValue("--aws-layout-bg").trim(),
        fg: cs.getPropertyValue("--aws-fg").trim(),
        link: cs.getPropertyValue("--aws-link").trim(),
      };
    });
    expect(expandHex(tokens.surface)).toBe("#161d26");
    expect(tokens.layout).toBe(tokens.surface);
    expect(expandHex(tokens.fg)).toBe("#c6c6cd");
    expect(expandHex(tokens.link)).toBe("#42b4ff");
  });

  test("keeps the not-supported badge's grey background and readable text in both themes", async ({ page }) => {
    await page.goto("/ui/not-supported/ec2");
    for (const theme of ["light", "dark"] as const) {
      await ensureTheme(page, theme);
      // Scoped to the page-header badge by its accessible name: the side
      // navigation renders the same "Not supported" pill (decorative there,
      // its own accessible name lives on the enclosing link) for every
      // unimplemented service, so a class-only locator would match dozens.
      const badge = page.getByLabel("EC2 is not supported in this simulator");
      await expect(badge).toBeVisible();
      const colors = await badge.evaluate((node) => {
        const cs = getComputedStyle(node);
        return { bg: cs.backgroundColor, fg: cs.color };
      });
      // #f9f9fa in both themes — color-text-badge-grey.
      expect(colors.fg).toBe("rgb(249, 249, 250)");
      expect(colors.bg).not.toBe("rgba(0, 0, 0, 0)");
    }
  });
});

/**
 * A WCAG contrast measurement that runs inside the page. It walks up from the
 * sampled element for the first ancestor with an opaque background — the
 * colour the browser actually paints behind the text, not a value asserted
 * from the token palette — and computes the WCAG relative-luminance contrast
 * ratio against it. This measures what actually renders rather than trusting
 * the tokens.css comment, and is more precise than axe-core's own contrast
 * heuristic, which is why the automated audit below disables that rule.
 *
 * Passed directly to `page.evaluate`, which serialises it by source and reruns
 * it in the browser context, so it must not close over anything from this
 * module — only browser globals (`document`, `getComputedStyle`).
 */
function sampleContrast(selectors: string[]): { light: { selector: string; ratio: number }[]; dark: { selector: string; ratio: number }[] } {
  const parse = (c: string) => {
    const m = (c.match(/[\d.]+/g) ?? []).map(Number);
    return [m[0] ?? 0, m[1] ?? 0, m[2] ?? 0, m[3] ?? 1] as const;
  };
  const lin = (v: number) => {
    const n = v / 255;
    return n <= 0.04045 ? n / 12.92 : Math.pow((n + 0.055) / 1.055, 2.4);
  };
  const lum = (c: readonly number[]) => 0.2126 * lin(c[0]) + 0.7152 * lin(c[1]) + 0.0722 * lin(c[2]);
  // `over` alpha-composites a translucent layer onto whatever is behind it.
  // The console's active-nav-link tint is a `color-mix(…, transparent)`
  // background (e.g. rgba(0,108,224,0.12)) — stopping at the first
  // non-fully-transparent layer and reading its raw channel values, the way a
  // simpler version of this measurement does, mistakes that 12%-strength blue
  // for a saturated one; only compositing every translucent layer down to an
  // opaque base gives the colour a viewer actually sees.
  const over = (top: readonly number[], bottom: readonly number[]) => {
    const a = top[3] + bottom[3] * (1 - top[3]);
    if (a === 0) return [255, 255, 255, 0] as const;
    const mix = (i: number) => (top[i] * top[3] + bottom[i] * bottom[3] * (1 - top[3])) / a;
    return [mix(0), mix(1), mix(2), a] as const;
  };
  const backgroundBehind = (el: Element) => {
    const layers: (readonly number[])[] = [];
    let node: Element | null = el;
    while (node && node !== document.documentElement) {
      const c = parse(getComputedStyle(node).backgroundColor);
      if (c[3] > 0) layers.push(c);
      if (c[3] >= 1) break;
      node = node.parentElement;
    }
    let result: readonly number[] = [255, 255, 255, 1];
    for (let i = layers.length - 1; i >= 0; i -= 1) result = over(layers[i], result);
    return result;
  };
  const ratio = (el: Element) => {
    const a = lum(parse(getComputedStyle(el).color));
    const b = lum(backgroundBehind(el));
    return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
  };
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
}

function measureContrast(page: Page, selectors: string[]) {
  return page.evaluate(sampleContrast, selectors);
}

test.describe("Contrast", () => {
  // Measured against the surfaces the browser actually paints in both themes,
  // for the console chrome and every resource-page text role — not asserted
  // from the tokens.css palette.
  test("every text role clears WCAG AA in both themes", async ({ page }) => {
    await page.goto("/ui/ecs");
    const results = await measureContrast(page, [
      ".aws-logo",
      ".aws-header-title",
      ".aws-breadcrumbs a",
      ".aws-page-header h1",
      ".aws-page-description",
      ".aws-button:not(:disabled)",
      ".aws-sidenav-link",
      ".aws-sidenav-link-active",
      ".aws-table th button",
      ".aws-empty strong",
    ]);
    for (const [theme, samples] of Object.entries(results)) {
      expect(samples.length).toBeGreaterThan(6);
      for (const sample of samples) {
        expect(sample.ratio, `${theme}: ${sample.selector} measured ${sample.ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(
          4.5,
        );
      }
    }
  });

  // The "Not supported" badge and its dimmer service-menu label are the
  // surfaces this pass added; they get the same measured-not-assumed check.
  test("the not-supported badge and its service-menu label clear WCAG AA in both themes", async ({ page }) => {
    await page.goto("/ui/not-supported/ec2");
    const results = await measureContrast(page, [
      ".aws-badge-grey",
      ".aws-sidenav-link-unsupported .aws-sidenav-link-label",
    ]);
    for (const [theme, samples] of Object.entries(results)) {
      expect(samples.length).toBe(2);
      for (const sample of samples) {
        expect(sample.ratio, `${theme}: ${sample.selector} measured ${sample.ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(
          4.5,
        );
      }
    }
  });

  // The status glyphs (success/error/warning/inactive) are UI components, not
  // body text, so WCAG holds them to 3:1 rather than 4.5:1 — measured here
  // against the AwsStatus rendering used across every resource table.
  test("status text clears the 3:1 non-text/large-text threshold in both themes", async ({ page }) => {
    await page.goto("/ui/ecs");
    const results = await measureContrast(page, [".aws-status-success", ".aws-status-error", ".aws-status-inactive"]);
    for (const [theme, samples] of Object.entries(results)) {
      for (const sample of samples) {
        expect(sample.ratio, `${theme}: ${sample.selector} measured ${sample.ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(
          3,
        );
      }
    }
  });
});

test.describe("Automated accessibility audit", () => {
  // axe-core is a coarser net than the hand-measured contrast and landmark
  // assertions above — it catches defect classes those checks do not aim at
  // (missing form labels, invalid ARIA usage, duplicate IDs, list structure).
  // It runs against both themes and both the ordinary and the not-supported
  // page shapes, disabling only the colour-contrast rule (already covered,
  // more precisely, by the dedicated Contrast suite above, which walks up to
  // the actually-painted background rather than axe's own heuristic).
  for (const theme of ["light", "dark"] as const) {
    for (const target of ["/ui/ecs", "/ui/not-supported/ec2"]) {
      test(`${target} has no detectable violations (${theme})`, async ({ page }) => {
        await page.goto(target);
        if (theme === "dark") {
          await page.evaluate(() => document.documentElement.classList.add("dark"));
        }
        const results = await new AxeBuilder({ page }).disableRules(["color-contrast"]).analyze();
        expect(results.violations, JSON.stringify(results.violations, null, 2)).toEqual([]);
      });
    }
  }
});

// The console's authenticated reads of the real AWS APIs are proven in the
// Shauth relying-party suite (ui/e2e/shauth-rps.mjs): with a live operator
// identity the console federates into credentials and reads the real APIs. This
// lightweight per-package suite has no identity provider, so the console reaches
// the simulator unauthenticated and the now-enforcing simulator rejects those
// reads exactly as real AWS would — data-render assertions therefore belong to
// the authenticated RPS path, not here. This suite covers the shell, the
// navigation, and the visual language, which do not depend on cloud reads.
