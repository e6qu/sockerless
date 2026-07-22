import { AwsResourceTable, AwsStatus, type AwsColumn } from "../console/index.js";
import { formatBytes, formatEpoch, formatRetention } from "../console/format.js";
import { fetchCWLogGroups, type CWLogGroup } from "../api.js";

const columns: AwsColumn<CWLogGroup>[] = [
  { id: "name", header: "Log group", cell: (row) => row.name, value: (row) => row.name },
  { id: "retentionInDays", header: "Retention", cell: (row) => formatRetention(row.retentionInDays), value: (row) => String(row.retentionInDays) },
  { id: "storedBytes", header: "Stored", cell: (row) => formatBytes(row.storedBytes), value: (row) => String(row.storedBytes) },
  { id: "creationTime", header: "Created at", cell: (row) => formatEpoch(row.creationTime), value: (row) => String(row.creationTime) },
];

export function LogGroupsPage() {
  return (
    <AwsResourceTable<CWLogGroup>
      title="Log groups"
      breadcrumbLabel="Log groups"
      description="Log groups in this account and Region."
      columns={columns}
      queryKey={["log-groups"]}
      queryFn={fetchCWLogGroups}
      filterPlaceholder="Find log groups"
      emptyTitle="No log groups"
      emptyDescription="No log groups exist in this account and Region."
      rowKey={(row) => row.name}
    />
  );
}
