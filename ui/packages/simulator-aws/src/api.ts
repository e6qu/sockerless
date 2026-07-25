import { awsJson, awsQuery, awsRestJson, awsRestXml } from "./console/federation.js";

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
