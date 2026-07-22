import { type ReactNode } from "react";
import { BrowserRouter, Routes, useLocation } from "react-router";
import { ErrorBoundary, OperatorAccount } from "@sockerless/ui-core/components";
import { AwsBreadcrumbs, AwsHeader, AwsSideNavigation, type NavGroup } from "./AwsConsole.js";

/**
 * AWS groups services in its navigation rather than listing them flat, so the
 * simulator does too: an operator looking for a registry looks under
 * containers, not through an undifferentiated list.
 */
const NAV_GROUPS: NavGroup[] = [
  { label: "Dashboard", items: [{ label: "Overview", to: "/ui/" }] },
  {
    label: "Compute",
    items: [
      { label: "Elastic Container Service", to: "/ui/ecs" },
      { label: "Lambda", to: "/ui/lambda" },
    ],
  },
  {
    label: "Storage and registry",
    items: [
      { label: "Elastic Container Registry", to: "/ui/ecr" },
      { label: "Simple Storage Service", to: "/ui/s3" },
    ],
  },
  { label: "Management", items: [{ label: "CloudWatch Logs", to: "/ui/logs" }] },
];

const CRUMBS: Record<string, string> = {
  "/ui/": "Overview",
  "/ui/ecs": "Elastic Container Service",
  "/ui/lambda": "Lambda",
  "/ui/ecr": "Elastic Container Registry",
  "/ui/s3": "Simple Storage Service",
  "/ui/logs": "CloudWatch Logs",
};

function ConsoleFrame({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  const current = CRUMBS[pathname] ?? "Overview";
  return (
    <div className="aws">
      <a href="#main-content" className="sl-skip-link">Skip to main content</a>
      <AwsHeader region="eu-west-1" account={<OperatorAccount />} />
      <AwsBreadcrumbs trail={[{ label: "Simulator", to: "/ui/" }, { label: current }]} />
      <div className="aws-body">
        <AwsSideNavigation serviceName="Simulator" groups={NAV_GROUPS} />
        <main id="main-content" tabIndex={-1} className="aws-main">
          {children}
        </main>
      </div>
    </div>
  );
}

export function AwsApp({ children }: { children: ReactNode }) {
  return (
    <ErrorBoundary>
      <BrowserRouter>
        <ConsoleFrame>
          <Routes>{children}</Routes>
        </ConsoleFrame>
      </BrowserRouter>
    </ErrorBoundary>
  );
}
