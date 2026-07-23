import { Link } from "react-router";
import { GcpResourceTable, type GcpColumn } from "../console/index.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { fetchGCSBuckets, type GCSBucket } from "../api.js";

const columns: GcpColumn<GCSBucket>[] = [
  {
    id: "name",
    header: "Name",
    cell: (row) => <Link className="gc-cell-link" to={`/ui/gcs/${shortName(row.name)}`}>{shortName(row.name)}</Link>,
    value: (row) => shortName(row.name),
  },
  { id: "location", header: "Location", cell: (row) => row.location ?? "—", value: (row) => row.location ?? "" },
  { id: "storageClass", header: "Storage class", cell: (row) => row.storageClass ?? "—", value: (row) => row.storageClass ?? "" },
  { id: "timeCreated", header: "Created", cell: (row) => formatTimestamp(row.timeCreated ?? ""), value: (row) => row.timeCreated ?? "" },
];

export function GCSBucketsPage() {
  return (
    <GcpResourceTable<GCSBucket>
      title="Cloud Storage"
      description="Buckets hold your objects — durable, scalable storage for any amount of data."
      actions={[{ label: "Create bucket", icon: "add", primary: true, disabled: true }]}
      columns={columns}
      queryKey={["gcs-buckets-real"]}
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
