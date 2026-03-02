import { useQuery } from "@tanstack/react-query";
import { DataTable, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { fetchProjects } from "../api.js";
import type { GitlabhubProject } from "../types.js";

const col = createColumnHelper<GitlabhubProject>();

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const columns: any[] = [
  col.accessor("id", { header: "ID" }),
  col.accessor("name", { header: "Name" }),
];

export function ProjectsPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["projects"],
    queryFn: fetchProjects,
  });

  if (isLoading || !data) return <Spinner />;

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Projects ({data.length})</h2>
      <DataTable data={data} columns={columns} filterPlaceholder="Filter projects..." />
    </div>
  );
}
