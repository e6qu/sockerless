async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

export interface CloudRunJob {
  name: string;
  createTime: string;
  executionCount: number;
  launchStage: string;
}

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

export const fetchCloudRunJobs = () => fetchJSON<CloudRunJob[]>("/sim/v1/cloudrun/jobs");
export const fetchCloudFunctions = () => fetchJSON<CloudFunction[]>("/sim/v1/functions");
export const fetchARRepos = () => fetchJSON<ARRepo[]>("/sim/v1/ar/repositories");
export const fetchGCSBuckets = () => fetchJSON<GCSBucket[]>("/sim/v1/gcs/buckets");
export const fetchLogEntries = () => fetchJSON<LogEntry[]>("/sim/v1/logging/entries");
