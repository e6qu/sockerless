import { GcpResourceTable, type GcpColumn } from "../console/index.js";
import { shortName } from "../console/format.js";
import { fetchGCSBuckets, type GCSBucket } from "../api.js";

const columns: GcpColumn<GCSBucket>[] = [
  { id: "name", header: "Name", cell: (row) => shortName(row.name), value: (row) => shortName(row.name) },
];

export function GCSBucketsPage() {
  return (
    <GcpResourceTable<GCSBucket>
      title="Cloud Storage"
      description="Buckets hold your objects — durable, scalable storage for any amount of data."
      actions={[{ label: "Create bucket", glyph: "+", primary: true, disabled: true }]}
      columns={columns}
      queryKey={["gcs-buckets"]}
      queryFn={fetchGCSBuckets}
      filterPlaceholder="Filter buckets"
      resourceNoun="buckets"
      empty={{
        headline: "Store any amount of data",
        description: "Create a bucket to store and serve objects with Cloud Storage.",
        primaryLabel: "Create bucket",
      }}
      rowKey={(row) => row.name}
    />
  );
}
