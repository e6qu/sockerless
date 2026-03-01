import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { Route } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SimulatorApp } from "@sockerless/ui-core/components";
import { OverviewPage } from "./pages/OverviewPage.js";
import { ContainerAppsPage } from "./pages/ContainerAppsPage.js";
import { AzureFunctionsPage } from "./pages/AzureFunctionsPage.js";
import { ACRRegistriesPage } from "./pages/ACRRegistriesPage.js";
import { StorageAccountsPage } from "./pages/StorageAccountsPage.js";
import { MonitorPage } from "./pages/MonitorPage.js";
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
  { label: "Container Apps", to: "/ui/container-apps" },
  { label: "Functions", to: "/ui/functions" },
  { label: "ACR", to: "/ui/acr" },
  { label: "Storage", to: "/ui/storage" },
  { label: "Monitor", to: "/ui/monitor" },
];

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <SimulatorApp title="Azure Simulator" navItems={navItems}>
        <Route path="/ui/" element={<OverviewPage />} />
        <Route path="/ui/container-apps" element={<ContainerAppsPage />} />
        <Route path="/ui/functions" element={<AzureFunctionsPage />} />
        <Route path="/ui/acr" element={<ACRRegistriesPage />} />
        <Route path="/ui/storage" element={<StorageAccountsPage />} />
        <Route path="/ui/monitor" element={<MonitorPage />} />
      </SimulatorApp>
    </QueryClientProvider>
  </StrictMode>,
);
