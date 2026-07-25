import { useState, type ReactNode } from "react";
import { BrowserRouter, Routes, useLocation } from "react-router";
import { ErrorBoundary, OperatorAccount } from "@sockerless/ui-core/components";
import AppLayout from "@cloudscape-design/components/app-layout";
import { AwsBreadcrumbs, AwsHeader, AwsSideNavigation } from "./AwsConsole.js";
import { REGION } from "./federation.js";
import { findNavService, NAV_GROUPS } from "./serviceCatalog.js";

const CRUMBS: Record<string, string> = {
  "/ui/": "Overview",
  "/ui/ecs": "Elastic Container Service",
  "/ui/lambda": "Lambda",
  "/ui/ecr": "Elastic Container Registry",
  "/ui/s3": "Simple Storage Service",
  "/ui/logs": "CloudWatch Logs",
  "/ui/organizations": "AWS Organizations",
  "/ui/iam": "Identity and Access Management",
};

const IAM_USER_PREFIX = "/ui/iam/users/";
const ORG_ACCOUNT_PREFIX = "/ui/organizations/accounts/";
const NOT_SUPPORTED_PREFIX = "/ui/not-supported/";

// Resource detail routes nest under their service the way the real console
// breadcrumbs them: <service> > <resource identifier>. Every prefix here has
// a matching `:id`/`:name` route in main.tsx.
const RESOURCE_DETAIL_PREFIXES: { prefix: string; service: string; to: string }[] = [
  { prefix: "/ui/ecs/", service: "Elastic Container Service", to: "/ui/ecs" },
  { prefix: "/ui/lambda/", service: "Lambda", to: "/ui/lambda" },
  { prefix: "/ui/ecr/", service: "Elastic Container Registry", to: "/ui/ecr" },
  { prefix: "/ui/s3/", service: "Simple Storage Service", to: "/ui/s3" },
  { prefix: "/ui/logs/", service: "CloudWatch Logs", to: "/ui/logs" },
];

function crumbTrail(pathname: string): { label: string; to?: string }[] {
  // Detail pages nest under their service the way the real console breadcrumbs
  // them: IAM > Users > <user name>, AWS Organizations > <account id>.
  if (pathname.startsWith(IAM_USER_PREFIX)) {
    return [
      { label: "Simulator", to: "/ui/" },
      { label: "Identity and Access Management", to: "/ui/iam" },
      { label: decodeURIComponent(pathname.slice(IAM_USER_PREFIX.length)) },
    ];
  }
  if (pathname.startsWith(ORG_ACCOUNT_PREFIX)) {
    return [
      { label: "Simulator", to: "/ui/" },
      { label: "AWS Organizations", to: "/ui/organizations" },
      { label: decodeURIComponent(pathname.slice(ORG_ACCOUNT_PREFIX.length)) },
    ];
  }
  for (const entry of RESOURCE_DETAIL_PREFIXES) {
    if (pathname.startsWith(entry.prefix)) {
      return [
        { label: "Simulator", to: "/ui/" },
        { label: entry.service, to: entry.to },
        { label: decodeURIComponent(pathname.slice(entry.prefix.length)) },
      ];
    }
  }
  if (pathname.startsWith(NOT_SUPPORTED_PREFIX)) {
    const slug = pathname.slice(NOT_SUPPORTED_PREFIX.length);
    return [
      { label: "Simulator", to: "/ui/" },
      { label: findNavService(slug)?.label ?? slug },
    ];
  }
  return [
    { label: "Simulator", to: "/ui/" },
    { label: CRUMBS[pathname] ?? "Overview" },
  ];
}

function ConsoleFrame({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  // Cloudscape's own `AppLayout` keeps the navigation open by default and
  // owns the toggle control (and its accessible name) that collapses it —
  // this is the only navigation-open state the console keeps.
  const [navigationOpen, setNavigationOpen] = useState(true);
  return (
    <div className="aws">
      <a href="#main-content" className="sl-skip-link">Skip to main content</a>
      <AwsHeader region={REGION} account={<OperatorAccount />} services={NAV_GROUPS} />
      <AppLayout
        navigation={<AwsSideNavigation groups={NAV_GROUPS} activeHref={pathname} />}
        navigationOpen={navigationOpen}
        onNavigationChange={(event) => setNavigationOpen(event.detail.open)}
        breadcrumbs={<AwsBreadcrumbs trail={crumbTrail(pathname)} />}
        toolsHide
        content={
          // AppLayout's own `content` slot already renders inside a real
          // `<main>` landmark — wrapping it in a second one here duplicated
          // the main landmark (axe: landmark-no-duplicate-main). This stays
          // a plain element so `#main-content`/the skip link still has
          // something to focus.
          <div id="main-content" tabIndex={-1}>
            {children}
          </div>
        }
        ariaLabels={{
          navigation: "Service",
          navigationToggle: "Open navigation",
          navigationClose: "Close navigation",
        }}
      />
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
