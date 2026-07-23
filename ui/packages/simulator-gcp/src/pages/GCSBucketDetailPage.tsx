import { useParams } from "react-router";
import { GcpResourceDetail } from "../console/index.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { fetchGCSBucket, type GCSBucket } from "../api.js";

export function GCSBucketDetailPage() {
  const { name = "" } = useParams();
  return (
    <GcpResourceDetail<GCSBucket>
      title={shortName(name)}
      description="Cloud Storage bucket"
      backTo="/ui/gcs"
      backLabel="Cloud Storage"
      queryKey={["gcs-bucket", name]}
      queryFn={() => fetchGCSBucket(name)}
      properties={(bucket) => [
        { label: "Location", value: bucket.location ?? "—" },
        { label: "Storage class", value: bucket.storageClass ?? "—" },
        { label: "Created", value: formatTimestamp(bucket.timeCreated ?? "") },
        { label: "Updated", value: formatTimestamp(bucket.updated ?? "") },
        { label: "Bucket ID", value: bucket.id ?? "—" },
      ]}
    />
  );
}
