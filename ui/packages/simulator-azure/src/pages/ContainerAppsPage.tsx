import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, Spinner } from "@sockerless/ui-core/components";
import { fetchContainerAppJobs, type ContainerAppJob } from "../api.js";

const columns: ColumnDef<ContainerAppJob, any>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "location", header: "Location" },
  { accessorKey: "type", header: "Type" },
  { accessorKey: "id", header: "Resource ID" },
];

export function ContainerAppsPage() {
  const { data, isLoading } = useQuery({ queryKey: ["ca-jobs"], queryFn: fetchContainerAppJobs, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">Container Apps Jobs</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
