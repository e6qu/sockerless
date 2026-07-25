import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AwsButton, AwsContainer, AwsPageHeader, AwsResourceTable, type AwsColumn } from "../console/index.js";
import { formatBytes, formatEpoch, formatRetention } from "../console/format.js";
import { fetchCWLogEvents, fetchCWLogGroup, fetchCWLogStreams, type CWLogGroup, type CWLogStream } from "../api.js";
import { DeleteLogGroupsModal } from "./LogGroupsPage.js";

// Amazon CloudWatch Logs — Log group detail. Reads the real CloudWatch Logs
// awsjson1.1 API (DescribeLogGroups narrowed to an exact name, since there is
// no singular GetLogGroup operation; DescribeLogStreams for the sub-resource
// table; GetLogEvents for the recent-events view of a selected stream) with
// the operator's federated credentials, and reuses the Log groups list
// page's real DeleteLogGroup action.

const streamColumns: AwsColumn<CWLogStream>[] = [
  { id: "name", header: "Log stream", cell: (row) => row.name, value: (row) => row.name },
  {
    id: "lastEventTimestamp",
    header: "Last event time",
    cell: (row) => (row.lastEventTimestamp ? formatEpoch(row.lastEventTimestamp) : "–"),
    value: (row) => String(row.lastEventTimestamp),
  },
  {
    id: "creationTime",
    header: "Created",
    cell: (row) => formatEpoch(row.creationTime),
    value: (row) => String(row.creationTime),
  },
];

function RecentEventsPanel({ logGroupName, logStreamName }: { logGroupName: string; logStreamName: string }) {
  const events = useQuery({
    queryKey: ["log-events", logGroupName, logStreamName],
    queryFn: () => fetchCWLogEvents(logGroupName, logStreamName),
  });
  return (
    <AwsContainer>
      <h2 className="aws-subheading">Recent events — {logStreamName}</h2>
      {events.isError ? (
        <div className="aws-flash aws-flash-error" role="alert" data-testid="logs-events-error">
          <strong>Could not load events.</strong>{" "}
          {events.error instanceof Error ? events.error.message : "The request failed."}
        </div>
      ) : events.isLoading ? (
        <div className="aws-empty" role="status">
          Loading events…
        </div>
      ) : (events.data ?? []).length === 0 ? (
        <div className="aws-empty">
          <strong>No events</strong>
          <p>This log stream has no events yet.</p>
        </div>
      ) : (
        <div className="aws-log-events" data-testid="logs-events-panel">
          {(events.data ?? []).map((event, index) => (
            <div className="aws-log-event-line" key={`${event.timestamp}-${index}`}>
              <span className="aws-log-event-timestamp">{formatEpoch(event.timestamp)}</span>
              <span className="aws-log-event-message">{event.message}</span>
            </div>
          ))}
        </div>
      )}
    </AwsContainer>
  );
}

export function LogGroupDetailPage() {
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [deleting, setDeleting] = useState(false);
  const [viewingStream, setViewingStream] = useState<string | null>(null);
  const group = useQuery({ queryKey: ["log-group", name], queryFn: () => fetchCWLogGroup(name) });

  const asCWLogGroup: CWLogGroup | null = group.data ?? null;

  return (
    <>
      <AwsPageHeader
        title={name}
        description="Log group in Amazon CloudWatch Logs."
        actions={
          <AwsButton data-testid="logs-log-group-delete" disabled={!group.isSuccess} onClick={() => setDeleting(true)}>
            Delete
          </AwsButton>
        }
      />
      <AwsContainer>
        {group.isError ? (
          <div className="aws-flash aws-flash-error" role="alert" data-testid="logs-log-group-error">
            <strong>Could not load the log group.</strong>{" "}
            {group.error instanceof Error ? group.error.message : "The request failed."}
          </div>
        ) : group.isLoading ? (
          <div className="aws-empty" role="status">
            Loading log group…
          </div>
        ) : group.data ? (
          <dl className="aws-key-value" data-testid="logs-log-group-summary">
            <dt>Retention</dt>
            <dd>{formatRetention(group.data.retentionInDays)}</dd>
            <dt>Stored bytes</dt>
            <dd>{formatBytes(group.data.storedBytes)}</dd>
            <dt>Created at</dt>
            <dd>{formatEpoch(group.data.creationTime)}</dd>
          </dl>
        ) : null}
      </AwsContainer>
      <AwsResourceTable<CWLogStream>
        title="Log streams"
        description="Log streams in this log group. Select one and choose View recent events to read it."
        columns={streamColumns}
        queryKey={["log-streams", name]}
        queryFn={() => fetchCWLogStreams(name)}
        filterPlaceholder="Find log streams"
        emptyTitle="No log streams"
        emptyDescription="This log group has no log streams."
        rowKey={(row) => row.name}
        tableTestId="logs-streams-table"
        rowTestId={(row) => `logs-stream-row-${row.name}`}
        actions={({ selected, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="logs-view-stream-events"
              disabled={selected.length !== 1}
              onClick={() => setViewingStream(selected[0].name)}
            >
              View recent events
            </AwsButton>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
          </>
        )}
      />
      {viewingStream && <RecentEventsPanel logGroupName={name} logStreamName={viewingStream} />}
      {deleting && asCWLogGroup && (
        <DeleteLogGroupsModal
          groups={[asCWLogGroup]}
          clearSelection={() => navigate("/ui/logs")}
          onClose={() => setDeleting(false)}
        />
      )}
    </>
  );
}
