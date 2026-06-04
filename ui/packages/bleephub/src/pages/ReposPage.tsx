import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import {
  DataTable,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { type ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { fetchRepos } from "../api.js";
import type { BleephubRepo } from "../types.js";

const col = createColumnHelper<BleephubRepo>();

const columns: ColumnDef<BleephubRepo>[] = [
  col.accessor("full_name", {
    header: "Repository",
    cell: (info) => {
      const fullName = info.getValue() as string;
      const [owner, repo] = fullName.split("/");
      return (
        <Link
          to={`/ui/repos/${owner}/${repo}`}
          style={{ color: "var(--color-accent)", fontWeight: 500, textDecoration: "none" }}
          onMouseEnter={(e) => { (e.currentTarget as HTMLElement).style.textDecoration = "underline"; }}
          onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.textDecoration = "none"; }}
        >
          {fullName}
        </Link>
      );
    },
  }),
  col.accessor("description", {
    header: "Description",
    cell: (info) => (
      <span style={{ color: "var(--color-fg-muted)" }}>
        {info.getValue() || "—"}
      </span>
    ),
  }),
  col.accessor("default_branch", {
    header: "Branch",
    cell: (info) => (
      <span style={{ color: "var(--color-accent)" }}>{info.getValue()}</span>
    ),
  }),
  col.accessor("visibility", {
    header: "Visibility",
    cell: (info) => <StatusBadge status={info.getValue()} />,
  }),
  col.accessor("created_at", {
    header: "Created",
    cell: (info) => new Date(info.getValue()).toLocaleString(),
  }),
];

export function ReposPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["repos"],
    queryFn: fetchRepos,
    refetchInterval: 10000,
  });

  if (isLoading || !data) return <Spinner label="loading repos" />;

  return (
    <div>
      <PageHeading
        kicker="bleephub · repos"
        title={<>Repositories</>}
        meta={`${data.length} repo${data.length === 1 ? "" : "s"} indexed`}
      />
      <DataTable
        data={data}
        columns={columns}
        filterPlaceholder="Filter repos…"
        emptyMessage="No repositories. Create one via POST /api/v3/user/repos or push to git."
      />
    </div>
  );
}
