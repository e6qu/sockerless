import { AwsButton, AwsResourceTable, AwsStatus, type AwsColumn } from "../console/index.js";
import { formatTimestamp } from "../console/format.js";
import { fetchScheduleGroups, fetchSchedules, type Schedule, type ScheduleGroup } from "../api.js";

// Amazon EventBridge Scheduler — Schedules and Schedule groups. ListSchedules
// and ListScheduleGroups on the real Scheduler REST-JSON API.

const scheduleColumns: AwsColumn<Schedule>[] = [
  { id: "name", header: "Schedule name", cell: (row) => row.name, value: (row) => row.name },
  { id: "groupName", header: "Group", cell: (row) => row.groupName || "default", value: (row) => row.groupName },
  { id: "state", header: "State", cell: (row) => <AwsStatus status={row.state} />, value: (row) => row.state },
  { id: "targetArn", header: "Target", cell: (row) => row.targetArn || "–", value: (row) => row.targetArn },
  {
    id: "creationDate",
    header: "Created",
    cell: (row) => formatTimestamp(row.creationDate),
    value: (row) => row.creationDate,
  },
];

const groupColumns: AwsColumn<ScheduleGroup>[] = [
  { id: "name", header: "Group name", cell: (row) => row.name, value: (row) => row.name },
  { id: "state", header: "State", cell: (row) => <AwsStatus status={row.state} />, value: (row) => row.state },
  {
    id: "creationDate",
    header: "Created",
    cell: (row) => formatTimestamp(row.creationDate),
    value: (row) => row.creationDate,
  },
];

export function SchedulerPage() {
  return (
    <>
      <AwsResourceTable<Schedule>
        title="Schedules"
        description="EventBridge Scheduler schedules in this account and Region."
        columns={scheduleColumns}
        queryKey={["scheduler-schedules"]}
        queryFn={fetchSchedules}
        filterPlaceholder="Find schedules"
        emptyTitle="No schedules"
        emptyDescription="No EventBridge Scheduler schedules exist in this account and Region."
        rowKey={(row) => row.arn || `${row.groupName}/${row.name}`}
        tableTestId="scheduler-schedules-table"
        errorTestId="scheduler-schedules-error"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
      <AwsResourceTable<ScheduleGroup>
        title="Schedule groups"
        headingVariant="h2"
        description="The groups schedules are organized into."
        columns={groupColumns}
        queryKey={["scheduler-groups"]}
        queryFn={fetchScheduleGroups}
        filterPlaceholder="Find schedule groups"
        emptyTitle="No schedule groups"
        emptyDescription="No EventBridge Scheduler schedule groups exist in this account and Region."
        rowKey={(row) => row.name}
        tableTestId="scheduler-groups-table"
        errorTestId="scheduler-groups-error"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
    </>
  );
}
