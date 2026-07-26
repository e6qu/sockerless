import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

const SERVICES = [
  { path: "/ui/cloudrun", nav: "Cloud Run jobs", title: "Cloud Run jobs", columns: ["Name", "Status of last execution", "Created", "Executions", "Launch stage"] },
  { path: "/ui/functions", nav: "Cloud Run functions", title: "Cloud Run functions", columns: ["Name", "State", "Environment"] },
  { path: "/ui/ar", nav: "Artifact Registry", title: "Artifact Registry", columns: ["Name", "Format", "Created"] },
  { path: "/ui/gcs", nav: "Cloud Storage", title: "Cloud Storage", columns: ["Name"] },
  { path: "/ui/serviceaccounts", nav: "Service accounts", title: "Service accounts", columns: ["Email", "Status", "Name", "Description", "Actions"] },
  { path: "/ui/projects", nav: "Manage resources", title: "Manage resources", columns: ["Name", "ID", "Number", "State", "Created", "Actions"] },
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
    const toggle = page.getByRole("button", { name: /Switch to (dark|light) theme/ });
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

  test("focuses the search field on \"/\", the shortcut the placeholder names", async ({ page }) => {
    await page.goto("/ui/");
    const search = page.getByLabel("Search resources, docs, products, and more");
    await expect(search).not.toBeFocused();
    await page.keyboard.press("/");
    await expect(search).toBeFocused();
  });

  test("does not steal a literal \"/\" typed into another field", async ({ page }) => {
    await page.goto("/ui/logging");
    const query = page.getByTestId("log-query-input");
    await query.fill("logName:\"run.googleapis.com/stdout\"");
    await expect(query).toHaveValue('logName:"run.googleapis.com/stdout"');
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

test.describe("Project picker", () => {
  // The lightweight suite has no identity provider, so the dialog's project
  // list reaches the enforcing simulator unauthenticated and reports the
  // API's own error; the selected-project chip, the dialog, and the New
  // project form are the console's own and must render regardless. The
  // authenticated list/create/switch flows are proven in the relying-party
  // suite (ui/e2e/shauth-rps.mjs).
  test("names the selected project on the chip and opens the project dialog", async ({ page }) => {
    await page.goto("/ui/");
    const chip = page.getByTestId("project-picker");
    await expect(chip).toBeVisible();
    await expect(chip).toContainText("sockerless");
    await chip.click();
    await expect(page.getByTestId("project-dialog")).toBeVisible();
    await expect(page.getByLabel("Search projects and folders")).toBeVisible();
    await expect(page.getByTestId("project-manage-link")).toBeVisible();
  });

  test("offers the New project form with a derived, validated project ID", async ({ page }) => {
    await page.goto("/ui/");
    await page.getByTestId("project-picker").click();
    await page.getByTestId("project-create-open").click();
    const name = page.getByTestId("project-create-name");
    await expect(name).toBeVisible();
    await expect(page.getByTestId("project-create-submit")).toBeDisabled();
    await name.fill("Console Test Project");
    await expect(page.getByTestId("project-create-id")).toHaveValue("console-test-project");
    await expect(page.getByTestId("project-create-submit")).toBeEnabled();
  });

  test("navigates to Manage resources from the dialog", async ({ page }) => {
    await page.goto("/ui/");
    await page.getByTestId("project-picker").click();
    await page.getByTestId("project-manage-link").click();
    await expect(page).toHaveURL(/\/ui\/projects$/);
    await expect(page.getByRole("heading", { name: "Manage resources" })).toBeVisible();
  });
});

test.describe("Overview", () => {
  test("presents the overview", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
    // The per-resource counts require an authenticated read; this lightweight
    // suite has no identity provider, so the console reaches the enforcing
    // simulator unauthenticated. The counts and the links they carry are proven
    // in the authenticated relying-party suite (ui/e2e/shauth-rps.mjs).
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
      const link = page.getByRole("link", { name: service.nav, exact: true });
      await expect(link).toHaveClass(/gc-nav-link-active/);
      // react-router's NavLink sets aria-current="page" on the active link by
      // default; a screen reader announces which product the pill highlights
      // visually, not just its colour.
      await expect(link).toHaveAttribute("aria-current", "page");
    }
    await page.getByRole("link", { name: "Overview", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  });
});

test.describe("Product catalog navigation drawer", () => {
  // The real console's hamburger opens the full Google Cloud product catalog,
  // grouped the way the console groups it. The simulator implements a slice of
  // Google Cloud, so the catalog stays truthful: products it implements link to
  // their page, the rest carry a "Not supported" chip and route to a short
  // honest explanation rather than being silently omitted or faked.
  test("opens from the hamburger with the grouped product catalog", async ({ page }) => {
    await page.goto("/ui/");
    const menu = page.getByRole("button", { name: "Main menu" });
    await expect(menu).toHaveAttribute("aria-expanded", "false");
    await menu.click();
    const drawer = page.getByRole("dialog", { name: "Google Cloud products" });
    await expect(drawer).toBeVisible();
    await expect(menu).toHaveAttribute("aria-expanded", "true");
    await expect(drawer.getByRole("heading", { name: "Compute" })).toBeVisible();
    await expect(drawer.getByRole("heading", { name: "IAM & Admin" })).toBeVisible();
    await expect(drawer.getByTestId("drawer-item-cloud-run")).toBeVisible();
    await expect(drawer.getByTestId("drawer-item-compute-engine")).toBeVisible();
  });

  test("links a supported product to its real page and closes the drawer", async ({ page }) => {
    await page.goto("/ui/");
    await page.getByRole("button", { name: "Main menu" }).click();
    await page.getByTestId("drawer-item-cloud-run").click();
    await expect(page).toHaveURL(/\/ui\/cloudrun$/);
    await expect(page.getByRole("heading", { name: "Cloud Run jobs" })).toBeVisible();
    await expect(page.getByTestId("nav-drawer")).toHaveCount(0);
  });

  test("marks an unsupported product with a Not supported chip conveyed in text, not colour alone", async ({ page }) => {
    await page.goto("/ui/");
    await page.getByRole("button", { name: "Main menu" }).click();
    const item = page.getByTestId("drawer-item-compute-engine");
    await expect(item.locator(".gc-chip-unsupported")).toHaveText("Not supported");
    // The link carries an explicit aria-label so the accessible name reads as a
    // single clean phrase ("<product>, not supported in this simulator"),
    // matching the AWS and Azure consoles; the visible "Not supported" chip
    // conveys the same meaning in text, not colour alone.
    await expect(item).toHaveAccessibleName("Compute Engine, not supported in this simulator");
  });

  test("routes an unsupported product to a short, honest explanation instead of a dead link", async ({ page }) => {
    await page.goto("/ui/");
    await page.getByRole("button", { name: "Main menu" }).click();
    await page.getByTestId("drawer-item-compute-engine").click();
    await expect(page).toHaveURL(/\/ui\/not-supported\/compute-engine$/);
    await expect(page.getByRole("heading", { name: "Compute Engine", exact: true })).toBeVisible();
    await expect(page.getByText(/isn't implemented by the Sockerless simulator/)).toBeVisible();
    await expect(page.getByRole("link", { name: "Back to Overview" })).toBeVisible();

    // A direct visit (not only a drawer click) resolves the same way, and an
    // unrecognised slug still renders honestly rather than crashing.
    await page.goto("/ui/not-supported/kubernetes-engine");
    await expect(page.getByRole("heading", { name: "Kubernetes Engine", exact: true })).toBeVisible();
  });

  test("closes on Escape and returns focus to the hamburger", async ({ page }) => {
    await page.goto("/ui/");
    const menu = page.getByRole("button", { name: "Main menu" });
    await menu.click();
    await expect(page.getByTestId("nav-drawer")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByTestId("nav-drawer")).toHaveCount(0);
    await expect(menu).toBeFocused();
  });

  test("closes on a scrim click", async ({ page }) => {
    await page.goto("/ui/");
    await page.getByRole("button", { name: "Main menu" }).click();
    const viewport = page.viewportSize()!;
    await page.locator(".gc-drawer-overlay").click({ position: { x: viewport.width - 20, y: 20 } });
    await expect(page.getByTestId("nav-drawer")).toHaveCount(0);
  });
});

test.describe("Dialog keyboard operability", () => {
  // GcpDialog backs the project picker and every create/delete confirmation;
  // proving it once here covers all of them rather than each caller
  // reimplementing Escape-to-close and focus return.
  test("the project picker dialog closes on Escape and returns focus to its trigger", async ({ page }) => {
    await page.goto("/ui/");
    const trigger = page.getByTestId("project-picker");
    await trigger.click();
    await expect(page.getByTestId("project-dialog")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByTestId("project-dialog")).toHaveCount(0);
    await expect(trigger).toBeFocused();
  });
});

test.describe("Accessibility landmarks", () => {
  test("exposes banner, main and product-navigation landmarks", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.getByRole("banner")).toBeVisible();
    await expect(page.getByRole("main")).toBeVisible();
    await expect(page.getByRole("navigation", { name: "Product" })).toBeVisible();
  });

  test("the document declares its language", async ({ page }) => {
    await page.goto("/ui/");
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
  });
});

// Shared by every contrast test below: measured against the surfaces the
// browser actually paints, walking up for the first opaque background,
// rather than asserted from the palette. Disabled controls are excluded —
// WCAG 1.4.3 exempts inactive components, and the console greys them
// deliberately. Runs in the browser via page.evaluate, so it must be a
// standalone function with no closure over the test's outer scope.
function sampleContrast(selectors: string[]) {
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
}

function assertAA(results: { light: { tag: string; ratio: number }[]; dark: { tag: string; ratio: number }[] }) {
  for (const [theme, samples] of Object.entries(results)) {
    expect(samples.length).toBeGreaterThan(0);
    for (const sample of samples) {
      expect(sample.ratio, `${theme}: ${sample.tag} measured ${sample.ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(4.5);
    }
  }
}

test.describe("Contrast", () => {
  // The tightest role here is around 4.68:1, so the guard has little room to
  // spare and is worth holding.
  test("every enabled text role clears WCAG AA in both themes", async ({ page }) => {
    await page.goto("/ui/cloudrun");
    const results = await page.evaluate(sampleContrast, [
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
    ]);
    expect(results.light.length).toBeGreaterThan(10);
    assertAA(results);
  });

  // The navigation drawer and its "Not supported" chip are new surfaces this
  // pass added; they get the same empirical guard as the rest of the shell.
  test("the navigation drawer and its Not supported chip clear WCAG AA in both themes", async ({ page }) => {
    await page.goto("/ui/");
    await page.getByRole("button", { name: "Main menu" }).click();
    await expect(page.getByTestId("nav-drawer")).toBeVisible();
    const results = await page.evaluate(sampleContrast, [
      ".gc-drawer-title",
      ".gc-drawer-note",
      ".gc-drawer-group-label",
      ".gc-drawer-link",
      ".gc-drawer-link-active",
      ".gc-drawer-link-unsupported",
      ".gc-chip-unsupported",
    ]);
    expect(results.light.length).toBeGreaterThan(10);
    assertAA(results);
  });

  test("the not-supported page clears WCAG AA in both themes", async ({ page }) => {
    await page.goto("/ui/not-supported/compute-engine");
    const results = await page.evaluate(sampleContrast, [
      ".gc-page-header h1",
      ".gc-page-description",
      ".gc-empty-headline",
      ".gc-empty-description",
      ".gc-button-primary",
    ]);
    assertAA(results);
  });

  // The Logs Explorer query bar is this pass's new surface: unlike the
  // authenticated tabs and sub-resource tables, it renders unconditionally,
  // so it gets the same empirical guard here.
  test("the Logs Explorer query bar clears WCAG AA in both themes", async ({ page }) => {
    await page.goto("/ui/logging");
    const results = await page.evaluate(sampleContrast, [".gc-log-query-field", '[data-testid="log-query-run"]']);
    assertAA(results);
  });

  // The Create bucket / Create repository dialogs are this pass's new
  // surface: their labels, hints and the Artifact Registry format select
  // get the same empirical guard as the rest of the shell.
  test("the resource-creation dialogs clear WCAG AA in both themes", async ({ page }) => {
    await page.goto("/ui/gcs");
    await page.getByTestId("gcs-create-bucket").click();
    await expect(page.getByTestId("gcs-create-dialog")).toBeVisible();
    const gcsResults = await page.evaluate(sampleContrast, [
      ".gc-dialog-title",
      ".gc-field",
      ".gc-field-hint",
      ".gc-button-text",
      ".gc-button-primary",
    ]);
    assertAA(gcsResults);

    await page.goto("/ui/ar");
    await page.getByTestId("ar-create-repo").click();
    await expect(page.getByTestId("ar-create-dialog")).toBeVisible();
    const arResults = await page.evaluate(sampleContrast, [
      ".gc-dialog-title",
      ".gc-field",
      ".gc-field-hint",
      ".gc-button-text",
      ".gc-button-primary",
    ]);
    assertAA(arResults);
  });
});

// Cloud Storage buckets and Artifact Registry repositories used to render a
// disabled "Create …" action even though the real console lets an operator
// create either from the same page. Each now offers a real "Create …" header
// action and empty-state primary that open a GcpDialog with client-side name
// validation, wired to the real storage.buckets.insert / Artifact Registry
// create operation. The header action and the dialog it opens render before
// (and regardless of) any cloud read, so they are assertable here without an
// identity provider; this suite's unauthenticated read is rejected by the
// enforcing simulator (see the Visual fidelity describe above), so the
// empty-state primary — which only renders on a successful empty read — and
// the authenticated create→appears round trip are exercised by the
// mocked-fetch suite (src/__tests__/ResourceCreateFlows.test.tsx), the same
// split the AWS console's ECR/S3/CloudWatch Logs create dialogs document.
test.describe("Resource creation", () => {
  const cases = [
    {
      path: "/ui/gcs",
      createTestId: "gcs-create-bucket",
      dialogTestId: "gcs-create-dialog",
      dialogName: "Create a bucket",
      inputTestId: "gcs-create-name",
      submitTestId: "gcs-create-submit",
      validName: "my-new-bucket",
    },
    {
      path: "/ui/ar",
      createTestId: "ar-create-repo",
      dialogTestId: "ar-create-dialog",
      dialogName: "Create repository",
      inputTestId: "ar-create-id",
      submitTestId: "ar-create-submit",
      validName: "my-new-repo",
    },
  ];

  for (const { path, createTestId, dialogTestId, dialogName, inputTestId, submitTestId, validName } of cases) {
    test(`${path} offers ${dialogName} and opens the create dialog with an initially-disabled submit`, async ({ page }) => {
      await page.goto(path);
      const create = page.getByTestId(createTestId);
      await expect(create).toBeVisible();
      await create.click();
      const dialog = page.getByTestId(dialogTestId);
      await expect(dialog).toBeVisible();
      await expect(dialog).toHaveAccessibleName(dialogName);
      const input = dialog.getByTestId(inputTestId);
      await expect(input).toBeVisible();
      // An empty or invalid name must not be submittable.
      await expect(dialog.getByTestId(submitTestId)).toBeDisabled();
      await input.fill(validName);
      await expect(dialog.getByTestId(submitTestId)).toBeEnabled();
      await dialog.getByRole("button", { name: "Cancel" }).click();
      await expect(page.getByTestId(dialogTestId)).toHaveCount(0);
    });

    test(`${path} closes ${dialogName} on Escape and returns focus to its trigger`, async ({ page }) => {
      await page.goto(path);
      const trigger = page.getByTestId(createTestId);
      await trigger.click();
      const dialog = page.getByTestId(dialogTestId);
      await expect(dialog).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(page.getByTestId(dialogTestId)).toHaveCount(0);
      await expect(trigger).toBeFocused();
    });
  }

  test("the Create repository dialog offers a format picker defaulting to DOCKER", async ({ page }) => {
    await page.goto("/ui/ar");
    await page.getByTestId("ar-create-repo").click();
    const dialog = page.getByTestId("ar-create-dialog");
    const format = dialog.getByTestId("ar-create-format");
    await expect(format).toHaveValue("DOCKER");
    await expect(format.locator("option")).toHaveCount(8);
  });
});

test.describe("Resource detail pages", () => {
  // This lightweight suite has no identity provider (see the Overview and
  // Project picker describes above), so every detail read reaches the
  // enforcing simulator unauthenticated and reports the API's own error
  // rather than rendering the resource. That still proves the page shell
  // itself — breadcrumb, page header and description — renders honestly on a
  // failed read instead of crashing or fabricating data; the authenticated
  // property grids, tabs and sub-resource tables (Executions, Images,
  // Objects) are proven in the relying-party suite (ui/e2e/shauth-rps.mjs).
  const DETAILS = [
    { path: "/ui/cloudrun/example-job", back: "Cloud Run jobs", backHref: "/ui/cloudrun" },
    { path: "/ui/functions/example-function", back: "Cloud Run functions", backHref: "/ui/functions" },
    { path: "/ui/ar/example-repo", back: "Artifact Registry", backHref: "/ui/ar" },
    { path: "/ui/gcs/example-bucket", back: "Cloud Storage", backHref: "/ui/gcs" },
    { path: "/ui/serviceaccounts/example@sockerless.iam.gserviceaccount.com", back: "Service accounts", backHref: "/ui/serviceaccounts" },
  ];

  for (const detail of DETAILS) {
    test(`renders the breadcrumb and page shell for ${detail.path}`, async ({ page }) => {
      await page.goto(detail.path);
      const back = page.getByRole("link", { name: `‹ ${detail.back}` });
      await expect(back).toBeVisible();
      await expect(back).toHaveAttribute("href", detail.backHref);
      await expect(page.locator(".gc-message-error, .gc-loading")).toBeVisible();
    });
  }
});

test.describe("Logs Explorer query bar", () => {
  // The query bar itself — unlike the results table — needs no authenticated
  // read to render, so its structure and interaction are provable in this
  // lightweight suite; the real filtered results are proven in the
  // relying-party suite.
  test("offers a Cloud Logging query field and a minimum-severity picker", async ({ page }) => {
    await page.goto("/ui/logging");
    const query = page.getByTestId("log-query-input");
    await expect(query).toBeVisible();
    const severity = page.getByTestId("log-severity-select");
    await expect(severity).toBeVisible();
    // "Any" plus the 9 Cloud Logging severity levels (DEFAULT..EMERGENCY).
    await expect(severity.locator("option")).toHaveCount(10);
    await expect(page.getByTestId("log-query-run")).toBeVisible();
  });

  test("composes the typed query and severity into the request without reloading the page", async ({ page }) => {
    await page.goto("/ui/logging");
    await page.getByTestId("log-query-input").fill('resource.type="cloud_run_job"');
    await page.getByTestId("log-severity-select").selectOption("ERROR");
    await page.getByTestId("log-query-run").click();
    // The query composes client-side; the page never navigates away, and the
    // Logs Explorer heading and query bar are still present, unauthenticated
    // read of the composed query notwithstanding.
    await expect(page.getByRole("heading", { name: "Logs Explorer" })).toBeVisible();
    await expect(page.getByTestId("log-query-input")).toHaveValue('resource.type="cloud_run_job"');
  });
});

test.describe("Visual fidelity", () => {
  // Structural proxies for the visual rebuild, so the icons, typeface, and
  // account avatar the console gained can't silently regress to the earlier
  // glyph-and-text sketch.
  test("renders a real icon on every navigation item", async ({ page }) => {
    await page.goto("/ui/");
    const links = page.getByRole("navigation", { name: "Product" }).getByRole("link");
    const count = await links.count();
    expect(count).toBeGreaterThanOrEqual(6);
    for (let index = 0; index < count; index += 1) {
      await expect(links.nth(index).locator("svg")).toHaveCount(1);
    }
  });

  test("carries the header tool cluster and an account avatar, not an error string", async ({ page }) => {
    await page.goto("/ui/");
    // Several tool icons in the header rather than bare text.
    const headerIcons = page.locator(".gc-header-right svg");
    expect(await headerIcons.count()).toBeGreaterThanOrEqual(5);
    // An unauthenticated console shows a neutral avatar, never "unavailable".
    await expect(page.locator(".gc-avatar")).toBeVisible();
    await expect(page.getByText(/identity is unavailable/i)).toHaveCount(0);
  });

  test("applies the Roboto typeface", async ({ page }) => {
    await page.goto("/ui/");
    const family = await page.evaluate(() => getComputedStyle(document.querySelector(".gcp")!).fontFamily);
    expect(family).toContain("Roboto");
  });

  // The empty state renders only on a successful empty read; this suite has no
  // identity provider, so the console reaches the enforcing simulator
  // unauthenticated and the read is rejected. The empty state (and every other
  // data-dependent view) is exercised over an authenticated read in the
  // relying-party suite (ui/e2e/shauth-rps.mjs).
});

test.describe("Automated accessibility audit", () => {
  // axe-core is a coarser net than the hand-measured contrast and landmark
  // assertions above — it catches defect classes those checks do not aim at
  // (missing form labels, invalid ARIA usage, duplicate IDs, list structure).
  // It runs against both themes and the ordinary, not-supported, and resource
  // detail page shapes, disabling only the colour-contrast rule (already
  // covered, more precisely, by the dedicated Contrast suite above, which
  // walks up to the actually-painted background rather than axe's own
  // heuristic).
  for (const theme of ["light", "dark"] as const) {
    for (const target of ["/ui/cloudrun", "/ui/logging", "/ui/not-supported/compute-engine", "/ui/cloudrun/example-job"]) {
      test(`${target} has no detectable violations (${theme})`, async ({ page }) => {
        await page.goto(target);
        if (theme === "dark") {
          await page.evaluate(() => document.documentElement.classList.add("dark"));
        }
        const results = await new AxeBuilder({ page }).disableRules(["color-contrast"]).analyze();
        expect(results.violations, JSON.stringify(results.violations, null, 2)).toEqual([]);
      });
    }

    const CREATE_DIALOGS = [
      { path: "/ui/gcs", createTestId: "gcs-create-bucket", dialogTestId: "gcs-create-dialog" },
      { path: "/ui/ar", createTestId: "ar-create-repo", dialogTestId: "ar-create-dialog" },
    ];
    for (const { path, createTestId, dialogTestId } of CREATE_DIALOGS) {
      test(`the ${dialogTestId} dialog has no detectable violations (${theme})`, async ({ page }) => {
        await page.goto(path);
        if (theme === "dark") {
          await page.evaluate(() => document.documentElement.classList.add("dark"));
        }
        await page.getByTestId(createTestId).click();
        await expect(page.getByTestId(dialogTestId)).toBeVisible();
        const results = await new AxeBuilder({ page }).disableRules(["color-contrast"]).analyze();
        expect(results.violations, JSON.stringify(results.violations, null, 2)).toEqual([]);
      });
    }
  }
});
