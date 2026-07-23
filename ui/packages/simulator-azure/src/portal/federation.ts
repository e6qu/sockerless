// The portal reads the real Azure Resource Manager APIs, and it reaches them
// exactly as it would reach real Azure: it federates the signed-in operator's
// Shauth assertion into an Azure token through Microsoft Entra's own Workload
// Identity Federation, then calls the real Azure Resource Manager paths with it.
// Only the coordinates — the endpoint base URLs, the tenant, and the identity
// the console federates as — change between the simulator and real Azure. There
// is no simulator-versus-cloud branch and no simulator-served credential broker.
//
// Whether a credential is attached is a real deployment condition, not a
// fallback: a portal wired to an identity provider federates the operator and
// every call carries a token, and a federation failure there is surfaced rather
// than hidden; a portal with no identity provider — a local or test instance —
// runs unauthenticated, the mode the account control reports. The two are
// distinguished by the portal's own configuration, never guessed.

interface PortalConfig {
  identityEndpoint?: string;
  // Cloud data-plane coordinates. Empty means the portal's own origin.
  cloudApiEndpoint?: string;
  // Azure Monitor's Log Analytics query API is a distinct host from Azure
  // Resource Manager (api.loganalytics.io vs management.azure.com), reached with
  // its own token audience. Empty means the portal's own origin.
  logsApiEndpoint?: string;
  federationEndpoint?: string;
  federationTenant?: string;
  federationClientId?: string;
  // Where the portal's auth layer exposes the operator's assertion.
  federationSubject?: string;
}

// Azure Resource Manager and Log Analytics are separate resources, each reached
// with a token scoped to it. Real Azure issues an ARM token and a Log Analytics
// token from the same federated assertion; the simulator does the same.
export type CloudScope = "arm" | "logs";

const SCOPES: Record<CloudScope, string> = {
  arm: "https://management.azure.com/.default",
  logs: "https://api.loganalytics.io/.default",
};

let configPromise: Promise<PortalConfig> | null = null;
const cachedTokens: Partial<Record<CloudScope, { value: string; expiresAt: number }>> = {};

async function portalConfig(): Promise<PortalConfig> {
  if (!configPromise) {
    configPromise = fetch("/ui/config.json", { credentials: "include" }).then((response) => {
      if (!response.ok) throw new Error(`/ui/config.json returned HTTP ${response.status}`);
      return response.json() as Promise<PortalConfig>;
    });
  }
  return configPromise;
}

// FederationSetup is a complete, validated federation configuration. Every field
// is required and non-empty; a base endpoint that is the empty string is the
// deliberate "same origin as the portal" coordinate, not a missing value.
interface FederationSetup {
  subject: string;
  clientId: string;
  tenant: string;
  endpoint: string;
}

// resolveFederation turns the portal's configuration into an explicit decision,
// with no implicit defaults and no partial fall-through:
//   - A cloud identity to federate as (federationClientId) is the signal that a
//     deployment intends to federate. When it is set, every coordinate the
//     exchange needs must also be set; an incomplete set is a deployment error
//     surfaced here, never patched with a default (an empty tenant, for one,
//     would build the protocol-relative `//oauth2/v2.0/token`).
//   - With no cloud identity configured, the portal does not federate; reads go
//     unauthenticated and the cloud decides what they get (real Azure answers
//     401; the simulator answers as it does any unauthenticated read). This is a
//     deliberate, configuration-driven mode — a local instance, or a deployment
//     that has not set federation up — chosen by the absence of the coordinate,
//     not a fallback from a failed attempt.
function resolveFederation(config: PortalConfig): FederationSetup | null {
  if (!config.federationClientId) {
    return null;
  }
  const missing = (
    [
      ["identityEndpoint", config.identityEndpoint],
      ["federationSubject", config.federationSubject],
      ["federationTenant", config.federationTenant],
    ] as const
  )
    .filter(([, value]) => !value)
    .map(([name]) => name);
  if (missing.length > 0) {
    throw new Error(
      `incomplete federation configuration: federationClientId is set but ${missing.join(", ")} ${missing.length === 1 ? "is" : "are"} missing`,
    );
  }
  // federationEndpoint is allowed to be empty: that is the portal's own origin,
  // the coordinate for a console co-served with the cloud it reads.
  return {
    subject: config.federationSubject!,
    clientId: config.federationClientId,
    tenant: config.federationTenant!,
    endpoint: config.federationEndpoint ?? "",
  };
}

// federatedToken exchanges the operator's assertion for a short-lived Azure
// token scoped to a resource at Microsoft Entra — the real Workload Identity
// Federation client-assertion grant, at whichever coordinate the portal is
// configured for.
async function federatedToken(setup: FederationSetup, scope: CloudScope): Promise<string> {
  const now = Date.now();
  const cached = cachedTokens[scope];
  if (cached && cached.expiresAt - 30_000 > now) {
    return cached.value;
  }

  const subjectResponse = await fetch(setup.subject, { credentials: "include" });
  if (!subjectResponse.ok) {
    throw new Error(`could not read the operator assertion: HTTP ${subjectResponse.status}`);
  }
  const { subject_token: subjectToken } = (await subjectResponse.json()) as { subject_token: string };

  const body = new URLSearchParams({
    grant_type: "client_credentials",
    client_id: setup.clientId,
    scope: SCOPES[scope],
    client_assertion_type: "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
    client_assertion: subjectToken,
  });
  const exchange = await fetch(`${setup.endpoint}/${setup.tenant}/oauth2/v2.0/token`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
  });
  if (!exchange.ok) {
    throw new Error(`Microsoft Entra token exchange failed: HTTP ${exchange.status}`);
  }
  const token = (await exchange.json()) as { access_token: string; expires_in: number };
  cachedTokens[scope] = { value: token.access_token, expiresAt: now + token.expires_in * 1000 };
  return token.access_token;
}

function baseFor(config: PortalConfig, scope: CloudScope): string {
  return (scope === "logs" ? config.logsApiEndpoint : config.cloudApiEndpoint) ?? "";
}

// authorizedFetch reaches a real Azure path at the configured cloud coordinate
// for the given scope, attaching a scope-specific federated credential when the
// portal is configured to federate (see resolveFederation).
export async function authorizedFetch(path: string, scope: CloudScope = "arm", init: RequestInit = {}): Promise<Response> {
  const config = await portalConfig();
  const setup = resolveFederation(config);
  const headers = new Headers(init.headers);
  if (setup) {
    headers.set("Authorization", `Bearer ${await federatedToken(setup, scope)}`);
  }
  return fetch(`${baseFor(config, scope)}${path}`, { ...init, headers, credentials: "include" });
}

// authorizedJSON reads a real Azure API path as JSON, raising the API's own
// error rather than masking it.
export async function authorizedJSON<T>(path: string, scope: CloudScope = "arm"): Promise<T> {
  const response = await authorizedFetch(path, scope);
  if (!response.ok) {
    throw new Error(`${path} returned HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}

// authorizedJSONPost posts a JSON body to a real Azure API path — the shape Log
// Analytics' query endpoint and other POST reads take.
export async function authorizedJSONPost<T>(path: string, requestBody: unknown, scope: CloudScope = "arm"): Promise<T> {
  const response = await authorizedFetch(path, scope, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(requestBody),
  });
  if (!response.ok) {
    throw new Error(`${path} returned HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}
