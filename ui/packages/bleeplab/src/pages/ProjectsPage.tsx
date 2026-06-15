import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import {
  PageHeading,
  DataTable,
  Spinner,
  InlineError,
} from "@sockerless/ui-core/components";
import type { ColumnDef } from "@tanstack/react-table";
import { api } from "../api.js";
import type { Project } from "../types.js";
import { shortSHA } from "../format.js";

const columns: ColumnDef<Project, unknown>[] = [
  { accessorKey: "id", header: "#" },
  { accessorKey: "path", header: "Project" },
  { accessorKey: "default_branch", header: "Branch" },
  {
    accessorKey: "sha",
    header: "HEAD",
    cell: (c) => <code>{shortSHA(String(c.getValue()))}</code>,
  },
  { accessorKey: "pipelines", header: "Pipelines" },
];

export function ProjectsPage() {
  const navigate = useNavigate();
  const projects = useQuery({ queryKey: ["projects"], queryFn: api.projects });

  if (projects.isLoading) return <Spinner />;
  if (projects.error) {
    return <InlineError title="Failed to load projects" detail={projects.error as Error} />;
  }

  return (
    <div>
      <PageHeading kicker="Resources" title="Projects" meta={`${projects.data!.length} total`} />
      <DataTable
        data={projects.data!}
        columns={columns}
        filterPlaceholder="Filter projects…"
        emptyMessage="No projects yet."
        onRowClick={(p) => navigate(`/ui/projects/${p.id}`)}
      />
    </div>
  );
}
