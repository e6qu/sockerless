import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, Spinner } from "@sockerless/ui-core/components";
import { fetchARRepos, type ARRepo } from "../api.js";

const columns: ColumnDef<ARRepo, any>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "format", header: "Format" },
  { accessorKey: "createTime", header: "Created" },
];

export function ArtifactRegistryPage() {
  const { data, isLoading } = useQuery({ queryKey: ["ar-repos"], queryFn: fetchARRepos, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">Artifact Registry</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
