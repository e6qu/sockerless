import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchDependabotAlerts,
  fetchDependabotAlert,
  updateDependabotAlert,
  fetchDependabotRepoSecrets,
  fetchDependabotRepoPublicKey,
  putDependabotRepoSecret,
  deleteDependabotRepoSecret,
} from "../api.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import { RepoHeader } from "../components/Shell.js";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import { Box, Button, FormLabel, ErrorBanner } from "../components/ui.js";
import { LockIcon } from "../components/octicons.js";
import { sealSecret } from "../utils/sealedBox.js";
import type {
  GithubDependabotAlert,
  GithubDependabotDismissedReason,
  GithubDependabotSecret,
} from "../types.js";

type FilterState = "all" | "open" | "dismissed" | "fixed" | "auto_dismissed";
type SeverityFilter = "all" | "critical" | "high" | "medium" | "low";

const DISMISSED_REASONS: { value: GithubDependabotDismissedReason; label: string }[] = [
  { value: "fix_started", label: "Fix started" },
  { value: "inaccurate", label: "Inaccurate" },
  { value: "no_bandwidth", label: "No bandwidth" },
  { value: "not_used", label: "Not used" },
  { value: "tolerable_risk", label: "Tolerable risk" },
];

export function DependabotPage() {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  const [stateFilter, setStateFilter] = useState<FilterState>("all");
  const [severityFilter, setSeverityFilter] = useState<SeverityFilter>("all");
  const [selected, setSelected] = useState<GithubDependabotAlert | null>(null);
  const counts = useOpenCounts(owner, repo);
  const queryClient = useQueryClient();

  const filters = {
    state: stateFilter === "all" ? undefined : stateFilter,
    severity: severityFilter === "all" ? undefined : severityFilter,
  };
  const {
    data: alerts = [],
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["dependabot", owner, repo, stateFilter, severityFilter],
    queryFn: () => fetchDependabotAlerts(owner, repo, filters),
    enabled: !!owner && !!repo,
  });

  const { data: selectedDetail } = useQuery({
    queryKey: ["dependabot-alert", owner, repo, selected?.number],
    queryFn: () => fetchDependabotAlert(owner, repo, selected!.number),
    enabled: !!selected,
  });

  const updateMutation = useMutation({
    mutationFn: (payload: {
      number: number;
      state: "open" | "dismissed";
      dismissed_reason?: GithubDependabotDismissedReason;
      dismissed_comment?: string;
    }) => updateDependabotAlert(owner, repo, payload.number, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["dependabot", owner, repo] });
      if (selected) {
        queryClient.invalidateQueries({ queryKey: ["dependabot-alert", owner, repo, selected.number] });
      }
    },
  });

  useEffect(() => {
    setSelected(null);
  }, [owner, repo]);

  if (isLoading) return <Spinner label={`loading ${owner}/${repo} dependabot`} />;
  if (isError) return <InlineError title="Failed to load Dependabot alerts" detail={String(error)} />;

  const activeAlert = selectedDetail ?? selected;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="security" {...counts} />

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <label style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>State:</label>
        <select
          value={stateFilter}
          onChange={(e) => setStateFilter(e.target.value as FilterState)}
          style={{ fontSize: "0.85rem", padding: "0.35rem 0.5rem" }}
        >
          <option value="all">All</option>
          <option value="open">Open</option>
          <option value="dismissed">Dismissed</option>
          <option value="fixed">Fixed</option>
          <option value="auto_dismissed">Auto-dismissed</option>
        </select>

        <label style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)", marginLeft: "0.5rem" }}>Severity:</label>
        <select
          value={severityFilter}
          onChange={(e) => setSeverityFilter(e.target.value as SeverityFilter)}
          style={{ fontSize: "0.85rem", padding: "0.35rem 0.5rem" }}
        >
          <option value="all">All</option>
          <option value="critical">Critical</option>
          <option value="high">High</option>
          <option value="medium">Medium</option>
          <option value="low">Low</option>
        </select>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "1rem" }}>
        <Box>
          <h3 style={{ marginTop: 0, marginBottom: "0.75rem" }}>Alerts ({alerts.length})</h3>
          {alerts.length === 0 ? (
            <p style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>No Dependabot alerts.</p>
          ) : (
            <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
              {alerts.map((alert) => (
                <li
                  key={alert.number}
                  onClick={() => setSelected(alert)}
                  style={{
                    padding: "0.6rem 0.4rem",
                    borderBottom: "1px solid var(--color-border)",
                    cursor: "pointer",
                    background: selected?.number === alert.number ? "var(--color-accent-subtle)" : "transparent",
                  }}
                >
                  <div style={{ fontWeight: 600, fontSize: "0.9rem" }}>
                    #{alert.number} {alert.security_advisory.summary}
                  </div>
                  <div style={{ fontSize: "0.8rem", color: "var(--color-fg-muted)" }}>
                    {alert.state}
                    {alert.dismissed_reason ? ` — ${alert.dismissed_reason}` : ""}
                    {` · ${alert.security_vulnerability.severity}`}
                    {` · ${alert.dependency.package.ecosystem}`}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Box>

        <Box>
          {activeAlert ? (
            <AlertDetail
              alert={activeAlert}
              onDismiss={(reason, comment) =>
                updateMutation.mutate({
                  number: activeAlert.number,
                  state: "dismissed",
                  dismissed_reason: reason,
                  dismissed_comment: comment,
                })
              }
              onReopen={() => updateMutation.mutate({ number: activeAlert.number, state: "open" })}
            />
          ) : (
            <p style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>Select an alert to view details.</p>
          )}
        </Box>
      </div>

      <RepoSecretsSection owner={owner} repo={repo} />
    </div>
  );
}

function AlertDetail({
  alert,
  onDismiss,
  onReopen,
}: {
  alert: GithubDependabotAlert;
  onDismiss: (reason: GithubDependabotDismissedReason, comment: string) => void;
  onReopen: () => void;
}) {
  const [reason, setReason] = useState<GithubDependabotDismissedReason>("fix_started");
  const [comment, setComment] = useState("");

  return (
    <div>
      <h3 style={{ marginTop: 0, marginBottom: "0.75rem" }}>Alert #{alert.number}</h3>
      <div style={{ fontSize: "0.85rem", marginBottom: "0.75rem" }}>
        <div>
          <strong>Package:</strong> {alert.dependency.package.name} ({alert.dependency.package.ecosystem})
        </div>
        <div>
          <strong>Manifest:</strong> {alert.dependency.manifest_path}
        </div>
        <div>
          <strong>State:</strong> {alert.state}
        </div>
        <div>
          <strong>Severity:</strong> {alert.security_vulnerability.severity}
        </div>
        <div>
          <strong>Advisory:</strong> {alert.security_advisory.ghsa_id}
        </div>
        {alert.security_advisory.cve_id && (
          <div>
            <strong>CVE:</strong> {alert.security_advisory.cve_id}
          </div>
        )}
        <div>
          <strong>Affected range:</strong> {alert.security_vulnerability.vulnerable_version_range}
        </div>
        {alert.security_vulnerability.first_patched_version && (
          <div>
            <strong>Patched in:</strong> {alert.security_vulnerability.first_patched_version.identifier}
          </div>
        )}
        <div style={{ marginTop: "0.5rem" }}>{alert.security_advisory.description}</div>
      </div>

      {alert.state === "open" ? (
        <div style={{ marginBottom: "1rem" }}>
          <label style={{ fontSize: "0.85rem" }}>Dismiss reason</label>
          <select
            value={reason}
            onChange={(e) => setReason(e.target.value as GithubDependabotDismissedReason)}
            style={{ fontSize: "0.85rem", padding: "0.35rem 0.5rem", display: "block", marginBottom: "0.5rem" }}
          >
            {DISMISSED_REASONS.map((r) => (
              <option key={r.value} value={r.value}>
                {r.label}
              </option>
            ))}
          </select>
          <input
            type="text"
            placeholder="Comment (optional)"
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            style={{ fontSize: "0.85rem", padding: "0.35rem 0.5rem", width: "100%", marginBottom: "0.5rem" }}
          />
          <button
            type="button"
            onClick={() => onDismiss(reason, comment)}
            style={{ fontSize: "0.85rem", padding: "0.4rem 0.8rem" }}
          >
            Dismiss
          </button>
        </div>
      ) : (
        <div style={{ marginBottom: "1rem" }}>
          <button type="button" onClick={onReopen} style={{ fontSize: "0.85rem", padding: "0.4rem 0.8rem" }}>
            Reopen
          </button>
        </div>
      )}
    </div>
  );
}

function RepoSecretsSection({ owner, repo }: { owner: string; repo: string }) {
  const qc = useQueryClient();
  const [creating, setCreating] = useState(false);

  const secretsQ = useQuery({
    queryKey: ["dependabot-secrets", owner, repo],
    queryFn: () => fetchDependabotRepoSecrets(owner, repo),
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => deleteDependabotRepoSecret(owner, repo, name),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["dependabot-secrets", owner, repo] }),
  });

  return (
    <Box className="mt-4">
      <div className="mb-2 flex items-center justify-between">
        <h3 style={{ margin: 0 }}>Dependabot secrets</h3>
        <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
          New secret
        </Button>
      </div>
      {secretsQ.isLoading && <Spinner label="loading secrets" />}
      {secretsQ.isError && <InlineError title="Failed to load secrets" detail={String(secretsQ.error)} />}
      {deleteMutation.isError && <ErrorBanner>{String(deleteMutation.error)}</ErrorBanner>}
      {secretsQ.data &&
        (secretsQ.data.items.length === 0 ? (
          <p style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>No Dependabot secrets configured.</p>
        ) : (
          <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
            {secretsQ.data.items.map((s) => (
              <SecretRow
                key={s.name}
                owner={owner}
                repo={repo}
                secret={s}
                onDelete={() => deleteMutation.mutate(s.name)}
              />
            ))}
          </ul>
        ))}
      {creating && (
        <SecretModal owner={owner} repo={repo} existingName={null} onClose={() => setCreating(false)} />
      )}
    </Box>
  );
}

function SecretRow({
  owner,
  repo,
  secret,
  onDelete,
}: {
  owner: string;
  repo: string;
  secret: GithubDependabotSecret;
  onDelete: () => void;
}) {
  const [editing, setEditing] = useState(false);
  return (
    <li
      className="flex items-center gap-2"
      style={{ padding: "0.55rem 0", borderBottom: "1px solid var(--color-border)" }}
    >
      <LockIcon size={14} style={{ color: "var(--color-fg-muted)" }} />
      <span className="min-w-0 flex-1 truncate font-mono" style={{ fontSize: "0.84rem" }}>
        {secret.name}
      </span>
      <span style={{ fontSize: "0.74rem", color: "var(--color-fg-muted)" }}>
        updated {new Date(secret.updated_at).toLocaleDateString()}
      </span>
      <Button variant="ghost" size="sm" onClick={() => setEditing(true)}>
        Update
      </Button>
      <Button variant="danger" size="sm" onClick={onDelete}>
        Delete
      </Button>
      {editing && <SecretModal owner={owner} repo={repo} existingName={secret.name} onClose={() => setEditing(false)} />}
    </li>
  );
}

function SecretModal({
  owner,
  repo,
  existingName,
  onClose,
}: {
  owner: string;
  repo: string;
  existingName: string | null;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState(existingName ?? "");
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: async () => {
      const secretName = name.trim();
      if (!secretName) throw new Error("Name is required");
      if (!value) throw new Error("Value is required");
      const pk = await fetchDependabotRepoPublicKey(owner, repo);
      const encrypted = await sealSecret(value, pk.key);
      await putDependabotRepoSecret(owner, repo, secretName, { encrypted_value: encrypted, key_id: pk.key_id });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["dependabot-secrets", owner, repo] });
      onClose();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <div className="mt-3 rounded border p-3" style={{ borderColor: "var(--color-border)", background: "var(--color-bg-subtle)" }}>
      {existingName === null && (
        <>
          <FormLabel id="dependabot-secret-name">Name</FormLabel>
          <input
            id="dependabot-secret-name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="YOUR_SECRET_NAME"
            className="mb-3 w-full font-mono"
          />
        </>
      )}
      <FormLabel id="dependabot-secret-value">Value</FormLabel>
      <textarea
        id="dependabot-secret-value"
        rows={3}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        className="mb-3 w-full font-mono"
        style={{ resize: "vertical" }}
      />
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <div className="flex gap-2">
        <Button size="sm" onClick={() => mutation.mutate()} disabled={mutation.isPending}>
          {mutation.isPending ? "Saving..." : existingName ? "Update" : "Create"}
        </Button>
        <Button variant="ghost" size="sm" onClick={onClose}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
