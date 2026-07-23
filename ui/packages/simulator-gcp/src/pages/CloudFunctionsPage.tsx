import { Link } from "react-router";
import { GcpResourceTable, GcpStatus, type GcpColumn } from "../console/index.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { fetchCloudFunctions, type CloudFunction } from "../api.js";

const columns: GcpColumn<CloudFunction>[] = [
  {
    id: "name",
    header: "Name",
    cell: (row) => <Link className="gc-cell-link" to={`/ui/functions/${shortName(row.name)}`}>{shortName(row.name)}</Link>,
    value: (row) => shortName(row.name),
  },
  { id: "state", header: "State", cell: (row) => <GcpStatus status={row.state ?? "UNKNOWN"} />, value: (row) => row.state ?? "" },
  { id: "environment", header: "Environment", cell: (row) => row.environment ?? "—", value: (row) => row.environment ?? "" },
  { id: "createTime", header: "Created", cell: (row) => formatTimestamp(row.createTime ?? ""), value: (row) => row.createTime ?? "" },
];

export function CloudFunctionsPage() {
  return (
    <GcpResourceTable<CloudFunction>
      title="Cloud Run functions"
      description="Run your code in response to events without provisioning or managing servers."
      actions={[{ label: "Create function", icon: "add", primary: true, disabled: true }]}
      columns={columns}
      queryKey={["cloud-functions-real"]}
      queryFn={fetchCloudFunctions}
      filterPlaceholder="Filter functions"
      resourceNoun="functions"
      empty={{
        headline: "Write and deploy your first function",
        description: "Functions run your code in response to events, scaling from zero automatically.",
        primaryLabel: "Create function",
      }}
      rowKey={(row) => row.name}
    />
  );
}
