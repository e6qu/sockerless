import { type ColumnDef } from "@tanstack/react-table";
import { ResourceListPage } from "@sockerless/ui-core/components";
import { fetchARRepos, type ARRepo } from "../api.js";

const columns: ColumnDef<ARRepo, unknown>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "format", header: "Format" },
  { accessorKey: "createTime", header: "Created" },
];

export function ArtifactRegistryPage() {
  return (
    <ResourceListPage<ARRepo>
      kicker="gcp · simulator · artifact registry"
      title={<>Repositories</>}
      countNoun="repository"
      columns={columns}
      queryKey={["ar-repos"]}
      queryFn={fetchARRepos}
      filterPlaceholder="Filter repositories…"
      emptyMessage="No Artifact Registry repositories tracked."
    />
  );
}
