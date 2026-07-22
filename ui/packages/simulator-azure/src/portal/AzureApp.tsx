import { type ReactNode } from "react";
import { BrowserRouter, Routes, useLocation } from "react-router";
import { ErrorBoundary, OperatorAccount } from "@sockerless/ui-core/components";
import {
  AzureBreadcrumbs,
  AzureHeader,
  AzureResourceTitle,
  AzureServiceMenu,
  type ServiceMenuGroup,
  type ServiceMenuItem,
} from "./AzurePortal.js";

/** The portal opens a resource on the items that apply to any resource, then
 *  groups the rest. The simulator's menu follows the same shape. */
const FLAT_ITEMS: ServiceMenuItem[] = [{ label: "Overview", to: "/ui/" }];

const MENU_GROUPS: ServiceMenuGroup[] = [
  {
    label: "Compute",
    items: [
      { label: "Container Apps", to: "/ui/container-apps" },
      { label: "Function Apps", to: "/ui/functions" },
    ],
  },
  {
    label: "Storage and registry",
    items: [
      { label: "Container registries", to: "/ui/acr" },
      { label: "Storage accounts", to: "/ui/storage" },
    ],
  },
  { label: "Monitoring", items: [{ label: "Logs", to: "/ui/monitor" }] },
];

const PANE: Record<string, { crumb: string; title: string; kind: string }> = {
  "/ui/": { crumb: "Overview", title: "Simulator", kind: "Subscription" },
  "/ui/container-apps": { crumb: "Container Apps", title: "Container Apps", kind: "Container Apps job" },
  "/ui/functions": { crumb: "Function Apps", title: "Function Apps", kind: "Function App" },
  "/ui/acr": { crumb: "Container registries", title: "Container registries", kind: "Container registry" },
  "/ui/storage": { crumb: "Storage accounts", title: "Storage accounts", kind: "Storage account" },
  "/ui/monitor": { crumb: "Logs", title: "Logs", kind: "Log Analytics workspace" },
};

function PortalFrame({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  const pane = PANE[pathname] ?? PANE["/ui/"];
  return (
    <div className="azure">
      <a href="#main-content" className="sl-skip-link">Skip to main content</a>
      <AzureHeader account={<OperatorAccount />} />
      <AzureBreadcrumbs trail={[{ label: "Home", to: "/ui/" }, { label: pane.crumb }]} />
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
