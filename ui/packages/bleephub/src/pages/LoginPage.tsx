import { useState } from "react";
import { getToken, setToken, verifyToken } from "../api.js";

export function LoginPage() {
  const [token, setTokenValue] = useState(getToken() ?? "");
  const [error, setError] = useState("");
  const [verifying, setVerifying] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setVerifying(true);
    const valid = await verifyToken(token);
    if (valid) {
      setToken(token);
      window.location.href = "/ui/";
    } else {
      setError("Invalid token");
      setVerifying(false);
    }
  }

  return (
    <div
      className="flex min-h-screen items-center justify-center"
      style={{ background: "var(--color-bg)" }}
    >
      <div
        className="w-full max-w-sm rounded-lg border p-6"
        style={{
          borderColor: "var(--color-border)",
          background: "var(--color-bg-elevated)",
        }}
      >
        <div className="mb-4 text-center">
          <h1
            className="text-xl font-bold"
            style={{ color: "var(--color-fg)" }}
          >
            bleephub
          </h1>
          <p
            className="mt-1 text-xs"
            style={{ color: "var(--color-fg-subtle)" }}
          >
            Enter your admin token to continue
          </p>
        </div>

        <form onSubmit={handleSubmit}>
          <input
            type="password"
            value={token}
            onChange={(e) => setTokenValue(e.target.value)}
            placeholder="BLEEPHUB_ADMIN_TOKEN"
            autoFocus
            disabled={verifying}
            className="mb-3 w-full rounded border px-3 py-2 font-mono text-sm"
            style={{
              borderColor: "var(--color-border)",
              color: "var(--color-fg)",
              background: "var(--color-bg)",
            }}
          />
          {error && (
            <p className="mb-3 text-xs" style={{ color: "var(--color-error)" }}>
              {error}
            </p>
          )}
          <button
            type="submit"
            disabled={verifying || !token}
            className="w-full rounded px-4 py-2 text-sm font-medium"
            style={{
              background: "var(--color-accent)",
              color: "#000",
              opacity: verifying || !token ? 0.5 : 1,
            }}
          >
            {verifying ? "Verifying..." : "Sign in"}
          </button>
        </form>
      </div>
    </div>
  );
}
