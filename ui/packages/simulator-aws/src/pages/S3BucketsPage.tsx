import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, Spinner } from "@sockerless/ui-core/components";
import { fetchS3Buckets, type S3Bucket } from "../api.js";

const columns: ColumnDef<S3Bucket, any>[] = [
  { accessorKey: "name", header: "Name" },
  { accessorKey: "creationDate", header: "Created" },
];

export function S3BucketsPage() {
  const { data, isLoading } = useQuery({ queryKey: ["s3-buckets"], queryFn: fetchS3Buckets, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">S3 Buckets</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
