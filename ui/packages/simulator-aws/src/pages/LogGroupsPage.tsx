import { useState } from "react";
import { useNavigate } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import Input from "@cloudscape-design/components/input";
import FormField from "@cloudscape-design/components/form-field";
import { AwsButton, AwsErrorAlert, AwsModal, AwsResourceTable, AwsRowLink, type AwsColumn } from "../console/index.js";
import { formatBytes, formatEpoch, formatRetention } from "../console/format.js";
import { createCWLogGroup, deleteCWLogGroup, fetchCWLogGroups, type CWLogGroup } from "../api.js";

// Amazon CloudWatch Logs — Log groups. The list and Delete both go through
// the real CloudWatch Logs awsjson1.1 API (DescribeLogGroups for the table,
// DeleteLogGroup for the header action) with the operator's federated
// credentials.

const columns: AwsColumn<CWLogGroup>[] = [
  {
    id: "name",
    header: "Log group",
    cell: (row) => <AwsRowLink to={`/ui/logs/${encodeURIComponent(row.name)}`}>{row.name}</AwsRowLink>,
    value: (row) => row.name,
  },
  { id: "retentionInDays", header: "Retention", cell: (row) => formatRetention(row.retentionInDays), value: (row) => String(row.retentionInDays) },
  { id: "storedBytes", header: "Stored", cell: (row) => formatBytes(row.storedBytes), value: (row) => String(row.storedBytes) },
  { id: "creationTime", header: "Created at", cell: (row) => formatEpoch(row.creationTime), value: (row) => String(row.creationTime) },
];

// The name shape real CloudWatch Logs enforces on CreateLogGroup: 1–512
// characters of letters, numbers, and `. - _ / #` — validating it inline is
// what the real console does before it lets the request go out.
const LOG_GROUP_NAME_PATTERN = /^[.\-_/#A-Za-z0-9]{1,512}$/;

function CreateLogGroupModal({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const create = useMutation({
    mutationFn: createCWLogGroup,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["log-groups"] });
      onClose();
    },
  });
  const trimmed = name.trim();
  const valid = LOG_GROUP_NAME_PATTERN.test(trimmed);
  return (
    <AwsModal
      title="Create log group"
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="logs-create-log-group-submit"
            disabled={!valid || create.isPending}
            onClick={() => create.mutate(trimmed)}
          >
            {create.isPending ? "Creating…" : "Create log group"}
          </AwsButton>
        </>
      }
    >
      <p>A log group is a container for log streams that share the same retention and access-control settings.</p>
      <FormField label="Log group name" constraintText="Up to 512 characters. Letters, numbers, and . - _ / # are allowed.">
        <Input
          value={name}
          onChange={(event) => setName(event.detail.value)}
          nativeInputAttributes={{ "data-testid": "logs-log-group-name-input" }}
        />
      </FormField>
      {create.isError && (
        <AwsErrorAlert>
          <strong>Could not create the log group.</strong>{" "}
          {create.error instanceof Error ? create.error.message : "The request failed."}
        </AwsErrorAlert>
      )}
    </AwsModal>
  );
}

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
        <AwsErrorAlert>
          <strong>Could not delete.</strong>{" "}
          {remove.error instanceof Error ? remove.error.message : "The request failed."}
        </AwsErrorAlert>
      )}
    </AwsModal>
  );
}

export function LogGroupsPage() {
  const navigate = useNavigate();
  const [creating, setCreating] = useState(false);
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
            <AwsButton variant="primary" data-testid="logs-create-log-group" onClick={() => setCreating(true)}>
              Create log group
            </AwsButton>
          </>
        )}
      />
      {creating && <CreateLogGroupModal onClose={() => setCreating(false)} />}
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
