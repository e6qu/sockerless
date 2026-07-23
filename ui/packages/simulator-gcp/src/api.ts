import { authorizedJSON, authorizedJSONPost } from "./console/federation.js";

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

// The real Cloud Functions (Gen2) resource, as the API returns it.
export interface CloudFunction {
  name: string;
  state?: CloudFunctionState;
  environment?: string;
  description?: string;
  createTime?: string;
  updateTime?: string;
  labels?: Record<string, string>;
  serviceConfig?: { uri?: string; service?: string };
  buildConfig?: { runtime?: string; entryPoint?: string };
}

// The real Artifact Registry repository resource.
export interface ARRepo {
  name: string;
  format?: string;
  mode?: string;
  description?: string;
  createTime?: string;
  updateTime?: string;
  labels?: Record<string, string>;
}

// The real Cloud Storage bucket resource (storage#bucket).
export interface GCSBucket {
  name: string;
  id?: string;
  location?: string;
  storageClass?: string;
  timeCreated?: string;
  updated?: string;
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


const functionsParent = `/v2/projects/${CONSOLE_PROJECT}/locations/${CONSOLE_REGION}/functions`;
const repositoriesParent = `/v1/projects/${CONSOLE_PROJECT}/locations/${CONSOLE_REGION}/repositories`;

export const fetchCloudFunctions = async (): Promise<CloudFunction[]> =>
  (await authorizedJSON<{ functions?: CloudFunction[] }>(functionsParent)).functions ?? [];

export const fetchCloudFunction = (name: string): Promise<CloudFunction> =>
  authorizedJSON<CloudFunction>(`${functionsParent}/${name}`);

export const fetchARRepos = async (): Promise<ARRepo[]> =>
  (await authorizedJSON<{ repositories?: ARRepo[] }>(repositoriesParent)).repositories ?? [];

export const fetchARRepo = (name: string): Promise<ARRepo> =>
  authorizedJSON<ARRepo>(`${repositoriesParent}/${name}`);

export const fetchGCSBuckets = async (): Promise<GCSBucket[]> =>
  (await authorizedJSON<{ items?: GCSBucket[] }>(`/storage/v1/b?project=${CONSOLE_PROJECT}`)).items ?? [];

export const fetchGCSBucket = (name: string): Promise<GCSBucket> =>
  authorizedJSON<GCSBucket>(`/storage/v1/b/${name}`);

// Cloud Logging lists entries by POST, filtered to the project's logs.
export const fetchLogEntries = async (): Promise<LogEntry[]> =>
  (
    await authorizedJSONPost<{ entries?: LogEntry[] }>("/v2/entries:list", {
      resourceNames: [`projects/${CONSOLE_PROJECT}`],
      orderBy: "timestamp desc",
      pageSize: 100,
    })
  ).entries ?? [];
