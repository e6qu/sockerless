import { AwsResourceTable, AwsStatus, type AwsColumn } from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import { fetchECRRepos, type ECRRepo } from "../api.js";

const columns: AwsColumn<ECRRepo>[] = [
  { id: "name", header: "Repository name", cell: (row) => row.name, value: (row) => row.name },
  { id: "uri", header: "URI", cell: (row) => row.uri, value: (row) => row.uri },
  { id: "createdAt", header: "Created at", cell: (row) => formatEpoch(row.createdAt), value: (row) => String(row.createdAt) },
];

export function ECRReposPage() {
  return (
    <AwsResourceTable<ECRRepo>
      title="Repositories"
      breadcrumbLabel="Repositories"
      description="Private repositories in this account and Region."
      columns={columns}
      queryKey={["ecr-repos"]}
      queryFn={fetchECRRepos}
      filterPlaceholder="Find repositories"
      emptyTitle="No repositories"
      emptyDescription="No private repositories exist in this account and Region."
      rowKey={(row) => row.name}
    />
  );
}
