import { describe, expect, it } from "vitest";
import { SERVICE_CATALOG, catalogItemForPath } from "../catalog.js";

describe("SERVICE_CATALOG", () => {
  const items = SERVICE_CATALOG.flatMap((group) => group.items);

  it("gives every item a unique route", () => {
    const routes = items.map((item) => item.to);
    expect(new Set(routes).size).toBe(routes.length);
  });

  it("routes every unsupported item to the shared not-implemented page", () => {
    for (const item of items.filter((candidate) => !candidate.supported)) {
      expect(item.to).toMatch(/^\/ui\/not-supported\/[a-z0-9-]+$/);
    }
  });

  it("routes every supported item to its own page rather than the not-implemented one", () => {
    for (const item of items.filter((candidate) => candidate.supported)) {
      expect(item.to).not.toMatch(/^\/ui\/not-supported\//);
    }
  });

  it("carries at least one supported item per Azure resource family the simulator implements", () => {
    const supportedLabels = items.filter((item) => item.supported).map((item) => item.label);
    expect(supportedLabels).toEqual(
      expect.arrayContaining([
        "Subscriptions",
        "Function Apps",
        "Container Apps",
        "Container registries",
        "Storage accounts",
        "Logs",
        "App registrations",
      ]),
    );
  });

  it("resolves a catalog item by its route for both supported and unsupported items", () => {
    expect(catalogItemForPath("/ui/container-apps")?.label).toBe("Container Apps");
    expect(catalogItemForPath("/ui/not-supported/virtual-machines")?.label).toBe("Virtual machines");
    expect(catalogItemForPath("/ui/not-supported/virtual-machines")?.supported).toBe(false);
    expect(catalogItemForPath("/ui/does-not-exist")).toBeUndefined();
  });
});
