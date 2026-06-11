import { type ColumnDef } from "@tanstack/react-table";
import { ResourceListPage } from "@sockerless/ui-core/components";
import { fetchMonitorLogs, type MonitorLogRow } from "../api.js";

const columns: ColumnDef<MonitorLogRow, unknown>[] = [
  {
    accessorFn: (row) => row["TimeGenerated"] ?? "",
    id: "time",
    header: "Time",
  },
  {
    accessorFn: (row) =>
      row["ContainerGroupName_s"] ?? row["AppRoleName"] ?? "",
    id: "source",
    header: "Source",
  },
  {
    accessorFn: (row) => row["Log_s"] ?? row["Message"] ?? "",
    id: "message",
    header: "Message",
  },
];

export function MonitorPage() {
  return (
    <ResourceListPage<MonitorLogRow>
      kicker="azure · simulator · monitor"
      title={<>Logs</>}
      countNoun="entry"
      columns={columns}
      queryKey={["monitor-logs"]}
      queryFn={fetchMonitorLogs}
      filterPlaceholder="Filter logs…"
      emptyMessage="No monitor logs tracked."
    />
  );
}
