import { useState } from "react";
import { PageTitle, Button, Box, CodeBlock, ErrorBanner } from "../components/ui.js";

export function OAuthPage() {
  return (
    <div>
      <PageTitle
        title="OAuth flows"
        meta="Device flow and web flow controls through GitHub OAuth endpoints."
      />

      <FlowSimulator />
    </div>
  );
}

function FlowSimulator() {
  const [clientID, setClientID] = useState("");
  const [redirectURI, setRedirectURI] = useState("http://localhost:8080/callback");
  const [scope, setScope] = useState("repo read:org");
  const [state, setState] = useState("STATE-1");
  const [deviceCode, setDeviceCode] = useState("");
  const [result, setResult] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  function startWebFlow() {
    setError(null);
    setResult(null);
    const url =
      `/login/oauth/authorize?client_id=${encodeURIComponent(clientID)}` +
      `&redirect_uri=${encodeURIComponent(redirectURI)}` +
      `&scope=${encodeURIComponent(scope)}` +
      `&state=${encodeURIComponent(state)}`;
    window.open(url, "_blank", "noopener");
    setResult(`Opened ${url} in a new tab.`);
  }

  async function startDeviceFlow() {
    setError(null);
    setResult(null);
    try {
      const body = new URLSearchParams();
      body.set("client_id", clientID);
      body.set("scope", scope);
      const res = await fetch("/login/device/code", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body,
      });
      if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
      const json = await res.json();
      if (typeof json.device_code === "string") {
        setDeviceCode(json.device_code);
      }
      setResult(JSON.stringify(json, null, 2));
    } catch (e) {
      setError(String(e));
    }
  }

  async function pollDeviceToken() {
    setError(null);
    setResult(null);
    try {
      const body = new URLSearchParams();
      body.set("client_id", clientID);
      body.set("grant_type", "urn:ietf:params:oauth:grant-type:device_code");
      body.set("device_code", deviceCode);
      const res = await fetch("/login/oauth/access_token", {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/x-www-form-urlencoded",
        },
        body,
      });
      if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
      setResult(JSON.stringify(await res.json(), null, 2));
    } catch (e) {
      setError(String(e));
    }
  }

  return (
    <Box className="mb-6" header={<span style={{ fontWeight: 600, color: "var(--color-fg)" }}>OAuth flow controls</span>}>
      <div style={{ padding: "1rem" }}>
        <div className="mb-4 grid gap-3 md:grid-cols-2">
          <Field label="Client identifier" value={clientID} onChange={setClientID} />
          <Field label="State" value={state} onChange={setState} />
          <Field label="Redirect Uniform Resource Identifier" value={redirectURI} onChange={setRedirectURI} />
          <Field label="Scope" value={scope} onChange={setScope} />
          <Field label="Device code" value={deviceCode} onChange={setDeviceCode} />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="primary" size="sm" onClick={startWebFlow}>
            Web flow
          </Button>
          <Button variant="secondary" size="sm" onClick={startDeviceFlow}>
            Device flow
          </Button>
          <Button variant="secondary" size="sm" onClick={pollDeviceToken} disabled={!deviceCode.trim()}>
            Poll device token
          </Button>
        </div>
        {result && (
          <div className="mt-4">
            <CodeBlock>{result}</CodeBlock>
          </div>
        )}
        {error && <div className="mt-4"><ErrorBanner>{error}</ErrorBanner></div>}
      </div>
    </Box>
  );
}

function Field({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
}) {
  const id = `oauth-${label.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "")}`;
  return (
    <div>
      <label htmlFor={id} className="mb-1 block" style={{ fontSize: "0.82rem", fontWeight: 600, color: "var(--color-fg)" }}>
        {label}
      </label>
      <input id={id} type="text" value={value} onChange={(e) => onChange(e.target.value)} className="w-full" />
    </div>
  );
}
