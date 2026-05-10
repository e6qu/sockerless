/** Admin API types */

export interface AdminComponent {
  name: string;
  type: string;
  addr: string;
  health: string;
  uptime: number;
}

export interface OverviewResponse {
  components_up: number;
  components_down: number;
  components_total: number;
  total_containers: number;
  backends: number;
  components: AdminComponent[];
}

export interface AdminContainer {
  id: string;
  name: string;
  image: string;
  state: string;
  created: string;
  pod_name?: string;
  backend: string;
}

export interface ContextInfo {
  name: string;
  active: boolean;
  backend?: string;
  frontend_addr?: string;
  backend_addr?: string;
}

export interface ProcessInfo {
  name: string;
  binary: string;
  status: string;
  pid: number;
  addr: string;
  started_at: string;
  exit_code: number;
  type: string;
}

export interface CleanupItem {
  category: string;
  name: string;
  description: string;
  size?: number;
  age: string;
}

export interface CleanupScanResult {
  items: CleanupItem[];
  scanned_at: string;
}

export interface ProviderInfo {
  provider: string;
  mode: string;
  region?: string;
  endpoint?: string;
  resources?: Record<string, string>;
}

export interface ComponentStatus {
  name: string;
  type: string;
  health: string;
  address: string;
  uptime: number;
  containers: number;
  [key: string]: unknown;
}

export interface ComponentMetrics {
  [key: string]: unknown;
}

export type CloudType = "aws" | "gcp" | "azure";
export type BackendType = "ecs" | "lambda" | "cloudrun" | "gcf" | "aca" | "azf";

export type InstanceKind = "sim" | "backend" | "bleephub";

export interface TopologyInstance {
  name: string;
  kind: InstanceKind;
  cloud?: CloudType;
  backend?: BackendType;
  port: number;
  sim?: string;
  config?: Record<string, string>;
}

export interface TopologyProject {
  name: string;
  cloud?: CloudType;
  backend?: BackendType;
  log_level?: string;
  sim_port?: number;
  backend_port?: number;
  created_at?: string;
  instances?: TopologyInstance[];
}

export interface PortRange {
  from: number;
  to: number;
}

export interface PortConfig {
  ranges?: Partial<Record<InstanceKind, PortRange>>;
}

export interface Topology {
  projects?: TopologyProject[];
  ports?: PortConfig;
}

export interface InstanceRef {
  project: string;
  instance: TopologyInstance;
}

export type HealthState = "ok" | "unhealthy" | "unknown";

export interface InstanceStatus {
  project: string;
  name: string;
  running: boolean;
  pid: number;
  health: HealthState;
  health_detail?: string;
}

/** Error thrown when the admin API returns a non-ok response. */
export class AdminApiError extends Error {
  constructor(
    public status: number,
    public statusText: string,
    public body: string,
  ) {
    super(`Admin API error ${status}: ${statusText}`);
    this.name = "AdminApiError";
  }
}

/** Admin API client. */
export class AdminApiClient {
  private async request<T>(path: string): Promise<T> {
    const res = await fetch(path);
    if (!res.ok) {
      const body = await res.text();
      throw new AdminApiError(res.status, res.statusText, body);
    }
    return res.json() as Promise<T>;
  }

  private async post<T>(path: string): Promise<T> {
    const res = await fetch(path, { method: "POST" });
    if (!res.ok) {
      const body = await res.text();
      throw new AdminApiError(res.status, res.statusText, body);
    }
    return res.json() as Promise<T>;
  }

  private async postJSON<T>(path: string, body: unknown): Promise<T> {
    const res = await fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const text = await res.text();
      throw new AdminApiError(res.status, res.statusText, text);
    }
    return res.json() as Promise<T>;
  }

  private async putJSON<T>(path: string, body: unknown): Promise<T> {
    const res = await fetch(path, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const text = await res.text();
      throw new AdminApiError(res.status, res.statusText, text);
    }
    return res.json() as Promise<T>;
  }

  private async del<T>(path: string): Promise<T> {
    const res = await fetch(path, { method: "DELETE" });
    if (!res.ok) {
      const body = await res.text();
      throw new AdminApiError(res.status, res.statusText, body);
    }
    return res.json() as Promise<T>;
  }

  components(): Promise<AdminComponent[]> {
    return this.request("/api/v1/components");
  }

  overview(): Promise<OverviewResponse> {
    return this.request("/api/v1/overview");
  }

  componentStatus(name: string): Promise<ComponentStatus> {
    return this.request(
      `/api/v1/components/${encodeURIComponent(name)}/status`,
    );
  }

  componentMetrics(name: string): Promise<ComponentMetrics> {
    return this.request(
      `/api/v1/components/${encodeURIComponent(name)}/metrics`,
    );
  }

  componentReload(name: string): Promise<ComponentStatus> {
    return this.post(`/api/v1/components/${encodeURIComponent(name)}/reload`);
  }

  containers(): Promise<AdminContainer[]> {
    return this.request("/api/v1/containers");
  }

  contexts(): Promise<ContextInfo[]> {
    return this.request("/api/v1/contexts");
  }

  // Process management
  processes(): Promise<ProcessInfo[]> {
    return this.request("/api/v1/processes");
  }

  processStart(name: string): Promise<ProcessInfo> {
    return this.post(`/api/v1/processes/${encodeURIComponent(name)}/start`);
  }

  processStop(name: string): Promise<ProcessInfo> {
    return this.post(`/api/v1/processes/${encodeURIComponent(name)}/stop`);
  }

  processLogs(name: string, lines?: number): Promise<string[]> {
    const qs = lines ? `?lines=${lines}` : "";
    return this.request(
      `/api/v1/processes/${encodeURIComponent(name)}/logs${qs}`,
    );
  }

  // Cleanup
  cleanupScan(): Promise<CleanupScanResult> {
    return this.request("/api/v1/cleanup/scan");
  }

  cleanupProcesses(): Promise<{ cleaned: number }> {
    return this.post("/api/v1/cleanup/processes");
  }

  cleanupTmp(): Promise<{ cleaned: number }> {
    return this.post("/api/v1/cleanup/tmp");
  }

  cleanupContainers(): Promise<{ cleaned: number }> {
    return this.post("/api/v1/cleanup/containers");
  }

  // Provider info
  componentProvider(name: string): Promise<ProviderInfo> {
    return this.request(
      `/api/v1/components/${encodeURIComponent(name)}/provider`,
    );
  }

  topology(): Promise<Topology> {
    return this.request("/api/v1/topology");
  }

  topologyReplace(t: Topology): Promise<Topology> {
    return this.putJSON("/api/v1/topology", t);
  }

  topologyInstances(): Promise<InstanceRef[]> {
    return this.request("/api/v1/topology/instances");
  }

  topologyAddProject(p: TopologyProject): Promise<TopologyProject> {
    return this.postJSON("/api/v1/topology/projects", p);
  }

  topologyRemoveProject(name: string): Promise<{ removed: string }> {
    return this.del(`/api/v1/topology/projects/${encodeURIComponent(name)}`);
  }

  topologyAddInstance(
    project: string,
    inst: TopologyInstance,
  ): Promise<InstanceRef> {
    return this.postJSON(
      `/api/v1/topology/projects/${encodeURIComponent(project)}/instances`,
      inst,
    );
  }

  topologyUpdateInstance(
    project: string,
    inst: TopologyInstance,
  ): Promise<InstanceRef> {
    return this.putJSON(
      `/api/v1/topology/projects/${encodeURIComponent(project)}/instances/${encodeURIComponent(inst.name)}`,
      inst,
    );
  }

  topologyRemoveInstance(
    project: string,
    name: string,
  ): Promise<{ removed: string }> {
    return this.del(
      `/api/v1/topology/projects/${encodeURIComponent(project)}/instances/${encodeURIComponent(name)}`,
    );
  }

  topologyInstanceStart(
    project: string,
    name: string,
  ): Promise<{ status: string }> {
    return this.post(
      `/api/v1/topology/projects/${encodeURIComponent(project)}/instances/${encodeURIComponent(name)}/start`,
    );
  }

  topologyInstanceStop(
    project: string,
    name: string,
  ): Promise<{ status: string }> {
    return this.post(
      `/api/v1/topology/projects/${encodeURIComponent(project)}/instances/${encodeURIComponent(name)}/stop`,
    );
  }

  topologyInstanceRebuild(
    project: string,
    name: string,
  ): Promise<{ status: string }> {
    return this.post(
      `/api/v1/topology/projects/${encodeURIComponent(project)}/instances/${encodeURIComponent(name)}/rebuild`,
    );
  }

  topologyInstanceStatus(
    project: string,
    name: string,
  ): Promise<InstanceStatus> {
    return this.request(
      `/api/v1/topology/projects/${encodeURIComponent(project)}/instances/${encodeURIComponent(name)}/status`,
    );
  }

  topologyAllocatePort(
    kind: InstanceKind,
  ): Promise<{ kind: InstanceKind; port: number }> {
    return this.post(
      `/api/v1/topology/allocate-port?kind=${encodeURIComponent(kind)}`,
    );
  }

  topologyInstanceProxy(
    project: string,
    name: string,
    req: ProxyRequest,
  ): Promise<ProxyResponse> {
    return this.postJSON(
      `/api/v1/topology/projects/${encodeURIComponent(project)}/instances/${encodeURIComponent(name)}/proxy`,
      req,
    );
  }

  topologyResources(active?: boolean): Promise<RollupResponse> {
    const qs = active ? "?active=true" : "";
    return this.request(`/api/v1/topology/resources${qs}`);
  }
}

export interface RollupSource {
  project: string;
  instance: string;
  cloud?: string;
  backend?: string;
  port: number;
  ok: boolean;
  error?: string;
  resource_count: number;
}

export interface RollupResource {
  project: string;
  instance: string;
  cloud?: string;
  backend?: string;
  port: number;
  container_id?: string;
  resource_type: string;
  resource_id: string;
  instance_id?: string;
  created_at?: string;
  cleaned_up: boolean;
  status?: string;
}

export interface RollupResponse {
  sources: RollupSource[];
  resources: RollupResource[];
}

export interface ProxyRequest {
  method: string;
  path: string;
  headers?: Record<string, string>;
  body?: string;
}

export interface ProxyResponse {
  status: number;
  status_text: string;
  headers: Record<string, string>;
  body: string;
  duration_ms: number;
}
