import { useState, type ReactNode } from "react";
import { BrowserRouter, Routes, useLocation } from "react-router";
import { ErrorBoundary, OperatorAccount } from "@sockerless/ui-core/components";
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
const SIDENAV_ID = "aws-sidenav";

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
  // The header's hamburger control and the side navigation share this state.
  // Cloudscape's own AppLayout keeps the navigation open by default and lets
  // an operator collapse it; the collapse only changes anything visually
  // below the responsive breakpoint (see console.css), but the control stays
  // wired and keyboard-operable regardless of viewport width.
  const [navExpanded, setNavExpanded] = useState(true);
  return (
    <div className="aws">
      <a href="#main-content" className="sl-skip-link">Skip to main content</a>
      <AwsHeader
        region={REGION}
        account={<OperatorAccount />}
        navExpanded={navExpanded}
        onToggleNav={() => setNavExpanded((current) => !current)}
        navId={SIDENAV_ID}
      />
      <AwsBreadcrumbs trail={crumbTrail(pathname)} />
      <div className="aws-body">
        <AwsSideNavigation serviceName="Simulator" groups={NAV_GROUPS} id={SIDENAV_ID} expanded={navExpanded} />
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
