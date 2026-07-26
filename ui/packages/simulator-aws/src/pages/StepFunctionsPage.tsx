import { useState } from "react";
import { useNavigate } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AwsButton, AwsErrorAlert, AwsModal, AwsResourceTable, AwsRowLink, type AwsColumn } from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import { deleteStateMachine, fetchStateMachines, type StateMachine } from "../api.js";

// AWS Step Functions — State machines. ListStateMachines and
// DeleteStateMachine on the real Step Functions API (X-Amz-Target
// AWSStepFunctions.<Op>).

const columns: AwsColumn<StateMachine>[] = [
  {
    id: "name",
    header: "Name",
    cell: (row) => (
      <AwsRowLink to={`/ui/stepfunctions/${encodeURIComponent(row.stateMachineArn)}`}>{row.name}</AwsRowLink>
    ),
    value: (row) => row.name,
  },
  { id: "type", header: "Type", cell: (row) => row.type, value: (row) => row.type },
  { id: "arn", header: "ARN", cell: (row) => row.stateMachineArn, value: (row) => row.stateMachineArn },
  {
    id: "creationDate",
    header: "Created",
    cell: (row) => formatEpoch(row.creationDate),
    value: (row) => String(row.creationDate),
  },
];

export function DeleteStateMachinesModal({
  machines,
  onClose,
  clearSelection,
}: {
  machines: StateMachine[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: async () => {
      for (const machine of machines) {
        await deleteStateMachine(machine.stateMachineArn);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["sfn-state-machines"] }),
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  return (
    <AwsModal
      title={machines.length === 1 ? `Delete ${machines[0].name}?` : `Delete ${machines.length} state machines?`}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="sfn-delete-state-machine-confirm"
            disabled={remove.isPending}
            onClick={() => remove.mutate()}
          >
            {remove.isPending ? "Deleting…" : "Delete"}
          </AwsButton>
        </>
      }
    >
      <p>Step Functions marks a state machine for deletion and removes it once its running executions have finished.</p>
      <ul>
        {machines.map((machine) => (
          <li key={machine.stateMachineArn}>
            <code>{machine.name}</code>
          </li>
        ))}
      </ul>
      {remove.isError && (
        <AwsErrorAlert>
          <strong>Could not delete.</strong>{" "}
          {remove.error instanceof Error ? remove.error.message : "The request failed."}
        </AwsErrorAlert>
      )}
    </AwsModal>
  );
}

export function StepFunctionsPage() {
  const navigate = useNavigate();
  const [deleting, setDeleting] = useState<{ machines: StateMachine[]; clearSelection: () => void } | null>(null);
  return (
    <>
      <AwsResourceTable<StateMachine>
        title="State machines"
        description="AWS Step Functions state machines in this account and Region."
        columns={columns}
        queryKey={["sfn-state-machines"]}
        queryFn={fetchStateMachines}
        filterPlaceholder="Find state machines"
        emptyTitle="No state machines"
        emptyDescription="No AWS Step Functions state machines exist in this account and Region."
        rowKey={(row) => row.stateMachineArn}
        tableTestId="sfn-state-machines-table"
        errorTestId="sfn-state-machines-error"
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="sfn-view-state-machine"
              disabled={selected.length !== 1}
              onClick={() => navigate(`/ui/stepfunctions/${encodeURIComponent(selected[0].stateMachineArn)}`)}
            >
              View details
            </AwsButton>
            <AwsButton
              data-testid="sfn-delete-state-machine"
              disabled={selected.length === 0}
              onClick={() => setDeleting({ machines: selected, clearSelection })}
            >
              Delete
            </AwsButton>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
          </>
        )}
      />
      {deleting && (
        <DeleteStateMachinesModal
          machines={deleting.machines}
          clearSelection={deleting.clearSelection}
          onClose={() => setDeleting(null)}
        />
      )}
    </>
  );
}
