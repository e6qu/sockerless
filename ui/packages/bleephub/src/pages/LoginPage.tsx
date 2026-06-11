import { useState } from "react";
import { getToken, setToken, verifyToken } from "../api.js";
import { Mark } from "../components/octicons.js";
import { Button } from "../components/ui.js";

export function LoginPage() {
  const [token, setTokenValue] = useState(getToken() ?? "");
  const [error, setError] = useState("");
  const [verifying, setVerifying] = useState(false);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError("");
    setVerifying(true);
    const valid = await verifyToken(token);
    if (valid) {
      setToken(token);
      window.location.href = "/ui/";
    } else {
      setError(
        "Token rejected. The dashboard needs a personal access token (PAT) or the admin token — gho_/ghu_/ghs_ OAuth and installation tokens are not accepted.",
      );
      setVerifying(false);
    }
  }

  return (
    <div
      className="flex min-h-screen flex-col items-center justify-center px-4"
      style={{ background: "var(--color-bg-subtle)" }}
    >
      <div className="mb-5 flex flex-col items-center gap-2">
        <Mark size={42} />
        <h1 style={{ fontSize: "1.4rem", fontWeight: 600, color: "var(--color-fg)" }}>
          Sign in to bleephub
        </h1>
      </div>
      <div
        className="w-full max-w-sm"
        style={{
          border: "1px solid var(--color-border)",
          borderRadius: "var(--radius-md)",
          background: "var(--color-surface)",
          padding: "1.25rem",
        }}
      >
        <form onSubmit={handleSubmit}>
          <label
            htmlFor="token"
            className="mb-1 block"
            style={{ fontSize: "0.85rem", fontWeight: 600, color: "var(--color-fg)" }}
          >
            Admin token
          </label>
          <input
            id="token"
            type="password"
            value={token}
            onChange={(e) => setTokenValue(e.target.value)}
            placeholder="BLEEPHUB_ADMIN_TOKEN"
            autoFocus
            disabled={verifying}
            className="mb-1 w-full"
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: "0.85rem",
            }}
          />
          <p className="mb-3" style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
            Use the admin token the server was started with, or a personal access token.
          </p>
          {error && (
            <p className="mb-3" style={{ fontSize: "0.82rem", color: "var(--color-status-error)" }}>
              {error}
            </p>
          )}
          <Button
            type="submit"
            variant="primary"
            disabled={verifying || !token}
            style={{ width: "100%", opacity: verifying || !token ? 0.6 : 1 }}
          >
            {verifying ? "Verifying…" : "Sign in"}
          </Button>
        </form>
      </div>
    </div>
  );
}
