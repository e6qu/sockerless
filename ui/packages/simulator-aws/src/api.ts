async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

export interface ECSTask {
  taskArn: string;
  status: string;
  clusterArn: string;
  launchType: string;
  cpu: string;
  memory: string;
}

export interface LambdaFunction {
  name: string;
  runtime: string;
  state: string;
  memorySize: number;
  timeout: number;
  lastModified: string;
}

export interface ECRRepo {
  name: string;
  uri: string;
  createdAt: number;
}

export interface S3Bucket {
  name: string;
  creationDate: string;
}

export interface CWLogGroup {
  name: string;
  creationTime: number;
  retentionInDays: number;
  storedBytes: number;
}

export const fetchECSTasks = () => fetchJSON<ECSTask[]>("/sim/v1/ecs/tasks");
export const fetchLambdaFunctions = () => fetchJSON<LambdaFunction[]>("/sim/v1/lambda/functions");
export const fetchECRRepos = () => fetchJSON<ECRRepo[]>("/sim/v1/ecr/repositories");
export const fetchS3Buckets = () => fetchJSON<S3Bucket[]>("/sim/v1/s3/buckets");
export const fetchCWLogGroups = () => fetchJSON<CWLogGroup[]>("/sim/v1/cloudwatch/log-groups");
