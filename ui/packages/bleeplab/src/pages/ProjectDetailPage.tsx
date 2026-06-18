import { useQuery } from "@tanstack/react-query";
import { useParams, useNavigate } from "react-router";
import {
  PageHeading,
  DataTable,
  StatusBadge,
  Spinner,
  InlineError,
} from "@sockerless/ui-core/components";
import type { ColumnDef } from "@tanstack/react-table";
import { api } from "../api.js";
import type { Pipeline } from "../types.js";
import { shortSHA } from "../format.js";

const columns: ColumnDef<Pipeline, unknown>[] = [
  { accessorKey: "id", header: "#" },
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

export function ProjectDetailPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const projects = useQuery({ queryKey: ["projects"], queryFn: api.projects });
  const pipelines = useQuery({ queryKey: ["pipelines"], queryFn: api.pipelines });

  if (projects.isLoading) return <Spinner />;
  if (projects.error) {
    return <InlineError title="Failed to load project" detail={projects.error as Error} />;
  }
  const project = projects.data!.find((p) => String(p.id) === id);
  if (!project) return <InlineError title={`Project ${id} not found`} />;
  const mine = (pipelines.data ?? []).filter((p) => p.project_id === project.id);

  return (
    <div>
      <PageHeading
        kicker="Project"
        title={project.path}
        meta={`default branch ${project.default_branch} · HEAD ${shortSHA(project.sha)} · clone ${project.path}.git`}
      />
      <h2 className="font-display mb-3 text-sm font-semibold uppercase tracking-wide">
        Pipelines
      </h2>
      {pipelines.isError ? (
        <InlineError inline title="Failed to load pipelines" detail={pipelines.error as Error} />
      ) : (
        <DataTable
          data={mine}
          columns={columns}
          emptyMessage="No pipelines for this project yet."
          onRowClick={(p) => navigate(`/ui/pipelines/${p.id}`)}
        />
      )}
    </div>
  );
}
