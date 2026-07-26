import { AwsButton, AwsResourceTable, type AwsColumn } from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import { fetchAmplifyApps, type AmplifyApp } from "../api.js";

// AWS Amplify — Apps. ListApps on the real Amplify REST-JSON API (GET /apps).

const columns: AwsColumn<AmplifyApp>[] = [
  { id: "name", header: "App name", cell: (row) => row.name, value: (row) => row.name },
  { id: "appId", header: "App ID", cell: (row) => row.appId, value: (row) => row.appId },
  { id: "platform", header: "Platform", cell: (row) => row.platform || "–", value: (row) => row.platform },
  {
    id: "defaultDomain",
    header: "Default domain",
    cell: (row) => row.defaultDomain || "–",
    value: (row) => row.defaultDomain,
  },
  { id: "repository", header: "Repository", cell: (row) => row.repository || "–", value: (row) => row.repository },
  {
    id: "createTime",
    header: "Created",
    cell: (row) => formatEpoch(row.createTime),
    value: (row) => String(row.createTime),
  },
];

export function AmplifyPage() {
  return (
    <AwsResourceTable<AmplifyApp>
      title="Apps"
      description="AWS Amplify apps in this account and Region."
      columns={columns}
      queryKey={["amplify-apps"]}
      queryFn={fetchAmplifyApps}
      filterPlaceholder="Find apps"
      emptyTitle="No apps"
      emptyDescription="No AWS Amplify apps exist in this account and Region."
      rowKey={(row) => row.appId}
      tableTestId="amplify-table"
      errorTestId="amplify-error"
      actions={({ refetch, isFetching }) => (
        <AwsButton onClick={refetch} disabled={isFetching}>
          {isFetching ? "Refreshing…" : "Refresh"}
        </AwsButton>
      )}
    />
  );
}
