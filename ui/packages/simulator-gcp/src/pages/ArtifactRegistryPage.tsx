import { GcpResourceTable, type GcpColumn } from "../console/index.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { fetchARRepos, type ARRepo } from "../api.js";

const columns: GcpColumn<ARRepo>[] = [
  { id: "name", header: "Name", cell: (row) => shortName(row.name), value: (row) => shortName(row.name) },
  { id: "format", header: "Format", cell: (row) => row.format, value: (row) => row.format },
  { id: "createTime", header: "Created", cell: (row) => formatTimestamp(row.createTime), value: (row) => row.createTime },
];

export function ArtifactRegistryPage() {
  return (
    <GcpResourceTable<ARRepo>
      title="Artifact Registry"
      description="Store and manage your build artifacts — container images and language packages — in one place."
      actions={[{ label: "Create repository", glyph: "+", primary: true, disabled: true }]}
      columns={columns}
      queryKey={["ar-repos"]}
      queryFn={fetchARRepos}
      filterPlaceholder="Filter repositories"
      resourceNoun="repositories"
      empty={{
        headline: "Store your build artifacts",
        description: "Create a repository to store container images and language packages.",
        primaryLabel: "Create repository",
      }}
      rowKey={(row) => row.name}
    />
  );
}
