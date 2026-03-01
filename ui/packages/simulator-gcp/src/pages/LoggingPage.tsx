import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, StatusBadge, Spinner } from "@sockerless/ui-core/components";
import { fetchLogEntries, type LogEntry } from "../api.js";

const columns: ColumnDef<LogEntry, any>[] = [
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
  const { data, isLoading } = useQuery({ queryKey: ["log-entries"], queryFn: fetchLogEntries, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">Cloud Logging</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
