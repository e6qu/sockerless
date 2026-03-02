import { useQuery } from "@tanstack/react-query";
import { DataTable, Spinner, StatusBadge } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { fetchRunners } from "../api.js";
import type { GitlabhubRunner } from "../types.js";

const col = createColumnHelper<GitlabhubRunner>();

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const columns: any[] = [
  col.accessor("id", { header: "ID" }),
  col.accessor("description", { header: "Description" }),
  col.accessor("active", {
    header: "Active",
    cell: (info) => (
      <StatusBadge status={info.getValue() ? "ok" : "inactive"} />
    ),
  }),
  col.accessor("tag_list", {
    header: "Tags",
    cell: (info) => {
      const tags = info.getValue();
      return tags && tags.length > 0 ? tags.join(", ") : "—";
    },
  }),
];

export function RunnersPage() {
  const { data, isLoading } = useQuery({
    queryKey: ["runners"],
    queryFn: fetchRunners,
    refetchInterval: 5000,
  });

  if (isLoading || !data) return <Spinner />;

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Runners ({data.length})</h2>
      <DataTable data={data} columns={columns} filterPlaceholder="Filter runners..." />
    </div>
  );
}
