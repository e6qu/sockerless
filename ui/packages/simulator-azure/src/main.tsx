import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { Route } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AzureApp } from "./portal/index.js";
import { OverviewPage } from "./pages/OverviewPage.js";
import { SubscriptionsPage } from "./pages/SubscriptionsPage.js";
import { SubscriptionDetailPage } from "./pages/SubscriptionDetailPage.js";
import { ContainerAppsPage } from "./pages/ContainerAppsPage.js";
import { ContainerAppDetailPage } from "./pages/ContainerAppDetailPage.js";
import { AzureFunctionsPage } from "./pages/AzureFunctionsPage.js";
import { FunctionAppDetailPage } from "./pages/FunctionAppDetailPage.js";
import { ACRRegistriesPage } from "./pages/ACRRegistriesPage.js";
import { ACRRegistryDetailPage } from "./pages/ACRRegistryDetailPage.js";
import { StorageAccountsPage } from "./pages/StorageAccountsPage.js";
import { StorageAccountDetailPage } from "./pages/StorageAccountDetailPage.js";
import { MonitorPage } from "./pages/MonitorPage.js";
import { AppRegistrationsPage } from "./pages/AppRegistrationsPage.js";
import { AppRegistrationDetailPage } from "./pages/AppRegistrationDetailPage.js";
import { NotSupportedPage } from "./pages/NotSupportedPage.js";
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
      <AzureApp>
        <Route path="/ui/" element={<OverviewPage />} />
        <Route path="/ui/subscriptions" element={<SubscriptionsPage />} />
        <Route path="/ui/subscriptions/:subscriptionId" element={<SubscriptionDetailPage />} />
        <Route path="/ui/container-apps" element={<ContainerAppsPage />} />
        <Route path="/ui/container-apps/:name" element={<ContainerAppDetailPage />} />
        <Route path="/ui/functions" element={<AzureFunctionsPage />} />
        <Route path="/ui/functions/:name" element={<FunctionAppDetailPage />} />
        <Route path="/ui/acr" element={<ACRRegistriesPage />} />
        <Route path="/ui/acr/:name" element={<ACRRegistryDetailPage />} />
        <Route path="/ui/storage" element={<StorageAccountsPage />} />
        <Route path="/ui/storage/:name" element={<StorageAccountDetailPage />} />
        <Route path="/ui/monitor" element={<MonitorPage />} />
        <Route path="/ui/entra/app-registrations" element={<AppRegistrationsPage />} />
        <Route path="/ui/entra/app-registrations/:objectId" element={<AppRegistrationDetailPage />} />
        <Route path="/ui/not-supported/:slug" element={<NotSupportedPage />} />
      </AzureApp>
    </QueryClientProvider>
  </StrictMode>,
);
