import { useQuery } from "@tanstack/react-query";
import { Spinner } from "@sockerless/ui-core/components";
import { useState } from "react";
import { fetchOAuthState } from "../api.js";
import type { BleephubAuthCode, BleephubDeviceCode } from "../types.js";

export function OAuthPage() {
  const { data, isLoading, refetch } = useQuery({
    queryKey: ["oauth_state"],
    queryFn: fetchOAuthState,
    refetchInterval: 3000,
  });

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold">OAuth Debug</h2>
      <FlowSimulator />
      {isLoading || !data ? (
        <Spinner />
      ) : (
        <>
          <DeviceCodesTable codes={data.deviceCodes} />
          <AuthCodesTable codes={data.authCodes} />
        </>
      )}
      <div>
        <button
          type="button"
          onClick={() => refetch()}
          className="text-sm text-blue-600 hover:underline"
        >
          Refresh
        </button>
      </div>
    </div>
  );
}

// FlowSimulator lets the operator kick off either an OAuth web-flow
// authorize URL (auto-approve mode) or a device-flow code, and surfaces
// the resulting redirect / payload inline.
function FlowSimulator() {
  const [clientID, setClientID] = useState("Iv1.test");
  const [redirectURI, setRedirectURI] = useState("http://localhost:8080/callback");
  const [scope, setScope] = useState("repo read:org");
  const [state, setState] = useState("STATE-1");
  const [result, setResult] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function startWebFlow(auto: boolean) {
    setError(null);
    setResult(null);
    const url = `/login/oauth/authorize?client_id=${encodeURIComponent(clientID)}` +
      `&redirect_uri=${encodeURIComponent(redirectURI)}` +
      `&scope=${encodeURIComponent(scope)}` +
      `&state=${encodeURIComponent(state)}` +
      (auto ? "&auto=1" : "");
    if (auto) {
      // Don't follow the redirect — fetch it and surface the Location header.
      try {
        const res = await fetch(url, { redirect: "manual" });
        if (res.status === 0 || res.type === "opaqueredirect") {
          setResult(`(opaque redirect — open ${url} in a new tab to see the redirect target)`);
        } else if (res.status === 302) {
          setResult(`302 → ${res.headers.get("Location") ?? "(no Location header)"}`);
        } else {
          setResult(`${res.status} (unexpected — auto=1 should 302)`);
        }
      } catch (e) {
        setError(String(e));
      }
    } else {
      // Open the authorize HTML form in a new tab; the user clicks Authorize there.
      window.open(url, "_blank", "noopener");
      setResult(`Opened ${url} in a new tab`);
    }
  }

  async function startDeviceFlow() {
    setError(null);
    setResult(null);
    try {
      const res = await fetch("/login/device/code", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: `scope=${encodeURIComponent(scope)}`,
      });
      if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
      const json = await res.json();
      setResult(JSON.stringify(json, null, 2));
    } catch (e) {
      setError(String(e));
    }
  }

  return (
    <div className="border border-gray-200 rounded-lg p-4 space-y-3">
      <div className="text-sm font-medium text-gray-900">Flow simulator</div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Client ID" value={clientID} onChange={setClientID} />
        <Field label="State" value={state} onChange={setState} />
        <Field label="Redirect URI" value={redirectURI} onChange={setRedirectURI} />
        <Field label="Scope" value={scope} onChange={setScope} />
      </div>
      <div className="flex gap-2 flex-wrap">
        <button
          type="button"
          onClick={() => startWebFlow(true)}
          className="px-3 py-1.5 border border-blue-600 text-blue-600 rounded-md text-sm hover:bg-blue-50"
        >
          Web flow (auto-approve)
        </button>
        <button
          type="button"
          onClick={() => startWebFlow(false)}
          className="px-3 py-1.5 border border-gray-300 rounded-md text-sm text-gray-700 hover:bg-gray-50"
        >
          Web flow (open form)
        </button>
        <button
          type="button"
          onClick={startDeviceFlow}
          className="px-3 py-1.5 border border-gray-300 rounded-md text-sm text-gray-700 hover:bg-gray-50"
        >
          Device flow
        </button>
      </div>
      {result && (
        <pre className="bg-gray-50 border border-gray-200 rounded p-2 text-xs overflow-x-auto">
          {result}
        </pre>
      )}
      {error && <div className="text-sm text-red-600">{error}</div>}
    </div>
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
  return (
    <div>
      <label className="block text-xs font-medium text-gray-700">{label}</label>
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="mt-1 block w-full px-2 py-1 border border-gray-300 rounded text-sm font-mono"
      />
    </div>
  );
}

function DeviceCodesTable({ codes }: { codes: BleephubDeviceCode[] }) {
  return (
    <div>
      <h3 className="text-sm font-semibold text-gray-900 mb-2">
        Active device codes ({codes.length})
      </h3>
      {codes.length === 0 ? (
        <div className="text-sm text-gray-500">none</div>
      ) : (
        <table className="min-w-full text-sm border border-gray-200 rounded">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-3 py-2 text-left">user_code</th>
              <th className="px-3 py-2 text-left">code</th>
              <th className="px-3 py-2 text-left">scopes</th>
              <th className="px-3 py-2 text-left">expires</th>
            </tr>
          </thead>
          <tbody>
            {codes.map((c) => (
              <tr key={c.code} className="border-t border-gray-100">
                <td className="px-3 py-2 font-mono">{c.userCode}</td>
                <td className="px-3 py-2 font-mono text-xs text-gray-600">{c.code.slice(0, 8)}…</td>
                <td className="px-3 py-2">{c.scopes}</td>
                <td className="px-3 py-2">{new Date(c.expiresAt).toLocaleTimeString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function AuthCodesTable({ codes }: { codes: BleephubAuthCode[] }) {
  return (
    <div>
      <h3 className="text-sm font-semibold text-gray-900 mb-2">
        Active authorization codes ({codes.length})
      </h3>
      {codes.length === 0 ? (
        <div className="text-sm text-gray-500">none</div>
      ) : (
        <table className="min-w-full text-sm border border-gray-200 rounded">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-3 py-2 text-left">client_id</th>
              <th className="px-3 py-2 text-left">redirect</th>
              <th className="px-3 py-2 text-left">scopes</th>
              <th className="px-3 py-2 text-left">state</th>
              <th className="px-3 py-2 text-left">expires</th>
            </tr>
          </thead>
          <tbody>
            {codes.map((c) => (
              <tr key={c.code} className="border-t border-gray-100">
                <td className="px-3 py-2 font-mono">{c.clientId}</td>
                <td className="px-3 py-2 text-xs text-gray-600">{c.redirectUri}</td>
                <td className="px-3 py-2">{c.scopes}</td>
                <td className="px-3 py-2 font-mono text-xs">{c.state}</td>
                <td className="px-3 py-2">{new Date(c.expiresAt).toLocaleTimeString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
