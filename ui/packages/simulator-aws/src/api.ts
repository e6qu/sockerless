import {
  awsFetch,
  AwsApiError,
  awsJson,
  awsQuery,
  awsRestJson,
  awsRestJsonDelete,
  awsRestXml,
  awsRestXmlDelete,
  awsRestXmlPut,
} from "./console/federation.js";

// restJson issues an AWS REST-JSON request that carries a JSON body and reads a
// JSON (or empty 204) response — the shape Lambda's CreateFunction,
// UpdateFunctionConfiguration, and TagResource operations speak. It signs
// through the same federated awsFetch every other reader uses; only the request
// shaping lives here. A failure carries the protocol's `__type` error code
// (stripped of any `namespace#` prefix, the way SDKs resolve it) so the operator
// reads the real service error, exactly as awsJson does for the awsjson1.1
// services.
async function restJson<T>(
  service: string,
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const hasBody = body !== undefined && method !== "GET" && method !== "HEAD";
  const response = await awsFetch({
    service,
    method,
    path,
    headers: hasBody ? { "content-type": "application/json" } : undefined,
    body: hasBody ? JSON.stringify(body) : undefined,
  });
  const operation = path;
  if (!response.ok) {
    const text = await response.text();
    let type = "";
    let message = "";
    try {
      const parsed = JSON.parse(text) as { __type?: string; message?: string; Message?: string };
      type = parsed.__type ?? "";
      message = parsed.message ?? parsed.Message ?? "";
    } catch {
      // Not the protocol's JSON error shape — the HTTP status is all there is.
    }
    const code = type.slice(type.lastIndexOf("#") + 1);
    throw new AwsApiError(
      code ? `${operation}: ${code}: ${message}` : `${operation} returned HTTP ${response.status}`,
      code,
    );
  }
  const text = await response.text();
  return (text ? JSON.parse(text) : {}) as T;
}

// The console reads the real AWS APIs — ECS, Lambda, ECR, S3, CloudWatch Logs,
// IAM, and AWS Organizations — over federated, SigV4-signed requests, rendering
// the true resource shapes rather than a hand-picked subset.

export type ECSTaskStatus = "PROVISIONING" | "PENDING" | "RUNNING" | "STOPPED" | "DEPROVISIONING";

export interface ECSTask {
  taskArn: string;
  status: ECSTaskStatus;
  clusterArn: string;
  launchType: string;
  cpu: string;
  memory: string;
}

interface DescribeTasksTask {
  taskArn?: string;
  lastStatus?: string;
  clusterArn?: string;
  launchType?: string;
  cpu?: string;
  memory?: string;
}

// ECS tasks live in clusters, and ListTasks is per-cluster, so the console
// enumerates clusters first — the way the real console shows tasks per cluster.
export const fetchECSTasks = async (): Promise<ECSTask[]> => {
  const clusters = await awsJson<{ clusterArns?: string[] }>(
    "ecs",
    "AmazonEC2ContainerServiceV20141113.ListClusters",
    {},
  );
  const tasks: ECSTask[] = [];
  for (const cluster of clusters.clusterArns ?? []) {
    const listed = await awsJson<{ taskArns?: string[] }>("ecs", "AmazonEC2ContainerServiceV20141113.ListTasks", {
      cluster,
    });
    const taskArns = listed.taskArns ?? [];
    if (taskArns.length === 0) continue;
    const described = await awsJson<{ tasks?: DescribeTasksTask[] }>(
      "ecs",
      "AmazonEC2ContainerServiceV20141113.DescribeTasks",
      { cluster, tasks: taskArns },
    );
    for (const task of described.tasks ?? []) {
      tasks.push({
        taskArn: task.taskArn ?? "",
        status: (task.lastStatus ?? "PROVISIONING") as ECSTaskStatus,
        clusterArn: task.clusterArn ?? "",
        launchType: task.launchType ?? "",
        cpu: task.cpu ?? "",
        memory: task.memory ?? "",
      });
    }
  }
  return tasks;
};

// Real ECS never deletes a task record on request — a task is stopped, and
// the service reaps the STOPPED record on its own schedule — so the console's
// task action is StopTask, matching what the real console's "Stop" offers.
export const stopECSTask = async (clusterArn: string, taskArn: string): Promise<void> => {
  await awsJson("ecs", "AmazonEC2ContainerServiceV20141113.StopTask", {
    cluster: clusterArn,
    task: taskArn,
    reason: "Stopped from the AWS console simulator",
  });
};

// ECS task detail — DescribeTasks (the single-task read) and
// DescribeTaskDefinition, the two real ECS operations the task detail page
// reads. A task ARN carries its cluster's short name
// (`arn:aws:ecs:<region>:<account>:task/<cluster>/<task-id>`), which is what
// DescribeTasks accepts for `cluster` — the same short name or full ARN the
// aws CLI and SDKs accept.
function clusterNameFromTaskArn(taskArn: string): string {
  const match = /:task\/([^/]+)\//.exec(taskArn);
  if (!match) throw new Error(`could not read a cluster name from task ARN: ${taskArn}`);
  return match[1];
}

export interface ECSContainerDetail {
  name: string;
  image: string;
  lastStatus: string;
  exitCode?: number;
  privateIpv4Address?: string;
}

export interface ECSAttachmentDetail {
  id: string;
  type: string;
  status: string;
  details: { name: string; value: string }[];
}

export interface ECSTaskDetail {
  taskArn: string;
  taskDefinitionArn: string;
  clusterArn: string;
  status: ECSTaskStatus;
  desiredStatus: string;
  connectivity: string;
  launchType: string;
  cpu: string;
  memory: string;
  group: string;
  createdAt?: number;
  startedAt?: number;
  stoppedAt?: number;
  stopCode: string;
  stoppedReason: string;
  containers: ECSContainerDetail[];
  attachments: ECSAttachmentDetail[];
  networkConfiguration?: { subnets: string[]; securityGroups: string[]; assignPublicIp: string };
}

interface DescribeTasksTaskFull extends DescribeTasksTask {
  taskDefinitionArn?: string;
  desiredStatus?: string;
  connectivity?: string;
  group?: string;
  createdAt?: number;
  startedAt?: number;
  stoppedAt?: number;
  stopCode?: string;
  stoppedReason?: string;
  containers?: {
    name?: string;
    lastStatus?: string;
    exitCode?: number;
    networkInterfaces?: { privateIpv4Address?: string }[];
  }[];
  attachments?: {
    id?: string;
    type?: string;
    status?: string;
    details?: { name?: string; value?: string }[];
  }[];
  networkConfiguration?: {
    awsvpcConfiguration?: { subnets?: string[]; securityGroups?: string[]; assignPublicIp?: string };
  };
}

export const fetchECSTaskDetail = async (taskArn: string): Promise<ECSTaskDetail> => {
  const cluster = clusterNameFromTaskArn(taskArn);
  const described = await awsJson<{ tasks?: DescribeTasksTaskFull[] }>(
    "ecs",
    "AmazonEC2ContainerServiceV20141113.DescribeTasks",
    { cluster, tasks: [taskArn] },
  );
  const task = described.tasks?.[0];
  if (!task) throw new Error(`DescribeTasks returned no task for ${taskArn}`);
  const vpc = task.networkConfiguration?.awsvpcConfiguration;
  return {
    taskArn: task.taskArn ?? taskArn,
    taskDefinitionArn: task.taskDefinitionArn ?? "",
    clusterArn: task.clusterArn ?? "",
    status: (task.lastStatus ?? "PROVISIONING") as ECSTaskStatus,
    desiredStatus: task.desiredStatus ?? "",
    connectivity: task.connectivity ?? "",
    launchType: task.launchType ?? "",
    cpu: task.cpu ?? "",
    memory: task.memory ?? "",
    group: task.group ?? "",
    createdAt: task.createdAt,
    startedAt: task.startedAt,
    stoppedAt: task.stoppedAt,
    stopCode: task.stopCode ?? "",
    stoppedReason: task.stoppedReason ?? "",
    containers: (task.containers ?? []).map((container) => ({
      name: container.name ?? "",
      // DescribeTasks doesn't echo the image — it lives on the task
      // definition's container definitions, joined in by the page.
      image: "",
      lastStatus: container.lastStatus ?? "",
      exitCode: container.exitCode,
      privateIpv4Address: container.networkInterfaces?.[0]?.privateIpv4Address,
    })),
    attachments: (task.attachments ?? []).map((attachment) => ({
      id: attachment.id ?? "",
      type: attachment.type ?? "",
      status: attachment.status ?? "",
      details: (attachment.details ?? []).map((detail) => ({ name: detail.name ?? "", value: detail.value ?? "" })),
    })),
    networkConfiguration: vpc
      ? { subnets: vpc.subnets ?? [], securityGroups: vpc.securityGroups ?? [], assignPublicIp: vpc.assignPublicIp ?? "" }
      : undefined,
  };
};

export interface ECSContainerDefinitionDetail {
  name: string;
  image: string;
  cpu?: number;
  memory?: number;
  memoryReservation?: number;
  essential: boolean;
  environment: { name: string; value: string }[];
  portMappings: { containerPort: number; hostPort?: number; protocol?: string }[];
  entryPoint: string[];
  command: string[];
  logDriver?: string;
}

export interface ECSTaskDefinitionDetail {
  taskDefinitionArn: string;
  family: string;
  revision: number;
  cpu: string;
  memory: string;
  networkMode: string;
  requiresCompatibilities: string[];
  executionRoleArn: string;
  taskRoleArn: string;
  containerDefinitions: ECSContainerDefinitionDetail[];
}

interface TaskDefinitionWire {
  taskDefinitionArn?: string;
  family?: string;
  revision?: number;
  cpu?: string;
  memory?: string;
  networkMode?: string;
  requiresCompatibilities?: string[];
  executionRoleArn?: string;
  taskRoleArn?: string;
  containerDefinitions?: {
    name?: string;
    image?: string;
    cpu?: number;
    memory?: number;
    memoryReservation?: number;
    essential?: boolean;
    environment?: { name?: string; value?: string }[];
    portMappings?: { containerPort?: number; hostPort?: number; protocol?: string }[];
    entryPoint?: string[];
    command?: string[];
    logConfiguration?: { logDriver?: string };
  }[];
}

export const fetchECSTaskDefinition = async (taskDefinitionArn: string): Promise<ECSTaskDefinitionDetail> => {
  const described = await awsJson<{ taskDefinition?: TaskDefinitionWire }>(
    "ecs",
    "AmazonEC2ContainerServiceV20141113.DescribeTaskDefinition",
    { taskDefinition: taskDefinitionArn },
  );
  const td = described.taskDefinition;
  if (!td) throw new Error(`DescribeTaskDefinition returned no taskDefinition for ${taskDefinitionArn}`);
  return {
    taskDefinitionArn: td.taskDefinitionArn ?? taskDefinitionArn,
    family: td.family ?? "",
    revision: td.revision ?? 0,
    cpu: td.cpu ?? "",
    memory: td.memory ?? "",
    networkMode: td.networkMode ?? "",
    requiresCompatibilities: td.requiresCompatibilities ?? [],
    executionRoleArn: td.executionRoleArn ?? "",
    taskRoleArn: td.taskRoleArn ?? "",
    containerDefinitions: (td.containerDefinitions ?? []).map((container) => ({
      name: container.name ?? "",
      image: container.image ?? "",
      cpu: container.cpu,
      memory: container.memory,
      memoryReservation: container.memoryReservation,
      essential: container.essential ?? true,
      environment: (container.environment ?? []).map((entry) => ({ name: entry.name ?? "", value: entry.value ?? "" })),
      portMappings: (container.portMappings ?? []).map((mapping) => ({
        containerPort: mapping.containerPort ?? 0,
        hostPort: mapping.hostPort,
        protocol: mapping.protocol,
      })),
      entryPoint: container.entryPoint ?? [],
      command: container.command ?? [],
      logDriver: container.logConfiguration?.logDriver,
    })),
  };
};

// ECS clusters and task-definition families — ListClusters and
// ListTaskDefinitionFamilies feed the "Run task" form's cluster and task
// definition pickers, the same reads the real console's Run task wizard makes
// to populate its dropdowns.
export const fetchECSClusters = async (): Promise<string[]> => {
  const listed = await awsJson<{ clusterArns?: string[] }>(
    "ecs",
    "AmazonEC2ContainerServiceV20141113.ListClusters",
    {},
  );
  return listed.clusterArns ?? [];
};

export const fetchECSTaskDefinitionFamilies = async (): Promise<string[]> => {
  const listed = await awsJson<{ families?: string[] }>(
    "ecs",
    "AmazonEC2ContainerServiceV20141113.ListTaskDefinitionFamilies",
    { status: "ACTIVE" },
  );
  return listed.families ?? [];
};

// RegisterTaskDefinition — the console registers a minimal single-container
// task definition inline when the operator chooses to define one in the Run
// task form, the same operation the real console's inline task-definition
// authoring drives. Returns the family:revision reference RunTask accepts.
export interface RegisterTaskDefinitionInput {
  family: string;
  image: string;
  cpu: string;
  memory: string;
  launchType: "EC2" | "FARGATE";
}

export const registerECSTaskDefinition = async (input: RegisterTaskDefinitionInput): Promise<string> => {
  const requiresCompatibilities = input.launchType === "FARGATE" ? ["FARGATE"] : ["EC2"];
  const body: Record<string, unknown> = {
    family: input.family,
    requiresCompatibilities,
    networkMode: input.launchType === "FARGATE" ? "awsvpc" : "bridge",
    containerDefinitions: [
      {
        name: input.family,
        image: input.image,
        essential: true,
        ...(input.cpu ? { cpu: Number(input.cpu) } : {}),
        ...(input.memory ? { memory: Number(input.memory) } : {}),
      },
    ],
  };
  // Fargate requires task-level cpu and memory; EC2 does not, so the console
  // sends them only when the operator supplied them.
  if (input.cpu) body.cpu = input.cpu;
  if (input.memory) body.memory = input.memory;
  const registered = await awsJson<{ taskDefinition?: { family?: string; revision?: number } }>(
    "ecs",
    "AmazonEC2ContainerServiceV20141113.RegisterTaskDefinition",
    body,
  );
  const td = registered.taskDefinition;
  if (!td?.family || td.revision === undefined) throw new Error("RegisterTaskDefinition returned no task definition");
  return `${td.family}:${td.revision}`;
};

// RunTask — launches the given task definition on the chosen cluster, the same
// operation the real console's "Run task" wizard drives.
export interface RunTaskInput {
  cluster: string;
  taskDefinition: string;
  launchType: "EC2" | "FARGATE";
  count: number;
}

export const runECSTask = async (input: RunTaskInput): Promise<string[]> => {
  const run = await awsJson<{ tasks?: { taskArn?: string }[] }>(
    "ecs",
    "AmazonEC2ContainerServiceV20141113.RunTask",
    {
      cluster: input.cluster,
      taskDefinition: input.taskDefinition,
      launchType: input.launchType,
      count: input.count,
    },
  );
  return (run.tasks ?? []).map((task) => task.taskArn ?? "");
};

export type LambdaState = "Pending" | "Active" | "Inactive" | "Failed";

export interface LambdaFunction {
  name: string;
  runtime: string;
  state: LambdaState;
  memorySize: number;
  timeout: number;
  lastModified: string;
}

interface LambdaListEntry {
  FunctionName?: string;
  Runtime?: string;
  State?: string;
  MemorySize?: number;
  Timeout?: number;
  LastModified?: string;
}

export const fetchLambdaFunctions = async (): Promise<LambdaFunction[]> => {
  const listed = await awsRestJson<{ Functions?: LambdaListEntry[] }>("lambda", "/2015-03-31/functions");
  return (listed.Functions ?? []).map((fn) => ({
    name: fn.FunctionName ?? "",
    runtime: fn.Runtime ?? "",
    state: (fn.State ?? "Active") as LambdaState,
    memorySize: fn.MemorySize ?? 0,
    timeout: fn.Timeout ?? 0,
    lastModified: fn.LastModified ?? "",
  }));
};

export const deleteLambdaFunction = async (functionName: string): Promise<void> => {
  await awsRestJsonDelete("lambda", `/2015-03-31/functions/${encodeURIComponent(functionName)}`);
};

// Lambda function detail — GetFunction, which answers Configuration, Code,
// and Tags in one read (there is no separate GetFunctionConfiguration call
// this page needs: GetFunction already carries every configuration field).

export interface LambdaFunctionDetail {
  name: string;
  arn: string;
  runtime: string;
  role: string;
  handler: string;
  codeSha256: string;
  codeSize: number;
  description: string;
  memorySize: number;
  timeout: number;
  environment: { name: string; value: string }[];
  state: LambdaState;
  lastUpdateStatus: string;
  lastModified: string;
  revisionId: string;
  version: string;
  packageType: string;
  architectures: string[];
  vpcConfig?: { subnetIds: string[]; securityGroupIds: string[]; vpcId: string };
}

export interface LambdaFunctionCode {
  location: string;
  repositoryType: string;
  imageUri?: string;
  resolvedImageUri?: string;
}

interface LambdaGetFunctionResponse {
  Configuration?: {
    FunctionName?: string;
    FunctionArn?: string;
    Runtime?: string;
    Role?: string;
    Handler?: string;
    CodeSha256?: string;
    CodeSize?: number;
    Description?: string;
    MemorySize?: number;
    Timeout?: number;
    Environment?: { Variables?: Record<string, string> };
    State?: string;
    LastUpdateStatus?: string;
    LastModified?: string;
    RevisionId?: string;
    Version?: string;
    PackageType?: string;
    Architectures?: string[];
    VpcConfig?: { SubnetIds?: string[]; SecurityGroupIds?: string[]; VpcId?: string };
  };
  Code?: { Location?: string; RepositoryType?: string; ImageUri?: string; ResolvedImageUri?: string };
  Tags?: Record<string, string>;
}

export const fetchLambdaFunctionDetail = async (
  functionName: string,
): Promise<{ configuration: LambdaFunctionDetail; code: LambdaFunctionCode; tags: Record<string, string> }> => {
  const response = await awsRestJson<LambdaGetFunctionResponse>(
    "lambda",
    `/2015-03-31/functions/${encodeURIComponent(functionName)}`,
  );
  const cfg = response.Configuration ?? {};
  const variables = cfg.Environment?.Variables ?? {};
  return {
    configuration: {
      name: cfg.FunctionName ?? functionName,
      arn: cfg.FunctionArn ?? "",
      runtime: cfg.Runtime ?? "",
      role: cfg.Role ?? "",
      handler: cfg.Handler ?? "",
      codeSha256: cfg.CodeSha256 ?? "",
      codeSize: cfg.CodeSize ?? 0,
      description: cfg.Description ?? "",
      memorySize: cfg.MemorySize ?? 0,
      timeout: cfg.Timeout ?? 0,
      environment: Object.entries(variables).map(([name, value]) => ({ name, value })),
      state: (cfg.State ?? "Active") as LambdaState,
      lastUpdateStatus: cfg.LastUpdateStatus ?? "",
      lastModified: cfg.LastModified ?? "",
      revisionId: cfg.RevisionId ?? "",
      version: cfg.Version ?? "",
      packageType: cfg.PackageType ?? "",
      architectures: cfg.Architectures ?? [],
      vpcConfig: cfg.VpcConfig
        ? {
            subnetIds: cfg.VpcConfig.SubnetIds ?? [],
            securityGroupIds: cfg.VpcConfig.SecurityGroupIds ?? [],
            vpcId: cfg.VpcConfig.VpcId ?? "",
          }
        : undefined,
    },
    code: {
      location: response.Code?.Location ?? "",
      repositoryType: response.Code?.RepositoryType ?? "",
      imageUri: response.Code?.ImageUri,
      resolvedImageUri: response.Code?.ResolvedImageUri,
    },
    tags: response.Tags ?? {},
  };
};

// UpdateFunctionConfiguration — PUT /2015-03-31/functions/{name}/configuration,
// the real Lambda operation the console's "Edit configuration" flow drives.
// The console sends only the fields the operator edited (memory, timeout,
// description, environment); real Lambda leaves any omitted field unchanged,
// so the caller passes exactly what changed.
export interface LambdaConfigurationUpdate {
  memorySize?: number;
  timeout?: number;
  description?: string;
  environment?: { name: string; value: string }[];
}

export const updateLambdaFunctionConfiguration = async (
  functionName: string,
  update: LambdaConfigurationUpdate,
): Promise<void> => {
  const body: Record<string, unknown> = {};
  if (update.memorySize !== undefined) body.MemorySize = update.memorySize;
  if (update.timeout !== undefined) body.Timeout = update.timeout;
  if (update.description !== undefined) body.Description = update.description;
  if (update.environment !== undefined) {
    body.Environment = {
      Variables: Object.fromEntries(update.environment.map((entry) => [entry.name, entry.value])),
    };
  }
  await restJson(
    "lambda",
    "PUT",
    `/2015-03-31/functions/${encodeURIComponent(functionName)}/configuration`,
    body,
  );
};

// CreateFunction — POST /2015-03-31/functions. The console offers the two
// package types real Lambda accepts: a container image (Code.ImageUri, the
// simplest faithful path) or a Zip archive staged in Amazon S3 (Code.S3Bucket
// / Code.S3Key with a runtime and handler), matching what the real console's
// "Create function" wizard collects.
export interface CreateLambdaFunctionInput {
  functionName: string;
  role: string;
  memorySize: number;
  timeout: number;
  description?: string;
  packageType: "Image" | "Zip";
  imageUri?: string;
  runtime?: string;
  handler?: string;
  s3Bucket?: string;
  s3Key?: string;
}

export const createLambdaFunction = async (input: CreateLambdaFunctionInput): Promise<void> => {
  const body: Record<string, unknown> = {
    FunctionName: input.functionName,
    Role: input.role,
    MemorySize: input.memorySize,
    Timeout: input.timeout,
    PackageType: input.packageType,
  };
  if (input.description) body.Description = input.description;
  if (input.packageType === "Image") {
    body.Code = { ImageUri: input.imageUri };
  } else {
    body.Runtime = input.runtime;
    body.Handler = input.handler;
    body.Code = { S3Bucket: input.s3Bucket, S3Key: input.s3Key };
  }
  await restJson("lambda", "POST", "/2015-03-31/functions", body);
};

export interface LambdaInvokeResult {
  payload: string;
  functionError: string;
}

// Invoke — POST /2015-03-31/functions/{name}/invocations. A RequestResponse
// invocation returns the function's raw response payload; real Lambda reports a
// handler error out-of-band in the X-Amz-Function-Error response header (the
// payload then carries the error document), which the console surfaces exactly
// as the real console's "Test" tab does.
export const invokeLambdaFunction = async (
  functionName: string,
  payload: string,
): Promise<LambdaInvokeResult> => {
  const response = await awsFetch({
    service: "lambda",
    method: "POST",
    path: `/2015-03-31/functions/${encodeURIComponent(functionName)}/invocations`,
    headers: { "content-type": "application/json" },
    body: payload,
  });
  const text = await response.text();
  if (!response.ok) {
    let type = "";
    let message = "";
    try {
      const parsed = JSON.parse(text) as { __type?: string; message?: string; Message?: string };
      type = parsed.__type ?? "";
      message = parsed.message ?? parsed.Message ?? "";
    } catch {
      // Not the protocol's JSON error shape — the HTTP status is all there is.
    }
    const code = type.slice(type.lastIndexOf("#") + 1);
    throw new AwsApiError(
      code ? `Invoke: ${code}: ${message}` : `Invoke returned HTTP ${response.status}`,
      code,
    );
  }
  return { payload: text, functionError: response.headers.get("X-Amz-Function-Error") ?? "" };
};

// Lambda resource tagging — TagResource (POST /2017-03-31/tags/{arn}) sets or
// updates the given tags; UntagResource (DELETE …?tagKeys=…) removes keys. The
// current tags are read from GetFunction (fetchLambdaFunctionDetail), so the
// console diffs the operator's edits against them to make exactly these two
// calls, the way the real console's Tags editor does.
export const tagLambdaResource = async (arn: string, tags: Record<string, string>): Promise<void> => {
  await restJson("lambda", "POST", `/2017-03-31/tags/${encodeURIComponent(arn)}`, { Tags: tags });
};

export const untagLambdaResource = async (arn: string, tagKeys: string[]): Promise<void> => {
  const query = tagKeys.map((key) => `tagKeys=${encodeURIComponent(key)}`).join("&");
  await restJson("lambda", "DELETE", `/2017-03-31/tags/${encodeURIComponent(arn)}?${query}`);
};

export interface ECRRepo {
  name: string;
  arn: string;
  uri: string;
  createdAt: number;
  imageTagMutability: string;
  scanOnPush: boolean;
}

interface ECRRepository {
  repositoryName?: string;
  repositoryArn?: string;
  repositoryUri?: string;
  createdAt?: number;
  imageTagMutability?: string;
  imageScanningConfiguration?: { scanOnPush?: boolean };
}

const ecrRepoFromWire = (repo: ECRRepository, fallbackName: string): ECRRepo => ({
  name: repo.repositoryName ?? fallbackName,
  arn: repo.repositoryArn ?? "",
  uri: repo.repositoryUri ?? "",
  createdAt: repo.createdAt ?? 0,
  imageTagMutability: repo.imageTagMutability ?? "MUTABLE",
  scanOnPush: repo.imageScanningConfiguration?.scanOnPush ?? false,
});

export const fetchECRRepos = async (): Promise<ECRRepo[]> => {
  const described = await awsJson<{ repositories?: ECRRepository[] }>(
    "ecr",
    "AmazonEC2ContainerRegistry_V20150921.DescribeRepositories",
    {},
  );
  return (described.repositories ?? []).map((repo) => ecrRepoFromWire(repo, ""));
};

// CreateRepository — the real console's "Create repository" dialog collects
// just the name (image tag mutability, scanning, and encryption all keep
// ECR's own defaults, the same as a bare `aws ecr create-repository`).
export const createECRRepository = async (repositoryName: string): Promise<ECRRepo> => {
  const created = await awsJson<{ repository?: ECRRepository }>(
    "ecr",
    "AmazonEC2ContainerRegistry_V20150921.CreateRepository",
    { repositoryName },
  );
  const repo = created.repository;
  if (!repo) throw new Error("CreateRepository returned no repository");
  return ecrRepoFromWire(repo, repositoryName);
};

// DeleteRepository with force: true — the real console's delete dialog warns
// that a non-empty repository's images are deleted along with it and asks the
// operator to confirm by typing the repository name, then sends force so the
// non-empty case succeeds rather than answering RepositoryNotEmptyException.
export const deleteECRRepo = async (repositoryName: string): Promise<void> => {
  await awsJson("ecr", "AmazonEC2ContainerRegistry_V20150921.DeleteRepository", {
    repositoryName,
    force: true,
  });
};

// ECR repository detail — DescribeRepositories filtered to one name (there is
// no singular DescribeRepository operation), and DescribeImages for the image
// list, the same two calls the real console's repository page reads.
export const fetchECRRepo = async (repositoryName: string): Promise<ECRRepo> => {
  const described = await awsJson<{ repositories?: ECRRepository[] }>(
    "ecr",
    "AmazonEC2ContainerRegistry_V20150921.DescribeRepositories",
    { repositoryNames: [repositoryName] },
  );
  const repo = described.repositories?.[0];
  if (!repo) throw new Error(`DescribeRepositories returned no repository for ${repositoryName}`);
  return ecrRepoFromWire(repo, repositoryName);
};

// PutImageTagMutability and PutImageScanningConfiguration — the two real ECR
// operations the console's "Edit" flow drives for a repository's registry
// settings, matching what the real console's repository settings page changes.
export const putECRImageTagMutability = async (
  repositoryName: string,
  imageTagMutability: "MUTABLE" | "IMMUTABLE",
): Promise<void> => {
  await awsJson("ecr", "AmazonEC2ContainerRegistry_V20150921.PutImageTagMutability", {
    repositoryName,
    imageTagMutability,
  });
};

export const putECRImageScanningConfiguration = async (
  repositoryName: string,
  scanOnPush: boolean,
): Promise<void> => {
  await awsJson("ecr", "AmazonEC2ContainerRegistry_V20150921.PutImageScanningConfiguration", {
    repositoryName,
    imageScanningConfiguration: { scanOnPush },
  });
};

// ECR resource tagging — ListTagsForResource reads the current tags,
// TagResource sets/updates them, and UntagResource removes keys, all keyed by
// the repository ARN, the same three operations the real console's Tags tab
// drives.
export const fetchECRTags = async (resourceArn: string): Promise<Record<string, string>> => {
  const listed = await awsJson<{ tags?: { Key?: string; Value?: string }[] }>(
    "ecr",
    "AmazonEC2ContainerRegistry_V20150921.ListTagsForResource",
    { resourceArn },
  );
  return Object.fromEntries((listed.tags ?? []).map((tag) => [tag.Key ?? "", tag.Value ?? ""]));
};

export const tagECRResource = async (resourceArn: string, tags: Record<string, string>): Promise<void> => {
  await awsJson("ecr", "AmazonEC2ContainerRegistry_V20150921.TagResource", {
    resourceArn,
    tags: Object.entries(tags).map(([Key, Value]) => ({ Key, Value })),
  });
};

export const untagECRResource = async (resourceArn: string, tagKeys: string[]): Promise<void> => {
  await awsJson("ecr", "AmazonEC2ContainerRegistry_V20150921.UntagResource", { resourceArn, tagKeys });
};

export interface ECRImage {
  digest: string;
  tags: string[];
  sizeBytes: number;
  pushedAt: number;
}

interface ECRImageDetailEntry {
  imageDigest?: string;
  imageTags?: string[];
  imageSizeInBytes?: number;
  imagePushedAt?: number;
}

export const fetchECRImages = async (repositoryName: string): Promise<ECRImage[]> => {
  const described = await awsJson<{ imageDetails?: ECRImageDetailEntry[] }>(
    "ecr",
    "AmazonEC2ContainerRegistry_V20150921.DescribeImages",
    { repositoryName },
  );
  return (described.imageDetails ?? []).map((image) => ({
    digest: image.imageDigest ?? "",
    tags: image.imageTags ?? [],
    sizeBytes: image.imageSizeInBytes ?? 0,
    pushedAt: image.imagePushedAt ?? 0,
  }));
};

export interface S3Bucket {
  name: string;
  creationDate: string;
}

export const fetchS3Buckets = async (): Promise<S3Bucket[]> => {
  const xml = await awsRestXml("s3", "/");
  return Array.from(xml.getElementsByTagName("Bucket")).map((bucket) => ({
    name: bucket.getElementsByTagName("Name")[0]?.textContent ?? "",
    creationDate: bucket.getElementsByTagName("CreationDate")[0]?.textContent ?? "",
  }));
};

// CreateBucket — PUT /{bucket} with an empty body, the same request the real
// console's "Create bucket" issues for a bucket in this console's own Region
// (us-east-1 is the one Region CreateBucket rejects a LocationConstraint
// for, so the console sends none rather than special-casing it).
export const createS3Bucket = async (bucketName: string): Promise<void> => {
  await awsRestXmlPut("s3", `/${encodeURIComponent(bucketName)}`);
};

// DeleteBucket only succeeds on an empty bucket — the same constraint the
// real console enforces, surfacing S3's BucketNotEmpty error rather than
// emptying the bucket on the operator's behalf.
export const deleteS3Bucket = async (bucketName: string): Promise<void> => {
  await awsRestXmlDelete("s3", `/${encodeURIComponent(bucketName)}`);
};

// S3 bucket detail — GetBucketLocation for the Region, and ListObjectsV2
// (`?list-type=2`) for the object listing, the same sub-resource GET
// requests the real console's bucket page issues. S3 has no per-bucket
// "properties" read for the creation date; the real console reads it from
// the same ListBuckets response the buckets page already fetches, so the
// detail page's caller does the same rather than inventing a value.
export const fetchS3BucketLocation = async (bucketName: string): Promise<string> => {
  const xml = await awsRestXml("s3", `/${encodeURIComponent(bucketName)}?location`);
  const constraint = xml.documentElement?.textContent?.trim();
  // An empty LocationConstraint means us-east-1 — the one real S3 Region that
  // reports as empty rather than naming itself.
  return constraint || "us-east-1";
};

export interface S3Object {
  key: string;
  size: number;
  lastModified: string;
  etag: string;
}

export const fetchS3Objects = async (bucketName: string): Promise<S3Object[]> => {
  const xml = await awsRestXml("s3", `/${encodeURIComponent(bucketName)}?list-type=2`);
  return Array.from(xml.getElementsByTagName("Contents")).map((entry) => ({
    key: entry.getElementsByTagName("Key")[0]?.textContent ?? "",
    size: Number(entry.getElementsByTagName("Size")[0]?.textContent ?? "0"),
    lastModified: entry.getElementsByTagName("LastModified")[0]?.textContent ?? "",
    etag: (entry.getElementsByTagName("ETag")[0]?.textContent ?? "").replace(/^"|"$/g, ""),
  }));
};

const S3_XMLNS = "http://s3.amazonaws.com/doc/2006-03-01/";

// GetBucketVersioning — GET /{bucket}?versioning. An unversioned bucket answers
// an empty `<VersioningConfiguration/>` (no Status element), the same shape the
// real console reads before it shows the versioning toggle as off.
export type S3VersioningStatus = "Enabled" | "Suspended" | "Disabled";

export const fetchS3BucketVersioning = async (bucketName: string): Promise<S3VersioningStatus> => {
  const xml = await awsRestXml("s3", `/${encodeURIComponent(bucketName)}?versioning`);
  const status = xml.getElementsByTagName("Status")[0]?.textContent?.trim();
  return status === "Enabled" || status === "Suspended" ? status : "Disabled";
};

// PutBucketVersioning — PUT /{bucket}?versioning. Real S3 never removes
// versioning once enabled: it is Enabled or Suspended, so the console's toggle
// sends exactly those two, matching the real console.
export const putS3BucketVersioning = async (
  bucketName: string,
  status: "Enabled" | "Suspended",
): Promise<void> => {
  const body = `<VersioningConfiguration xmlns="${S3_XMLNS}"><Status>${status}</Status></VersioningConfiguration>`;
  await awsRestXmlPut("s3", `/${encodeURIComponent(bucketName)}?versioning`, body);
};

// GetBucketTagging — GET /{bucket}?tagging. A bucket with no tag set answers
// S3's NoSuchTagSet error rather than an empty document; the real console reads
// that as "no tags", so the console does the same rather than surfacing it as a
// load failure.
export const fetchS3BucketTagging = async (bucketName: string): Promise<Record<string, string>> => {
  const response = await awsFetch({ service: "s3", method: "GET", path: `/${encodeURIComponent(bucketName)}?tagging` });
  const text = await response.text();
  const xml = new DOMParser().parseFromString(text, "text/xml");
  if (!response.ok) {
    const code = xml.getElementsByTagName("Code")[0]?.textContent ?? "";
    if (code === "NoSuchTagSet") return {};
    const message = xml.getElementsByTagName("Message")[0]?.textContent ?? "";
    throw new Error(code ? `${code}: ${message}` : `GET /${bucketName}?tagging returned HTTP ${response.status}`);
  }
  const tags: Record<string, string> = {};
  for (const tag of Array.from(xml.getElementsByTagName("Tag"))) {
    const key = tag.getElementsByTagName("Key")[0]?.textContent ?? "";
    if (key) tags[key] = tag.getElementsByTagName("Value")[0]?.textContent ?? "";
  }
  return tags;
};

// PutBucketTagging replaces the whole tag set (S3 tagging has no per-key add);
// clearing every tag is DeleteBucketTagging, the same two operations the real
// console's bucket Tags editor drives.
export const putS3BucketTagging = async (bucketName: string, tags: Record<string, string>): Promise<void> => {
  const entries = Object.entries(tags);
  if (entries.length === 0) {
    await awsRestXmlDelete("s3", `/${encodeURIComponent(bucketName)}?tagging`);
    return;
  }
  const tagXml = entries
    .map(([key, value]) => `<Tag><Key>${escapeXml(key)}</Key><Value>${escapeXml(value)}</Value></Tag>`)
    .join("");
  const body = `<Tagging xmlns="${S3_XMLNS}"><TagSet>${tagXml}</TagSet></Tagging>`;
  await awsRestXmlPut("s3", `/${encodeURIComponent(bucketName)}?tagging`, body);
};

function escapeXml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");
}

export interface CWLogGroup {
  name: string;
  creationTime: number;
  retentionInDays: number;
  storedBytes: number;
}

interface CWLogGroupEntry {
  logGroupName?: string;
  creationTime?: number;
  retentionInDays?: number;
  storedBytes?: number;
}

// AWS Identity and Access Management (IAM) — the query protocol (Action +
// Version=2010-05-08, form-encoded, XML responses), the same wire the aws CLI
// signs. The console drives it with the operator's federated credentials, so
// minting a user and an access key here is the same call an administrator's
// CLI would make.

const IAM_VERSION = "2010-05-08";

const elementText = (parent: Element | Document, tag: string): string =>
  parent.getElementsByTagName(tag)[0]?.textContent ?? "";

export interface IAMUserSummary {
  userName: string;
  userId: string;
  arn: string;
  path: string;
  createDate: string;
}

const iamUserFromElement = (element: Element): IAMUserSummary => ({
  userName: elementText(element, "UserName"),
  userId: elementText(element, "UserId"),
  arn: elementText(element, "Arn"),
  path: elementText(element, "Path"),
  createDate: elementText(element, "CreateDate"),
});

export const fetchIAMUsers = async (): Promise<IAMUserSummary[]> => {
  const xml = await awsQuery("iam", IAM_VERSION, "ListUsers");
  return Array.from(xml.getElementsByTagName("member")).map(iamUserFromElement);
};

export const fetchIAMUser = async (userName: string): Promise<IAMUserSummary> => {
  const xml = await awsQuery("iam", IAM_VERSION, "GetUser", { UserName: userName });
  const user = xml.getElementsByTagName("User")[0];
  if (!user) throw new Error(`GetUser returned no User element for ${userName}`);
  return iamUserFromElement(user);
};

export const createIAMUser = async (userName: string): Promise<void> => {
  await awsQuery("iam", IAM_VERSION, "CreateUser", { UserName: userName });
};

export const deleteIAMUser = async (userName: string): Promise<void> => {
  await awsQuery("iam", IAM_VERSION, "DeleteUser", { UserName: userName });
};

export interface IAMAccessKeyMetadata {
  accessKeyId: string;
  status: string;
  createDate: string;
}

// ListAccessKeys returns metadata only — never the secret. The secret exists
// exactly once, in the CreateAccessKey response.
export const fetchIAMAccessKeys = async (userName: string): Promise<IAMAccessKeyMetadata[]> => {
  const xml = await awsQuery("iam", IAM_VERSION, "ListAccessKeys", { UserName: userName });
  return Array.from(xml.getElementsByTagName("member")).map((member) => ({
    accessKeyId: elementText(member, "AccessKeyId"),
    status: elementText(member, "Status"),
    createDate: elementText(member, "CreateDate"),
  }));
};

export interface IAMCreatedAccessKey {
  accessKeyId: string;
  secretAccessKey: string;
}

export const createIAMAccessKey = async (userName: string): Promise<IAMCreatedAccessKey> => {
  const xml = await awsQuery("iam", IAM_VERSION, "CreateAccessKey", { UserName: userName });
  const created = {
    accessKeyId: elementText(xml, "AccessKeyId"),
    secretAccessKey: elementText(xml, "SecretAccessKey"),
  };
  if (!created.accessKeyId || !created.secretAccessKey) {
    throw new Error("CreateAccessKey returned no credential material");
  }
  return created;
};

export const deleteIAMAccessKey = async (userName: string, accessKeyId: string): Promise<void> => {
  await awsQuery("iam", IAM_VERSION, "DeleteAccessKey", { UserName: userName, AccessKeyId: accessKeyId });
};

export const updateIAMAccessKeyStatus = async (
  userName: string,
  accessKeyId: string,
  status: "Active" | "Inactive",
): Promise<void> => {
  await awsQuery("iam", IAM_VERSION, "UpdateAccessKey", {
    UserName: userName,
    AccessKeyId: accessKeyId,
    Status: status,
  });
};

// AWS Organizations — awsjson1.1, X-Amz-Target AWSOrganizationsV20161128.<Op>,
// the wire the aws CLI and SDKs sign for `aws organizations …`. The console's
// AWS accounts page is a plain Organizations client: ListAccounts for the
// table, the async CreateAccount → DescribeCreateAccountStatus request flow,
// and RemoveAccountFromOrganization / CloseAccount for the account actions the
// real console offers.

const ORG_TARGET_PREFIX = "AWSOrganizationsV20161128.";

const orgJson = <T>(operation: string, input: Record<string, unknown> = {}): Promise<T> =>
  awsJson<T>("organizations", `${ORG_TARGET_PREFIX}${operation}`, input);

export interface OrgAccount {
  id: string;
  arn: string;
  name: string;
  email: string;
  status: string;
  joinedMethod: string;
  joinedTimestamp: number;
}

interface OrgAccountEntry {
  Id?: string;
  Arn?: string;
  Name?: string;
  Email?: string;
  Status?: string;
  JoinedMethod?: string;
  JoinedTimestamp?: number;
}

const orgAccountFromEntry = (entry: OrgAccountEntry): OrgAccount => ({
  id: entry.Id ?? "",
  arn: entry.Arn ?? "",
  name: entry.Name ?? "",
  email: entry.Email ?? "",
  status: entry.Status ?? "",
  joinedMethod: entry.JoinedMethod ?? "",
  joinedTimestamp: entry.JoinedTimestamp ?? 0,
});

export const fetchOrgAccounts = async (): Promise<OrgAccount[]> => {
  const listed = await orgJson<{ Accounts?: OrgAccountEntry[] }>("ListAccounts");
  return (listed.Accounts ?? []).map(orgAccountFromEntry);
};

export const fetchOrgAccount = async (accountId: string): Promise<OrgAccount> => {
  const described = await orgJson<{ Account?: OrgAccountEntry }>("DescribeAccount", { AccountId: accountId });
  if (!described.Account) throw new Error(`DescribeAccount returned no Account for ${accountId}`);
  return orgAccountFromEntry(described.Account);
};

export type OrgCreateAccountState = "IN_PROGRESS" | "SUCCEEDED" | "FAILED";

export interface OrgCreateAccountStatus {
  id: string;
  accountName: string;
  state: OrgCreateAccountState;
  requestedTimestamp: number;
  completedTimestamp: number;
  accountId: string;
  failureReason: string;
}

interface OrgCreateAccountStatusEntry {
  Id?: string;
  AccountName?: string;
  State?: string;
  RequestedTimestamp?: number;
  CompletedTimestamp?: number;
  AccountId?: string;
  FailureReason?: string;
}

const orgCreateStatusFromEntry = (entry: OrgCreateAccountStatusEntry): OrgCreateAccountStatus => ({
  id: entry.Id ?? "",
  accountName: entry.AccountName ?? "",
  state: (entry.State ?? "IN_PROGRESS") as OrgCreateAccountState,
  requestedTimestamp: entry.RequestedTimestamp ?? 0,
  completedTimestamp: entry.CompletedTimestamp ?? 0,
  accountId: entry.AccountId ?? "",
  failureReason: entry.FailureReason ?? "",
});

export const createOrgAccount = async (accountName: string, email: string): Promise<OrgCreateAccountStatus> => {
  const created = await orgJson<{ CreateAccountStatus?: OrgCreateAccountStatusEntry }>("CreateAccount", {
    AccountName: accountName,
    Email: email,
  });
  if (!created.CreateAccountStatus) throw new Error("CreateAccount returned no CreateAccountStatus");
  return orgCreateStatusFromEntry(created.CreateAccountStatus);
};

export const fetchOrgCreateAccountStatus = async (requestId: string): Promise<OrgCreateAccountStatus> => {
  const described = await orgJson<{ CreateAccountStatus?: OrgCreateAccountStatusEntry }>(
    "DescribeCreateAccountStatus",
    { CreateAccountRequestId: requestId },
  );
  if (!described.CreateAccountStatus) {
    throw new Error(`DescribeCreateAccountStatus returned no CreateAccountStatus for ${requestId}`);
  }
  return orgCreateStatusFromEntry(described.CreateAccountStatus);
};

export const fetchOrgCreateAccountStatuses = async (): Promise<OrgCreateAccountStatus[]> => {
  const listed = await orgJson<{ CreateAccountStatuses?: OrgCreateAccountStatusEntry[] }>("ListCreateAccountStatus");
  return (listed.CreateAccountStatuses ?? []).map(orgCreateStatusFromEntry);
};

export const createOrganization = async (): Promise<void> => {
  await orgJson("CreateOrganization", { FeatureSet: "ALL" });
};

export const removeOrgAccount = async (accountId: string): Promise<void> => {
  await orgJson("RemoveAccountFromOrganization", { AccountId: accountId });
};

export const closeOrgAccount = async (accountId: string): Promise<void> => {
  await orgJson("CloseAccount", { AccountId: accountId });
};

export const fetchCWLogGroups = async (): Promise<CWLogGroup[]> => {
  const described = await awsJson<{ logGroups?: CWLogGroupEntry[] }>("logs", "Logs_20140328.DescribeLogGroups", {});
  return (described.logGroups ?? []).map((group) => ({
    name: group.logGroupName ?? "",
    creationTime: group.creationTime ?? 0,
    retentionInDays: group.retentionInDays ?? 0,
    storedBytes: group.storedBytes ?? 0,
  }));
};

// CreateLogGroup — the real console's "Create log group" dialog collects
// just the name; retention and KMS key both keep CloudWatch Logs' own
// defaults (never expire, service-managed encryption).
export const createCWLogGroup = async (logGroupName: string): Promise<void> => {
  await awsJson("logs", "Logs_20140328.CreateLogGroup", { logGroupName });
};

export const deleteCWLogGroup = async (logGroupName: string): Promise<void> => {
  await awsJson("logs", "Logs_20140328.DeleteLogGroup", { logGroupName });
};

// Retention — PutRetentionPolicy sets a fixed retention in days;
// DeleteRetentionPolicy clears it so events never expire. CloudWatch has no
// "0 days" retention, so "Never expire" is the delete, matching what the real
// console's retention editor does.
export const putCWLogGroupRetention = async (
  logGroupName: string,
  retentionInDays: number,
): Promise<void> => {
  if (retentionInDays <= 0) {
    await awsJson("logs", "Logs_20140328.DeleteRetentionPolicy", { logGroupName });
    return;
  }
  await awsJson("logs", "Logs_20140328.PutRetentionPolicy", { logGroupName, retentionInDays });
};

// CloudWatch Logs log group detail — DescribeLogGroups has no singular
// "GetLogGroup" counterpart, so the detail page reads it the way the real
// console does: `logGroupNamePrefix` narrowed to an exact match.
export const fetchCWLogGroup = async (logGroupName: string): Promise<CWLogGroup> => {
  const described = await awsJson<{ logGroups?: CWLogGroupEntry[] }>("logs", "Logs_20140328.DescribeLogGroups", {
    logGroupNamePrefix: logGroupName,
  });
  const group = (described.logGroups ?? []).find((entry) => entry.logGroupName === logGroupName);
  if (!group) throw new Error(`DescribeLogGroups returned no log group named ${logGroupName}`);
  return {
    name: group.logGroupName ?? logGroupName,
    creationTime: group.creationTime ?? 0,
    retentionInDays: group.retentionInDays ?? 0,
    storedBytes: group.storedBytes ?? 0,
  };
};

export interface CWLogStream {
  name: string;
  creationTime: number;
  firstEventTimestamp: number;
  lastEventTimestamp: number;
  lastIngestionTime: number;
}

interface CWLogStreamEntry {
  logStreamName?: string;
  creationTime?: number;
  firstEventTimestamp?: number;
  lastEventTimestamp?: number;
  lastIngestionTime?: number;
}

export const fetchCWLogStreams = async (logGroupName: string): Promise<CWLogStream[]> => {
  const described = await awsJson<{ logStreams?: CWLogStreamEntry[] }>("logs", "Logs_20140328.DescribeLogStreams", {
    logGroupName,
    orderBy: "LastEventTime",
    descending: true,
  });
  return (described.logStreams ?? []).map((stream) => ({
    name: stream.logStreamName ?? "",
    creationTime: stream.creationTime ?? 0,
    firstEventTimestamp: stream.firstEventTimestamp ?? 0,
    lastEventTimestamp: stream.lastEventTimestamp ?? 0,
    lastIngestionTime: stream.lastIngestionTime ?? 0,
  }));
};

export interface CWLogEvent {
  timestamp: number;
  message: string;
}

// GetLogEvents with startFromHead: false (the documented default) returns the
// tail of the stream — the most recent events, the same window the real
// console's log-stream viewer opens on.
export const fetchCWLogEvents = async (logGroupName: string, logStreamName: string): Promise<CWLogEvent[]> => {
  const described = await awsJson<{ events?: { timestamp?: number; message?: string }[] }>(
    "logs",
    "Logs_20140328.GetLogEvents",
    { logGroupName, logStreamName, limit: 100, startFromHead: false },
  );
  return (described.events ?? []).map((event) => ({ timestamp: event.timestamp ?? 0, message: event.message ?? "" }));
};
