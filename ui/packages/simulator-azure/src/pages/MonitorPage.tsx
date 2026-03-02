import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, Spinner } from "@sockerless/ui-core/components";
import { fetchMonitorLogs, type MonitorLogRow } from "../api.js";

const columns: ColumnDef<MonitorLogRow, any>[] = [
  { accessorFn: (row) => row["TimeGenerated"] as string, id: "time", header: "Time" },
  { accessorFn: (row) => row["ContainerGroupName_s"] ?? row["AppRoleName"] ?? "", id: "source", header: "Source" },
  { accessorFn: (row) => row["Log_s"] ?? row["Message"] ?? "", id: "message", header: "Message" },
];

export function MonitorPage() {
  const { data, isLoading } = useQuery({ queryKey: ["monitor-logs"], queryFn: fetchMonitorLogs, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">Azure Monitor Logs</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
