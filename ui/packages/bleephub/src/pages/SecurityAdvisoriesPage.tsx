import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchSecurityAdvisories,
  createSecurityAdvisory,
  requestCVE,
  reportVulnerability,
} from "../api.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import { RepoHeader } from "../components/Shell.js";
import { Box, Button, Modal, FormLabel, ErrorBanner, DialogActions } from "../components/ui.js";
import type {
  GithubSecurityAdvisory,
  GithubSecurityAdvisorySeverity,
  GithubSecurityAdvisoryCreatePayload,
  GithubVulnerabilityReportPayload,
} from "../types.js";

type SeverityFilter = "all" | GithubSecurityAdvisorySeverity;

const SEVERITIES: GithubSecurityAdvisorySeverity[] = ["critical", "high", "medium", "low"];

const STATE_LABELS: Record<string, string> = {
  draft: "Draft",
  published: "Published",
  closed: "Closed",
};

export function SecurityAdvisoriesPage() {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  const [severityFilter, setSeverityFilter] = useState<SeverityFilter>("all");
  const [selected, setSelected] = useState<GithubSecurityAdvisory | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [showReport, setShowReport] = useState(false);
  const counts = useOpenCounts(owner, repo);
  const queryClient = useQueryClient();

  const {
    data: advisories = [],
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["security-advisories", owner, repo],
    queryFn: () => fetchSecurityAdvisories(owner, repo),
    enabled: !!owner && !!repo,
  });

  const createMutation = useMutation({
    mutationFn: (payload: GithubSecurityAdvisoryCreatePayload) =>
      createSecurityAdvisory(owner, repo, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["security-advisories", owner, repo] });
      setShowCreate(false);
    },
  });

  const reportMutation = useMutation({
    mutationFn: (payload: GithubVulnerabilityReportPayload) => reportVulnerability(owner, repo, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["security-advisories", owner, repo] });
      setShowReport(false);
    },
  });

  const cveMutation = useMutation({
    mutationFn: (ghsaId: string) => requestCVE(owner, repo, ghsaId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["security-advisories", owner, repo] });
      if (selected) {
        queryClient.invalidateQueries({ queryKey: ["security-advisory", owner, repo, selected.ghsa_id] });
      }
    },
  });

  useEffect(() => {
    setSelected(null);
  }, [owner, repo]);

  const filtered =
    severityFilter === "all"
      ? advisories
      : advisories.filter((a) => a.severity === severityFilter);

  if (isLoading) return <Spinner label={`loading ${owner}/${repo} security advisories`} />;
  if (isError) return <InlineError title="Failed to load security advisories" detail={String(error)} />;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="security" {...counts} />

      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <label style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>Severity:</label>
          <select
            value={severityFilter}
            onChange={(e) => setSeverityFilter(e.target.value as SeverityFilter)}
            style={{ fontSize: "0.85rem", padding: "0.35rem 0.5rem" }}
          >
            <option value="all">All</option>
            {SEVERITIES.map((s) => (
              <option key={s} value={s}>
                {s[0].toUpperCase() + s.slice(1)}
              </option>
            ))}
          </select>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="secondary" size="sm" onClick={() => setShowReport(true)}>
            Report vulnerability
          </Button>
          <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
            New advisory
          </Button>
        </div>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "1rem" }}>
        <Box>
          <h3 style={{ marginTop: 0, marginBottom: "0.75rem" }}>Advisories ({filtered.length})</h3>
          {filtered.length === 0 ? (
            <p style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>No security advisories.</p>
          ) : (
            <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
              {filtered.map((advisory) => (
                <li
                  key={advisory.ghsa_id}
                  onClick={() => setSelected(advisory)}
                  style={{
                    padding: "0.6rem 0.4rem",
                    borderBottom: "1px solid var(--color-border)",
                    cursor: "pointer",
                    background: selected?.ghsa_id === advisory.ghsa_id ? "var(--color-accent-subtle)" : "transparent",
                  }}
                >
                  <div style={{ fontWeight: 600, fontSize: "0.9rem" }}>{advisory.summary}</div>
                  <div style={{ fontSize: "0.8rem", color: "var(--color-fg-muted)" }}>
                    {advisory.ghsa_id}
                    {advisory.cve_id ? ` / ${advisory.cve_id}` : ""}
                    {" · "}
                    {STATE_LABELS[advisory.state] ?? advisory.state}
                    {" · "}
                    {advisory.severity}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Box>

        <Box>
          {selected ? (
            <AdvisoryDetail
              advisory={selected}
              onRequestCVE={() => cveMutation.mutate(selected.ghsa_id)}
              cvePending={cveMutation.isPending}
              cveError={cveMutation.error}
            />
          ) : (
            <p style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>Select an advisory to view details.</p>
          )}
        </Box>
      </div>

      {showCreate && (
        <AdvisoryFormModal
          title="Create security advisory"
          onClose={() => setShowCreate(false)}
          onSubmit={(payload) => createMutation.mutate(payload)}
          pending={createMutation.isPending}
          error={createMutation.error}
        />
      )}

      {showReport && (
        <AdvisoryFormModal
          title="Report vulnerability"
          onClose={() => setShowReport(false)}
          onSubmit={(payload) =>
            reportMutation.mutate({
              summary: payload.summary,
              description: payload.description,
              severity: payload.severity,
              cwe_ids: payload.cwe_ids,
            })
          }
          pending={reportMutation.isPending}
          error={reportMutation.error}
        />
      )}
    </div>
  );
}

function AdvisoryDetail({
  advisory,
  onRequestCVE,
  cvePending,
  cveError,
}: {
  advisory: GithubSecurityAdvisory;
  onRequestCVE: () => void;
  cvePending: boolean;
  cveError: Error | null;
}) {
  return (
    <div>
      <h3 style={{ marginTop: 0, marginBottom: "0.75rem" }}>{advisory.summary}</h3>
      <div style={{ fontSize: "0.85rem", marginBottom: "0.75rem" }}>
        <div>
          <strong>GHSA:</strong> {advisory.ghsa_id}
        </div>
        {advisory.cve_id && (
          <div>
            <strong>CVE:</strong> {advisory.cve_id}
          </div>
        )}
        <div>
          <strong>State:</strong> {STATE_LABELS[advisory.state] ?? advisory.state}
        </div>
        <div>
          <strong>Severity:</strong> {advisory.severity}
        </div>
        {advisory.cwe_ids && advisory.cwe_ids.length > 0 && (
          <div>
            <strong>CWEs:</strong> {advisory.cwe_ids.join(", ")}
          </div>
        )}
        <div>
          <strong>Created:</strong> {new Date(advisory.created_at).toLocaleString()}
        </div>
        <div>
          <strong>Updated:</strong> {new Date(advisory.updated_at).toLocaleString()}
        </div>
        {advisory.published_at && (
          <div>
            <strong>Published:</strong> {new Date(advisory.published_at).toLocaleString()}
          </div>
        )}
        <div style={{ marginTop: "0.5rem", whiteSpace: "pre-wrap" }}>{advisory.description}</div>
      </div>

      {!advisory.cve_id && advisory.state !== "closed" && (
        <div className="flex flex-wrap items-center gap-2" style={{ marginBottom: "1rem" }}>
          <Button variant="secondary" size="sm" onClick={onRequestCVE} disabled={cvePending}>
            {cvePending ? "Requesting CVE…" : "Request CVE"}
          </Button>
        </div>
      )}

      {cveError && (
        <div style={{ color: "var(--color-status-error)", fontSize: "0.85rem" }}>
          {cveError instanceof Error ? cveError.message : String(cveError)}
        </div>
      )}
    </div>
  );
}

function AdvisoryFormModal({
  title,
  onClose,
  onSubmit,
  pending,
  error,
}: {
  title: string;
  onClose: () => void;
  onSubmit: (payload: GithubSecurityAdvisoryCreatePayload) => void;
  pending: boolean;
  error: Error | null;
}) {
  const [summary, setSummary] = useState("");
  const [description, setDescription] = useState("");
  const [severity, setSeverity] = useState<GithubSecurityAdvisorySeverity>("medium");
  const [cwe, setCwe] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);

  const handleSubmit = () => {
    setValidationError(null);
    if (!summary.trim()) {
      setValidationError("Summary is required.");
      return;
    }
    if (!description.trim()) {
      setValidationError("Description is required.");
      return;
    }
    const payload: GithubSecurityAdvisoryCreatePayload = {
      summary: summary.trim(),
      description: description.trim(),
      severity,
    };
    const cweIds = cwe
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    if (cweIds.length > 0) {
      payload.cwe_ids = cweIds;
    }
    onSubmit(payload);
  };

  return (
    <Modal title={title} onClose={onClose}>
      <FormLabel id="advisory-summary">Summary</FormLabel>
      <input
        id="advisory-summary"
        type="text"
        value={summary}
        onChange={(e) => setSummary(e.target.value)}
        className="mb-4 w-full"
      />

      <FormLabel id="advisory-description">Description</FormLabel>
      <textarea
        id="advisory-description"
        rows={5}
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="mb-4 w-full"
        style={{ resize: "vertical" }}
      />

      <FormLabel id="advisory-severity">Severity</FormLabel>
      <select
        id="advisory-severity"
        value={severity}
        onChange={(e) => setSeverity(e.target.value as GithubSecurityAdvisorySeverity)}
        className="mb-4 w-full"
      >
        {SEVERITIES.map((s) => (
          <option key={s} value={s}>
            {s[0].toUpperCase() + s.slice(1)}
          </option>
        ))}
      </select>

      <FormLabel id="advisory-cwe">CWE IDs (comma separated)</FormLabel>
      <input
        id="advisory-cwe"
        type="text"
        value={cwe}
        onChange={(e) => setCwe(e.target.value)}
        placeholder="CWE-79, CWE-89"
        className="mb-4 w-full"
      />

      {(validationError || error) && <ErrorBanner>{validationError ?? (error instanceof Error ? error.message : String(error))}</ErrorBanner>}

      <DialogActions>
        <Button onClick={onClose} disabled={pending} variant="ghost">
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={pending} variant="primary">
          {pending ? "Saving…" : "Submit"}
        </Button>
      </DialogActions>
    </Modal>
  );
}
