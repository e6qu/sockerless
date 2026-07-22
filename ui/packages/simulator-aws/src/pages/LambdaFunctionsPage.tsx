import { AwsResourceTable, AwsStatus, type AwsColumn } from "../console/index.js";
import { formatTimestamp } from "../console/format.js";
import { fetchLambdaFunctions, type LambdaFunction } from "../api.js";

const columns: AwsColumn<LambdaFunction>[] = [
  { id: "name", header: "Function name", cell: (row) => row.name, value: (row) => row.name },
  { id: "state", header: "State", cell: (row) => <AwsStatus status={row.state} />, value: (row) => row.state },
  { id: "runtime", header: "Runtime", cell: (row) => row.runtime, value: (row) => row.runtime },
  { id: "memorySize", header: "Memory", cell: (row) => `${row.memorySize} MB`, value: (row) => String(row.memorySize) },
  { id: "timeout", header: "Timeout", cell: (row) => `${row.timeout} s`, value: (row) => String(row.timeout) },
  { id: "lastModified", header: "Last modified", cell: (row) => formatTimestamp(row.lastModified), value: (row) => row.lastModified },
];

export function LambdaFunctionsPage() {
  return (
    <AwsResourceTable<LambdaFunction>
      title="Functions"
      breadcrumbLabel="Functions"
      description="Functions in this account and Region."
      columns={columns}
      queryKey={["lambda-functions"]}
      queryFn={fetchLambdaFunctions}
      filterPlaceholder="Find functions"
      emptyTitle="No functions"
      emptyDescription="No functions exist in this account and Region."
      rowKey={(row) => row.name}
    />
  );
}
