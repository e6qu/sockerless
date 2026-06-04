import { useQuery } from "@tanstack/react-query";
import { fetchMetrics, fetchStatus } from "../api.js";
import type { BleephubMetrics, BleephubStatus } from "../types.js";

/** Shared hook for metrics + status data used by OverviewPage and MetricsPage. */
export function useMetricsData(): {
  metrics: BleephubMetrics | undefined;
  status: BleephubStatus | undefined;
  isLoading: boolean;
  isError: boolean;
} {
  const { data: metrics, isLoading: ml, isError: me } = useQuery({
    queryKey: ["metrics"],
    queryFn: fetchMetrics,
    refetchInterval: 5000,
  });
  const { data: status, isLoading: sl, isError: se } = useQuery({
    queryKey: ["status"],
    queryFn: fetchStatus,
    refetchInterval: 5000,
  });
  return {
    metrics,
    status,
    isLoading: ml || sl,
    isError: me || se,
  };
}
