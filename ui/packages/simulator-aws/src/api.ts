import {
  awsJson,
  awsQuery,
  awsRestJson,
  awsRestJsonDelete,
  awsRestXml,
  awsRestXmlDelete,
  awsRestXmlPut,
} from "./console/federation.js";

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

export interface ECRRepo {
  name: string;
  uri: string;
  createdAt: number;
}

interface ECRRepository {
  repositoryName?: string;
  repositoryUri?: string;
  createdAt?: number;
}

export const fetchECRRepos = async (): Promise<ECRRepo[]> => {
  const described = await awsJson<{ repositories?: ECRRepository[] }>(
    "ecr",
    "AmazonEC2ContainerRegistry_V20150921.DescribeRepositories",
    {},
  );
  return (described.repositories ?? []).map((repo) => ({
    name: repo.repositoryName ?? "",
    uri: repo.repositoryUri ?? "",
    createdAt: repo.createdAt ?? 0,
  }));
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
  return { name: repo.repositoryName ?? repositoryName, uri: repo.repositoryUri ?? "", createdAt: repo.createdAt ?? 0 };
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
  return { name: repo.repositoryName ?? repositoryName, uri: repo.repositoryUri ?? "", createdAt: repo.createdAt ?? 0 };
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
