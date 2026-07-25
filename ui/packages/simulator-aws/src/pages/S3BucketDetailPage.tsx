import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AwsButton,
  AwsContainer,
  AwsEmptyState,
  AwsErrorAlert,
  AwsKeyValue,
  AwsPageHeader,
  AwsResourceTable,
  type AwsColumn,
} from "../console/index.js";
import { formatBytes, formatTimestamp } from "../console/format.js";
import { fetchS3BucketLocation, fetchS3Buckets, fetchS3Objects, type S3Bucket, type S3Object } from "../api.js";
import { DeleteBucketsModal } from "./S3BucketsPage.js";

// Amazon Simple Storage Service (S3) — Bucket detail. Reads the real S3
// REST-XML API (GetBucketLocation for the Region, ListObjectsV2 for the
// object listing) with the operator's federated credentials, and reuses the
// Buckets list page's real DeleteBucket action. S3 has no per-bucket
// "properties" read for the creation date — the real console reads it from
// the same ListBuckets response the Buckets page already fetches, and this
// page does the same rather than inventing a value.

const objectColumns: AwsColumn<S3Object>[] = [
  { id: "key", header: "Key", cell: (row) => row.key, value: (row) => row.key },
  { id: "size", header: "Size", cell: (row) => formatBytes(row.size), value: (row) => String(row.size) },
  {
    id: "lastModified",
    header: "Last modified",
    cell: (row) => formatTimestamp(row.lastModified),
    value: (row) => row.lastModified,
  },
];

export function S3BucketDetailPage() {
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [deleting, setDeleting] = useState(false);

  const buckets = useQuery({ queryKey: ["s3-buckets"], queryFn: fetchS3Buckets });
  const location = useQuery({ queryKey: ["s3-bucket-location", name], queryFn: () => fetchS3BucketLocation(name) });
  const bucket = buckets.data?.find((candidate) => candidate.name === name);
  const isError = buckets.isError || location.isError;
  const isLoading = buckets.isLoading || location.isLoading;

  const asS3Bucket: S3Bucket | null = bucket ?? null;

  return (
    <>
      <AwsPageHeader
        title={name}
        description="Bucket in Amazon Simple Storage Service."
        actions={
          <AwsButton data-testid="s3-bucket-delete" disabled={!location.isSuccess} onClick={() => setDeleting(true)}>
            Delete
          </AwsButton>
        }
      />
      <AwsContainer>
        {isError ? (
          <AwsErrorAlert testId="s3-bucket-error">
            <strong>Could not load the bucket.</strong>{" "}
            {(buckets.error ?? location.error) instanceof Error
              ? ((buckets.error ?? location.error) as Error).message
              : "The request failed."}
          </AwsErrorAlert>
        ) : isLoading ? (
          <AwsEmptyState title="Loading bucket…" loading />
        ) : (
          <div data-testid="s3-bucket-summary">
            <AwsKeyValue
              items={[
                { label: "Region", value: location.data },
                { label: "Creation date", value: bucket ? formatTimestamp(bucket.creationDate) : "–" },
              ]}
            />
          </div>
        )}
      </AwsContainer>
      <AwsResourceTable<S3Object>
        title="Objects"
        headingVariant="h2"
        description="Objects stored in this bucket."
        columns={objectColumns}
        queryKey={["s3-objects", name]}
        queryFn={() => fetchS3Objects(name)}
        filterPlaceholder="Find objects"
        emptyTitle="No objects"
        emptyDescription="This bucket has no objects."
        rowKey={(row) => row.key}
        tableTestId="s3-objects-table"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
      {deleting && asS3Bucket && (
        <DeleteBucketsModal
          buckets={[asS3Bucket]}
          clearSelection={() => navigate("/ui/s3")}
          onClose={() => {
            setDeleting(false);
            void queryClient.invalidateQueries({ queryKey: ["s3-buckets"] });
          }}
        />
      )}
    </>
  );
}
