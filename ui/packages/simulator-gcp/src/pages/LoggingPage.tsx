import { GcpResourceTable, GcpStatus, type GcpColumn } from "../console/index.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { fetchLogEntries, type LogEntry } from "../api.js";
import { useProject } from "../console/project.js";

// Cloud Logging omits severity when it is the default; the console reads that
// as DEFAULT rather than a blank cell.
const severityOf = (row: LogEntry) => row.severity ?? "DEFAULT";

const columns: GcpColumn<LogEntry>[] = [
  { id: "timestamp", header: "Timestamp", cell: (row) => formatTimestamp(row.timestamp), value: (row) => row.timestamp },
  { id: "severity", header: "Severity", cell: (row) => <GcpStatus status={severityOf(row)} />, value: severityOf },
  { id: "logName", header: "Log name", cell: (row) => shortName(row.logName), value: (row) => row.logName },
  { id: "message", header: "Summary", cell: (row) => row.textPayload ?? "", value: (row) => row.textPayload ?? "" },
];

export function LoggingPage() {
  const { project } = useProject();
  return (
    <GcpResourceTable<LogEntry>
      title="Logs Explorer"
      description="Search, filter, and inspect log entries across the project."
      columns={columns}
      queryKey={["log-entries-real", project]}
      queryFn={() => fetchLogEntries(project)}
      filterPlaceholder="Filter entries"
      resourceNoun="log entries"
      empty={{
        headline: "No log entries match this query",
        description: "Entries written across the project appear here as it runs.",
        primaryLabel: "Refresh",
      }}
      rowKey={(row) => `${row.timestamp}|${row.logName}|${row.textPayload ?? ""}`}
    />
  );
}
