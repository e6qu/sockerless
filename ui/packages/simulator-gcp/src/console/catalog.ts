import type { IconName } from "./icons.js";

// The real console's hamburger menu opens a navigation drawer listing the
// Google Cloud product catalog, grouped the way the console groups it
// (Compute, Serverless, Storage, Databases, Networking, Operations,
// Analytics, Artificial Intelligence, Developer Tools, IAM & Admin,
// Security). The simulator implements a slice of Google Cloud — Cloud Run,
// Cloud Run functions, Artifact Registry, Cloud Storage, IAM service
// accounts, Cloud Logging, and Resource Manager projects — so the catalog
// carries the real IA truthfully: every product Google Cloud offers is
// listed, the ones the simulator implements link to their page, and the
// rest are marked "Not supported" rather than omitted or silently faked.
//
// Icons are Material Symbols Outlined glyphs (the console's own icon set),
// not the trademarked per-product logos the real catalog uses — see
// PLAN.md § "Simulator Console Parity" ("without copying proprietary
// assets").

export interface CatalogItem {
  /** Route slug for the "not supported" page; also used as the React key. */
  id: string;
  /** The product's real Google Cloud name. */
  name: string;
  icon: IconName;
  /** Present, and `to` set, only for products the simulator implements. */
  to?: string;
}

export interface CatalogGroup {
  label: string;
  items: CatalogItem[];
}

export const CATALOG: CatalogGroup[] = [
  {
    label: "Compute",
    items: [
      { id: "compute-engine", name: "Compute Engine", icon: "computer" },
      { id: "kubernetes-engine", name: "Kubernetes Engine", icon: "hub" },
      { id: "cloud-run", name: "Cloud Run", icon: "deployed_code", to: "/ui/cloudrun" },
      { id: "app-engine", name: "App Engine", icon: "package_2" },
    ],
  },
  {
    label: "Serverless",
    items: [
      { id: "cloud-run-functions", name: "Cloud Run functions", icon: "function", to: "/ui/functions" },
      { id: "workflows", name: "Workflows", icon: "list_alt" },
      { id: "eventarc", name: "Eventarc", icon: "notifications" },
    ],
  },
  {
    label: "Storage",
    items: [
      { id: "cloud-storage", name: "Cloud Storage", icon: "database", to: "/ui/gcs" },
      { id: "filestore", name: "Filestore", icon: "inventory_2" },
    ],
  },
  {
    label: "Databases",
    items: [
      { id: "cloud-sql", name: "Cloud SQL", icon: "database" },
      { id: "firestore", name: "Firestore", icon: "database" },
      { id: "spanner", name: "Spanner", icon: "database" },
      { id: "bigtable", name: "Bigtable", icon: "database" },
    ],
  },
  {
    label: "Networking",
    items: [
      { id: "vpc-network", name: "VPC network", icon: "lan" },
      { id: "cloud-load-balancing", name: "Cloud Load Balancing", icon: "lan" },
      { id: "cloud-dns", name: "Cloud DNS", icon: "lan" },
    ],
  },
  {
    label: "Operations",
    items: [
      { id: "logging", name: "Logging", icon: "monitoring", to: "/ui/logging" },
      { id: "monitoring", name: "Monitoring", icon: "query_stats" },
      { id: "error-reporting", name: "Error Reporting", icon: "help" },
    ],
  },
  {
    label: "Analytics",
    items: [
      { id: "bigquery", name: "BigQuery", icon: "query_stats" },
      { id: "pub-sub", name: "Pub/Sub", icon: "notifications" },
      { id: "dataflow", name: "Dataflow", icon: "integration_instructions" },
    ],
  },
  {
    label: "Artificial Intelligence",
    items: [{ id: "vertex-ai", name: "Vertex AI", icon: "smart_toy" }],
  },
  {
    label: "Developer Tools",
    items: [
      { id: "artifact-registry", name: "Artifact Registry", icon: "inventory_2", to: "/ui/ar" },
      { id: "cloud-build", name: "Cloud Build", icon: "integration_instructions" },
      { id: "cloud-source-repositories", name: "Cloud Source Repositories", icon: "list_alt" },
    ],
  },
  {
    label: "IAM & Admin",
    items: [
      { id: "service-accounts", name: "Service accounts", icon: "manage_accounts", to: "/ui/serviceaccounts" },
      { id: "manage-resources", name: "Manage resources", icon: "folder_data", to: "/ui/projects" },
      { id: "iam", name: "IAM", icon: "token" },
      { id: "organization-policies", name: "Organization policies", icon: "security" },
    ],
  },
  {
    label: "Security",
    items: [
      { id: "security-command-center", name: "Security Command Center", icon: "security" },
      { id: "cloud-kms", name: "Cloud KMS", icon: "token" },
    ],
  },
];

export function findCatalogItem(id: string | undefined): CatalogItem | undefined {
  if (!id) return undefined;
  for (const group of CATALOG) {
    const item = group.items.find((candidate) => candidate.id === id);
    if (item) return item;
  }
  return undefined;
}
