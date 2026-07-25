import { useState } from "react";
import { NavLink, useNavigate } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AwsButton, AwsModal, AwsResourceTable, type AwsColumn } from "../console/index.js";
import { formatBytes, formatEpoch, formatRetention } from "../console/format.js";
import { deleteCWLogGroup, fetchCWLogGroups, type CWLogGroup } from "../api.js";

// Amazon CloudWatch Logs — Log groups. The list and Delete both go through
// the real CloudWatch Logs awsjson1.1 API (DescribeLogGroups for the table,
// DeleteLogGroup for the header action) with the operator's federated
// credentials.

const columns: AwsColumn<CWLogGroup>[] = [
  {
    id: "name",
    header: "Log group",
    cell: (row) => <NavLink to={`/ui/logs/${encodeURIComponent(row.name)}`}>{row.name}</NavLink>,
    value: (row) => row.name,
  },
  { id: "retentionInDays", header: "Retention", cell: (row) => formatRetention(row.retentionInDays), value: (row) => String(row.retentionInDays) },
  { id: "storedBytes", header: "Stored", cell: (row) => formatBytes(row.storedBytes), value: (row) => String(row.storedBytes) },
  { id: "creationTime", header: "Created at", cell: (row) => formatEpoch(row.creationTime), value: (row) => String(row.creationTime) },
];

export function DeleteLogGroupsModal({
  groups,
  onClose,
  clearSelection,
}: {
  groups: CWLogGroup[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    // DeleteLogGroup is per-group on the wire; a failure part-way surfaces as
    // the real API error, with the already-deleted groups gone from the
    // refreshed list.
    mutationFn: async () => {
      for (const group of groups) {
        await deleteCWLogGroup(group.name);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["log-groups"] }),
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  return (
    <AwsModal
      title={groups.length === 1 ? `Delete ${groups[0].name}?` : `Delete ${groups.length} log groups?`}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="logs-delete-log-group-confirm"
            disabled={remove.isPending}
            onClick={() => remove.mutate()}
          >
            {remove.isPending ? "Deleting…" : "Delete"}
          </AwsButton>
        </>
      }
    >
      <p>Deleting a log group is permanent and deletes every log stream and event it holds.</p>
      <ul>
        {groups.map((group) => (
          <li key={group.name}>
            <code>{group.name}</code>
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

export function LogGroupsPage() {
  const navigate = useNavigate();
  const [deleting, setDeleting] = useState<{ groups: CWLogGroup[]; clearSelection: () => void } | null>(null);
  return (
    <>
      <AwsResourceTable<CWLogGroup>
        title="Log groups"
        description="Log groups in this account and Region."
        columns={columns}
        queryKey={["log-groups"]}
        queryFn={fetchCWLogGroups}
        filterPlaceholder="Find log groups"
        emptyTitle="No log groups"
        emptyDescription="No log groups exist in this account and Region."
        rowKey={(row) => row.name}
        tableTestId="log-groups-table"
        rowTestId={(row) => `log-group-row-${row.name}`}
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="logs-view-log-group"
              disabled={selected.length !== 1}
              onClick={() => navigate(`/ui/logs/${encodeURIComponent(selected[0].name)}`)}
            >
              View details
            </AwsButton>
            <AwsButton
              data-testid="logs-delete-log-group"
              disabled={selected.length === 0}
              onClick={() => setDeleting({ groups: selected, clearSelection })}
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
        <DeleteLogGroupsModal
          groups={deleting.groups}
          clearSelection={deleting.clearSelection}
          onClose={() => setDeleting(null)}
        />
      )}
    </>
  );
}
