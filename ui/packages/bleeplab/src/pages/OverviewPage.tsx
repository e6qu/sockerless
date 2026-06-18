import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import {
  PageHeading,
  MetricsCard,
  DataTable,
  StatusBadge,
  Spinner,
  InlineError,
} from "@sockerless/ui-core/components";
import type { ColumnDef } from "@tanstack/react-table";
import { api } from "../api.js";
import type { Pipeline } from "../types.js";
import { shortSHA } from "../format.js";

const pipelineColumns: ColumnDef<Pipeline, unknown>[] = [
  { accessorKey: "id", header: "#" },
  { accessorKey: "project_name", header: "Project" },
  { accessorKey: "ref", header: "Ref" },
  {
    accessorKey: "sha",
    header: "Commit",
    cell: (c) => <code>{shortSHA(String(c.getValue()))}</code>,
  },
  { accessorKey: "jobs", header: "Jobs" },
  {
    accessorKey: "status",
    header: "Status",
    cell: (c) => <StatusBadge status={String(c.getValue())} />,
  },
];

export function OverviewPage() {
  const navigate = useNavigate();
  const status = useQuery({ queryKey: ["status"], queryFn: api.status });
  const pipelines = useQuery({ queryKey: ["pipelines"], queryFn: api.pipelines });
  const storage = useQuery({ queryKey: ["storage"], queryFn: api.storage });

  if (status.isLoading) return <Spinner />;
  if (status.error) {
    return <InlineError title="Failed to load status" detail={status.error as Error} />;
  }
  const s = status.data!;
  const recent = (pipelines.data ?? []).slice(0, 10);

  return (
    <div>
      <PageHeading
        kicker="Dashboard"
        title="Overview"
        meta={`uptime ${Math.floor(s.uptime_seconds / 60)}m ${s.uptime_seconds % 60}s`}
      />

      <div className="mb-8 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <MetricsCard title="Projects" value={s.projects} emphasized />
        <MetricsCard title="Pipelines" value={s.pipelines} />
        <MetricsCard title="Jobs" value={s.jobs} />
        <MetricsCard title="Runners" value={s.connected_runners} />
      </div>

      {storage.isError ? (
        <div className="mb-8">
          <InlineError inline title="Failed to load storage backends" detail={storage.error as Error} />
        </div>
      ) : (
        storage.data && (
          <div className="mb-8 grid grid-cols-1 gap-3 sm:grid-cols-2">
            <MetricsCard
              title="Git storage"
              value={storage.data.git.backend}
              subtitle={storage.data.git.detail}
            />
            <MetricsCard
              title="Artifact storage"
              value={storage.data.artifacts.backend}
              subtitle={storage.data.artifacts.detail}
            />
          </div>
        )
      )}

      <h2 className="font-display mb-3 text-sm font-semibold uppercase tracking-wide">
        Recent pipelines
      </h2>
      {pipelines.isError ? (
        <InlineError inline title="Failed to load pipelines" detail={pipelines.error as Error} />
      ) : pipelines.isLoading ? (
        <Spinner />
      ) : (
        <DataTable
          data={recent}
          columns={pipelineColumns}
          emptyMessage="No pipelines yet — trigger one against the control plane."
          onRowClick={(p) => navigate(`/ui/pipelines/${p.id}`)}
        />
      )}
    </div>
  );
}
