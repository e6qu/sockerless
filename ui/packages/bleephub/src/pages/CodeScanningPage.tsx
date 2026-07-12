import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchCodeScanningAlerts,
  fetchCodeScanningAlertInstances,
  fetchCodeScanningAnalyses,
  updateCodeScanningAlert,
  deleteCodeScanningAnalysis,
  uploadSARIF,
  fetchSARIFStatus,
  fetchCodeQLDatabases,
  deleteCodeQLDatabase,
  downloadCodeQLDatabase,
  fetchRepoDetail,
  fetchRepoBranch,
} from "../api.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import { RepoHeader } from "../components/Shell.js";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import { Box } from "../components/ui.js";
import type {
  GithubCodeScanningAlert,
  GithubCodeScanningAlertInstance,
  GithubCodeScanningAnalysis,
  GithubCodeScanningDismissedReason,
  GithubCodeQLDatabase,
} from "../types.js";

type FilterState = "all" | "open" | "dismissed" | "fixed";
type SeverityFilter = "all" | "error" | "warning" | "note" | "none";

const DISMISSED_REASONS: { value: GithubCodeScanningDismissedReason; label: string }[] = [
  { value: "false_positive", label: "False positive" },
  { value: "won't_fix", label: "Won't fix" },
  { value: "used_in_tests", label: "Used in tests" },
  { value: "ignored", label: "Ignored" },
];

export function CodeScanningPage() {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  const [stateFilter, setStateFilter] = useState<FilterState>("all");
  const [severityFilter, setSeverityFilter] = useState<SeverityFilter>("all");
  const [selected, setSelected] = useState<GithubCodeScanningAlert | null>(null);
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const counts = useOpenCounts(owner, repo);
  const queryClient = useQueryClient();

  const { data: repository } = useQuery({
    queryKey: ["repo", owner, repo],
    queryFn: () => fetchRepoDetail(owner, repo),
    enabled: !!owner && !!repo,
  });
  const {
    data: defaultBranch,
    isLoading: isDefaultBranchLoading,
    isError: isDefaultBranchError,
  } = useQuery({
    queryKey: ["repo-branch", owner, repo, repository?.default_branch],
    queryFn: () => fetchRepoBranch(owner, repo, repository!.default_branch),
    enabled: !!repository?.default_branch,
    retry: false,
  });

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
    queryKey: ["code-scanning", owner, repo, stateFilter, severityFilter],
    queryFn: () => fetchCodeScanningAlerts(owner, repo, filters),
    enabled: !!owner && !!repo,
  });

  const { data: instances = [] } = useQuery({
    queryKey: ["code-scanning-instances", owner, repo, selected?.number],
    queryFn: () => fetchCodeScanningAlertInstances(owner, repo, selected!.number),
    enabled: !!selected,
  });

  const { data: analyses = [] } = useQuery({
    queryKey: ["code-scanning-analyses", owner, repo],
    queryFn: () => fetchCodeScanningAnalyses(owner, repo),
    enabled: !!owner && !!repo,
  });

  const { data: databases = [], isError: isDatabaseError } = useQuery({
    queryKey: ["codeql-databases", owner, repo],
    queryFn: () => fetchCodeQLDatabases(owner, repo),
    enabled: !!owner && !!repo,
  });

  const updateMutation = useMutation({
    mutationFn: (payload: {
      number: number;
      state: "open" | "dismissed" | "fixed";
      dismissed_reason?: GithubCodeScanningDismissedReason;
      dismissed_comment?: string;
    }) => updateCodeScanningAlert(owner, repo, payload.number, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["code-scanning", owner, repo] });
      queryClient.invalidateQueries({ queryKey: ["code-scanning-analyses", owner, repo] });
      if (selected) {
        queryClient.invalidateQueries({ queryKey: ["code-scanning-instances", owner, repo, selected.number] });
      }
    },
  });

  const uploadMutation = useMutation({
    mutationFn: async (file: File) => {
	  if (!repository || !defaultBranch) throw new Error("The default branch has no commit to analyze.");
      const text = await readFileText(file);
      const res = await uploadSARIF(owner, repo, {
		commit_sha: defaultBranch.commit.sha,
		ref: `refs/heads/${repository.default_branch}`,
		sarif: utf8Base64(text),
      });
      return res;
    },
    onSuccess: async (res) => {
      setUploadFile(null);
      setUploadError(null);
      await fetchSARIFStatus(owner, repo, res.id);
      queryClient.invalidateQueries({ queryKey: ["code-scanning", owner, repo] });
      queryClient.invalidateQueries({ queryKey: ["code-scanning-analyses", owner, repo] });
    },
    onError: (err: Error) => {
      setUploadError(err.message);
    },
  });

  const deleteDatabaseMutation = useMutation({
    mutationFn: (language: string) => deleteCodeQLDatabase(owner, repo, language),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["codeql-databases", owner, repo] }),
  });

  const downloadDatabaseMutation = useMutation({
    mutationFn: async (database: GithubCodeQLDatabase) => ({
      database,
      blob: await downloadCodeQLDatabase(owner, repo, database.language),
    }),
    onSuccess: ({ database, blob }) => {
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      const name = database.name || `${database.language}-database`;
      link.download = name.toLowerCase().endsWith(".zip") ? name : `${name}.zip`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    },
  });

  useEffect(() => {
    setSelected(null);
  }, [owner, repo]);

  if (isLoading) return <Spinner label={`loading ${owner}/${repo} code scanning`} />;
  if (isError) return <InlineError title="Failed to load code scanning alerts" detail={String(error)} />;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="security" {...counts} />

      <section className="security-hero" aria-labelledby="code-scanning-title">
        <div className="security-hero-icon" aria-hidden="true">◈</div>
        <div>
          <p className="security-eyebrow">Security · Code security</p>
          <h1 id="code-scanning-title">Code scanning</h1>
          <p>Find vulnerable patterns, inspect analyses, and manage the CodeQL databases produced from this repository.</p>
        </div>
        <div className="security-summary" aria-label="Code scanning summary">
          <span><strong>{alerts.filter((alert) => alert.state === "open").length}</strong> open</span>
          <span><strong>{analyses.length}</strong> analyses</span>
          <span><strong>{databases.length}</strong> databases</span>
        </div>
      </section>

      <div className="security-filter-bar">
        <div className="security-filter-group">
          <label htmlFor="code-state-filter">State</label>
          <select id="code-state-filter" value={stateFilter} onChange={(e) => setStateFilter(e.target.value as FilterState)}>
            <option value="all">All</option><option value="open">Open</option><option value="dismissed">Dismissed</option><option value="fixed">Fixed</option>
          </select>
        </div>
        <div className="security-filter-group">
          <label htmlFor="code-severity-filter">Severity</label>
          <select id="code-severity-filter" value={severityFilter} onChange={(e) => setSeverityFilter(e.target.value as SeverityFilter)}>
            <option value="all">All</option><option value="error">Error</option><option value="warning">Warning</option><option value="note">Note</option><option value="none">None</option>
          </select>
        </div>
        <span className="security-coordinate">
          {defaultBranch ? <><code>{repository?.default_branch}</code><span>{defaultBranch.commit.sha.slice(0, 7)}</span></> : "No default-branch commit"}
        </span>
      </div>

      <div className="security-alert-grid">
        <Box className="security-panel">
          <div className="security-panel-heading"><div><span className="security-kicker pink">Findings</span><h2>Alerts</h2></div><span className="security-count">{alerts.length}</span></div>
          {alerts.length === 0 ? (
            <div className="security-empty"><strong>No code scanning alerts</strong><span>New results from SARIF analyses appear here.</span></div>
          ) : (
            <ul className="security-list">
              {alerts.map((alert) => (
                <li key={alert.number}>
                  <button type="button" className={selected?.number === alert.number ? "security-alert-row selected" : "security-alert-row"} onClick={() => setSelected(alert)}>
                    <span className={`security-severity ${alert.rule.severity ?? "none"}`} aria-hidden="true" />
                    <span><strong>#{alert.number} {alert.rule.name}</strong><small>
                    {alert.state}
                    {alert.dismissed_reason ? ` — ${alert.dismissed_reason}` : ""}
                    {alert.rule.severity ? ` · ${alert.rule.severity}` : ""}
                    </small></span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </Box>

        <Box className="security-panel security-detail-panel">
          {selected ? (
            <AlertDetail
              alert={selected}
              instances={instances}
              onDismiss={(reason, comment) =>
                updateMutation.mutate({ number: selected.number, state: "dismissed", dismissed_reason: reason, dismissed_comment: comment })
              }
              onReopen={() => updateMutation.mutate({ number: selected.number, state: "open" })}
              onFix={() => updateMutation.mutate({ number: selected.number, state: "fixed" })}
            />
          ) : (
            <div className="security-empty"><strong>Select an alert</strong><span>Review its rule, source location, and resolution controls.</span></div>
          )}
        </Box>
      </div>

      <Box className="security-panel security-database-panel mt-4">
        <div className="security-panel-heading"><div><span className="security-kicker purple">CodeQL</span><h2>Databases</h2><p>Relocatable databases uploaded by the CodeQL analysis workflow on the default branch.</p></div><span className="security-count">{databases.length}</span></div>
        {isDatabaseError ? <InlineError title="Failed to load CodeQL databases" detail="The database API did not return a usable response." /> : databases.length === 0 ? (
          <div className="security-empty database"><strong>No CodeQL databases yet</strong><span>Run <code>github/codeql-action/analyze</code> with database upload enabled on the default branch.</span></div>
        ) : (
          <div className="security-database-list">
            {databases.map((database) => (
              <article className="security-database-row" key={database.id}>
                <span className="security-language-orb" aria-hidden="true">{database.language.slice(0, 2).toUpperCase()}</span>
                <div className="security-database-copy">
                  <strong>{database.name}</strong>
                  <span>{database.language} · {formatBytes(database.size)} · updated {formatDate(database.updated_at)}</span>
                  {database.commit_oid && <code>{database.commit_oid.slice(0, 12)}</code>}
                </div>
                <div className="security-row-actions">
                  <button type="button" className="security-button" onClick={() => downloadDatabaseMutation.mutate(database)} disabled={downloadDatabaseMutation.isPending}>Download</button>
                  <button type="button" className="security-button danger" aria-label={`Delete ${database.language} CodeQL database`} onClick={() => deleteDatabaseMutation.mutate(database.language)} disabled={deleteDatabaseMutation.isPending}>Delete</button>
                </div>
              </article>
            ))}
          </div>
        )}
        {(deleteDatabaseMutation.isError || downloadDatabaseMutation.isError) && <p className="security-inline-error">{String(deleteDatabaseMutation.error || downloadDatabaseMutation.error)}</p>}
      </Box>

      <Box className="security-panel mt-4">
        <div className="security-panel-heading"><div><span className="security-kicker cyan">Results</span><h2>Analyses</h2></div><span className="security-count">{analyses.length}</span></div>
        {analyses.length === 0 ? (
          <div className="security-empty"><strong>No analyses</strong><span>Upload a SARIF result or run a CodeQL workflow.</span></div>
        ) : (
          <ul className="security-list">
            {analyses.map((analysis) => (
              <AnalysisItem key={analysis.id} analysis={analysis} owner={owner} repo={repo} />
            ))}
          </ul>
        )}

        <div className="security-upload">
          <div><strong>Upload SARIF</strong><span>{defaultBranch ? <>Results attach to <code>{repository?.default_branch}@{defaultBranch.commit.sha.slice(0, 7)}</code>.</> : "Create a default-branch commit before uploading results."}</span></div>
          <input
            aria-label="SARIF file"
            type="file"
            accept=".sarif,.json"
            onChange={(e) => setUploadFile(e.target.files?.[0] ?? null)}
          />
          <button
            type="button"
            className="security-primary-button"
            disabled={!uploadFile || !defaultBranch || isDefaultBranchLoading || uploadMutation.isPending}
            onClick={() => uploadFile && uploadMutation.mutate(uploadFile)}
          >
            {uploadMutation.isPending ? "Uploading…" : "Upload SARIF"}
          </button>
          {(uploadError || isDefaultBranchError) && <div className="security-inline-error">{uploadError || "The default branch does not have a readable head commit."}</div>}
        </div>
      </Box>
    </div>
  );
}

function utf8Base64(value: string): string {
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (let offset = 0; offset < bytes.length; offset += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + 0x8000));
  }
  return btoa(binary);
}

function readFileText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(reader.error ?? new Error("Unable to read the SARIF file."));
    reader.readAsText(file, "utf-8");
  });
}

function formatBytes(size: number): string {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium" }).format(new Date(value));
}

function AnalysisItem({
  analysis,
  owner,
  repo,
}: {
  analysis: GithubCodeScanningAnalysis;
  owner: string;
  repo: string;
}) {
  const queryClient = useQueryClient();
  const deleteMutation = useMutation({
    mutationFn: () => deleteCodeScanningAnalysis(owner, repo, analysis.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["code-scanning-analyses", owner, repo] });
    },
  });

  return (
    <li style={{ fontSize: "0.85rem", padding: "0.4rem 0", borderBottom: "1px solid var(--color-border)" }}>
      <div>
        <strong>#{analysis.id}</strong> {analysis.tool.name || "unknown tool"} · {analysis.ref}
      </div>
      <div style={{ color: "var(--color-fg-muted)" }}>
        {analysis.results_count} results · {analysis.rules_count} rules
      </div>
      <button
        type="button"
        onClick={() => deleteMutation.mutate()}
        disabled={deleteMutation.isPending}
        style={{ fontSize: "0.8rem", padding: "0.2rem 0.5rem", marginTop: "0.25rem" }}
      >
        Delete
      </button>
    </li>
  );
}

function AlertDetail({
  alert,
  instances,
  onDismiss,
  onReopen,
  onFix,
}: {
  alert: GithubCodeScanningAlert;
  instances: GithubCodeScanningAlertInstance[];
  onDismiss: (reason: GithubCodeScanningDismissedReason, comment: string) => void;
  onReopen: () => void;
  onFix: () => void;
}) {
  const [reason, setReason] = useState<GithubCodeScanningDismissedReason>("false_positive");
  const [comment, setComment] = useState("");

  return (
    <div>
      <h3 style={{ marginTop: 0, marginBottom: "0.75rem" }}>Alert #{alert.number}</h3>
      <div style={{ fontSize: "0.85rem", marginBottom: "0.75rem" }}>
        <div>
          <strong>Rule:</strong> {alert.rule.name}
        </div>
        <div>
          <strong>State:</strong> {alert.state}
        </div>
        {alert.rule.severity && (
          <div>
            <strong>Severity:</strong> {alert.rule.severity}
          </div>
        )}
        {alert.dismissed_reason && (
          <div>
            <strong>Dismissed reason:</strong> {alert.dismissed_reason}
          </div>
        )}
        <div>
          <strong>Tool:</strong> {alert.tool.name || "unknown"}
        </div>
      </div>

      {alert.state === "open" ? (
        <div style={{ marginBottom: "1rem" }}>
          <label style={{ fontSize: "0.85rem" }}>Dismissed reason</label>
          <select
            value={reason}
            onChange={(e) => setReason(e.target.value as GithubCodeScanningDismissedReason)}
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
          <div className="flex gap-2">
            <button type="button" onClick={() => onDismiss(reason, comment)} style={{ fontSize: "0.85rem", padding: "0.4rem 0.8rem" }}>
              Dismiss
            </button>
            <button type="button" onClick={onFix} style={{ fontSize: "0.85rem", padding: "0.4rem 0.8rem" }}>
              Mark fixed
            </button>
          </div>
        </div>
      ) : (
        <div style={{ marginBottom: "1rem" }}>
          <button type="button" onClick={onReopen} style={{ fontSize: "0.85rem", padding: "0.4rem 0.8rem" }}>
            Reopen
          </button>
        </div>
      )}

      <h4 style={{ fontSize: "0.9rem", marginBottom: "0.5rem" }}>Instances ({instances.length})</h4>
      {instances.length === 0 ? (
        <p style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>No instances.</p>
      ) : (
        <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
          {instances.map((inst, idx) => (
            <li key={idx} style={{ fontSize: "0.85rem", padding: "0.4rem 0", borderBottom: "1px solid var(--color-border)" }}>
              <div>
                <strong>{inst.location.path || "—"}</strong>
              </div>
              <div style={{ color: "var(--color-fg-muted)" }}>
                lines {inst.location.start_line}–{inst.location.end_line}, columns {inst.location.start_column}–
                {inst.location.end_column}
              </div>
              <div style={{ color: "var(--color-fg-muted)" }}>commit {inst.commit_sha.slice(0, 7)}</div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
