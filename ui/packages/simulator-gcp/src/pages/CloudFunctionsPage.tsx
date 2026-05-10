import { type ColumnDef } from "@tanstack/react-table";
import {
  ResourceListPage,
  StatusBadge,
} from "@sockerless/ui-core/components";
import { fetchCloudFunctions, type CloudFunction } from "../api.js";

const columns: ColumnDef<CloudFunction, unknown>[] = [
  { accessorKey: "name", header: "Name" },
  {
    accessorKey: "state",
    header: "State",
    cell: ({ getValue }) => <StatusBadge status={getValue<string>()} />,
  },
  { accessorKey: "environment", header: "Environment" },
  { accessorKey: "createTime", header: "Created" },
];

export function CloudFunctionsPage() {
  return (
    <ResourceListPage<CloudFunction>
      kicker="gcp · simulator · functions"
      title={<>Functions</>}
      countNoun="function"
      columns={columns}
      queryKey={["cloud-functions"]}
      queryFn={fetchCloudFunctions}
      filterPlaceholder="Filter functions…"
      emptyMessage="No Cloud Functions tracked."
    />
  );
}
