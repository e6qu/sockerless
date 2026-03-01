export interface FrontendHealth {
  status: string;
  component: string;
  uptime_seconds: number;
}

export interface FrontendStatus {
  status: string;
  component: string;
  docker_addr: string;
  backend_addr: string;
  uptime_seconds: number;
}

export interface FrontendMetrics {
  component: string;
  uptime_seconds: number;
  docker_requests: number;
  goroutines: number;
  heap_alloc_mb: number;
}

async function fetchJson<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

export function fetchHealth(): Promise<FrontendHealth> {
  return fetchJson("/healthz");
}

export function fetchStatus(): Promise<FrontendStatus> {
  return fetchJson("/status");
}

export function fetchMetrics(): Promise<FrontendMetrics> {
  return fetchJson("/metrics");
}
