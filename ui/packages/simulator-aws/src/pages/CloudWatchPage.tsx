import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AwsButton, AwsErrorAlert, AwsModal, AwsResourceTable, AwsStatus, type AwsColumn } from "../console/index.js";
import { formatBytes, formatTimestamp } from "../console/format.js";
import { deleteCWAlarms, fetchCWAlarms, fetchCWDashboards, type CWAlarm, type CWDashboard } from "../api.js";

// Amazon CloudWatch — Alarms and Dashboards. DescribeAlarms, DeleteAlarms, and
// ListDashboards on the real CloudWatch Query API (Version 2010-08-01), the API
// that sits beside the separate CloudWatch Logs API this console also reads.

const alarmColumns: AwsColumn<CWAlarm>[] = [
  { id: "name", header: "Name", cell: (row) => row.alarmName, value: (row) => row.alarmName },
  {
    id: "state",
    header: "State",
    cell: (row) => <AwsStatus status={row.stateValue} kind={row.stateValue === "ALARM" ? "error" : row.stateValue === "OK" ? "success" : "inactive"} />,
    value: (row) => row.stateValue,
  },
  {
    id: "condition",
    header: "Conditions",
    cell: (row) => `${row.statistic || ""} ${row.metricName} ${row.comparisonOperator} ${row.threshold}`.trim(),
    value: (row) => `${row.metricName} ${row.comparisonOperator} ${row.threshold}`,
  },
  { id: "namespace", header: "Namespace", cell: (row) => row.namespace, value: (row) => row.namespace },
  {
    id: "stateUpdatedTimestamp",
    header: "Last state update",
    cell: (row) => formatTimestamp(row.stateUpdatedTimestamp),
    value: (row) => row.stateUpdatedTimestamp,
  },
];

const dashboardColumns: AwsColumn<CWDashboard>[] = [
  { id: "name", header: "Name", cell: (row) => row.dashboardName, value: (row) => row.dashboardName },
  { id: "size", header: "Size", cell: (row) => formatBytes(row.size), value: (row) => String(row.size) },
  {
    id: "lastModified",
    header: "Last modified",
    cell: (row) => formatTimestamp(row.lastModified),
    value: (row) => row.lastModified,
  },
];

function DeleteAlarmsModal({
  alarms,
  onClose,
  clearSelection,
}: {
  alarms: CWAlarm[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    // DeleteAlarms takes the whole list in one call, which is what the real
    // console's delete does for a multi-row selection.
    mutationFn: () => deleteCWAlarms(alarms.map((alarm) => alarm.alarmName)),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["cw-alarms"] }),
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  return (
    <AwsModal
      title={alarms.length === 1 ? `Delete ${alarms[0].alarmName}?` : `Delete ${alarms.length} alarms?`}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="cloudwatch-delete-alarm-confirm"
            disabled={remove.isPending}
            onClick={() => remove.mutate()}
          >
            {remove.isPending ? "Deleting…" : "Delete"}
          </AwsButton>
        </>
      }
    >
      <p>Deleting an alarm stops it evaluating its metric and removes its history.</p>
      <ul>
        {alarms.map((alarm) => (
          <li key={alarm.alarmName}>
            <code>{alarm.alarmName}</code>
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

export function CloudWatchPage() {
  const [deleting, setDeleting] = useState<{ alarms: CWAlarm[]; clearSelection: () => void } | null>(null);
  return (
    <>
      <AwsResourceTable<CWAlarm>
        title="Alarms"
        description="CloudWatch alarms in this account and Region."
        columns={alarmColumns}
        queryKey={["cw-alarms"]}
        queryFn={fetchCWAlarms}
        filterPlaceholder="Find alarms"
        emptyTitle="No alarms"
        emptyDescription="No CloudWatch alarms exist in this account and Region."
        rowKey={(row) => row.alarmName}
        tableTestId="cloudwatch-alarms-table"
        errorTestId="cloudwatch-alarms-error"
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="cloudwatch-delete-alarm"
              disabled={selected.length === 0}
              onClick={() => setDeleting({ alarms: selected, clearSelection })}
            >
              Delete
            </AwsButton>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
          </>
        )}
      />
      <AwsResourceTable<CWDashboard>
        title="Dashboards"
        headingVariant="h2"
        description="CloudWatch dashboards in this account."
        columns={dashboardColumns}
        queryKey={["cw-dashboards"]}
        queryFn={fetchCWDashboards}
        filterPlaceholder="Find dashboards"
        emptyTitle="No dashboards"
        emptyDescription="No CloudWatch dashboards exist in this account."
        rowKey={(row) => row.dashboardName}
        tableTestId="cloudwatch-dashboards-table"
        errorTestId="cloudwatch-dashboards-error"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
      {deleting && (
        <DeleteAlarmsModal alarms={deleting.alarms} clearSelection={deleting.clearSelection} onClose={() => setDeleting(null)} />
      )}
    </>
  );
}
