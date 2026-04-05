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

export interface AdminResource {
  containerId: string;
  backend: string;
  resourceType: string;
  resourceId: string;
  instanceId: string;
  createdAt: string;
  cleanedUp: boolean;
  status?: string;
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

export interface ProjectConfig {
  name: string;
  cloud: CloudType;
  backend: BackendType;
  log_level: string;
  sim_port: number;
  backend_port: number;
  frontend_port: number;
  frontend_mgmt_port: number;
  created_at: string;
}

export interface ProjectStatus extends ProjectConfig {
  status: string;
  sim_status: string;
  backend_status: string;
  frontend_status: string;
}

export interface ProjectConnection {
  docker_host: string;
  env_export: string;
  podman_connection: string;
  simulator_addr: string;
  backend_addr: string;
  frontend_addr: string;
  frontend_mgmt_addr: string;
}

export interface CreateProjectRequest {
  name: string;
  cloud: CloudType;
  backend: BackendType;
  log_level?: string;
  sim_port?: number;
  backend_port?: number;
  frontend_port?: number;
  frontend_mgmt_port?: number;
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

  resources(active?: boolean): Promise<AdminResource[]> {
    const qs = active ? "?active=true" : "";
    return this.request(`/api/v1/resources${qs}`);
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

  // Projects
  projects(): Promise<ProjectStatus[]> {
    return this.request("/api/v1/projects");
  }

  projectGet(name: string): Promise<ProjectStatus> {
    return this.request(`/api/v1/projects/${encodeURIComponent(name)}`);
  }

  projectCreate(req: CreateProjectRequest): Promise<ProjectStatus> {
    return this.postJSON("/api/v1/projects", req);
  }

  projectStart(name: string): Promise<ProjectStatus> {
    return this.post(`/api/v1/projects/${encodeURIComponent(name)}/start`);
  }

  projectStop(name: string): Promise<ProjectStatus> {
    return this.post(`/api/v1/projects/${encodeURIComponent(name)}/stop`);
  }

  projectDelete(name: string): Promise<{ deleted: string }> {
    return this.del(`/api/v1/projects/${encodeURIComponent(name)}`);
  }

  projectLogs(
    name: string,
    component?: string,
    lines?: number,
  ): Promise<string[]> {
    const params = new URLSearchParams();
    if (component) params.set("component", component);
    if (lines) params.set("lines", String(lines));
    const qs = params.toString() ? `?${params.toString()}` : "";
    return this.request(
      `/api/v1/projects/${encodeURIComponent(name)}/logs${qs}`,
    );
  }

  projectConnection(name: string): Promise<ProjectConnection> {
    return this.request(
      `/api/v1/projects/${encodeURIComponent(name)}/connection`,
    );
  }
}
