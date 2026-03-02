import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, Spinner } from "@sockerless/ui-core/components";
import { fetchECRRepos, type ECRRepo } from "../api.js";

const columns: ColumnDef<ECRRepo, any>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "uri", header: "URI" },
  {
    accessorKey: "createdAt",
    header: "Created",
    cell: ({ getValue }) => new Date(getValue<number>() * 1000).toLocaleString(),
    sortDescFirst: true,
  },
];

export function ECRReposPage() {
  const { data, isLoading } = useQuery({ queryKey: ["ecr-repos"], queryFn: fetchECRRepos, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">ECR Repositories</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
