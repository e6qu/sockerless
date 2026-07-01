import { useEffect, useState } from "react";
import { useParams } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchSecretScanningAlerts,
  fetchSecretScanningAlert,
  fetchSecretScanningAlertLocations,
  updateSecretScanningAlert,
} from "../api.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import { RepoHeader } from "../components/Shell.js";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import { Box } from "../components/ui.js";
import type {
  GithubSecretScanningAlert,
  GithubSecretScanningLocation,
  GithubSecretScanningResolution,
} from "../types.js";

type FilterState = "all" | "open" | "resolved";

const RESOLUTIONS: { value: GithubSecretScanningResolution; label: string }[] = [
  { value: "false_positive", label: "False positive" },
  { value: "wont_fix", label: "Won't fix" },
  { value: "revoked", label: "Revoked" },
  { value: "used_in_tests", label: "Used in tests" },
  { value: "pattern_deleted", label: "Pattern deleted" },
  { value: "pattern_edited", label: "Pattern edited" },
];

export function SecretScanningPage() {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  const [filter, setFilter] = useState<FilterState>("all");
  const [selected, setSelected] = useState<GithubSecretScanningAlert | null>(null);
  const counts = useOpenCounts(owner, repo);
  const queryClient = useQueryClient();

  const filters = { state: filter === "all" ? undefined : filter };
  const {
    data: alerts = [],
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["secret-scanning", owner, repo, filter],
    queryFn: () => fetchSecretScanningAlerts(owner, repo, filters),
    enabled: !!owner && !!repo,
  });

  const { data: locations = [] } = useQuery({
    queryKey: ["secret-scanning-locations", owner, repo, selected?.number],
    queryFn: () => fetchSecretScanningAlertLocations(owner, repo, selected!.number),
    enabled: !!selected,
  });

  const resolveMutation = useMutation({
    mutationFn: (payload: {
      number: number;
      state: "open" | "resolved";
      resolution?: GithubSecretScanningResolution;
      resolution_comment?: string;
    }) => updateSecretScanningAlert(owner, repo, payload.number, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["secret-scanning", owner, repo] });
      if (selected) {
        queryClient.invalidateQueries({ queryKey: ["secret-scanning-locations", owner, repo, selected.number] });
      }
    },
  });

  useEffect(() => {
    setSelected(null);
  }, [owner, repo]);

  if (isLoading) return <Spinner label={`loading ${owner}/${repo} secret scanning`} />;
  if (isError) return <InlineError title="Failed to load secret scanning alerts" detail={String(error)} />;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="security" {...counts} />

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <label style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>State:</label>
        <select
          value={filter}
          onChange={(e) => setFilter(e.target.value as FilterState)}
          style={{ fontSize: "0.85rem", padding: "0.35rem 0.5rem" }}
        >
          <option value="all">All</option>
          <option value="open">Open</option>
          <option value="resolved">Resolved</option>
        </select>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "1rem" }}>
        <Box>
          <h3 style={{ marginTop: 0, marginBottom: "0.75rem" }}>Alerts ({alerts.length})</h3>
          {alerts.length === 0 ? (
            <p style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>No secret scanning alerts.</p>
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
                    #{alert.number} {alert.secret_type_display_name}
                  </div>
                  <div style={{ fontSize: "0.8rem", color: "var(--color-fg-muted)" }}>
                    {alert.state}
                    {alert.resolution ? ` — ${alert.resolution}` : ""}
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
              locations={locations}
              onResolve={(resolution, comment) =>
                resolveMutation.mutate({ number: selected.number, state: "resolved", resolution, resolution_comment: comment })
              }
              onReopen={() => resolveMutation.mutate({ number: selected.number, state: "open" })}
            />
          ) : (
            <p style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>Select an alert to view details.</p>
          )}
        </Box>
      </div>
    </div>
  );
}

function AlertDetail({
  alert,
  locations,
  onResolve,
  onReopen,
}: {
  alert: GithubSecretScanningAlert;
  locations: GithubSecretScanningLocation[];
  onResolve: (resolution: GithubSecretScanningResolution, comment: string) => void;
  onReopen: () => void;
}) {
  const [resolution, setResolution] = useState<GithubSecretScanningResolution>("false_positive");
  const [comment, setComment] = useState("");

  return (
    <div>
      <h3 style={{ marginTop: 0, marginBottom: "0.75rem" }}>Alert #{alert.number}</h3>
      <div style={{ fontSize: "0.85rem", marginBottom: "0.75rem" }}>
        <div>
          <strong>Type:</strong> {alert.secret_type_display_name}
        </div>
        <div>
          <strong>State:</strong> {alert.state}
        </div>
        {alert.resolution && (
          <div>
            <strong>Resolution:</strong> {alert.resolution}
          </div>
        )}
        <div>
          <strong>Created:</strong> {new Date(alert.created_at).toLocaleString()}
        </div>
      </div>

      {alert.state === "open" ? (
        <div style={{ marginBottom: "1rem" }}>
          <label style={{ fontSize: "0.85rem" }}>Resolution</label>
          <select
            value={resolution}
            onChange={(e) => setResolution(e.target.value as GithubSecretScanningResolution)}
            style={{ fontSize: "0.85rem", padding: "0.35rem 0.5rem", display: "block", marginBottom: "0.5rem" }}
          >
            {RESOLUTIONS.map((r) => (
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
            onClick={() => onResolve(resolution, comment)}
            style={{ fontSize: "0.85rem", padding: "0.4rem 0.8rem" }}
          >
            Resolve
          </button>
        </div>
      ) : (
        <div style={{ marginBottom: "1rem" }}>
          <button type="button" onClick={onReopen} style={{ fontSize: "0.85rem", padding: "0.4rem 0.8rem" }}>
            Reopen
          </button>
        </div>
      )}

      <h4 style={{ fontSize: "0.9rem", marginBottom: "0.5rem" }}>Locations ({locations.length})</h4>
      {locations.length === 0 ? (
        <p style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>No locations.</p>
      ) : (
        <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
          {locations.map((loc, idx) => (
            <li key={idx} style={{ fontSize: "0.85rem", padding: "0.4rem 0", borderBottom: "1px solid var(--color-border)" }}>
              <div>
                <strong>{loc.details.path}</strong>
              </div>
              <div style={{ color: "var(--color-fg-muted)" }}>
                lines {loc.details.start_line}–{loc.details.end_line}, columns {loc.details.start_column}–
                {loc.details.end_column}
              </div>
              <div style={{ color: "var(--color-fg-muted)" }}>commit {loc.details.commit_sha.slice(0, 7)}</div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
