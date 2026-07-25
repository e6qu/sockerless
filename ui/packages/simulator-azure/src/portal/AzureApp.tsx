import { type ReactNode } from "react";
import { BrowserRouter, Routes, useLocation } from "react-router";
import { ErrorBoundary, OperatorAccount } from "@sockerless/ui-core/components";
import { SERVICE_CATALOG, catalogItemForPath } from "../catalog.js";
import {
  AzureBreadcrumbs,
  AzureHeader,
  AzureResourceTitle,
  AzureServiceMenu,
  type ServiceMenuGroup,
  type ServiceMenuItem,
} from "./AzurePortal.js";

/** The portal opens a resource on the items that apply to any resource, then
 *  groups the rest. The simulator's menu follows the same shape. The groups
 *  themselves come from the service catalog — the real Azure "All services"
 *  categories, carrying every service this simulator implements and a
 *  faithful representative set of the ones it doesn't (see catalog.ts). */
const FLAT_ITEMS: ServiceMenuItem[] = [{ label: "Overview", to: "/ui/" }];

const MENU_GROUPS: ServiceMenuGroup[] = SERVICE_CATALOG.map((group) => ({
  label: group.label,
  items: group.items.map((item) => ({ label: item.label, to: item.to, supported: item.supported })),
}));

interface Pane {
  crumb: string;
  title: string;
  kind: string;
  /** An intermediate breadcrumb for panes nested under a listing blade. */
  parent?: { label: string; to: string };
}

const PANE: Record<string, Pane> = {
  "/ui/": { crumb: "Overview", title: "Simulator", kind: "Subscription" },
  "/ui/subscriptions": { crumb: "Subscriptions", title: "Subscriptions", kind: "Subscription" },
  "/ui/container-apps": { crumb: "Container Apps", title: "Container Apps", kind: "Container Apps job" },
  "/ui/functions": { crumb: "Function Apps", title: "Function Apps", kind: "Function App" },
  "/ui/acr": { crumb: "Container registries", title: "Container registries", kind: "Container registry" },
  "/ui/storage": { crumb: "Storage accounts", title: "Storage accounts", kind: "Storage account" },
  "/ui/monitor": { crumb: "Logs", title: "Logs", kind: "Log Analytics workspace" },
  "/ui/entra/app-registrations": {
    crumb: "App registrations",
    title: "App registrations",
    kind: "Microsoft Entra ID",
  },
};

function paneFor(pathname: string): Pane {
  const exact = PANE[pathname];
  if (exact) return exact;
  if (pathname.startsWith("/ui/subscriptions/")) {
    return {
      crumb: "Subscription",
      title: "Subscription",
      kind: "Subscription",
      parent: { label: "Subscriptions", to: "/ui/subscriptions" },
    };
  }
  if (pathname.startsWith("/ui/entra/app-registrations/")) {
    return {
      crumb: "Certificates & secrets",
      title: "App registration",
      kind: "Microsoft Entra ID",
      parent: { label: "App registrations", to: "/ui/entra/app-registrations" },
    };
  }
  if (pathname.startsWith("/ui/not-supported/")) {
    const item = catalogItemForPath(pathname);
    const label = item?.label ?? "Service";
    return { crumb: label, title: label, kind: item?.kind ?? "Service" };
  }
  return PANE["/ui/"];
}

function PortalFrame({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  const pane = paneFor(pathname);
  const trail = [
    { label: "Home", to: "/ui/" },
    ...(pane.parent ? [{ label: pane.parent.label, to: pane.parent.to }] : []),
    { label: pane.crumb },
  ];
  return (
    <div className="azure">
      <a href="#main-content" className="sl-skip-link">Skip to main content</a>
      <AzureHeader account={<OperatorAccount />} />
      <AzureBreadcrumbs trail={trail} />
      <div className="az-body">
        <AzureResourceTitle name={pane.title} kind={pane.kind} directory="Simulator" />
        <AzureServiceMenu flat={FLAT_ITEMS} groups={MENU_GROUPS} />
        <main id="main-content" tabIndex={-1}>
          {children}
        </main>
      </div>
    </div>
  );
}

export function AzureApp({ children }: { children: ReactNode }) {
  return (
    <ErrorBoundary>
      <BrowserRouter>
        <PortalFrame>
          <Routes>{children}</Routes>
        </PortalFrame>
      </BrowserRouter>
    </ErrorBoundary>
  );
}
