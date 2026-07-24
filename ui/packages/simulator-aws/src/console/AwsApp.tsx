import { type ReactNode } from "react";
import { BrowserRouter, Routes, useLocation } from "react-router";
import { ErrorBoundary, OperatorAccount } from "@sockerless/ui-core/components";
import { AwsBreadcrumbs, AwsHeader, AwsSideNavigation, type NavGroup } from "./AwsConsole.js";
import { REGION } from "./federation.js";

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
  {
    label: "Security, identity, and compliance",
    items: [{ label: "Identity and Access Management", to: "/ui/iam" }],
  },
];

const CRUMBS: Record<string, string> = {
  "/ui/": "Overview",
  "/ui/ecs": "Elastic Container Service",
  "/ui/lambda": "Lambda",
  "/ui/ecr": "Elastic Container Registry",
  "/ui/s3": "Simple Storage Service",
  "/ui/logs": "CloudWatch Logs",
  "/ui/iam": "Identity and Access Management",
};

const IAM_USER_PREFIX = "/ui/iam/users/";

function crumbTrail(pathname: string): { label: string; to?: string }[] {
  // The IAM user detail page nests under the service the way the real console
  // breadcrumbs it: IAM > Users > <user name>.
  if (pathname.startsWith(IAM_USER_PREFIX)) {
    return [
      { label: "Simulator", to: "/ui/" },
      { label: "Identity and Access Management", to: "/ui/iam" },
      { label: decodeURIComponent(pathname.slice(IAM_USER_PREFIX.length)) },
    ];
  }
  return [
    { label: "Simulator", to: "/ui/" },
    { label: CRUMBS[pathname] ?? "Overview" },
  ];
}

function ConsoleFrame({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  return (
    <div className="aws">
      <a href="#main-content" className="sl-skip-link">Skip to main content</a>
      <AwsHeader region={REGION} account={<OperatorAccount />} />
      <AwsBreadcrumbs trail={crumbTrail(pathname)} />
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
