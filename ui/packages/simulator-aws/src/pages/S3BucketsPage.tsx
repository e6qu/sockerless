import { useState } from "react";
import { useNavigate } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AwsButton, AwsErrorAlert, AwsModal, AwsResourceTable, AwsRowLink, type AwsColumn } from "../console/index.js";
import { formatTimestamp } from "../console/format.js";
import { deleteS3Bucket, fetchS3Buckets, type S3Bucket } from "../api.js";

// Amazon Simple Storage Service (S3) — Buckets. The list and Delete both go
// through the real S3 REST-XML API (GET / for the table, DELETE /{bucket} for
// the header action) with the operator's federated credentials. DeleteBucket
// only succeeds on an empty bucket — the console surfaces S3's BucketNotEmpty
// error rather than emptying it on the operator's behalf, the same
// restriction the real console enforces.

const columns: AwsColumn<S3Bucket>[] = [
  {
    id: "name",
    header: "Name",
    cell: (row) => <AwsRowLink to={`/ui/s3/${encodeURIComponent(row.name)}`}>{row.name}</AwsRowLink>,
    value: (row) => row.name,
  },
  { id: "creationDate", header: "Creation date", cell: (row) => formatTimestamp(row.creationDate), value: (row) => row.creationDate },
];

export function DeleteBucketsModal({
  buckets,
  onClose,
  clearSelection,
}: {
  buckets: S3Bucket[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    // DeleteBucket is per-bucket on the wire; a non-empty bucket fails with
    // BucketNotEmpty, surfaced as the real API error, with the already-empty
    // buckets among the selection gone from the refreshed list.
    mutationFn: async () => {
      for (const bucket of buckets) {
        await deleteS3Bucket(bucket.name);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["s3-buckets"] }),
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  return (
    <AwsModal
      title={buckets.length === 1 ? `Delete ${buckets[0].name}?` : `Delete ${buckets.length} buckets?`}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="s3-delete-bucket-confirm"
            disabled={remove.isPending}
            onClick={() => remove.mutate()}
          >
            {remove.isPending ? "Deleting…" : "Delete"}
          </AwsButton>
        </>
      }
    >
      <p>Deleting a bucket is permanent. S3 refuses to delete a bucket that still holds objects — empty it first.</p>
      <ul>
        {buckets.map((bucket) => (
          <li key={bucket.name}>
            <code>{bucket.name}</code>
          </li>
        ))}
      </ul>
      {remove.isError && (
        <AwsErrorAlert>
          <strong>Could not delete.</strong>{" "}
          {remove.error instanceof Error ? remove.error.message : "The request failed."}
        </AwsErrorAlert>
      )}
    </AwsModal>
  );
}

export function S3BucketsPage() {
  const navigate = useNavigate();
  const [deleting, setDeleting] = useState<{ buckets: S3Bucket[]; clearSelection: () => void } | null>(null);
  return (
    <>
      <AwsResourceTable<S3Bucket>
        title="Buckets"
        description="General purpose buckets in this account."
        columns={columns}
        queryKey={["s3-buckets"]}
        queryFn={fetchS3Buckets}
        filterPlaceholder="Find buckets"
        emptyTitle="No buckets"
        emptyDescription="No buckets exist in this account."
        rowKey={(row) => row.name}
        tableTestId="s3-buckets-table"
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="s3-view-bucket"
              disabled={selected.length !== 1}
              onClick={() => navigate(`/ui/s3/${encodeURIComponent(selected[0].name)}`)}
            >
              View details
            </AwsButton>
            <AwsButton
              data-testid="s3-delete-bucket"
              disabled={selected.length === 0}
              onClick={() => setDeleting({ buckets: selected, clearSelection })}
            >
              Delete
            </AwsButton>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
          </>
        )}
      />
      {deleting && (
        <DeleteBucketsModal
          buckets={deleting.buckets}
          clearSelection={deleting.clearSelection}
          onClose={() => setDeleting(null)}
        />
      )}
    </>
  );
}
