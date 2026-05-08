import { useQuery } from "@tanstack/react-query";
import {
  Button,
  PageHeading,
  Spinner,
} from "@sockerless/ui-core/components";
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
    <div>
      <PageHeading
        kicker="oauth · debug"
        title={<>OAuth flows</>}
        meta="Device flow + web flow simulator. Mint codes, exchange tokens, watch the live state below."
        actions={
          <Button size="sm" onClick={() => refetch()} variant="ghost">
            ↻ refresh
          </Button>
        }
      />

      <FlowSimulator />

      {isLoading || !data ? (
        <Spinner label="loading oauth state" />
      ) : (
        <div className="grid gap-6 md:grid-cols-2">
          <DeviceCodesCard codes={data.deviceCodes} />
          <AuthCodesCard codes={data.authCodes} />
        </div>
      )}
    </div>
  );
}

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
    const url =
      `/login/oauth/authorize?client_id=${encodeURIComponent(clientID)}` +
      `&redirect_uri=${encodeURIComponent(redirectURI)}` +
      `&scope=${encodeURIComponent(scope)}` +
      `&state=${encodeURIComponent(state)}` +
      (auto ? "&auto=1" : "");
    if (auto) {
      try {
        const res = await fetch(url, { redirect: "manual" });
        if (res.status === 0 || res.type === "opaqueredirect") {
          setResult(`(opaque redirect — open ${url} to see the redirect target)`);
        } else if (res.status === 302) {
          setResult(`302 → ${res.headers.get("Location") ?? "(no Location header)"}`);
        } else {
          setResult(`${res.status} (unexpected — auto=1 should 302)`);
        }
      } catch (e) {
        setError(String(e));
      }
    } else {
      window.open(url, "_blank", "noopener");
      setResult(`Opened ${url} in a new tab.`);
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
    <section
      className="mb-6 p-5"
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderLeft: "3px solid var(--color-accent)",
        borderRadius: "var(--radius-sm)",
      }}
    >
      <div
        className="mb-3 text-[10px] uppercase tracking-[0.22em]"
        style={{ color: "var(--color-fg-subtle)" }}
      >
        flow simulator
      </div>
      <div className="grid gap-3 md:grid-cols-2 mb-4">
        <Field label="Client ID" value={clientID} onChange={setClientID} />
        <Field label="State" value={state} onChange={setState} />
        <Field label="Redirect URI" value={redirectURI} onChange={setRedirectURI} />
        <Field label="Scope" value={scope} onChange={setScope} />
      </div>
      <div className="flex flex-wrap gap-2">
        <Button variant="primary" size="sm" onClick={() => startWebFlow(true)}>
          Web flow ⚡ auto
        </Button>
        <Button variant="secondary" size="sm" onClick={() => startWebFlow(false)}>
          Web flow → form
        </Button>
        <Button variant="secondary" size="sm" onClick={startDeviceFlow}>
          Device flow
        </Button>
      </div>
      {result && (
        <pre
          className="mt-4 px-3 py-2 font-mono"
          style={{
            background: "var(--color-bg-subtle)",
            border: "1px solid var(--color-border)",
            borderRadius: "var(--radius-sm)",
            fontSize: "0.7rem",
            color: "var(--color-fg)",
            overflow: "auto",
          }}
        >
          {result}
        </pre>
      )}
      {error && (
        <div
          className="mt-4 px-3 py-2 font-mono text-xs"
          style={{
            background: "var(--color-status-error-soft)",
            color: "var(--color-status-error)",
            border: "1px solid var(--color-status-error)",
            borderRadius: "var(--radius-sm)",
          }}
        >
          {error}
        </div>
      )}
    </section>
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
      <label
        className="mb-1 block text-[10px] uppercase tracking-[0.18em]"
        style={{ color: "var(--color-fg-subtle)" }}
      >
        {label}
      </label>
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full"
      />
    </div>
  );
}

function CardShell({
  title,
  count,
  children,
}: {
  title: string;
  count: number;
  children: React.ReactNode;
}) {
  return (
    <section
      className="p-5"
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-sm)",
      }}
    >
      <div className="mb-3 flex items-baseline justify-between">
        <div
          className="text-[10px] uppercase tracking-[0.22em]"
          style={{ color: "var(--color-fg-subtle)" }}
        >
          {title}
        </div>
        <div
          className="font-display tabular-nums"
          style={{
            fontSize: "1.6rem",
            fontStyle: "italic",
            fontWeight: 600,
            letterSpacing: "-0.02em",
            color: count > 0 ? "var(--color-accent)" : "var(--color-fg-subtle)",
            lineHeight: 1,
          }}
        >
          {count}
        </div>
      </div>
      {children}
    </section>
  );
}

function DeviceCodesCard({ codes }: { codes: BleephubDeviceCode[] }) {
  return (
    <CardShell title="Active device codes" count={codes.length}>
      {codes.length === 0 ? (
        <div
          className="py-6 text-center font-mono uppercase tracking-[0.2em]"
          style={{ fontSize: "0.7rem", color: "var(--color-fg-subtle)" }}
        >
          — none —
        </div>
      ) : (
        <table className="w-full font-mono" style={{ fontSize: "0.72rem" }}>
          <thead>
            <tr
              className="text-left uppercase tracking-[0.15em]"
              style={{ fontSize: "0.6rem", color: "var(--color-fg-subtle)" }}
            >
              <th className="py-1.5 font-medium">user_code</th>
              <th className="py-1.5 font-medium">code</th>
              <th className="py-1.5 font-medium">scopes</th>
              <th className="py-1.5 font-medium text-right">expires</th>
            </tr>
          </thead>
          <tbody>
            {codes.map((c) => (
              <tr
                key={c.code}
                style={{
                  borderTop:
                    "1px solid color-mix(in oklch, var(--color-border) 60%, transparent)",
                }}
              >
                <td className="py-1.5" style={{ color: "var(--color-accent)" }}>
                  {c.userCode}
                </td>
                <td className="py-1.5" style={{ color: "var(--color-fg-muted)" }}>
                  {c.code.slice(0, 8)}…
                </td>
                <td className="py-1.5" style={{ color: "var(--color-fg)" }}>
                  {c.scopes}
                </td>
                <td className="py-1.5 text-right" style={{ color: "var(--color-fg-muted)" }}>
                  {new Date(c.expiresAt).toLocaleTimeString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </CardShell>
  );
}

function AuthCodesCard({ codes }: { codes: BleephubAuthCode[] }) {
  return (
    <CardShell title="Active authorization codes" count={codes.length}>
      {codes.length === 0 ? (
        <div
          className="py-6 text-center font-mono uppercase tracking-[0.2em]"
          style={{ fontSize: "0.7rem", color: "var(--color-fg-subtle)" }}
        >
          — none —
        </div>
      ) : (
        <table className="w-full font-mono" style={{ fontSize: "0.72rem" }}>
          <thead>
            <tr
              className="text-left uppercase tracking-[0.15em]"
              style={{ fontSize: "0.6rem", color: "var(--color-fg-subtle)" }}
            >
              <th className="py-1.5 font-medium">client_id</th>
              <th className="py-1.5 font-medium">redirect</th>
              <th className="py-1.5 font-medium">state</th>
              <th className="py-1.5 font-medium text-right">expires</th>
            </tr>
          </thead>
          <tbody>
            {codes.map((c) => (
              <tr
                key={c.code}
                style={{
                  borderTop:
                    "1px solid color-mix(in oklch, var(--color-border) 60%, transparent)",
                }}
              >
                <td className="py-1.5" style={{ color: "var(--color-accent)" }}>
                  {c.clientId}
                </td>
                <td className="py-1.5" style={{ color: "var(--color-fg-muted)" }}>
                  {c.redirectUri}
                </td>
                <td className="py-1.5" style={{ color: "var(--color-fg)" }}>
                  {c.state || "—"}
                </td>
                <td className="py-1.5 text-right" style={{ color: "var(--color-fg-muted)" }}>
                  {new Date(c.expiresAt).toLocaleTimeString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </CardShell>
  );
}
