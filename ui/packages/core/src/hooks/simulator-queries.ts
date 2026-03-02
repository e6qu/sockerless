import { useQuery } from "@tanstack/react-query";

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

export interface SimHealth {
  status: string;
  provider: string;
}

export function useSimHealth() {
  return useQuery({
    queryKey: ["sim-health"],
    queryFn: () => fetchJSON<SimHealth>("/health"),
    refetchInterval: 10_000,
  });
}

export interface SimSummary {
  provider: string;
  services: Record<string, number>;
}

export function useSimSummary() {
  return useQuery({
    queryKey: ["sim-summary"],
    queryFn: () => fetchJSON<SimSummary>("/sim/v1/summary"),
    refetchInterval: 5_000,
  });
}
