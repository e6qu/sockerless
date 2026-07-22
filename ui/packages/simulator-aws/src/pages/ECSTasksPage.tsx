import { AwsResourceTable, AwsStatus, type AwsColumn } from "../console/index.js";
import { fetchECSTasks, type ECSTask } from "../api.js";

const columns: AwsColumn<ECSTask>[] = [
  { id: "taskArn", header: "Task ARN", cell: (row) => row.taskArn, value: (row) => row.taskArn },
  { id: "status", header: "Last status", cell: (row) => <AwsStatus status={row.status} />, value: (row) => row.status },
  { id: "clusterArn", header: "Cluster", cell: (row) => row.clusterArn, value: (row) => row.clusterArn },
  { id: "launchType", header: "Launch type", cell: (row) => row.launchType, value: (row) => row.launchType },
  { id: "cpu", header: "CPU", cell: (row) => row.cpu, value: (row) => row.cpu },
  { id: "memory", header: "Memory", cell: (row) => row.memory, value: (row) => row.memory },
];

export function ECSTasksPage() {
  return (
    <AwsResourceTable<ECSTask>
      title="Tasks"
      breadcrumbLabel="Tasks"
      description="Tasks running in this account and Region."
      columns={columns}
      queryKey={["ecs-tasks"]}
      queryFn={fetchECSTasks}
      filterPlaceholder="Find tasks"
      emptyTitle="No tasks"
      emptyDescription="No tasks are running in this account and Region."
      rowKey={(row) => row.taskArn}
    />
  );
}
