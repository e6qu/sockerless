import { AwsButton, AwsResourceTable, type AwsColumn } from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import { fetchCodeBuildProjects, type CodeBuildProject } from "../api.js";

// AWS CodeBuild — Build projects. ListProjects for the names and
// BatchGetProjects for the descriptions, the two real CodeBuild operations the
// console's Build projects page reads.

const columns: AwsColumn<CodeBuildProject>[] = [
  { id: "name", header: "Name", cell: (row) => row.name, value: (row) => row.name },
  { id: "sourceType", header: "Source provider", cell: (row) => row.sourceType || "–", value: (row) => row.sourceType },
  {
    id: "environmentImage",
    header: "Environment image",
    cell: (row) => row.environmentImage || "–",
    value: (row) => row.environmentImage,
  },
  {
    id: "environmentType",
    header: "Environment type",
    cell: (row) => row.environmentType || "–",
    value: (row) => row.environmentType,
  },
  { id: "serviceRole", header: "Service role", cell: (row) => row.serviceRole || "–", value: (row) => row.serviceRole },
  { id: "created", header: "Created", cell: (row) => formatEpoch(row.created), value: (row) => String(row.created) },
];

export function CodeBuildPage() {
  return (
    <AwsResourceTable<CodeBuildProject>
      title="Build projects"
      description="AWS CodeBuild projects in this account and Region."
      columns={columns}
      queryKey={["codebuild-projects"]}
      queryFn={fetchCodeBuildProjects}
      filterPlaceholder="Find build projects"
      emptyTitle="No build projects"
      emptyDescription="No AWS CodeBuild projects exist in this account and Region."
      rowKey={(row) => row.arn || row.name}
      tableTestId="codebuild-table"
      errorTestId="codebuild-error"
      actions={({ refetch, isFetching }) => (
        <AwsButton onClick={refetch} disabled={isFetching}>
          {isFetching ? "Refreshing…" : "Refresh"}
        </AwsButton>
      )}
    />
  );
}
