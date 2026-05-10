import { type ColumnDef } from "@tanstack/react-table";
import { ResourceListPage } from "@sockerless/ui-core/components";
import { fetchS3Buckets, type S3Bucket } from "../api.js";

const columns: ColumnDef<S3Bucket, unknown>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "creationDate", header: "Created" },
];

export function S3BucketsPage() {
  return (
    <ResourceListPage<S3Bucket>
      kicker="aws · simulator · s3"
      title={<>Buckets</>}
      countNoun="bucket"
      columns={columns}
      queryKey={["s3-buckets"]}
      queryFn={fetchS3Buckets}
      filterPlaceholder="Filter buckets…"
      emptyMessage="No S3 buckets tracked."
    />
  );
}
