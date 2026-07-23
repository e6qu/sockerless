// The console reads the real Google Cloud APIs, and a real console reaches them
// with a credential brokered from the signed-in session. This module obtains
// that credential and attaches it.
//
// Whether a credential is required is a real deployment condition, not a
// fallback: a simulator wired to a single sign-on provider authenticates the
// operator and every API call carries a federated token, and a broker failure
// there is surfaced rather than hidden; a simulator with no identity provider
// configured — a local development or test instance — runs unauthenticated, the
// same mode the account control reports. The two are distinguished by the
// console's own configuration, never guessed.

interface UIConfig {
  identityEndpoint?: string;
}

interface BrokeredToken {
  access_token: string;
  expires_in: number;
}

let configPromise: Promise<UIConfig> | null = null;
let cachedToken: { value: string; expiresAt: number } | null = null;

async function uiConfig(): Promise<UIConfig> {
  if (!configPromise) {
    configPromise = fetch("/ui/config.json", { credentials: "include" }).then((response) => {
      if (!response.ok) throw new Error(`/ui/config.json returned HTTP ${response.status}`);
      return response.json() as Promise<UIConfig>;
    });
  }
  return configPromise;
}

async function federatedToken(): Promise<string> {
  const now = Date.now();
  if (cachedToken && cachedToken.expiresAt - 30_000 > now) {
    return cachedToken.value;
  }
  const response = await fetch("/auth/cloud-token", { credentials: "include" });
  if (!response.ok) {
    throw new Error(`could not broker a cloud credential: HTTP ${response.status}`);
  }
  const token = (await response.json()) as BrokeredToken;
  cachedToken = { value: token.access_token, expiresAt: now + token.expires_in * 1000 };
  return token.access_token;
}

// authorizedFetch reaches a real Google Cloud API path, attaching a brokered
// federated credential when the console is wired to an identity provider.
export async function authorizedFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const config = await uiConfig();
  const headers = new Headers(init.headers);
  if (config.identityEndpoint) {
    headers.set("Authorization", `Bearer ${await federatedToken()}`);
  }
  return fetch(path, { ...init, headers, credentials: "include" });
}

// authorizedJSON reads a real Google Cloud API path as JSON, raising the API's
// own error rather than masking it.
export async function authorizedJSON<T>(path: string): Promise<T> {
  const response = await authorizedFetch(path);
  if (!response.ok) {
    throw new Error(`${path} returned HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}
