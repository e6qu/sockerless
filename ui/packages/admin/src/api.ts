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

  components(): Promise<AdminComponent[]> {
    return this.request("/api/v1/components");
  }

  overview(): Promise<OverviewResponse> {
    return this.request("/api/v1/overview");
  }

  componentHealth(name: string): Promise<unknown> {
    return this.request(`/api/v1/components/${name}/health`);
  }

  componentStatus(name: string): Promise<unknown> {
    return this.request(`/api/v1/components/${name}/status`);
  }

  componentMetrics(name: string): Promise<unknown> {
    return this.request(`/api/v1/components/${name}/metrics`);
  }

  componentReload(name: string): Promise<unknown> {
    return this.post(`/api/v1/components/${name}/reload`);
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
}
