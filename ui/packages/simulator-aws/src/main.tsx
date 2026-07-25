import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { Route } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AwsApp } from "./console/index.js";
import { OverviewPage } from "./pages/OverviewPage.js";
import { ECSTasksPage } from "./pages/ECSTasksPage.js";
import { ECSTaskDetailPage } from "./pages/ECSTaskDetailPage.js";
import { LambdaFunctionsPage } from "./pages/LambdaFunctionsPage.js";
import { LambdaFunctionDetailPage } from "./pages/LambdaFunctionDetailPage.js";
import { ECRReposPage } from "./pages/ECRReposPage.js";
import { ECRRepositoryDetailPage } from "./pages/ECRRepositoryDetailPage.js";
import { S3BucketsPage } from "./pages/S3BucketsPage.js";
import { S3BucketDetailPage } from "./pages/S3BucketDetailPage.js";
import { LogGroupsPage } from "./pages/LogGroupsPage.js";
import { LogGroupDetailPage } from "./pages/LogGroupDetailPage.js";
import { IAMUsersPage } from "./pages/IAMUsersPage.js";
import { IAMUserSecurityCredentialsPage } from "./pages/IAMUserSecurityCredentialsPage.js";
import { OrganizationsPage } from "./pages/OrganizationsPage.js";
import { OrgAccountDetailPage } from "./pages/OrgAccountDetailPage.js";
import { NotSupportedServicePage } from "./pages/NotSupportedServicePage.js";
import "./index.css";

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <AwsApp>
        <Route path="/ui/" element={<OverviewPage />} />
        <Route path="/ui/ecs" element={<ECSTasksPage />} />
        <Route path="/ui/ecs/:taskArn" element={<ECSTaskDetailPage />} />
        <Route path="/ui/lambda" element={<LambdaFunctionsPage />} />
        <Route path="/ui/lambda/:name" element={<LambdaFunctionDetailPage />} />
        <Route path="/ui/ecr" element={<ECRReposPage />} />
        <Route path="/ui/ecr/:name" element={<ECRRepositoryDetailPage />} />
        <Route path="/ui/s3" element={<S3BucketsPage />} />
        <Route path="/ui/s3/:name" element={<S3BucketDetailPage />} />
        <Route path="/ui/logs" element={<LogGroupsPage />} />
        <Route path="/ui/logs/:name" element={<LogGroupDetailPage />} />
        <Route path="/ui/organizations" element={<OrganizationsPage />} />
        <Route path="/ui/organizations/accounts/:accountId" element={<OrgAccountDetailPage />} />
        <Route path="/ui/iam" element={<IAMUsersPage />} />
        <Route path="/ui/iam/users/:userName" element={<IAMUserSecurityCredentialsPage />} />
        <Route path="/ui/not-supported/:service" element={<NotSupportedServicePage />} />
      </AwsApp>
    </QueryClientProvider>
  </StrictMode>,
);
