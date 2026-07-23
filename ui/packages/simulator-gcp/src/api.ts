import { authorizedJSON } from "./console/federation.js";

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

// The console reads one project and region at a time, the way the real console
// shows the selected project and region. These are the console's coordinates; a
// deployment points them at the project its workloads run in.
export const CONSOLE_PROJECT = "sockerless";
export const CONSOLE_REGION = "us-central1";

const jobsParent = `/v2/projects/${CONSOLE_PROJECT}/locations/${CONSOLE_REGION}/jobs`;

// The Cloud Run v2 Job resource, as the real API returns it. The console reads
// the true shape rather than a hand-picked subset.
export interface CloudRunJobCondition {
  type?: string;
  state?: string;
  message?: string;
}

export interface CloudRunJob {
  name: string;
  uid?: string;
  createTime?: string;
  updateTime?: string;
  launchStage?: string;
  executionCount?: number;
  reconciling?: boolean;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  terminalCondition?: CloudRunJobCondition;
  conditions?: CloudRunJobCondition[];
  latestCreatedExecution?: { name?: string; createTime?: string; completionTime?: string };
}

export interface CloudRunExecution {
  name: string;
  createTime?: string;
  completionTime?: string;
  succeededCount?: number;
  failedCount?: number;
  runningCount?: number;
}

export const fetchCloudRunJobsReal = async (): Promise<CloudRunJob[]> => {
  const page = await authorizedJSON<{ jobs?: CloudRunJob[] }>(jobsParent);
  return page.jobs ?? [];
};

export const fetchCloudRunJob = (name: string): Promise<CloudRunJob> =>
  authorizedJSON<CloudRunJob>(`${jobsParent}/${name}`);

export const fetchCloudRunJobExecutions = async (name: string): Promise<CloudRunExecution[]> => {
  const page = await authorizedJSON<{ executions?: CloudRunExecution[] }>(`${jobsParent}/${name}/executions`);
  return page.executions ?? [];
};

// Cloud Functions (Gen2) lifecycle states.
export type CloudFunctionState =
  | "ACTIVE"
  | "FAILED"
  | "DEPLOYING"
  | "DELETING"
  | "UNKNOWN";

export interface CloudFunction {
  name: string;
  state: CloudFunctionState;
  environment: string;
  createTime: string;
}

export interface ARRepo {
  name: string;
  format: string;
  createTime: string;
}

export interface GCSBucket {
  name: string;
  data: Record<string, unknown>;
}

// Cloud Logging LogSeverity enum (proto .String()).
export type LogSeverity =
  | "DEFAULT"
  | "DEBUG"
  | "INFO"
  | "NOTICE"
  | "WARNING"
  | "ERROR"
  | "CRITICAL"
  | "ALERT"
  | "EMERGENCY";

export interface LogEntry {
  logName: string;
  timestamp: string;
  // Omitted by the server (json:"severity,omitempty") when unset (DEFAULT).
  severity?: LogSeverity;
  textPayload?: string;
}

export const fetchCloudFunctions = () => fetchJSON<CloudFunction[]>("/sim/v1/functions");
export const fetchARRepos = () => fetchJSON<ARRepo[]>("/sim/v1/ar/repositories");
export const fetchGCSBuckets = () => fetchJSON<GCSBucket[]>("/sim/v1/gcs/buckets");
export const fetchLogEntries = () => fetchJSON<LogEntry[]>("/sim/v1/logging/entries");
