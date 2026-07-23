import { signedFetch, type AwsCredentials } from "./sigv4.js";

// The console reads the real AWS APIs, and it reaches them exactly as it would
// reach real AWS: it federates the signed-in operator's assertion into
// temporary credentials at the Security Token Service, then signs every request
// with them. Only the coordinates — the endpoint base URLs and the role to
// assume — change between the simulator and real AWS; there is no
// simulator-versus-cloud branch and no simulator-served credential broker.
//
// Whether requests are signed is a real deployment condition, not a fallback: a
// console wired to an identity provider federates the operator and signs every
// call, and a federation failure there is surfaced rather than hidden; a console
// with no identity provider — a local or test instance — reads unsigned, the
// mode the account control reports.

const REGION = "us-east-1";

interface ConsoleConfig {
  identityEndpoint?: string;
  cloudApiEndpoint?: string;
  federationEndpoint?: string;
  federationAudience?: string; // the role ARN to assume
  federationSubject?: string;
}

let configPromise: Promise<ConsoleConfig> | null = null;
let cachedCreds: { value: AwsCredentials; expiresAt: number } | null = null;

async function consoleConfig(): Promise<ConsoleConfig> {
  if (!configPromise) {
    configPromise = fetch("/ui/config.json", { credentials: "include" }).then((response) => {
      if (!response.ok) throw new Error(`/ui/config.json returned HTTP ${response.status}`);
      return response.json() as Promise<ConsoleConfig>;
    });
  }
  return configPromise;
}

function xmlText(xml: Document, tag: string): string {
  return xml.getElementsByTagName(tag)[0]?.textContent ?? "";
}

async function federatedCredentials(config: ConsoleConfig): Promise<AwsCredentials> {
  const now = Date.now();
  if (cachedCreds && cachedCreds.expiresAt - 60_000 > now) {
    return cachedCreds.value;
  }

  const subjectResponse = await fetch(config.federationSubject!, { credentials: "include" });
  if (!subjectResponse.ok) {
    throw new Error(`could not read the operator assertion: HTTP ${subjectResponse.status}`);
  }
  const { subject_token: subjectToken } = (await subjectResponse.json()) as { subject_token: string };

  const body = new URLSearchParams({
    Action: "AssumeRoleWithWebIdentity",
    Version: "2011-06-15",
    RoleArn: config.federationAudience!,
    RoleSessionName: "console",
    WebIdentityToken: subjectToken,
  });
  const response = await fetch(`${config.federationEndpoint ?? ""}/`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
    credentials: "include",
  });
  if (!response.ok) {
    throw new Error(`AssumeRoleWithWebIdentity failed: HTTP ${response.status}`);
  }
  const xml = new DOMParser().parseFromString(await response.text(), "text/xml");
  const credentials: AwsCredentials = {
    accessKeyId: xmlText(xml, "AccessKeyId"),
    secretAccessKey: xmlText(xml, "SecretAccessKey"),
    sessionToken: xmlText(xml, "SessionToken"),
  };
  const expiration = Date.parse(xmlText(xml, "Expiration")) || now + 3_600_000;
  cachedCreds = { value: credentials, expiresAt: expiration };
  return credentials;
}

export interface AwsRequest {
  service: string;
  method?: string;
  path?: string;
  headers?: Record<string, string>;
  body?: string;
}

// awsFetch reaches a real AWS API at the configured cloud coordinate, signing
// the request with federated credentials when the console is wired to an
// identity provider.
export async function awsFetch(request: AwsRequest): Promise<Response> {
  const config = await consoleConfig();
  const base = config.cloudApiEndpoint ?? "";
  const url = `${base}${request.path ?? "/"}`;
  const method = request.method ?? "POST";

  if (config.identityEndpoint) {
    return signedFetch({
      method,
      url,
      service: request.service,
      region: REGION,
      headers: request.headers,
      body: request.body,
      credentials: await federatedCredentials(config),
    });
  }
  return fetch(url, {
    method,
    headers: new Headers(request.headers),
    body: method === "GET" || method === "HEAD" ? undefined : request.body,
    credentials: "include",
  });
}

// awsJson calls an AWS JSON (1.1) target operation and returns the parsed
// result — the protocol ECS, ECR, and CloudWatch Logs speak.
export async function awsJson<T>(service: string, target: string, input: Record<string, unknown> = {}): Promise<T> {
  const response = await awsFetch({
    service,
    method: "POST",
    path: "/",
    headers: { "content-type": "application/x-amz-json-1.1", "x-amz-target": target },
    body: JSON.stringify(input),
  });
  if (!response.ok) {
    throw new Error(`${target} returned HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}

// awsRestJson calls an AWS REST-JSON GET operation (Lambda's list surface).
export async function awsRestJson<T>(service: string, path: string): Promise<T> {
  const response = await awsFetch({ service, method: "GET", path });
  if (!response.ok) {
    throw new Error(`${path} returned HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}

// awsRestXml calls an AWS REST-XML GET operation (S3's ListBuckets) and returns
// the parsed document.
export async function awsRestXml(service: string, path: string): Promise<Document> {
  const response = await awsFetch({ service, method: "GET", path });
  if (!response.ok) {
    throw new Error(`${path} returned HTTP ${response.status}`);
  }
  return new DOMParser().parseFromString(await response.text(), "text/xml");
}
