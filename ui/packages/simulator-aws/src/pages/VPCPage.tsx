import { useNavigate } from "react-router";
import { AwsButton, AwsResourceTable, AwsRowLink, AwsStatus, type AwsColumn } from "../console/index.js";
import { fetchEC2Vpcs, type EC2Vpc } from "../api.js";

// Amazon Virtual Private Cloud (VPC) — Your VPCs. DescribeVpcs on the real
// Amazon EC2 Query API, which is the API the VPC console reads: VPC has no API
// of its own, its resources live in the EC2 API surface.

const columns: AwsColumn<EC2Vpc>[] = [
  { id: "name", header: "Name", cell: (row) => row.name || "–", value: (row) => row.name },
  {
    id: "vpcId",
    header: "VPC ID",
    cell: (row) => <AwsRowLink to={`/ui/vpc/${encodeURIComponent(row.vpcId)}`}>{row.vpcId}</AwsRowLink>,
    value: (row) => row.vpcId,
  },
  { id: "state", header: "State", cell: (row) => <AwsStatus status={row.state} />, value: (row) => row.state },
  { id: "cidrBlock", header: "IPv4 CIDR", cell: (row) => row.cidrBlock, value: (row) => row.cidrBlock },
  {
    id: "isDefault",
    header: "Default VPC",
    cell: (row) => (row.isDefault ? "Yes" : "No"),
    value: (row) => (row.isDefault ? "Yes" : "No"),
  },
  { id: "tenancy", header: "Tenancy", cell: (row) => row.instanceTenancy || "–", value: (row) => row.instanceTenancy },
];

export function VPCPage() {
  const navigate = useNavigate();
  return (
    <AwsResourceTable<EC2Vpc>
      title="Your VPCs"
      description="Virtual private clouds in this account and Region."
      columns={columns}
      queryKey={["ec2-vpcs"]}
      queryFn={fetchEC2Vpcs}
      filterPlaceholder="Find VPCs"
      emptyTitle="No VPCs"
      emptyDescription="No virtual private clouds exist in this account and Region."
      rowKey={(row) => row.vpcId}
      tableTestId="vpc-table"
      errorTestId="vpc-error"
      actions={({ selected, refetch, isFetching }) => (
        <>
          <AwsButton
            data-testid="vpc-view-vpc"
            disabled={selected.length !== 1}
            onClick={() => navigate(`/ui/vpc/${encodeURIComponent(selected[0].vpcId)}`)}
          >
            View details
          </AwsButton>
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        </>
      )}
    />
  );
}
