import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { DataTable, Spinner } from "@sockerless/ui-core/components";
import { fetchGCSBuckets, type GCSBucket } from "../api.js";

const columns: ColumnDef<GCSBucket, any>[] = [
  { accessorKey: "name", header: "Name" },
];

export function GCSBucketsPage() {
  const { data, isLoading } = useQuery({ queryKey: ["gcs-buckets"], queryFn: fetchGCSBuckets, refetchInterval: 5000 });
  if (isLoading) return <Spinner />;
  return (
    <div>
      <h2 className="mb-4 text-2xl font-bold">GCS Buckets</h2>
      <DataTable columns={columns} data={data ?? []} />
    </div>
  );
}
