import { useEffect, useState } from "react";
import { getToken, setToken, verifyToken } from "../api.js";
import { Mark } from "../components/octicons.js";
import { Button } from "../components/ui.js";
import { BleephubBuildFooter } from "../components/Shell.js";

export function LoginPage() {
  const [token, setTokenValue] = useState(getToken() ?? "");
  const [error, setError] = useState("");
  const [verifying, setVerifying] = useState(false);
  const [githubAvailable, setGithubAvailable] = useState(false);
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [localSigningIn, setLocalSigningIn] = useState(false);

  useEffect(() => {
    void fetch("/auth/providers")
      .then(async (response): Promise<{ github?: boolean }> =>
        response.ok ? response.json() : {},
      )
      .then((providers) => setGithubAvailable(providers.github === true))
      .catch(() => setGithubAvailable(false));
  }, []);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError("");
    setVerifying(true);
    const valid = await verifyToken(token);
    if (valid) {
      setToken(token);
      const requested = new URLSearchParams(window.location.search).get("return_to");
      window.location.href = requested?.startsWith("/ui/") ? requested : "/ui/";
    } else {
      setError("Token rejected. Bleephub could not authenticate it through the GitHub REST user endpoint.");
      setVerifying(false);
    }
  }

  async function handleLocalSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError("");
    setLocalSigningIn(true);
    const response = await fetch("/auth/local", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ login, password }),
    });
    if (response.ok) {
      const requested = new URLSearchParams(window.location.search).get("return_to");
      window.location.href = requested?.startsWith("/ui/") || requested === "/control" ? requested : "/ui/";
      return;
    }
    setError("Local credentials were not accepted.");
    setLocalSigningIn(false);
  }

  const returnTo = new URLSearchParams(window.location.search).get("return_to");
  const githubHref = `/auth/github?return_to=${encodeURIComponent(
    returnTo?.startsWith("/") && !returnTo.startsWith("//") ? returnTo : "/ui/",
  )}`;

  return (
    <div
      className="flex min-h-screen flex-col items-center justify-center px-4"
      style={{ background: "var(--color-bg-subtle)" }}
    >
      <div className="flex w-full flex-1 flex-col items-center justify-center">
        <div className="mb-5 flex flex-col items-center gap-2">
          <Mark size={42} />
          <h1 style={{ fontSize: "1.4rem", fontWeight: 600, color: "var(--color-fg)" }}>
            Sign in to Bleephub
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
        {githubAvailable && (
          <a
            href={githubHref}
            className="mb-3 flex w-full items-center justify-center"
            style={{
              border: "1px solid var(--color-border)",
              borderRadius: "var(--radius-md)",
              minHeight: "2.25rem",
              color: "var(--color-fg)",
              fontWeight: 600,
              textDecoration: "none",
            }}
          >
            Continue with GitHub
          </a>
        )}
        <form onSubmit={handleLocalSubmit} className="mb-4">
          <label htmlFor="login" className="mb-1 block" style={{ fontSize: "0.85rem", fontWeight: 600, color: "var(--color-fg)" }}>
            Local account
          </label>
          <input id="login" value={login} onChange={(e) => setLogin(e.target.value)} placeholder="login" disabled={localSigningIn} className="mb-2 w-full" />
          <input aria-label="Local password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="password" disabled={localSigningIn} className="mb-2 w-full" />
          <Button type="submit" variant="ghost" disabled={localSigningIn || !login || !password} style={{ width: "100%" }}>
            {localSigningIn ? "Signing in…" : "Continue with local account"}
          </Button>
        </form>
        <div className="mb-4" style={{ borderTop: "1px solid var(--color-border)" }} />
        <form onSubmit={handleSubmit}>
          <label
            htmlFor="token"
            className="mb-1 block"
            style={{ fontSize: "0.85rem", fontWeight: 600, color: "var(--color-fg)" }}
          >
            Access token
          </label>
          <input
            id="token"
            type="password"
            value={token}
            onChange={(e) => setTokenValue(e.target.value)}
            placeholder="GitHub-compatible token"
            autoFocus
            disabled={verifying}
            className="mb-1 w-full"
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: "0.85rem",
            }}
          />
          <p className="mb-3" style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
            Use the admin token, a personal access token, or an OAuth token accepted by this Bleephub instance.
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
      <BleephubBuildFooter />
    </div>
  );
}
