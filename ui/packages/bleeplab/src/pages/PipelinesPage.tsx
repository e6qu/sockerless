import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router";
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
  { accessorKey: "project_name", header: "Project" },
  { accessorKey: "ref", header: "Ref" },
  {
    accessorKey: "sha",
    header: "Commit",
    cell: (c) => <code>{shortSHA(String(c.getValue()))}</code>,
  },
  {
    accessorKey: "stages",
    header: "Stages",
    cell: (c) => (c.getValue() as string[])?.join(" → "),
  },
  { accessorKey: "jobs", header: "Jobs" },
  {
    accessorKey: "status",
    header: "Status",
    cell: (c) => <StatusBadge status={String(c.getValue())} />,
  },
];

export function PipelinesPage() {
  const navigate = useNavigate();
  const pipelines = useQuery({ queryKey: ["pipelines"], queryFn: api.pipelines });

  if (pipelines.isLoading) return <Spinner />;
  if (pipelines.error) {
    return <InlineError title="Failed to load pipelines" detail={pipelines.error as Error} />;
  }

  return (
    <div>
      <PageHeading kicker="CI/CD" title="Pipelines" meta={`${pipelines.data!.length} total`} />
      <DataTable
        data={pipelines.data!}
        columns={columns}
        filterPlaceholder="Filter pipelines…"
        emptyMessage="No pipelines yet."
        onRowClick={(p) => navigate(`/ui/pipelines/${p.id}`)}
      />
    </div>
  );
}
