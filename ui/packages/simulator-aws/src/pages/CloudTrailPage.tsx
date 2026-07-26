import { AwsButton, AwsResourceTable, type AwsColumn } from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import { fetchCloudTrailEvents, fetchCloudTrailTrails, type CloudTrailEvent, type CloudTrailTrail } from "../api.js";

// AWS CloudTrail — Trails and Event history, the two surfaces the real console
// leads with. DescribeTrails and LookupEvents on the real CloudTrail API
// (X-Amz-Target CloudTrail_20131101.<Op>).

const trailColumns: AwsColumn<CloudTrailTrail>[] = [
  { id: "name", header: "Name", cell: (row) => row.name, value: (row) => row.name },
  { id: "homeRegion", header: "Home Region", cell: (row) => row.homeRegion, value: (row) => row.homeRegion },
  {
    id: "isMultiRegionTrail",
    header: "Multi-Region trail",
    cell: (row) => (row.isMultiRegionTrail ? "Yes" : "No"),
    value: (row) => (row.isMultiRegionTrail ? "Yes" : "No"),
  },
  { id: "s3BucketName", header: "S3 bucket", cell: (row) => row.s3BucketName || "–", value: (row) => row.s3BucketName },
  {
    id: "logFileValidationEnabled",
    header: "Log file validation",
    cell: (row) => (row.logFileValidationEnabled ? "Enabled" : "Disabled"),
    value: (row) => (row.logFileValidationEnabled ? "Enabled" : "Disabled"),
  },
];

const eventColumns: AwsColumn<CloudTrailEvent>[] = [
  { id: "eventName", header: "Event name", cell: (row) => row.eventName, value: (row) => row.eventName },
  {
    id: "eventTime",
    header: "Event time",
    cell: (row) => formatEpoch(row.eventTime),
    value: (row) => String(row.eventTime),
  },
  { id: "username", header: "User name", cell: (row) => row.username || "–", value: (row) => row.username },
  { id: "eventSource", header: "Event source", cell: (row) => row.eventSource, value: (row) => row.eventSource },
  { id: "readOnly", header: "Read-only", cell: (row) => row.readOnly || "–", value: (row) => row.readOnly },
];

export function CloudTrailPage() {
  return (
    <>
      <AwsResourceTable<CloudTrailTrail>
        title="Trails"
        description="AWS CloudTrail trails in this account and Region."
        columns={trailColumns}
        queryKey={["cloudtrail-trails"]}
        queryFn={fetchCloudTrailTrails}
        filterPlaceholder="Find trails"
        emptyTitle="No trails"
        emptyDescription="No AWS CloudTrail trails exist in this account and Region."
        rowKey={(row) => row.trailARN || row.name}
        tableTestId="cloudtrail-trails-table"
        errorTestId="cloudtrail-trails-error"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
      <AwsResourceTable<CloudTrailEvent>
        title="Event history"
        headingVariant="h2"
        description="The management events CloudTrail recorded for this account."
        columns={eventColumns}
        queryKey={["cloudtrail-events"]}
        queryFn={fetchCloudTrailEvents}
        filterPlaceholder="Find events"
        emptyTitle="No events"
        emptyDescription="CloudTrail has recorded no management events for this account yet."
        rowKey={(row) => row.eventId}
        tableTestId="cloudtrail-events-table"
        errorTestId="cloudtrail-events-error"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
    </>
  );
}
