import { AwsButton, AwsResourceTable, AwsStatus, type AwsColumn } from "../console/index.js";
import {
  fetchBatchComputeEnvironments,
  fetchBatchJobQueues,
  type BatchComputeEnvironment,
  type BatchJobQueue,
} from "../api.js";

// AWS Batch — Job queues and Compute environments, the two resources the real
// Batch console's dashboard leads with. Both are the real Batch REST-JSON
// operations: DescribeJobQueues and DescribeComputeEnvironments.

const queueColumns: AwsColumn<BatchJobQueue>[] = [
  { id: "name", header: "Name", cell: (row) => row.jobQueueName, value: (row) => row.jobQueueName },
  { id: "state", header: "State", cell: (row) => <AwsStatus status={row.state} />, value: (row) => row.state },
  { id: "status", header: "Status", cell: (row) => <AwsStatus status={row.status} />, value: (row) => row.status },
  { id: "priority", header: "Priority", cell: (row) => String(row.priority), value: (row) => String(row.priority) },
  {
    id: "computeEnvironments",
    header: "Compute environments",
    cell: (row) => row.computeEnvironments.join(", ") || "–",
    value: (row) => row.computeEnvironments.join(", "),
  },
];

const environmentColumns: AwsColumn<BatchComputeEnvironment>[] = [
  { id: "name", header: "Name", cell: (row) => row.computeEnvironmentName, value: (row) => row.computeEnvironmentName },
  { id: "type", header: "Type", cell: (row) => row.type, value: (row) => row.type },
  { id: "state", header: "State", cell: (row) => <AwsStatus status={row.state} />, value: (row) => row.state },
  { id: "status", header: "Status", cell: (row) => <AwsStatus status={row.status} />, value: (row) => row.status },
  { id: "serviceRole", header: "Service role", cell: (row) => row.serviceRole || "–", value: (row) => row.serviceRole },
];

export function BatchPage() {
  return (
    <>
      <AwsResourceTable<BatchJobQueue>
        title="Job queues"
        description="AWS Batch job queues in this account and Region."
        columns={queueColumns}
        queryKey={["batch-job-queues"]}
        queryFn={fetchBatchJobQueues}
        filterPlaceholder="Find job queues"
        emptyTitle="No job queues"
        emptyDescription="No AWS Batch job queues exist in this account and Region."
        rowKey={(row) => row.jobQueueArn || row.jobQueueName}
        tableTestId="batch-job-queues-table"
        errorTestId="batch-job-queues-error"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
      <AwsResourceTable<BatchComputeEnvironment>
        title="Compute environments"
        headingVariant="h2"
        description="The compute environments job queues dispatch to."
        columns={environmentColumns}
        queryKey={["batch-compute-environments"]}
        queryFn={fetchBatchComputeEnvironments}
        filterPlaceholder="Find compute environments"
        emptyTitle="No compute environments"
        emptyDescription="No AWS Batch compute environments exist in this account and Region."
        rowKey={(row) => row.computeEnvironmentArn || row.computeEnvironmentName}
        tableTestId="batch-compute-environments-table"
        errorTestId="batch-compute-environments-error"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
    </>
  );
}
