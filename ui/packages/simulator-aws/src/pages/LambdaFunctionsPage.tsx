import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AwsButton, AwsModal, AwsResourceTable, AwsStatus, type AwsColumn } from "../console/index.js";
import { formatTimestamp } from "../console/format.js";
import { deleteLambdaFunction, fetchLambdaFunctions, type LambdaFunction } from "../api.js";

// AWS Lambda — Functions. The list and Delete both go through the real
// Lambda REST-JSON API (GET /2015-03-31/functions for the table, DELETE
// /2015-03-31/functions/{FunctionName} for the header action) with the
// operator's federated credentials.

const columns: AwsColumn<LambdaFunction>[] = [
  { id: "name", header: "Function name", cell: (row) => row.name, value: (row) => row.name },
  { id: "state", header: "State", cell: (row) => <AwsStatus status={row.state} />, value: (row) => row.state },
  { id: "runtime", header: "Runtime", cell: (row) => row.runtime, value: (row) => row.runtime },
  { id: "memorySize", header: "Memory", cell: (row) => `${row.memorySize} MB`, value: (row) => String(row.memorySize) },
  { id: "timeout", header: "Timeout", cell: (row) => `${row.timeout} s`, value: (row) => String(row.timeout) },
  { id: "lastModified", header: "Last modified", cell: (row) => formatTimestamp(row.lastModified), value: (row) => row.lastModified },
];

function DeleteFunctionsModal({
  functions,
  onClose,
  clearSelection,
}: {
  functions: LambdaFunction[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    // DeleteFunction is per-function on the wire; a failure part-way (a
    // function another resource — an event source mapping — still references)
    // surfaces as the real API error, with the already-deleted functions gone
    // from the refreshed list.
    mutationFn: async () => {
      for (const fn of functions) {
        await deleteLambdaFunction(fn.name);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["lambda-functions"] }),
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  return (
    <AwsModal
      title={functions.length === 1 ? `Delete ${functions[0].name}?` : `Delete ${functions.length} functions?`}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="lambda-delete-function-confirm"
            disabled={remove.isPending}
            onClick={() => remove.mutate()}
          >
            {remove.isPending ? "Deleting…" : "Delete"}
          </AwsButton>
        </>
      }
    >
      <p>Deleting a function is permanent. Its versions, aliases, and event source mappings are deleted with it.</p>
      <ul>
        {functions.map((fn) => (
          <li key={fn.name}>
            <code>{fn.name}</code>
          </li>
        ))}
      </ul>
      {remove.isError && (
        <div className="aws-flash aws-flash-error" role="alert">
          <strong>Could not delete.</strong>{" "}
          {remove.error instanceof Error ? remove.error.message : "The request failed."}
        </div>
      )}
    </AwsModal>
  );
}

export function LambdaFunctionsPage() {
  const [deleting, setDeleting] = useState<{ functions: LambdaFunction[]; clearSelection: () => void } | null>(null);
  return (
    <>
      <AwsResourceTable<LambdaFunction>
        title="Functions"
        description="Functions in this account and Region."
        columns={columns}
        queryKey={["lambda-functions"]}
        queryFn={fetchLambdaFunctions}
        filterPlaceholder="Find functions"
        emptyTitle="No functions"
        emptyDescription="No functions exist in this account and Region."
        rowKey={(row) => row.name}
        tableTestId="lambda-functions-table"
        rowTestId={(row) => `lambda-function-row-${row.name}`}
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="lambda-delete-function"
              disabled={selected.length === 0}
              onClick={() => setDeleting({ functions: selected, clearSelection })}
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
        <DeleteFunctionsModal
          functions={deleting.functions}
          clearSelection={deleting.clearSelection}
          onClose={() => setDeleting(null)}
        />
      )}
    </>
  );
}
