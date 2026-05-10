import { type ColumnDef } from "@tanstack/react-table";
import { ResourceListPage } from "@sockerless/ui-core/components";
import { fetchECRRepos, type ECRRepo } from "../api.js";

const columns: ColumnDef<ECRRepo, unknown>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "uri", header: "URI" },
  {
    accessorKey: "createdAt",
    header: "Created",
    cell: ({ getValue }) =>
      new Date(getValue<number>() * 1000).toLocaleString(),
    sortDescFirst: true,
  },
];

export function ECRReposPage() {
  return (
    <ResourceListPage<ECRRepo>
      kicker="aws · simulator · ecr"
      title={<>Repositories</>}
      countNoun="repository"
      columns={columns}
      queryKey={["ecr-repos"]}
      queryFn={fetchECRRepos}
      filterPlaceholder="Filter repositories…"
      emptyMessage="No ECR repositories tracked."
    />
  );
}
