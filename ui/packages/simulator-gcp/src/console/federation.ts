// The console reads the real Google Cloud APIs, and it reaches them exactly as
// it would reach the real cloud: it federates the signed-in operator's Shauth
// assertion into a cloud credential through the cloud's own Security Token
// Service, then calls the real API paths with it. Only the coordinates — the
// endpoint base URLs and the federation audience — change between the simulator
// and the real cloud. There is no simulator-versus-cloud branch and no
// simulator-served credential broker.
//
// Whether a credential is attached is a real deployment condition, not a
// fallback: a console wired to an identity provider federates the operator and
// every call carries a token, and a federation failure there is surfaced rather
// than hidden; a console with no identity provider — a local or test instance —
// runs unauthenticated, the mode the account control reports. The two are
// distinguished by the console's own configuration, never guessed.

interface ConsoleConfig {
  identityEndpoint?: string;
  // Cloud data-plane coordinates. Empty means the console's own origin.
  cloudApiEndpoint?: string;
  federationEndpoint?: string;
  federationAudience?: string;
  // Where the console's auth layer exposes the operator's assertion.
  federationSubject?: string;
}

let configPromise: Promise<ConsoleConfig> | null = null;
let cachedToken: { value: string; expiresAt: number } | null = null;

async function consoleConfig(): Promise<ConsoleConfig> {
  if (!configPromise) {
    configPromise = fetch("/ui/config.json", { credentials: "include" }).then((response) => {
      if (!response.ok) throw new Error(`/ui/config.json returned HTTP ${response.status}`);
      return response.json() as Promise<ConsoleConfig>;
    });
  }
  return configPromise;
}

// federatedToken exchanges the operator's assertion for a short-lived cloud
// access token at the cloud's Security Token Service — the real Workforce
// Identity Federation token exchange, at whichever coordinate the console is
// configured for.
async function federatedToken(config: ConsoleConfig): Promise<string> {
  const now = Date.now();
  if (cachedToken && cachedToken.expiresAt - 30_000 > now) {
    return cachedToken.value;
  }

  const subjectResponse = await fetch(config.federationSubject!, { credentials: "include" });
  if (!subjectResponse.ok) {
    throw new Error(`could not read the operator assertion: HTTP ${subjectResponse.status}`);
  }
  const { subject_token: subjectToken } = (await subjectResponse.json()) as { subject_token: string };

  const body = new URLSearchParams({
    grant_type: "urn:ietf:params:oauth:grant-type:token-exchange",
    audience: config.federationAudience!,
    subject_token: subjectToken,
    subject_token_type: "urn:ietf:params:oauth:token-type:jwt",
    requested_token_type: "urn:ietf:params:oauth:token-type:access_token",
    scope: "https://www.googleapis.com/auth/cloud-platform",
  });
  const exchange = await fetch(`${config.federationEndpoint ?? ""}/v1/token`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
  });
  if (!exchange.ok) {
    throw new Error(`Security Token Service exchange failed: HTTP ${exchange.status}`);
  }
  const token = (await exchange.json()) as { access_token: string; expires_in: number };
  cachedToken = { value: token.access_token, expiresAt: now + token.expires_in * 1000 };
  return token.access_token;
}

// authorizedFetch reaches a real Google Cloud API path at the configured cloud
// coordinate, attaching a federated credential when the console is wired to an
// identity provider.
export async function authorizedFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const config = await consoleConfig();
  const headers = new Headers(init.headers);
  if (config.identityEndpoint) {
    headers.set("Authorization", `Bearer ${await federatedToken(config)}`);
  }
  return fetch(`${config.cloudApiEndpoint ?? ""}${path}`, { ...init, headers, credentials: "include" });
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
