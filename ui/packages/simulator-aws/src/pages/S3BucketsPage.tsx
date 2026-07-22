import { AwsResourceTable, AwsStatus, type AwsColumn } from "../console/index.js";
import { formatTimestamp } from "../console/format.js";
import { fetchS3Buckets, type S3Bucket } from "../api.js";

const columns: AwsColumn<S3Bucket>[] = [
  { id: "name", header: "Name", cell: (row) => row.name, value: (row) => row.name },
  { id: "creationDate", header: "Creation date", cell: (row) => formatTimestamp(row.creationDate), value: (row) => row.creationDate },
];

export function S3BucketsPage() {
  return (
    <AwsResourceTable<S3Bucket>
      title="Buckets"
      breadcrumbLabel="Buckets"
      description="General purpose buckets in this account."
      columns={columns}
      queryKey={["s3-buckets"]}
      queryFn={fetchS3Buckets}
      filterPlaceholder="Find buckets"
      emptyTitle="No buckets"
      emptyDescription="No buckets exist in this account."
      rowKey={(row) => row.name}
    />
  );
}
