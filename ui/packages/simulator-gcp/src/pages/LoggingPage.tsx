import { type ColumnDef } from "@tanstack/react-table";
import {
  ResourceListPage,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { fetchLogEntries, type LogEntry } from "../api.js";

const columns: ColumnDef<LogEntry, unknown>[] = [
  { accessorKey: "timestamp", header: "Timestamp" },
  {
    accessorKey: "severity",
    header: "Severity",
    cell: ({ getValue }) => <StatusBadge status={getValue<string>()} />,
  },
  { accessorKey: "logName", header: "Log Name" },
  { accessorKey: "textPayload", header: "Message" },
];

export function LoggingPage() {
  return (
    <ResourceListPage<LogEntry>
      kicker="gcp · simulator · logging"
      title={<>Entries</>}
      countNoun="entry"
      columns={columns}
      queryKey={["log-entries"]}
      queryFn={fetchLogEntries}
      filterPlaceholder="Filter entries…"
      emptyMessage="No log entries tracked."
    />
  );
}
