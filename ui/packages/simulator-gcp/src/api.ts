import {
  authorizedJSON,
  authorizedJSONDelete,
  authorizedJSONPatch,
  authorizedJSONPost,
} from "./console/federation.js";

// The console reads one project and region at a time, the way the real console
// shows the selected project and region. The region is a console coordinate; a
// deployment points it at the region its workloads run in. The project is SPA
// state: the header's project picker selects it (console/project.tsx persists
// the choice), so every fetcher takes the selected project and every query key
// carries it.
export const DEFAULT_CONSOLE_PROJECT = "sockerless";
export const CONSOLE_REGION = "us-central1";

const jobsParent = (project: string) => `/v2/projects/${project}/locations/${CONSOLE_REGION}/jobs`;

// The Cloud Run v2 Job resource, as the real API returns it. The console reads
// the true shape rather than a hand-picked subset.
export interface CloudRunJobCondition {
  type?: string;
  state?: string;
  message?: string;
}

// The Cloud Run v2 execution/task template, as the API nests it under a Job.
// The console reads and round-trips the real shape so an edit or create carries
// the same fields a real client (gcloud, terraform-provider-google) sends.
export interface CloudRunEnvVar {
  name: string;
  value?: string;
}

export interface CloudRunContainer {
  name?: string;
  image: string;
  command?: string[];
  args?: string[];
  env?: CloudRunEnvVar[];
  resources?: { limits?: Record<string, string> };
}

export interface CloudRunTaskTemplate {
  containers?: CloudRunContainer[];
  timeout?: string;
  maxRetries?: number;
  serviceAccount?: string;
}

export interface CloudRunExecutionTemplate {
  parallelism?: number;
  taskCount?: number;
  template?: CloudRunTaskTemplate;
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
  template?: CloudRunExecutionTemplate;
  terminalCondition?: CloudRunJobCondition;
  conditions?: CloudRunJobCondition[];
  latestCreatedExecution?: { name?: string; createTime?: string; completionTime?: string };
}

export interface CloudRunExecution {
  name: string;
  createTime?: string;
  startTime?: string;
  completionTime?: string;
  succeededCount?: number;
  failedCount?: number;
  runningCount?: number;
  cancelledCount?: number;
  taskCount?: number;
  conditions?: CloudRunJobCondition[];
}

export const fetchCloudRunJobsReal = async (project: string): Promise<CloudRunJob[]> => {
  const page = await authorizedJSON<{ jobs?: CloudRunJob[] }>(jobsParent(project));
  return page.jobs ?? [];
};

export const fetchCloudRunJob = (project: string, name: string): Promise<CloudRunJob> =>
  authorizedJSON<CloudRunJob>(`${jobsParent(project)}/${name}`);

export const fetchCloudRunJobExecutions = async (project: string, name: string): Promise<CloudRunExecution[]> => {
  const page = await authorizedJSON<{ executions?: CloudRunExecution[] }>(`${jobsParent(project)}/${name}/executions`);
  return page.executions ?? [];
};

// projects.locations.jobs.delete — a long-running operation, driven through
// the same operations.get poll waitV2Operation runs for Cloud Functions.
export const deleteCloudRunJob = (project: string, name: string): Promise<CrmOperation> =>
  authorizedJSONDelete<CrmOperation>(`${jobsParent(project)}/${name}`);

// The fields the Create job / Edit job forms collect. The console composes
// them into the real Cloud Run v2 Job body (a nested execution + task
// template) rather than a flattened sockerless shape.
export interface CloudRunJobConfig {
  image: string;
  taskCount: number;
  timeoutSeconds: number;
  env: CloudRunEnvVar[];
}

// jobBodyFromConfig builds the real projects.locations.jobs Job body from the
// form's fields — the same nested template shape gcloud and
// terraform-provider-google send.
const jobBodyFromConfig = (config: CloudRunJobConfig, labels?: Record<string, string>): CloudRunJob => ({
  name: "",
  ...(labels && Object.keys(labels).length > 0 ? { labels } : {}),
  template: {
    taskCount: config.taskCount,
    template: {
      timeout: `${config.timeoutSeconds}s`,
      containers: [
        {
          image: config.image,
          ...(config.env.length > 0 ? { env: config.env } : {}),
        },
      ],
    },
  },
});

// projects.locations.jobs.create — POST ?jobId=, a long-running operation the
// console drives to completion through the same operations.get poll
// (waitV2Operation) the delete flow uses.
export const createCloudRunJob = (
  project: string,
  jobId: string,
  config: CloudRunJobConfig,
): Promise<CrmOperation> =>
  authorizedJSONPost<CrmOperation>(
    `${jobsParent(project)}?jobId=${encodeURIComponent(jobId)}`,
    jobBodyFromConfig(config),
  );

// projects.locations.jobs.run — POST {job}:run creates an execution and
// returns a long-running Operation. The `:run` verb rides on the resource path
// (not query), so the job name is templated in directly rather than
// URL-encoded (which would escape the colon).
export const runCloudRunJob = (project: string, name: string): Promise<CrmOperation> =>
  authorizedJSONPost<CrmOperation>(`${jobsParent(project)}/${name}:run`, {});

// projects.locations.jobs.patch — UpdateJob replaces the full mutable
// resource (the real API and the simulator both round-trip the whole Job), so
// the console reads the loaded job, applies the edited template fields, and
// sends the complete Job back rather than a partial patch that would drop the
// rest of the template. Returns a long-running Operation.
export const updateCloudRunJob = (
  project: string,
  name: string,
  job: CloudRunJob,
): Promise<CrmOperation> =>
  authorizedJSONPatch<CrmOperation>(`${jobsParent(project)}/${name}`, job);

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
  serviceConfig?: {
    uri?: string;
    service?: string;
    timeoutSeconds?: number;
    availableMemory?: string;
    availableCpu?: string;
    maxInstanceCount?: number;
    minInstanceCount?: number;
    ingressSettings?: string;
    environmentVariables?: Record<string, string>;
  };
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

// The real Artifact Registry DockerImage resource
// (artifactregistry.googleapis.com/v1 projects.locations.repositories.dockerImages).
export interface ARDockerImage {
  name: string;
  uri?: string;
  tags?: string[];
  uploadTime?: string;
  mediaType?: string;
  buildTime?: string;
}

// The real Cloud Storage bucket resource (storage#bucket).
export interface GCSBucket {
  name: string;
  id?: string;
  location?: string;
  storageClass?: string;
  timeCreated?: string;
  updated?: string;
  labels?: Record<string, string>;
}

// The real Cloud Storage object resource (storage#object).
export interface GCSObject {
  name: string;
  bucket: string;
  size?: string;
  contentType?: string;
  storageClass?: string;
  timeCreated?: string;
  updated?: string;
  generation?: string;
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

// The monitored resource that produced a log entry (logging/v2
// MonitoredResource), as the real API returns it.
export interface LogEntryResource {
  type: string;
  labels?: Record<string, string>;
}

export interface LogEntry {
  logName: string;
  timestamp: string;
  // Omitted by the server (json:"severity,omitempty") when unset (DEFAULT).
  severity?: LogSeverity;
  textPayload?: string;
  jsonPayload?: Record<string, unknown>;
  insertId?: string;
  resource?: LogEntryResource;
  labels?: Record<string, string>;
}


const functionsParent = (project: string) => `/v2/projects/${project}/locations/${CONSOLE_REGION}/functions`;
const repositoriesParent = (project: string) => `/v1/projects/${project}/locations/${CONSOLE_REGION}/repositories`;

export const fetchCloudFunctions = async (project: string): Promise<CloudFunction[]> =>
  (await authorizedJSON<{ functions?: CloudFunction[] }>(functionsParent(project))).functions ?? [];

export const fetchCloudFunction = (project: string, name: string): Promise<CloudFunction> =>
  authorizedJSON<CloudFunction>(`${functionsParent(project)}/${name}`);

// projects.locations.functions.delete — a long-running operation, driven
// through the same operations.get poll waitV2Operation runs for Cloud Run jobs.
export const deleteCloudFunction = (project: string, name: string): Promise<CrmOperation> =>
  authorizedJSONDelete<CrmOperation>(`${functionsParent(project)}/${name}`);

// projects.locations.functions.patch — UpdateFunction, a long-running
// operation. The updateMask names the serviceConfig fields the form edits; the
// body carries the full merged serviceConfig (the whole object is replaced at
// serviceConfig granularity), so the caller passes the loaded function's
// serviceConfig with the edited fields applied to avoid dropping the rest.
export const updateCloudFunction = (
  project: string,
  name: string,
  serviceConfig: CloudFunction["serviceConfig"],
): Promise<CrmOperation> => {
  const mask = [
    "serviceConfig.availableMemory",
    "serviceConfig.timeoutSeconds",
    "serviceConfig.minInstanceCount",
    "serviceConfig.maxInstanceCount",
    "serviceConfig.environmentVariables",
  ].join(",");
  return authorizedJSONPatch<CrmOperation>(
    `${functionsParent(project)}/${name}?updateMask=${encodeURIComponent(mask)}`,
    { serviceConfig },
  );
};

// The fields the Create function form collects. Image-based deploys are the
// simplest faithful path against the simulator, but the real console's form
// leads with runtime + entry point, so the console collects those.
export interface CloudFunctionCreateConfig {
  runtime: string;
  entryPoint: string;
}

// projects.locations.functions.create — POST ?functionId=, a long-running
// operation. The console sends the real buildConfig (runtime + entry point)
// the way the create form collects it.
export const createCloudFunction = (
  project: string,
  functionId: string,
  config: CloudFunctionCreateConfig,
): Promise<CrmOperation> =>
  authorizedJSONPost<CrmOperation>(
    `${functionsParent(project)}?functionId=${encodeURIComponent(functionId)}`,
    { buildConfig: { runtime: config.runtime, entryPoint: config.entryPoint }, environment: "GEN_2" },
  );

// operations.get for the /v2/projects/.../locations/.../operations/{id}
// collection Cloud Functions and Cloud Run Jobs LROs live under — the v2
// counterpart to fetchArOperation's v1 collection.
export const fetchV2Operation = (name: string): Promise<CrmOperation> =>
  authorizedJSON<CrmOperation>(`/v2/${name}`);

// waitV2Operation drives a returned Operation to completion the way every
// real client does: poll operations.get until done, then surface the
// operation's own error if it failed.
export const waitV2Operation = async (operation: CrmOperation): Promise<CrmOperation> => {
  let current = operation;
  while (!current.done) {
    await new Promise((resolve) => setTimeout(resolve, 500));
    current = await fetchV2Operation(current.name);
  }
  if (current.error) {
    throw new Error(current.error.message ?? `operation ${current.name} failed`);
  }
  return current;
};

export const fetchARRepos = async (project: string): Promise<ARRepo[]> =>
  (await authorizedJSON<{ repositories?: ARRepo[] }>(repositoriesParent(project))).repositories ?? [];

export const fetchARRepo = (project: string, name: string): Promise<ARRepo> =>
  authorizedJSON<ARRepo>(`${repositoriesParent(project)}/${name}`);

// projects.locations.repositories.create — a long-running operation. The
// simulator (like real Artifact Registry for a plain repository create)
// settles it synchronously and returns it already `done`, but the console
// drives it through the same operations.get poll loop a real client uses
// (waitArOperation below) rather than assuming immediate completion.
export const createARRepository = (
  project: string,
  location: string,
  repositoryId: string,
  format = "DOCKER",
): Promise<CrmOperation> =>
  authorizedJSONPost<CrmOperation>(
    `/v1/projects/${project}/locations/${location}/repositories?repositoryId=${encodeURIComponent(repositoryId)}`,
    { format },
  );

// operations.get for the /v1/projects/.../locations/.../operations/{id}
// collection Artifact Registry (and Cloud Run Jobs/Services) LROs live
// under — distinct from the Cloud Resource Manager v3 operations collection
// fetchCrmOperation reads.
export const fetchArOperation = (name: string): Promise<CrmOperation> =>
  authorizedJSON<CrmOperation>(`/v1/${name}`);

// waitArOperation drives a returned Operation to completion the way every
// real client does: poll operations.get until done, then surface the
// operation's own error if it failed.
export const waitArOperation = async (operation: CrmOperation): Promise<CrmOperation> => {
  let current = operation;
  while (!current.done) {
    await new Promise((resolve) => setTimeout(resolve, 500));
    current = await fetchArOperation(current.name);
  }
  if (current.error) {
    throw new Error(current.error.message ?? `operation ${current.name} failed`);
  }
  return current;
};

// projects.locations.repositories.delete — a long-running operation, driven
// through the same operations.get poll (waitArOperation) the create flow uses.
export const deleteARRepository = (project: string, repo: string): Promise<CrmOperation> =>
  authorizedJSONDelete<CrmOperation>(`${repositoriesParent(project)}/${repo}`);

// projects.locations.repositories.patch — UpdateRepository. Unlike create and
// delete (which are long-running operations), UpdateRepository is synchronous:
// the real API (and the simulator) return the updated Repository directly, so
// there is no operation to poll.
export const updateARRepository = (
  project: string,
  repo: string,
  patch: { labels?: Record<string, string>; description?: string },
): Promise<ARRepo> =>
  authorizedJSONPatch<ARRepo>(`${repositoriesParent(project)}/${repo}`, patch);

// projects.locations.repositories.dockerImages.list — the repository's
// stored images, the real console's "Images" tab.
export const fetchARImages = async (project: string, repo: string): Promise<ARDockerImage[]> =>
  (
    await authorizedJSON<{ dockerImages?: ARDockerImage[] }>(`${repositoriesParent(project)}/${repo}/dockerImages`)
  ).dockerImages ?? [];

export const fetchGCSBuckets = async (project: string): Promise<GCSBucket[]> =>
  (await authorizedJSON<{ items?: GCSBucket[] }>(`/storage/v1/b?project=${project}`)).items ?? [];

// storage.buckets.insert — the wire body is just `{ "name": <name> }`; the
// simulator (and real GCS) fill in the rest (id, selfLink, timeCreated,
// location defaulting to "US", storageClass defaulting to "STANDARD").
export const createGCSBucket = (project: string, name: string): Promise<GCSBucket> =>
  authorizedJSONPost<GCSBucket>(`/storage/v1/b?project=${encodeURIComponent(project)}`, { name });

// Bucket names are global in Cloud Storage; the read addresses the bucket
// directly, without a project segment.
export const fetchGCSBucket = (name: string): Promise<GCSBucket> =>
  authorizedJSON<GCSBucket>(`/storage/v1/b/${name}`);

// storage.buckets.delete — bucket names are global, so (like the read) this
// addresses the bucket directly, without a project segment. The real API
// answers 204 No Content, which authorizedJSONDelete surfaces as void.
export const deleteGCSBucket = (name: string): Promise<void> =>
  authorizedJSONDelete<void>(`/storage/v1/b/${name}`);

// storage.buckets.patch — a synchronous update (the real API returns the
// updated bucket, not an operation). Bucket names are global, so (like the
// read) this addresses the bucket directly. `labels` is deep-merged by the
// API — a null value removes a label key — and the default storage class is
// overwritten wholesale.
export const updateGCSBucket = (
  name: string,
  patch: { labels?: Record<string, string | null>; storageClass?: string },
): Promise<GCSBucket> => authorizedJSONPatch<GCSBucket>(`/storage/v1/b/${name}`, patch);

// objects.list — the bucket's stored objects, the real console's "Objects" tab.
export const fetchGCSObjects = async (bucket: string): Promise<GCSObject[]> =>
  (await authorizedJSON<{ items?: GCSObject[] }>(`/storage/v1/b/${bucket}/o`)).items ?? [];

// The real IAM ServiceAccount resource (iam.googleapis.com v1), as the API
// returns it.
export interface ServiceAccount {
  name: string;
  projectId?: string;
  uniqueId?: string;
  email: string;
  displayName?: string;
  description?: string;
  disabled?: boolean;
}

// The real IAM ServiceAccountKey resource. privateKeyData — the base64-encoded
// credential file — is returned by the API on create only, never on get/list.
export interface ServiceAccountKey {
  name: string;
  keyAlgorithm?: string;
  validAfterTime?: string;
  validBeforeTime?: string;
  keyType?: string;
  privateKeyData?: string;
  privateKeyType?: string;
}

const serviceAccountsParent = (project: string) => `/v1/projects/${project}/serviceAccounts`;

export const fetchServiceAccounts = async (project: string): Promise<ServiceAccount[]> =>
  (await authorizedJSON<{ accounts?: ServiceAccount[] }>(serviceAccountsParent(project))).accounts ?? [];

export const fetchServiceAccount = (project: string, email: string): Promise<ServiceAccount> =>
  authorizedJSON<ServiceAccount>(`${serviceAccountsParent(project)}/${email}`);

// serviceAccounts.create — the wire body is { accountId, serviceAccount }.
export const createServiceAccount = (
  project: string,
  accountId: string,
  displayName: string,
  description: string,
): Promise<ServiceAccount> =>
  authorizedJSONPost<ServiceAccount>(serviceAccountsParent(project), {
    accountId,
    serviceAccount: { displayName, description },
  });

export const deleteServiceAccount = (project: string, email: string): Promise<unknown> =>
  authorizedJSONDelete(`${serviceAccountsParent(project)}/${email}`);

export const fetchServiceAccountKeys = async (project: string, email: string): Promise<ServiceAccountKey[]> =>
  (await authorizedJSON<{ keys?: ServiceAccountKey[] }>(`${serviceAccountsParent(project)}/${email}/keys`)).keys ?? [];

// serviceAccounts.keys.create — the one response that carries privateKeyData.
export const createServiceAccountKey = (project: string, email: string): Promise<ServiceAccountKey> =>
  authorizedJSONPost<ServiceAccountKey>(`${serviceAccountsParent(project)}/${email}/keys`, {});

export const deleteServiceAccountKey = (project: string, email: string, keyId: string): Promise<unknown> =>
  authorizedJSONDelete(`${serviceAccountsParent(project)}/${email}/keys/${keyId}`);

// Cloud Logging lists entries by POST, filtered to the project's logs and,
// when given, the operator's own Cloud Logging query-language filter — the
// same `filter` field the real Logs Explorer's query box sends.
export const fetchLogEntries = async (project: string, filter?: string): Promise<LogEntry[]> =>
  (
    await authorizedJSONPost<{ entries?: LogEntry[] }>("/v2/entries:list", {
      resourceNames: [`projects/${project}`],
      orderBy: "timestamp desc",
      pageSize: 100,
      ...(filter ? { filter } : {}),
    })
  ).entries ?? [];

// The Cloud Resource Manager v3 Project resource, as the real API returns it:
// name carries the generated project number ("projects/415104041262"),
// projectId the caller-chosen ID, and state the ACTIVE ⇄ DELETE_REQUESTED
// soft-delete lifecycle.
export interface CrmProject {
  name: string;
  projectId: string;
  state?: "ACTIVE" | "DELETE_REQUESTED";
  displayName?: string;
  parent?: string;
  createTime?: string;
  deleteTime?: string;
  labels?: Record<string, string>;
}

// A google.longrunning.Operation, as projects.create/delete return it.
export interface CrmOperation {
  name: string;
  done?: boolean;
  response?: Record<string, unknown>;
  error?: { code?: number; message?: string };
}

// crmProjectNumber reads the project number out of the v3 resource name.
export const crmProjectNumber = (project: CrmProject): string =>
  project.name.replace(/^projects\//, "");

// projects.search — the read behind the real console's picker and Manage
// resources page: every project the caller can see, without requiring a
// parent. The query is the API's own search syntax (e.g. "state:ACTIVE").
export const searchProjects = async (query?: string): Promise<CrmProject[]> => {
  const path = query ? `/v3/projects:search?query=${encodeURIComponent(query)}` : "/v3/projects:search";
  return (await authorizedJSON<{ projects?: CrmProject[] }>(path)).projects ?? [];
};

// projects.create returns a long-running Operation.
export const createProject = (projectId: string, displayName: string): Promise<CrmOperation> =>
  authorizedJSONPost<CrmOperation>("/v3/projects", { projectId, displayName });

// operations.get — the poll the create dialog runs until the operation is
// done. The operation name is "operations/{id}", addressed under /v3.
export const fetchCrmOperation = (name: string): Promise<CrmOperation> =>
  authorizedJSON<CrmOperation>(`/v3/${name}`);

// projects.delete soft-deletes: the project enters DELETE_REQUESTED (the
// 30-day pending-deletion window) and the API returns an Operation.
export const deleteProject = (projectId: string): Promise<CrmOperation> =>
  authorizedJSONDelete<CrmOperation>(`/v3/projects/${projectId}`);

// waitCrmOperation drives a returned Operation to completion the way every
// real client does: poll operations.get until done, then surface the
// operation's own error if it failed.
export const waitCrmOperation = async (operation: CrmOperation): Promise<CrmOperation> => {
  let current = operation;
  while (!current.done) {
    await new Promise((resolve) => setTimeout(resolve, 500));
    current = await fetchCrmOperation(current.name);
  }
  if (current.error) {
    throw new Error(current.error.message ?? `operation ${current.name} failed`);
  }
  return current;
};
