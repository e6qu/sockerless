import { useQuery } from "@tanstack/react-query";
import {
  DataTable,
  MetricsCard,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { fetchSessions } from "../api.js";
import type { BleephubSession } from "../types.js";

const col = createColumnHelper<BleephubSession>();

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const columns: any[] = [
  col.display({
    id: "agentName",
    header: "Agent name",
    cell: (info) => (
      <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>
        {info.row.original.agent?.name ?? "—"}
      </span>
    ),
  }),
  col.display({
    id: "agentId",
    header: "Agent ID",
    cell: (info) => (
      <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
        {info.row.original.agent?.id ?? "—"}
      </span>
    ),
  }),
  col.display({
    id: "version",
    header: "Version",
    cell: (info) => (
      <span style={{ color: "var(--color-fg-muted)" }}>
        {info.row.original.agent?.version ?? "—"}
      </span>
    ),
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
    cell: (info) => {
      const names = info.row.original.agent?.labels?.map((l) => l.name) ?? [];
      if (names.length === 0) return <span style={{ color: "var(--color-fg-subtle)" }}>—</span>;
      return (
        <span
          className="font-mono"
          style={{ color: "var(--color-fg-muted)", fontSize: "0.7rem" }}
        >
          {names.join(", ")}
        </span>
      );
    },
  }),
  col.accessor("sessionId", {
    header: "Session ID",
    cell: (info) => (
      <span
        className="font-mono"
        style={{ color: "var(--color-fg-subtle)", fontSize: "0.7rem" }}
      >
        {(info.getValue() as string).slice(0, 12)}…
      </span>
    ),
  }),
  col.display({
    id: "ephemeral",
    header: "Ephemeral",
    cell: (info) => (
      <span
        className="font-mono uppercase tracking-[0.1em]"
        style={{
          color: info.row.original.agent?.ephemeral
            ? "var(--color-accent)"
            : "var(--color-fg-subtle)",
          fontSize: "0.65rem",
        }}
      >
        {info.row.original.agent?.ephemeral ? "yes" : "no"}
      </span>
    ),
  }),
];

export function RunnersPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["sessions"],
    queryFn: fetchSessions,
    refetchInterval: 5000,
  });

  if (isLoading || !data) return <Spinner label="loading runners" />;

  const totalPending = data.reduce((sum, s) => sum + s.pendingMessages, 0);
  const onlineCount = data.filter((s) => s.agent?.status === "online").length;

  return (
    <div>
      <PageHeading
        kicker="bleephub · runners"
        title={<>Connected runners</>}
        meta={`${data.length} session${data.length === 1 ? "" : "s"} · ${onlineCount} online · ${totalPending} pending message${totalPending === 1 ? "" : "s"}`}
      />

      <div className="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-3">
        <MetricsCard
          title="Connected sessions"
          value={data.length}
          emphasized={data.length > 0}
        />
        <MetricsCard title="Online" value={onlineCount} />
        <MetricsCard
          title="Pending messages"
          value={totalPending}
          emphasized={totalPending > 0}
        />
      </div>

      <DataTable
        data={data}
        columns={columns}
        filterPlaceholder="Filter runners…"
        emptyMessage="No runners connected. Start one with `actions/runner` pointing at this bleephub URL."
      />
    </div>
  );
}
