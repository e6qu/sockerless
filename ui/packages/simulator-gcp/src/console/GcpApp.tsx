import { type ReactNode } from "react";
import { BrowserRouter, Routes } from "react-router";
import { ErrorBoundary, OperatorAccount } from "@sockerless/ui-core/components";
import { GcpHeader, GcpProductNav, type GcpNavItem } from "./GcpConsole.js";

/** The console navigates a single product flat, the way Cloud Run lists
 *  Overview, Services, Jobs. The simulator spans several products, so the
 *  navigation is one flat list under a single product identity. */
const NAV_ITEMS: GcpNavItem[] = [
  { label: "Overview", to: "/ui/" },
  { label: "Cloud Run jobs", to: "/ui/cloudrun" },
  { label: "Cloud Run functions", to: "/ui/functions" },
  { label: "Artifact Registry", to: "/ui/ar" },
  { label: "Cloud Storage", to: "/ui/gcs" },
  { label: "Logs Explorer", to: "/ui/logging" },
];

function ConsoleFrame({ children }: { children: ReactNode }) {
  return (
    <div className="gcp">
      <a href="#main-content" className="sl-skip-link">Skip to main content</a>
      <GcpHeader project="Simulator" account={<OperatorAccount />} />
      <div className="gc-body">
        <GcpProductNav product="Simulator" items={NAV_ITEMS} />
        <main id="main-content" tabIndex={-1} className="gc-main">
          {children}
        </main>
      </div>
    </div>
  );
}

export function GcpApp({ children }: { children: ReactNode }) {
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
