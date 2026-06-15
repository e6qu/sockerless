import { useQuery } from "@tanstack/react-query";
import {
  PageHeading,
  DataTable,
  Spinner,
  InlineError,
} from "@sockerless/ui-core/components";
import type { ColumnDef } from "@tanstack/react-table";
import { api } from "../api.js";
import type { Runner } from "../types.js";

const columns: ColumnDef<Runner, unknown>[] = [
  { accessorKey: "id", header: "#" },
  {
    accessorKey: "token",
    header: "Token",
    cell: (c) => <code>{String(c.getValue())}</code>,
  },
];

export function RunnersPage() {
  const runners = useQuery({ queryKey: ["runners"], queryFn: api.runners });

  if (runners.isLoading) return <Spinner />;
  if (runners.error) {
    return <InlineError title="Failed to load runners" detail={runners.error as Error} />;
  }

  return (
    <div>
      <PageHeading
        kicker="CI/CD"
        title="Runners"
        meta={`${runners.data!.length} registered`}
      />
      <DataTable
        data={runners.data!}
        columns={columns}
        emptyMessage="No runners registered yet."
      />
    </div>
  );
}
