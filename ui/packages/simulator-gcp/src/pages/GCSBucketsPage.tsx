import { type ColumnDef } from "@tanstack/react-table";
import { ResourceListPage } from "@sockerless/ui-core/components";
import { fetchGCSBuckets, type GCSBucket } from "../api.js";

const columns: ColumnDef<GCSBucket, unknown>[] = [
  { accessorKey: "name", header: "Name" },
];

export function GCSBucketsPage() {
  return (
    <ResourceListPage<GCSBucket>
      kicker="gcp · simulator · gcs"
      title={<>Buckets</>}
      countNoun="bucket"
      columns={columns}
      queryKey={["gcs-buckets"]}
      queryFn={fetchGCSBuckets}
      filterPlaceholder="Filter buckets…"
      emptyMessage="No GCS buckets tracked."
    />
  );
}
