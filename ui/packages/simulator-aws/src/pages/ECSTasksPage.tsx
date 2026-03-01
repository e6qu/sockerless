import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, StatusBadge, Spinner } from "@sockerless/ui-core/components";
import { fetchECSTasks, type ECSTask } from "../api.js";

const columns: ColumnDef<ECSTask, any>[] = [
  { accessorKey: "taskArn", header: "Task ARN" },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ getValue }) => <StatusBadge status={getValue<string>()} />,
  },
  { accessorKey: "clusterArn", header: "Cluster" },
  { accessorKey: "launchType", header: "Launch Type" },
  { accessorKey: "cpu", header: "CPU" },
  { accessorKey: "memory", header: "Memory" },
];

export function ECSTasksPage() {
  const { data, isLoading } = useQuery({ queryKey: ["ecs-tasks"], queryFn: fetchECSTasks, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">ECS Tasks</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
