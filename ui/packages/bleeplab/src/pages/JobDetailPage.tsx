import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router";
import {
  PageHeading,
  StatusBadge,
  LogViewer,
  Spinner,
  InlineError,
} from "@sockerless/ui-core/components";
import { api } from "../api.js";
import { shortSHA, bytes } from "../format.js";
import type { Job } from "../types.js";

function ArtifactsPanel({ job }: { job: Job }) {
  if (job.artifact_size <= 0) return null;
  const filename = job.artifact_filename || "artifacts.zip";
  return (
    <section className="mb-6">
      <h2 className="font-display mb-3 text-sm font-semibold uppercase tracking-wide">
        Artifacts
      </h2>
      <div
        className="flex items-center gap-3"
        style={{
          padding: "0.6rem 1rem",
          border: "1px solid var(--color-border)",
          borderRadius: "0.5rem",
          background: "var(--color-bg-subtle)",
        }}
      >
        <span style={{ fontSize: "0.86rem", fontWeight: 500, color: "var(--color-fg)" }}>
          {filename}
        </span>
        <span
          className="tabular-nums"
          style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}
        >
          {bytes(job.artifact_size)}
        </span>
        <a
          href={`/internal/jobs/${job.id}/artifact`}
          className="ml-auto inline-flex items-center gap-1"
          style={{ fontSize: "0.8rem", color: "var(--color-accent)", textDecoration: "none" }}
          download
        >
          ↓ Download
        </a>
      </div>
    </section>
  );
}

export function JobDetailPage() {
  const { id } = useParams();
  const job = useQuery({ queryKey: ["job", id], queryFn: () => api.job(id!) });

  if (job.isLoading) return <Spinner />;
  if (job.error) {
    return <InlineError title={`Failed to load job ${id}`} detail={job.error as Error} />;
  }
  const j = job.data!;
  const lines = (j.trace ?? "").split("\n");

  return (
    <div>
      <PageHeading
        kicker={`Job #${j.id} · ${j.stage}`}
        title={j.name}
        meta={`${j.ref} · ${shortSHA(j.sha)}${j.artifact_size > 0 ? ` · artifact ${bytes(j.artifact_size)}` : ""}`}
        actions={<StatusBadge status={j.status} />}
      />
      <ArtifactsPanel job={j} />
      <h2 className="font-display mb-3 text-sm font-semibold uppercase tracking-wide">
        Trace
      </h2>
      {j.trace ? (
        <LogViewer lines={lines} maxHeight="60vh" />
      ) : (
        <p style={{ color: "var(--color-fg-muted)" }}>No trace captured yet.</p>
      )}
    </div>
  );
}
