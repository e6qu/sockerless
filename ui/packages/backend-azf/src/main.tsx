import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiClient } from "@sockerless/ui-core/api";
import { ApiClientProvider } from "@sockerless/ui-core/hooks";
import { BackendApp } from "@sockerless/ui-core/components";
import "./index.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

const apiClient = new ApiClient();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ApiClientProvider client={apiClient}>
        <BackendApp title="Azure Functions Backend" />
      </ApiClientProvider>
    </QueryClientProvider>
  </StrictMode>,
);
