import { useQuery } from "@tanstack/react-query";
import { fetchMetrics } from "../api.js";
import type { BleephubMetrics, BleephubStatus } from "../types.js";

/** Shared hook for metrics + status data used by OverviewPage and MetricsPage. */
export function useMetricsData(): {
  metrics: BleephubMetrics | undefined;
  status: BleephubStatus | undefined;
  isLoading: boolean;
  isError: boolean;
} {
  const { data: metrics, isLoading, isError } = useQuery({
    queryKey: ["metrics"],
    queryFn: fetchMetrics,
    refetchInterval: 5000,
  });
  const status = metrics
    ? {
      active_workflows: metrics.active_workflows,
      jobs_by_status: metrics.jobs_by_status,
      connected_runners: metrics.connected_runners,
    }
    : undefined;
  return {
    metrics,
    status,
    isLoading,
    isError,
  };
}
