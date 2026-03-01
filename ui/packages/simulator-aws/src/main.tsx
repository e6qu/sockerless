import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { Route } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SimulatorApp } from "@sockerless/ui-core/components";
import { OverviewPage } from "./pages/OverviewPage.js";
import { ECSTasksPage } from "./pages/ECSTasksPage.js";
import { LambdaFunctionsPage } from "./pages/LambdaFunctionsPage.js";
import { ECRReposPage } from "./pages/ECRReposPage.js";
import { S3BucketsPage } from "./pages/S3BucketsPage.js";
import { LogGroupsPage } from "./pages/LogGroupsPage.js";
import "./index.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

const navItems = [
  { label: "Overview", to: "/ui/" },
  { label: "ECS Tasks", to: "/ui/ecs" },
  { label: "Lambda", to: "/ui/lambda" },
  { label: "ECR", to: "/ui/ecr" },
  { label: "S3", to: "/ui/s3" },
  { label: "Logs", to: "/ui/logs" },
];

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <SimulatorApp title="AWS Simulator" navItems={navItems}>
        <Route path="/ui/" element={<OverviewPage />} />
        <Route path="/ui/ecs" element={<ECSTasksPage />} />
        <Route path="/ui/lambda" element={<LambdaFunctionsPage />} />
        <Route path="/ui/ecr" element={<ECRReposPage />} />
        <Route path="/ui/s3" element={<S3BucketsPage />} />
        <Route path="/ui/logs" element={<LogGroupsPage />} />
      </SimulatorApp>
    </QueryClientProvider>
  </StrictMode>,
);
