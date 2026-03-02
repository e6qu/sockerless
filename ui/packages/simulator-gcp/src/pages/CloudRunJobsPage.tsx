import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, Spinner } from "@sockerless/ui-core/components";
import { fetchCloudRunJobs, type CloudRunJob } from "../api.js";

const columns: ColumnDef<CloudRunJob, any>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "createTime", header: "Created" },
  { accessorKey: "executionCount", header: "Executions", sortDescFirst: true },
  { accessorKey: "launchStage", header: "Launch Stage" },
];

export function CloudRunJobsPage() {
  const { data, isLoading } = useQuery({ queryKey: ["cloudrun-jobs"], queryFn: fetchCloudRunJobs, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">Cloud Run Jobs</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
