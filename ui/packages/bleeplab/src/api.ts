// Thin client over bleeplab's read-only /internal aggregation surface. The
// dashboard is a local operator view of the control-plane simulator, so there
// is no auth gate (mirrors bleeplab's open /internal endpoints).
import type {
  Status,
  Project,
  Pipeline,
  PipelineDetail,
  Job,
  Runner,
  Storage,
} from "./types.js";

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path, { headers: { Accept: "application/json" } });
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`);
  }
  return (await res.json()) as T;
}

export const api = {
  status: () => getJSON<Status>("/internal/status"),
  projects: () => getJSON<Project[]>("/internal/projects"),
  pipelines: () => getJSON<Pipeline[]>("/internal/pipelines"),
  pipeline: (id: number | string) =>
    getJSON<PipelineDetail>(`/internal/pipelines/${id}`),
  job: (id: number | string) => getJSON<Job>(`/internal/jobs/${id}`),
  runners: () => getJSON<Runner[]>("/internal/runners"),
  storage: () => getJSON<Storage>("/internal/storage"),
};
