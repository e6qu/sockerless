// The service catalog the portal's left-hand service menu is built from —
// grouped the way the real Azure portal's "All services" page groups its
// catalog (General, Compute, Containers, Storage, Databases, Networking,
// Monitoring + management, Identity, Security, DevOps). Every group and every
// service name is a real Azure category and a real Azure service; what
// differs from the real portal is coverage, not naming — a service the
// simulator does not implement still appears where an operator would expect
// to find it, marked `supported: false` rather than omitted, so the menu
// reads as the real portal's information architecture while staying honest
// about what this simulator can do.
//
// `to` is present on every item: a supported service routes to its real
// page; an unsupported one routes to the shared "not implemented" page at a
// slug derived from its name, so the item stays a genuine, keyboard-reachable
// link rather than a dead end that looks clickable but isn't.

export interface CatalogItem {
  label: string;
  to: string;
  /** The Azure Resource Manager resource-type display name Essentials leads
   *  with — "Virtual machine", "Storage account", and so on. */
  kind: string;
  supported: boolean;
}

export interface CatalogGroup {
  label: string;
  items: CatalogItem[];
}

function slug(label: string): string {
  return label
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "");
}

function unsupported(label: string, kind: string): CatalogItem {
  return { label, to: `/ui/not-supported/${slug(label)}`, kind, supported: false };
}

export const SERVICE_CATALOG: CatalogGroup[] = [
  {
    label: "General",
    items: [
      { label: "Subscriptions", to: "/ui/subscriptions", kind: "Subscription", supported: true },
      unsupported("Resource groups", "Resource group"),
      unsupported("Management groups", "Management group"),
    ],
  },
  {
    label: "Compute",
    items: [
      unsupported("Virtual machines", "Virtual machine"),
      unsupported("App Service", "App Service (Web App)"),
      { label: "Function Apps", to: "/ui/functions", kind: "Function App", supported: true },
    ],
  },
  {
    label: "Containers",
    items: [
      { label: "Container Apps", to: "/ui/container-apps", kind: "Container Apps job", supported: true },
      unsupported("Azure Kubernetes Service", "Azure Kubernetes Service (AKS) cluster"),
      unsupported("Container instances", "Container group"),
      { label: "Container registries", to: "/ui/acr", kind: "Container registry", supported: true },
    ],
  },
  {
    label: "Storage",
    items: [{ label: "Storage accounts", to: "/ui/storage", kind: "Storage account", supported: true }],
  },
  {
    label: "Databases",
    items: [
      unsupported("Azure SQL", "SQL database"),
      unsupported("Azure Cosmos DB", "Cosmos DB account"),
      unsupported("Azure Database for PostgreSQL", "PostgreSQL flexible server"),
    ],
  },
  {
    label: "Networking",
    items: [
      unsupported("Virtual networks", "Virtual network"),
      unsupported("Load balancers", "Load balancer"),
      unsupported("Front Door and CDN profiles", "Front Door and CDN profile"),
    ],
  },
  {
    label: "Monitoring + management",
    items: [
      { label: "Logs", to: "/ui/monitor", kind: "Log Analytics workspace", supported: true },
      unsupported("Cost Management + Billing", "Billing account"),
    ],
  },
  {
    label: "Microsoft Entra ID",
    items: [
      { label: "App registrations", to: "/ui/entra/app-registrations", kind: "Microsoft Entra ID", supported: true },
    ],
  },
  {
    label: "Security",
    items: [unsupported("Microsoft Defender for Cloud", "Security posture"), unsupported("Key vaults", "Key vault")],
  },
  {
    label: "DevOps",
    items: [unsupported("Azure DevOps organizations", "Azure DevOps organization")],
  },
];

const BY_TO = new Map<string, CatalogItem>(SERVICE_CATALOG.flatMap((group) => group.items).map((item) => [item.to, item]));

export function catalogItemForPath(pathname: string): CatalogItem | undefined {
  return BY_TO.get(pathname);
}
