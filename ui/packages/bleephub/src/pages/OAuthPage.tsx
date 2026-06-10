import { useQuery } from "@tanstack/react-query";
import { Spinner } from "@sockerless/ui-core/components";
import { useState, type ReactNode } from "react";
import { fetchOAuthState } from "../api.js";
import type { BleephubAuthCode, BleephubDeviceCode } from "../types.js";
import { PageTitle, Button, Box, CodeBlock, ErrorBanner } from "../components/ui.js";
import { RefreshIcon } from "../components/octicons.js";

/** Shared table shell used by DeviceCodesCard and AuthCodesCard. */
function OAuthCodesTable<T>({
  cardTitle,
  count,
  items,
  headers,
  rowKey,
  renderRow,
}: {
  cardTitle: string;
  count: number;
  items: T[];
  headers: string[];
  rowKey: (item: T) => string;
  renderRow: (item: T) => ReactNode;
}) {
  return (
    <Box
      header={
        <div className="flex w-full items-center justify-between">
          <span style={{ fontWeight: 600, color: "var(--color-fg)" }}>{cardTitle}</span>
          <span
            className="tabular-nums"
            style={{ color: count > 0 ? "var(--color-accent)" : "var(--color-fg-muted)", fontWeight: 600 }}
          >
            {count}
          </span>
        </div>
      }
    >
      {items.length === 0 ? (
        <div style={{ padding: "1.25rem", textAlign: "center", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
          None active.
        </div>
      ) : (
        <table className="w-full" style={{ fontSize: "0.8rem" }}>
          <thead>
            <tr style={{ color: "var(--color-fg-muted)", textAlign: "left" }}>
              {headers.map((h) => (
                <th key={h} className="font-mono" style={{ padding: "0.4rem 1rem", fontWeight: 500 }}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={rowKey(item)} style={{ borderTop: "1px solid var(--color-border)" }}>
                {renderRow(item)}
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Box>
  );
}

export function OAuthPage() {
  const { data, isLoading, refetch } = useQuery({
    queryKey: ["oauth_state"],
    queryFn: fetchOAuthState,
    refetchInterval: 3000,
  });

  return (
    <div>
      <PageTitle
        title="OAuth flows"
        meta="Device flow + web flow simulator. Mint codes, exchange tokens, watch the live state below."
        actions={
          <Button size="sm" variant="secondary" onClick={() => refetch()}>
            <RefreshIcon size={14} /> Refresh
          </Button>
        }
      />

      <FlowSimulator />

      {isLoading || !data ? (
        <Spinner label="loading oauth state" />
      ) : (
        <div className="grid gap-5 md:grid-cols-2">
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
    <Box className="mb-6" header={<span style={{ fontWeight: 600, color: "var(--color-fg)" }}>Flow simulator</span>}>
      <div style={{ padding: "1rem" }}>
        <div className="mb-4 grid gap-3 md:grid-cols-2">
          <Field label="Client ID" value={clientID} onChange={setClientID} />
          <Field label="State" value={state} onChange={setState} />
          <Field label="Redirect URI" value={redirectURI} onChange={setRedirectURI} />
          <Field label="Scope" value={scope} onChange={setScope} />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="primary" size="sm" onClick={() => startWebFlow(true)}>
            Web flow (auto)
          </Button>
          <Button variant="secondary" size="sm" onClick={() => startWebFlow(false)}>
            Web flow → form
          </Button>
          <Button variant="secondary" size="sm" onClick={startDeviceFlow}>
            Device flow
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
  return (
    <div>
      <label className="mb-1 block" style={{ fontSize: "0.82rem", fontWeight: 600, color: "var(--color-fg)" }}>
        {label}
      </label>
      <input type="text" value={value} onChange={(e) => onChange(e.target.value)} className="w-full" />
    </div>
  );
}

function DeviceCodesCard({ codes }: { codes: BleephubDeviceCode[] }) {
  return (
    <OAuthCodesTable
      cardTitle="Active device codes"
      count={codes.length}
      items={codes}
      headers={["user_code", "code", "scopes", "expires"]}
      rowKey={(c) => c.code}
      renderRow={(c) => (
        <>
          <td className="font-mono" style={{ padding: "0.4rem 1rem", color: "var(--color-accent)" }}>{c.userCode}</td>
          <td className="font-mono" style={{ padding: "0.4rem 1rem", color: "var(--color-fg-muted)" }}>{c.code.slice(0, 8)}…</td>
          <td style={{ padding: "0.4rem 1rem", color: "var(--color-fg)" }}>{c.scopes}</td>
          <td style={{ padding: "0.4rem 1rem", color: "var(--color-fg-muted)" }}>{new Date(c.expiresAt).toLocaleTimeString()}</td>
        </>
      )}
    />
  );
}

function AuthCodesCard({ codes }: { codes: BleephubAuthCode[] }) {
  return (
    <OAuthCodesTable
      cardTitle="Active authorization codes"
      count={codes.length}
      items={codes}
      headers={["client_id", "redirect", "state", "expires"]}
      rowKey={(c) => c.code}
      renderRow={(c) => (
        <>
          <td className="font-mono" style={{ padding: "0.4rem 1rem", color: "var(--color-accent)" }}>{c.clientId}</td>
          <td className="font-mono" style={{ padding: "0.4rem 1rem", color: "var(--color-fg-muted)" }}>{c.redirectUri}</td>
          <td style={{ padding: "0.4rem 1rem", color: "var(--color-fg)" }}>{c.state || "—"}</td>
          <td style={{ padding: "0.4rem 1rem", color: "var(--color-fg-muted)" }}>{new Date(c.expiresAt).toLocaleTimeString()}</td>
        </>
      )}
    />
  );
}
