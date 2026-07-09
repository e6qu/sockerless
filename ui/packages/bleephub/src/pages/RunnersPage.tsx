import { useQuery } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner, StatusBadge } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { fetchRepos, fetchActionsRunners } from "../api.js";
import type { GithubRunner } from "../types.js";
import { PageTitle, StatCard } from "../components/ui.js";

const col = createColumnHelper<GithubRunner>();

const columns = [
  col.display({
    id: "name",
    header: "Runner name",
    cell: (info) => (
      <span style={{ color: "var(--color-fg)", fontWeight: 500 }}>
        {info.row.original.name}
      </span>
    ),
  }),
  col.display({
    id: "id",
    header: "Runner identifier",
    cell: (info) => (
      <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
        {info.row.original.id}
      </span>
    ),
  }),
  col.accessor("os", {
    header: "Operating system",
    cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>,
  }),
  col.display({
    id: "status",
    header: "Status",
    cell: (info) => <StatusBadge status={info.row.original.status} />,
  }),
  col.display({
    id: "busy",
    header: "Busy",
    cell: (info) => (
      <span
        style={{
          color: info.row.original.busy ? "var(--color-status-warn)" : "var(--color-fg-subtle)",
          fontSize: "0.78rem",
          fontWeight: info.row.original.busy ? 600 : 400,
        }}
      >
        {info.row.original.busy ? "yes" : "no"}
      </span>
    ),
  }),
  col.display({
    id: "labels",
    header: "Labels",
    cell: (info) => {
      const names = info.row.original.labels.map((l) => l.name);
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
];

export function RunnersPage() {
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
    return <InlineError title="Failed to load repositories for the runner registry" />;
  }
  if (reposQ.isLoading || !reposQ.data) return <Spinner label="loading runners" />;

  if (!firstRepo) {
    return (
      <div>
        <PageTitle title="Registered runners" meta="0 runners" />
        <DataTable
          data={[]}
          columns={columns}
          filterPlaceholder="Filter runners…"
          emptyMessage="Create a repository to query the GitHub Actions runner registry."
        />
      </div>
    );
  }

  if (runnersQ.isError) return <InlineError title="Failed to load registered runners" />;
  if (runnersQ.isLoading || !runnersQ.data) return <Spinner label="loading runners" />;

  const runners = runnersQ.data.items;
  const totalCount = runnersQ.data.totalCount;
  const online = runners.filter((runner) => runner.status === "online").length;
  const busy = runners.filter((runner) => runner.busy).length;

  return (
    <div>
      <PageTitle
        title="Registered runners"
        meta={`${totalCount} runner${totalCount === 1 ? "" : "s"} · ${firstRepo}`}
      />

      <div className="mb-6 grid grid-cols-3 gap-3">
        <StatCard
          title="Registered runners"
          value={totalCount}
          emphasized={runners.length > 0}
        />
        <StatCard title="Online runners" value={online} emphasized={online > 0} />
        <StatCard title="Busy runners" value={busy} emphasized={busy > 0} />
      </div>

      <DataTable
        data={runners}
        columns={columns}
        filterPlaceholder="Filter runners…"
        emptyMessage="No runners registered."
      />
    </div>
  );
}
