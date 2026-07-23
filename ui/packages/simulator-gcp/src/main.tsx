import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { Route } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { GcpApp } from "./console/index.js";
import { OverviewPage } from "./pages/OverviewPage.js";
import { CloudRunJobsPage } from "./pages/CloudRunJobsPage.js";
import { CloudFunctionsPage } from "./pages/CloudFunctionsPage.js";
import { ArtifactRegistryPage } from "./pages/ArtifactRegistryPage.js";
import { GCSBucketsPage } from "./pages/GCSBucketsPage.js";
import { LoggingPage } from "./pages/LoggingPage.js";
import "./index.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <GcpApp>
        <Route path="/ui/" element={<OverviewPage />} />
        <Route path="/ui/cloudrun" element={<CloudRunJobsPage />} />
        <Route path="/ui/functions" element={<CloudFunctionsPage />} />
        <Route path="/ui/ar" element={<ArtifactRegistryPage />} />
        <Route path="/ui/gcs" element={<GCSBucketsPage />} />
        <Route path="/ui/logging" element={<LoggingPage />} />
      </GcpApp>
    </QueryClientProvider>
  </StrictMode>,
);
