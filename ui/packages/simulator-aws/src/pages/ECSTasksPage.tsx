import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AwsButton, AwsModal, AwsResourceTable, AwsStatus, type AwsColumn } from "../console/index.js";
import { fetchECSTasks, stopECSTask, type ECSTask } from "../api.js";

// Amazon Elastic Container Service — Tasks. The list and the header action
// both go through the real ECS awsjson1.1 API (DescribeTasks feeding the
// table, StopTask for the action) with the operator's federated credentials.
// Real ECS never deletes a task record on request — a task is stopped, and
// the service reaps the STOPPED record on its own schedule — so the action
// here is Stop, matching what the real console's task Actions menu offers.

const columns: AwsColumn<ECSTask>[] = [
  { id: "taskArn", header: "Task ARN", cell: (row) => row.taskArn, value: (row) => row.taskArn },
  { id: "status", header: "Last status", cell: (row) => <AwsStatus status={row.status} />, value: (row) => row.status },
  { id: "clusterArn", header: "Cluster", cell: (row) => row.clusterArn, value: (row) => row.clusterArn },
  { id: "launchType", header: "Launch type", cell: (row) => row.launchType, value: (row) => row.launchType },
  { id: "cpu", header: "CPU", cell: (row) => row.cpu, value: (row) => row.cpu },
  { id: "memory", header: "Memory", cell: (row) => row.memory, value: (row) => row.memory },
];

// The states StopTask can still act on. A task already STOPPED or tearing
// down (DEPROVISIONING) has nothing left to stop, the same restriction the
// real console's Actions menu enforces by disabling Stop for that selection.
export function isStoppable(task: ECSTask): boolean {
  return task.status !== "STOPPED" && task.status !== "DEPROVISIONING";
}

function StopTasksModal({
  tasks,
  onClose,
  clearSelection,
}: {
  tasks: ECSTask[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const stop = useMutation({
    // StopTask is per-task on the wire; a failure part-way surfaces as the
    // real API error, with the already-stopped tasks reflected once the list
    // refetches.
    mutationFn: async () => {
      for (const task of tasks) {
        await stopECSTask(task.clusterArn, task.taskArn);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["ecs-tasks"] }),
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  return (
    <AwsModal
      title={tasks.length === 1 ? "Stop this task?" : `Stop ${tasks.length} tasks?`}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="ecs-stop-task-confirm"
            disabled={stop.isPending}
            onClick={() => stop.mutate()}
          >
            {stop.isPending ? "Stopping…" : "Stop"}
          </AwsButton>
        </>
      }
    >
      <p>
        Stopping a task sends its containers a SIGTERM and, after the stop timeout elapses, a SIGKILL. The task's
        status moves to STOPPED and it stops billing.
      </p>
      <ul>
        {tasks.map((task) => (
          <li key={task.taskArn}>
            <code>{task.taskArn}</code>
          </li>
        ))}
      </ul>
      {stop.isError && (
        <div className="aws-flash aws-flash-error" role="alert">
          <strong>Could not stop the task.</strong>{" "}
          {stop.error instanceof Error ? stop.error.message : "The request failed."}
        </div>
      )}
    </AwsModal>
  );
}

export function ECSTasksPage() {
  const [stopping, setStopping] = useState<{ tasks: ECSTask[]; clearSelection: () => void } | null>(null);
  return (
    <>
      <AwsResourceTable<ECSTask>
        title="Tasks"
        description="Tasks running in this account and Region."
        columns={columns}
        queryKey={["ecs-tasks"]}
        queryFn={fetchECSTasks}
        filterPlaceholder="Find tasks"
        emptyTitle="No tasks"
        emptyDescription="No tasks are running in this account and Region."
        rowKey={(row) => row.taskArn}
        tableTestId="ecs-tasks-table"
        rowTestId={(row) => `ecs-task-row-${row.taskArn}`}
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="ecs-stop-task"
              disabled={selected.length === 0 || !selected.every(isStoppable)}
              onClick={() => setStopping({ tasks: selected, clearSelection })}
            >
              Stop
            </AwsButton>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
          </>
        )}
      />
      {stopping && (
        <StopTasksModal tasks={stopping.tasks} clearSelection={stopping.clearSelection} onClose={() => setStopping(null)} />
      )}
    </>
  );
}
