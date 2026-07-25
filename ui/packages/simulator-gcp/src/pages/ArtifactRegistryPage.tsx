import { Link } from "react-router";
import { GcpResourceTable, type GcpColumn } from "../console/index.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { fetchARRepos, type ARRepo } from "../api.js";
import { useProject } from "../console/project.js";

const columns: GcpColumn<ARRepo>[] = [
  {
    id: "name",
    header: "Name",
    cell: (row) => <Link className="gc-cell-link" to={`/ui/ar/${shortName(row.name)}`}>{shortName(row.name)}</Link>,
    value: (row) => shortName(row.name),
  },
  { id: "format", header: "Format", cell: (row) => row.format ?? "—", value: (row) => row.format ?? "" },
  { id: "mode", header: "Mode", cell: (row) => row.mode ?? "Standard", value: (row) => row.mode ?? "" },
  { id: "createTime", header: "Created", cell: (row) => formatTimestamp(row.createTime ?? ""), value: (row) => row.createTime ?? "" },
];

export function ArtifactRegistryPage() {
  const { project } = useProject();
  return (
    <GcpResourceTable<ARRepo>
      title="Artifact Registry"
      description="Store and manage your build artifacts — container images and language packages — in one place."
      actions={[{ label: "Create repository", icon: "add", primary: true, disabled: true }]}
      columns={columns}
      queryKey={["ar-repos-real", project]}
      queryFn={() => fetchARRepos(project)}
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
