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
