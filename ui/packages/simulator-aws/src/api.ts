import { awsJson, awsRestJson, awsRestXml } from "./console/federation.js";

// The console reads the real AWS APIs — ECS, Lambda, ECR, S3, and CloudWatch
// Logs — over federated, SigV4-signed requests, rendering the true resource
// shapes rather than a hand-picked subset.

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

export const fetchCWLogGroups = async (): Promise<CWLogGroup[]> => {
  const described = await awsJson<{ logGroups?: CWLogGroupEntry[] }>("logs", "Logs_20140328.DescribeLogGroups", {});
  return (described.logGroups ?? []).map((group) => ({
    name: group.logGroupName ?? "",
    creationTime: group.creationTime ?? 0,
    retentionInDays: group.retentionInDays ?? 0,
    storedBytes: group.storedBytes ?? 0,
  }));
};
