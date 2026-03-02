import { useQuery } from "@tanstack/react-query";
import { DataTable, MetricsCard, Spinner, StatusBadge } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { fetchSessions } from "../api.js";
import type { BleephubSession } from "../types.js";

const col = createColumnHelper<BleephubSession>();

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const columns: any[] = [
  col.display({
    id: "agentName",
    header: "Agent Name",
    cell: (info) => info.row.original.agent?.name ?? "—",
  }),
  col.display({
    id: "agentId",
    header: "Agent ID",
    cell: (info) => info.row.original.agent?.id ?? "—",
  }),
  col.display({
    id: "version",
    header: "Version",
    cell: (info) => info.row.original.agent?.version ?? "—",
  }),
  col.display({
    id: "status",
    header: "Status",
    cell: (info) => {
      const status = info.row.original.agent?.status ?? "unknown";
      return <StatusBadge status={status} />;
    },
  }),
  col.display({
    id: "labels",
    header: "Labels",
    cell: (info) =>
      info.row.original.agent?.labels?.map((l) => l.name).join(", ") ?? "—",
  }),
  col.accessor("sessionId", { header: "Session ID" }),
  col.display({
    id: "ephemeral",
    header: "Ephemeral",
    cell: (info) => (info.row.original.agent?.ephemeral ? "Yes" : "No"),
  }),
];

export function RunnersPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["sessions"],
    queryFn: fetchSessions,
    refetchInterval: 5000,
  });

  if (isLoading || !data) return <Spinner />;

  const totalPending = data.reduce((sum, s) => sum + s.pendingMessages, 0);

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Runners ({data.length})</h2>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
        <MetricsCard title="Connected Sessions" value={data.length} />
        <MetricsCard title="Pending Messages" value={totalPending} />
      </div>

      <DataTable data={data} columns={columns} filterPlaceholder="Filter runners..." />
    </div>
  );
}
