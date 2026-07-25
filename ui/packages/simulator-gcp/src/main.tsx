import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { Route } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { GcpApp } from "./console/index.js";
import { OverviewPage } from "./pages/OverviewPage.js";
import { CloudRunJobsPage } from "./pages/CloudRunJobsPage.js";
import { CloudRunJobDetailPage } from "./pages/CloudRunJobDetailPage.js";
import { CloudFunctionsPage } from "./pages/CloudFunctionsPage.js";
import { CloudFunctionDetailPage } from "./pages/CloudFunctionDetailPage.js";
import { ArtifactRegistryPage } from "./pages/ArtifactRegistryPage.js";
import { ARRepoDetailPage } from "./pages/ARRepoDetailPage.js";
import { GCSBucketsPage } from "./pages/GCSBucketsPage.js";
import { GCSBucketDetailPage } from "./pages/GCSBucketDetailPage.js";
import { ServiceAccountsPage } from "./pages/ServiceAccountsPage.js";
import { ServiceAccountDetailPage } from "./pages/ServiceAccountDetailPage.js";
import { ManageProjectsPage } from "./pages/ManageProjectsPage.js";
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
        <Route path="/ui/cloudrun/:name" element={<CloudRunJobDetailPage />} />
        <Route path="/ui/functions" element={<CloudFunctionsPage />} />
        <Route path="/ui/functions/:name" element={<CloudFunctionDetailPage />} />
        <Route path="/ui/ar" element={<ArtifactRegistryPage />} />
        <Route path="/ui/ar/:name" element={<ARRepoDetailPage />} />
        <Route path="/ui/gcs" element={<GCSBucketsPage />} />
        <Route path="/ui/gcs/:name" element={<GCSBucketDetailPage />} />
        <Route path="/ui/serviceaccounts" element={<ServiceAccountsPage />} />
        <Route path="/ui/serviceaccounts/:email" element={<ServiceAccountDetailPage />} />
        <Route path="/ui/projects" element={<ManageProjectsPage />} />
        <Route path="/ui/logging" element={<LoggingPage />} />
      </GcpApp>
    </QueryClientProvider>
  </StrictMode>,
);
