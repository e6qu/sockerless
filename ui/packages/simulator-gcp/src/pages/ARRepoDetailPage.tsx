import { useParams } from "react-router";
import { GcpResourceDetail } from "../console/index.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { fetchARRepo, type ARRepo } from "../api.js";

export function ARRepoDetailPage() {
  const { name = "" } = useParams();
  return (
    <GcpResourceDetail<ARRepo>
      title={shortName(name)}
      description="Artifact Registry repository"
      backTo="/ui/ar"
      backLabel="Artifact Registry"
      queryKey={["ar-repo", name]}
      queryFn={() => fetchARRepo(name)}
      properties={(repo) => [
        { label: "Format", value: repo.format ?? "—" },
        { label: "Mode", value: repo.mode ?? "Standard" },
        { label: "Location", value: "us-central1" },
        { label: "Created", value: formatTimestamp(repo.createTime ?? "") },
        { label: "Updated", value: formatTimestamp(repo.updateTime ?? "") },
        { label: "Description", value: repo.description ?? "—" },
      ]}
    />
  );
}
