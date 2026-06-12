import { useQuery } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner, StatusBadge } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { fetchSessions, fetchRepos, fetchActionsRunners } from "../api.js";
import type { BleephubSession } from "../types.js";
import { Box, PageTitle, SectionLabel, StatCard } from "../components/ui.js";

const col = createColumnHelper<BleephubSession>();

const columns = [
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
        style={{
          color: info.row.original.agent?.ephemeral
            ? "var(--color-accent)"
            : "var(--color-fg-subtle)",
          fontSize: "0.78rem",
        }}
      >
        {info.row.original.agent?.ephemeral ? "yes" : "no"}
      </span>
    ),
  }),
];

export function RunnersPage() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["sessions"],
    queryFn: fetchSessions,
    refetchInterval: 5000,
  });

  if (isError) return <InlineError title="Failed to load runners" />;
  if (isLoading || !data) return <Spinner label="loading runners" />;

  const totalPending = data.reduce((sum, s) => sum + s.pendingMessages, 0);

  // No separate "online" stat: the server marks every agent with an active
  // session "online", so the figure would always mirror the session count.
  return (
    <div>
      <PageTitle
        title="Connected runners"
        meta={`${data.length} session${data.length === 1 ? "" : "s"} · ${totalPending} pending message${totalPending === 1 ? "" : "s"}`}
      />

      <div className="mb-6 grid grid-cols-2 gap-3">
        <StatCard title="Connected sessions" value={data.length} emphasized={data.length > 0} />
        <StatCard title="Pending messages" value={totalPending} emphasized={totalPending > 0} />
      </div>

      <DataTable
        data={data}
        columns={columns}
        filterPlaceholder="Filter runners…"
        emptyMessage="No runners connected. Start one with `actions/runner` pointing at this bleephub URL."
      />

      <RegisteredRunnersSection />
    </div>
  );
}

/**
 * GitHub-shape runner registry (GET .../actions/runners). The REST
 * endpoint is repo-scoped while bleephub registers agents globally, so
 * the section reads through the first repo's path — the server returns
 * the full registry for any repo. Hidden until a repo exists, because
 * without one there is no REST path to query.
 */
function RegisteredRunnersSection() {
  const reposQ = useQuery({ queryKey: ["repos"], queryFn: fetchRepos });
  const firstRepo = reposQ.data?.[0]?.full_name;
  const [owner, repo] = firstRepo ? firstRepo.split("/") : ["", ""];

  const runnersQ = useQuery({
    queryKey: ["gh-runners", firstRepo],
    queryFn: () => fetchActionsRunners(owner, repo),
    enabled: !!firstRepo,
    refetchInterval: 5000,
  });

  if (reposQ.isError) {
    return (
      <div className="mt-8">
        <InlineError title="Failed to load repos for the runner registry" detail={String(reposQ.error)} />
      </div>
    );
  }
  if (!firstRepo) return null;

  return (
    <section className="mt-8">
      <SectionLabel>Registered runners</SectionLabel>
      {runnersQ.isLoading && <Spinner label="loading registered runners" />}
      {runnersQ.isError && (
        <InlineError title="Failed to load registered runners" detail={String(runnersQ.error)} />
      )}
      {runnersQ.data &&
        (runnersQ.data.items.length === 0 ? (
          <p style={{ fontSize: "0.84rem", color: "var(--color-fg-muted)" }}>
            No runners registered.
          </p>
        ) : (
          <Box>
            {runnersQ.data.items.map((r, i) => (
              <div
                key={r.id}
                className="flex flex-wrap items-center gap-3"
                style={{
                  padding: "0.6rem 1rem",
                  borderBottom:
                    i < runnersQ.data.items.length - 1 ? "1px solid var(--color-border)" : "none",
                }}
              >
                <span
                  aria-label={r.status}
                  title={r.status}
                  style={{
                    width: 9,
                    height: 9,
                    borderRadius: "999px",
                    background: r.status === "online" ? "var(--gh-open)" : "var(--color-fg-subtle)",
                    flexShrink: 0,
                  }}
                />
                <span style={{ fontSize: "0.88rem", fontWeight: 600, color: "var(--color-fg)" }}>
                  {r.name}
                </span>
                <span style={{ fontSize: "0.76rem", color: "var(--color-fg-muted)" }}>{r.os}</span>
                {r.busy && (
                  <span
                    style={{
                      fontSize: "0.7rem",
                      fontWeight: 600,
                      color: "var(--color-status-warn)",
                      background: "var(--color-status-warn-soft)",
                      padding: "0.08rem 0.45rem",
                      borderRadius: "2rem",
                    }}
                  >
                    busy
                  </span>
                )}
                <span className="flex flex-wrap items-center gap-1">
                  {r.labels.map((l) => (
                    <span
                      key={l.id}
                      className="font-mono"
                      style={{
                        fontSize: "0.7rem",
                        color: "var(--color-accent)",
                        background: "var(--color-accent-soft)",
                        padding: "0.08rem 0.5rem",
                        borderRadius: "2rem",
                      }}
                    >
                      {l.name}
                    </span>
                  ))}
                </span>
              </div>
            ))}
          </Box>
        ))}
    </section>
  );
}
