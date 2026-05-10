import { type ColumnDef } from "@tanstack/react-table";
import {
  ResourceListPage,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { fetchECSTasks, type ECSTask } from "../api.js";

const columns: ColumnDef<ECSTask, unknown>[] = [
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
  return (
    <ResourceListPage<ECSTask>
      kicker="aws · simulator · ecs"
      title={<>Tasks</>}
      countNoun="task"
      columns={columns}
      queryKey={["ecs-tasks"]}
      queryFn={fetchECSTasks}
      filterPlaceholder="Filter tasks…"
      emptyMessage="No ECS tasks tracked."
    />
  );
}
