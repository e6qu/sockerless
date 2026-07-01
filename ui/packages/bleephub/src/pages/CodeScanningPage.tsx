import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchCodeScanningAlerts,
  fetchCodeScanningAlert,
  fetchCodeScanningAlertInstances,
  fetchCodeScanningAnalyses,
  updateCodeScanningAlert,
  deleteCodeScanningAnalysis,
  uploadSARIF,
  fetchSARIFStatus,
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
      const text = await file.text();
      const commitSha = "0000000000000000000000000000000000000000";
      const res = await uploadSARIF(owner, repo, {
        commit_sha: commitSha,
        ref: "refs/heads/main",
        sarif: btoa(text),
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

  useEffect(() => {
    setSelected(null);
  }, [owner, repo]);

  if (isLoading) return <Spinner label={`loading ${owner}/${repo} code scanning`} />;
  if (isError) return <InlineError title="Failed to load code scanning alerts" detail={String(error)} />;

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
        </select>

        <label style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)", marginLeft: "0.5rem" }}>Severity:</label>
        <select
          value={severityFilter}
          onChange={(e) => setSeverityFilter(e.target.value as SeverityFilter)}
          style={{ fontSize: "0.85rem", padding: "0.35rem 0.5rem" }}
        >
          <option value="all">All</option>
          <option value="error">Error</option>
          <option value="warning">Warning</option>
          <option value="note">Note</option>
          <option value="none">None</option>
        </select>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "1rem" }}>
        <Box>
          <h3 style={{ marginTop: 0, marginBottom: "0.75rem" }}>Alerts ({alerts.length})</h3>
          {alerts.length === 0 ? (
            <p style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>No code scanning alerts.</p>
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
                    #{alert.number} {alert.rule.name}
                  </div>
                  <div style={{ fontSize: "0.8rem", color: "var(--color-fg-muted)" }}>
                    {alert.state}
                    {alert.dismissed_reason ? ` — ${alert.dismissed_reason}` : ""}
                    {alert.rule.severity ? ` · ${alert.rule.severity}` : ""}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Box>

        <Box>
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
            <p style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>Select an alert to view details.</p>
          )}
        </Box>
      </div>

      <Box className="mt-4">
        <h3 style={{ marginTop: 0, marginBottom: "0.75rem" }}>Analyses ({analyses.length})</h3>
        {analyses.length === 0 ? (
          <p style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>No analyses.</p>
        ) : (
          <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
            {analyses.map((analysis) => (
              <AnalysisItem key={analysis.id} analysis={analysis} owner={owner} repo={repo} />
            ))}
          </ul>
        )}

        <div style={{ marginTop: "1rem" }}>
          <label style={{ fontSize: "0.85rem", display: "block", marginBottom: "0.5rem" }}>Upload SARIF</label>
          <input
            type="file"
            accept=".sarif,.json"
            onChange={(e) => setUploadFile(e.target.files?.[0] ?? null)}
            style={{ fontSize: "0.85rem", marginBottom: "0.5rem" }}
          />
          <button
            type="button"
            disabled={!uploadFile || uploadMutation.isPending}
            onClick={() => uploadFile && uploadMutation.mutate(uploadFile)}
            style={{ fontSize: "0.85rem", padding: "0.4rem 0.8rem" }}
          >
            {uploadMutation.isPending ? "Uploading..." : "Upload"}
          </button>
          {uploadError && <div style={{ color: "var(--color-danger-fg)", fontSize: "0.85rem", marginTop: "0.5rem" }}>{uploadError}</div>}
        </div>
      </Box>
    </div>
  );
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
