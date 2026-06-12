import { useState } from "react";
import { useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchEnvironments,
  fetchScopedSecrets,
  fetchScopedPublicKey,
  putScopedSecret,
  deleteScopedSecret,
  fetchScopedVariables,
  createScopedVariable,
  updateScopedVariable,
  deleteScopedVariable,
  type SecretsScope,
} from "../api.js";
import type { GithubOrgVisibility, GithubVariable } from "../types.js";
import { sealSecret } from "../utils/sealedBox.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import { RepoHeader } from "../components/Shell.js";
import {
  Box,
  Blankslate,
  Button,
  Tabs,
  Modal,
  FormLabel,
  ErrorBanner,
  DialogActions,
  SectionLabel,
} from "../components/ui.js";
import { LockIcon, KeyIcon } from "../components/octicons.js";

type ScopeKind = "repo" | "env" | "org";

/** Stable cache-key suffix for a scope. */
function scopeKey(scope: SecretsScope): string {
  switch (scope.kind) {
    case "repo":
      return `repo:${scope.owner}/${scope.repo}`;
    case "env":
      return `env:${scope.owner}/${scope.repo}/${scope.env}`;
    case "org":
      return `org:${scope.org}`;
  }
}

export function RepoSecretsPage() {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  const counts = useOpenCounts(owner, repo);
  const [kind, setKind] = useState<ScopeKind>("repo");
  const [envName, setEnvName] = useState("");

  const envsQ = useQuery({
    queryKey: ["environments", owner, repo],
    queryFn: () => fetchEnvironments(owner, repo),
    enabled: kind === "env" && !!owner && !!repo,
  });
  const envs = envsQ.data ?? [];
  const effectiveEnv = envName || envs[0]?.name || "";

  const scope: SecretsScope | null =
    kind === "repo"
      ? { kind: "repo", owner, repo }
      : kind === "org"
        ? { kind: "org", org: owner }
        : effectiveEnv
          ? { kind: "env", owner, repo, env: effectiveEnv }
          : null;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="settings" {...counts} />
      <SectionLabel>Actions secrets and variables</SectionLabel>
      <p className="mb-4" style={{ fontSize: "0.84rem", color: "var(--color-fg-muted)" }}>
        Secrets are encrypted in the browser with the scope&apos;s public key before upload and
        are never readable again from this page. Variables are stored as plain text.
      </p>
      <Tabs
        items={[
          { key: "repo", label: "Repository" },
          { key: "env", label: "Environments" },
          { key: "org", label: `Organization (${owner})` },
        ]}
        active={kind}
        onChange={setKind}
      />

      {kind === "env" && (
        <div className="mb-4 flex items-center gap-2">
          <label htmlFor="secrets-env-select" style={{ fontSize: "0.84rem", color: "var(--color-fg-muted)" }}>
            Environment
          </label>
          {envsQ.isLoading && <Spinner label="loading environments" />}
          {envsQ.isError && (
            <InlineError inline title="Failed to load environments" detail={String(envsQ.error)} />
          )}
          {envsQ.data &&
            (envs.length === 0 ? (
              <span style={{ fontSize: "0.84rem", color: "var(--color-fg-muted)" }}>
                This repository has no environments yet.
              </span>
            ) : (
              <select
                id="secrets-env-select"
                value={effectiveEnv}
                onChange={(e) => setEnvName(e.target.value)}
                style={{
                  padding: "0.28rem 0.55rem",
                  fontSize: "0.82rem",
                  background: "var(--color-bg-subtle)",
                  color: "var(--color-fg)",
                  border: "1px solid var(--color-border)",
                  borderRadius: "var(--radius-md)",
                }}
              >
                {envs.map((env) => (
                  <option key={env.name} value={env.name}>
                    {env.name}
                  </option>
                ))}
              </select>
            ))}
        </div>
      )}

      {scope && (
        <div className="grid gap-6 lg:grid-cols-2">
          <SecretsSection scope={scope} />
          <VariablesSection scope={scope} />
        </div>
      )}
    </div>
  );
}

// ─── Secrets ─────────────────────────────────────────────────────────────

function SecretsSection({ scope }: { scope: SecretsScope }) {
  const qc = useQueryClient();
  const key = scopeKey(scope);
  const [editing, setEditing] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  const secretsQ = useQuery({
    queryKey: ["actions-secrets", key],
    queryFn: () => fetchScopedSecrets(scope),
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => deleteScopedSecret(scope, name),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["actions-secrets", key] }),
  });

  return (
    <section aria-label="Secrets">
      <div className="mb-2 flex items-center justify-between">
        <SectionLabel>Secrets</SectionLabel>
        <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
          New secret
        </Button>
      </div>
      {secretsQ.isLoading && <Spinner label="loading secrets" />}
      {secretsQ.isError && (
        <InlineError title="Failed to load secrets" detail={String(secretsQ.error)} />
      )}
      {deleteMutation.isError && <ErrorBanner>{String(deleteMutation.error)}</ErrorBanner>}
      {secretsQ.data &&
        (secretsQ.data.items.length === 0 ? (
          <Blankslate icon={<LockIcon size={24} />} title="No secrets" />
        ) : (
          <Box>
            {secretsQ.data.items.map((s, i) => (
              <div
                key={s.name}
                className="flex items-center gap-2"
                style={{
                  padding: "0.55rem 1rem",
                  borderBottom:
                    i < secretsQ.data.items.length - 1 ? "1px solid var(--color-border)" : "none",
                }}
              >
                <LockIcon size={14} style={{ color: "var(--color-fg-muted)" }} />
                <span className="min-w-0 flex-1 truncate font-mono" style={{ fontSize: "0.84rem", color: "var(--color-fg)" }}>
                  {s.name}
                </span>
                {s.visibility && (
                  <span style={{ fontSize: "0.72rem", color: "var(--color-fg-subtle)" }}>
                    {s.visibility}
                  </span>
                )}
                <span style={{ fontSize: "0.74rem", color: "var(--color-fg-muted)" }}>
                  updated {new Date(s.updated_at).toLocaleDateString()}
                </span>
                <Button variant="ghost" size="sm" onClick={() => setEditing(s.name)}>
                  Update
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  disabled={deleteMutation.isPending}
                  onClick={() => deleteMutation.mutate(s.name)}
                >
                  Delete
                </Button>
              </div>
            ))}
          </Box>
        ))}
      {(creating || editing !== null) && (
        <SecretModal
          scope={scope}
          existingName={editing}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
        />
      )}
    </section>
  );
}

function SecretModal({
  scope,
  existingName,
  onClose,
}: {
  scope: SecretsScope;
  existingName: string | null;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState(existingName ?? "");
  const [value, setValue] = useState("");
  const [visibility, setVisibility] = useState<GithubOrgVisibility>("all");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: async () => {
      const secretName = name.trim();
      if (!secretName) throw new Error("Name is required");
      if (!value) throw new Error("Value is required");
      // Sealed-box encrypt against the scope's public key; the PUT body
      // carries ciphertext + key_id only.
      const pk = await fetchScopedPublicKey(scope);
      const encrypted = await sealSecret(value, pk.key);
      await putScopedSecret(scope, secretName, {
        encrypted_value: encrypted,
        key_id: pk.key_id,
        ...(scope.kind === "org" ? { visibility } : {}),
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["actions-secrets", scopeKey(scope)] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Modal title={existingName ? `Update secret ${existingName}` : "New secret"} onClose={onClose}>
      {existingName === null && (
        <>
          <FormLabel id="secret-name">Name</FormLabel>
          <input
            id="secret-name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="YOUR_SECRET_NAME"
            className="mb-4 w-full font-mono"
          />
        </>
      )}
      <FormLabel id="secret-value">Value</FormLabel>
      <textarea
        id="secret-value"
        rows={4}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        className="mb-4 w-full font-mono"
        style={{ resize: "vertical" }}
      />
      {scope.kind === "org" && (
        <>
          <FormLabel id="secret-visibility">Repository access</FormLabel>
          <select
            id="secret-visibility"
            value={visibility}
            onChange={(e) => setVisibility(e.target.value as GithubOrgVisibility)}
            className="mb-4 w-full"
          >
            <option value="all">All repositories</option>
            <option value="private">Private repositories</option>
            <option value="selected">Selected repositories</option>
          </select>
        </>
      )}
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <DialogActions>
        <Button onClick={onClose} disabled={mutation.isPending} variant="ghost">
          Cancel
        </Button>
        <Button
          onClick={() => {
            setError(null);
            mutation.mutate();
          }}
          disabled={mutation.isPending}
          variant="primary"
        >
          {mutation.isPending ? "Encrypting…" : existingName ? "Update secret" : "Add secret"}
        </Button>
      </DialogActions>
    </Modal>
  );
}

// ─── Variables ───────────────────────────────────────────────────────────

function VariablesSection({ scope }: { scope: SecretsScope }) {
  const qc = useQueryClient();
  const key = scopeKey(scope);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<GithubVariable | null>(null);

  const variablesQ = useQuery({
    queryKey: ["actions-variables", key],
    queryFn: () => fetchScopedVariables(scope),
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => deleteScopedVariable(scope, name),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["actions-variables", key] }),
  });

  return (
    <section aria-label="Variables">
      <div className="mb-2 flex items-center justify-between">
        <SectionLabel>Variables</SectionLabel>
        <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
          New variable
        </Button>
      </div>
      {variablesQ.isLoading && <Spinner label="loading variables" />}
      {variablesQ.isError && (
        <InlineError title="Failed to load variables" detail={String(variablesQ.error)} />
      )}
      {deleteMutation.isError && <ErrorBanner>{String(deleteMutation.error)}</ErrorBanner>}
      {variablesQ.data &&
        (variablesQ.data.items.length === 0 ? (
          <Blankslate icon={<KeyIcon size={24} />} title="No variables" />
        ) : (
          <Box>
            {variablesQ.data.items.map((v, i) => (
              <div
                key={v.name}
                className="flex items-center gap-2"
                style={{
                  padding: "0.55rem 1rem",
                  borderBottom:
                    i < variablesQ.data.items.length - 1 ? "1px solid var(--color-border)" : "none",
                }}
              >
                <span className="min-w-0 flex-1 truncate font-mono" style={{ fontSize: "0.84rem", color: "var(--color-fg)" }}>
                  {v.name}
                </span>
                <span
                  className="min-w-0 flex-1 truncate font-mono"
                  style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}
                >
                  {v.value}
                </span>
                {v.visibility && (
                  <span style={{ fontSize: "0.72rem", color: "var(--color-fg-subtle)" }}>
                    {v.visibility}
                  </span>
                )}
                <Button variant="ghost" size="sm" onClick={() => setEditing(v)}>
                  Edit
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  disabled={deleteMutation.isPending}
                  onClick={() => deleteMutation.mutate(v.name)}
                >
                  Delete
                </Button>
              </div>
            ))}
          </Box>
        ))}
      {(creating || editing !== null) && (
        <VariableModal
          scope={scope}
          existing={editing}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
        />
      )}
    </section>
  );
}

function VariableModal({
  scope,
  existing,
  onClose,
}: {
  scope: SecretsScope;
  existing: GithubVariable | null;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState(existing?.name ?? "");
  const [value, setValue] = useState(existing?.value ?? "");
  const [visibility, setVisibility] = useState<GithubOrgVisibility>(existing?.visibility ?? "all");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: async () => {
      const varName = name.trim();
      if (!varName) throw new Error("Name is required");
      if (existing) {
        await updateScopedVariable(scope, existing.name, { name: varName, value });
      } else {
        await createScopedVariable(scope, {
          name: varName,
          value,
          ...(scope.kind === "org" ? { visibility } : {}),
        });
      }
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["actions-variables", scopeKey(scope)] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Modal title={existing ? `Edit variable ${existing.name}` : "New variable"} onClose={onClose}>
      <FormLabel id="variable-name">Name</FormLabel>
      <input
        id="variable-name"
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="YOUR_VARIABLE_NAME"
        className="mb-4 w-full font-mono"
      />
      <FormLabel id="variable-value">Value</FormLabel>
      <textarea
        id="variable-value"
        rows={3}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        className="mb-4 w-full font-mono"
        style={{ resize: "vertical" }}
      />
      {scope.kind === "org" && !existing && (
        <>
          <FormLabel id="variable-visibility">Repository access</FormLabel>
          <select
            id="variable-visibility"
            value={visibility}
            onChange={(e) => setVisibility(e.target.value as GithubOrgVisibility)}
            className="mb-4 w-full"
          >
            <option value="all">All repositories</option>
            <option value="private">Private repositories</option>
            <option value="selected">Selected repositories</option>
          </select>
        </>
      )}
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <DialogActions>
        <Button onClick={onClose} disabled={mutation.isPending} variant="ghost">
          Cancel
        </Button>
        <Button
          onClick={() => {
            setError(null);
            mutation.mutate();
          }}
          disabled={mutation.isPending}
          variant="primary"
        >
          {mutation.isPending ? "Saving…" : existing ? "Update variable" : "Add variable"}
        </Button>
      </DialogActions>
    </Modal>
  );
}
