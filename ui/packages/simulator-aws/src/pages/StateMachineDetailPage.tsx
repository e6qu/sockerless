import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import SpaceBetween from "@cloudscape-design/components/space-between";
import Textarea from "@cloudscape-design/components/textarea";
import FormField from "@cloudscape-design/components/form-field";
import { fontFamilyMonospace } from "@cloudscape-design/design-tokens";
import {
  AwsButton,
  AwsContainer,
  AwsEmptyState,
  AwsErrorAlert,
  AwsKeyValue,
  AwsModal,
  AwsPageHeader,
  AwsResourceTable,
  AwsStatus,
  type AwsColumn,
} from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import {
  fetchStateMachine,
  fetchStateMachineExecutions,
  startStateMachineExecution,
  type StateMachineExecution,
} from "../api.js";
import { DeleteStateMachinesModal } from "./StepFunctionsPage.js";

// AWS Step Functions — State machine detail. DescribeStateMachine for the
// summary and the definition, ListExecutions for the executions table, and
// StartExecution for the "Start execution" action — the same operations the
// real console's state machine page drives.

const executionColumns: AwsColumn<StateMachineExecution>[] = [
  { id: "name", header: "Name", cell: (row) => row.name, value: (row) => row.name },
  { id: "status", header: "Status", cell: (row) => <AwsStatus status={row.status} />, value: (row) => row.status },
  {
    id: "startDate",
    header: "Started",
    cell: (row) => formatEpoch(row.startDate),
    value: (row) => String(row.startDate),
  },
  {
    id: "stopDate",
    header: "Ended",
    cell: (row) => (row.stopDate ? formatEpoch(row.stopDate) : "–"),
    value: (row) => String(row.stopDate),
  },
];

function StartExecutionModal({ stateMachineArn, onClose }: { stateMachineArn: string; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [input, setInput] = useState("{}");
  const start = useMutation({
    mutationFn: () => startStateMachineExecution(stateMachineArn, input),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["sfn-executions", stateMachineArn] });
      onClose();
    },
  });
  let inputIsJson = true;
  try {
    JSON.parse(input);
  } catch {
    inputIsJson = false;
  }
  return (
    <AwsModal
      title="Start execution"
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="sfn-start-execution-submit"
            disabled={!inputIsJson || start.isPending}
            onClick={() => start.mutate()}
          >
            {start.isPending ? "Starting…" : "Start execution"}
          </AwsButton>
        </>
      }
    >
      <SpaceBetween size="m">
        <FormField
          label="Input"
          constraintText="A JSON document passed to the state machine's first state."
          errorText={inputIsJson ? undefined : "Input must be a JSON document."}
        >
          <Textarea
            value={input}
            rows={6}
            onChange={(event) => setInput(event.detail.value)}
            ariaLabel="Execution input"
            spellcheck={false}
            data-testid="sfn-execution-input"
          />
        </FormField>
        {start.isError && (
          <AwsErrorAlert>
            <strong>Could not start the execution.</strong>{" "}
            {start.error instanceof Error ? start.error.message : "The request failed."}
          </AwsErrorAlert>
        )}
      </SpaceBetween>
    </AwsModal>
  );
}

export function StateMachineDetailPage() {
  const { stateMachineArn = "" } = useParams();
  const navigate = useNavigate();
  const [starting, setStarting] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const machine = useQuery({
    queryKey: ["sfn-state-machine", stateMachineArn],
    queryFn: () => fetchStateMachine(stateMachineArn),
  });

  return (
    <>
      <AwsPageHeader
        title={machine.data?.name || stateMachineArn}
        description="AWS Step Functions state machine in this account and Region."
        actions={
          <SpaceBetween direction="horizontal" size="xs">
            <AwsButton
              data-testid="sfn-state-machine-start"
              disabled={!machine.isSuccess}
              onClick={() => setStarting(true)}
            >
              Start execution
            </AwsButton>
            <AwsButton
              data-testid="sfn-state-machine-delete"
              disabled={!machine.isSuccess}
              onClick={() => setDeleting(true)}
            >
              Delete
            </AwsButton>
          </SpaceBetween>
        }
      />
      <AwsContainer>
        {machine.isError ? (
          <AwsErrorAlert testId="sfn-state-machine-error">
            <strong>Could not load the state machine.</strong>{" "}
            {machine.error instanceof Error ? machine.error.message : "The request failed."}
          </AwsErrorAlert>
        ) : machine.isLoading ? (
          <AwsEmptyState title="Loading state machine…" loading />
        ) : machine.data ? (
          <div data-testid="sfn-state-machine-summary">
            <AwsKeyValue
              ariaLabel="State machine details"
              items={[
                { label: "Status", value: <AwsStatus status={machine.data.status} /> },
                { label: "Type", value: machine.data.type },
                { label: "IAM role", value: machine.data.roleArn || "–" },
                { label: "Created", value: formatEpoch(machine.data.creationDate) },
                { label: "ARN", value: machine.data.stateMachineArn },
                {
                  label: "Definition",
                  value: <pre style={{ fontFamily: fontFamilyMonospace, margin: 0, whiteSpace: "pre-wrap" }}>{machine.data.definition}</pre>,
                },
              ]}
            />
          </div>
        ) : null}
      </AwsContainer>
      <AwsResourceTable<StateMachineExecution>
        title="Executions"
        headingVariant="h2"
        description="Executions of this state machine."
        columns={executionColumns}
        queryKey={["sfn-executions", stateMachineArn]}
        queryFn={() => fetchStateMachineExecutions(stateMachineArn)}
        filterPlaceholder="Find executions"
        emptyTitle="No executions"
        emptyDescription="This state machine has not been executed yet."
        rowKey={(row) => row.executionArn}
        tableTestId="sfn-executions-table"
        errorTestId="sfn-executions-error"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
      {starting && <StartExecutionModal stateMachineArn={stateMachineArn} onClose={() => setStarting(false)} />}
      {deleting && machine.data && (
        <DeleteStateMachinesModal
          machines={[machine.data]}
          clearSelection={() => navigate("/ui/stepfunctions")}
          onClose={() => setDeleting(false)}
        />
      )}
    </>
  );
}
