import { useState } from "react";
import { Link } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { GcpResourceTable, GcpStatus, type GcpColumn } from "../console/index.js";
import { GcpDialog } from "../console/GcpDialog.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { deleteCloudFunction, fetchCloudFunctions, waitV2Operation, type CloudFunction } from "../api.js";
import { useProject } from "../console/project.js";

const columns: GcpColumn<CloudFunction>[] = [
  {
    id: "name",
    header: "Name",
    cell: (row) => <Link className="gc-cell-link" to={`/ui/functions/${shortName(row.name)}`}>{shortName(row.name)}</Link>,
    value: (row) => shortName(row.name),
  },
  { id: "state", header: "State", cell: (row) => <GcpStatus status={row.state ?? "UNKNOWN"} />, value: (row) => row.state ?? "" },
  { id: "environment", header: "Environment", cell: (row) => row.environment ?? "—", value: (row) => row.environment ?? "" },
  { id: "createTime", header: "Created", cell: (row) => formatTimestamp(row.createTime ?? ""), value: (row) => row.createTime ?? "" },
];

// DeleteFunctionDialog is shared by the list's per-row action and the
// function detail page's header action — the same real
// projects.locations.functions.delete long-running operation, driven through
// the same operations.get poll (waitV2Operation) Cloud Run job deletion uses.
export function DeleteFunctionDialog({
  project,
  functionId,
  onClose,
  onDeleted,
}: {
  project: string;
  functionId: string;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const remove = useMutation({
    mutationFn: async () => waitV2Operation(await deleteCloudFunction(project, functionId)),
    onSuccess: onDeleted,
  });
  return (
    <GcpDialog title="Delete function?" testId="function-delete-dialog" onClose={onClose}>
      <p>
        Deleting <strong>{functionId}</strong> permanently removes the function. Any triggers or
        callers relying on it fail immediately. This can't be undone.
      </p>
      {remove.isError ? (
        <div className="gc-message gc-message-error" role="alert">
          <strong>Couldn't delete the function.</strong>{" "}
          {remove.error instanceof Error ? remove.error.message : "The API did not respond."}
        </div>
      ) : null}
      <div className="gc-dialog-actions">
        <button type="button" className="gc-button-text" onClick={onClose}>Cancel</button>
        <button
          type="button"
          className="gc-button-primary"
          data-testid="function-delete-confirm"
          disabled={remove.isPending}
          onClick={() => remove.mutate()}
        >
          {remove.isPending ? "Deleting…" : "Delete"}
        </button>
      </div>
    </GcpDialog>
  );
}

export function CloudFunctionsPage() {
  const { project } = useProject();
  const queryClient = useQueryClient();
  const [deleting, setDeleting] = useState<string | null>(null);

  const columnsWithActions: GcpColumn<CloudFunction>[] = [
    ...columns,
    {
      id: "actions",
      header: "Actions",
      cell: (row) => {
        const id = shortName(row.name);
        return (
          <button
            type="button"
            className="gc-button-text"
            data-testid={`function-delete-${id}`}
            aria-label={`Delete ${id}`}
            onClick={() => setDeleting(id)}
          >
            Delete
          </button>
        );
      },
      value: () => "",
    },
  ];

  return (
    <>
      <GcpResourceTable<CloudFunction>
        title="Cloud Run functions"
        description="Run your code in response to events without provisioning or managing servers."
        actions={[{ label: "Create function", icon: "add", primary: true, disabled: true }]}
        columns={columnsWithActions}
        queryKey={["cloud-functions-real", project]}
        queryFn={() => fetchCloudFunctions(project)}
        filterPlaceholder="Filter functions"
        resourceNoun="functions"
        empty={{
          headline: "Write and deploy your first function",
          description: "Functions run your code in response to events, scaling from zero automatically.",
          primaryLabel: "Create function",
        }}
        rowKey={(row) => row.name}
      />
      {deleting ? (
        <DeleteFunctionDialog
          project={project}
          functionId={deleting}
          onClose={() => setDeleting(null)}
          onDeleted={() => {
            setDeleting(null);
            void queryClient.invalidateQueries({ queryKey: ["cloud-functions-real", project] });
          }}
        />
      ) : null}
    </>
  );
}
