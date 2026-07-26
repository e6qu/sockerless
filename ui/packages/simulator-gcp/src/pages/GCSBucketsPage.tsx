import { useState } from "react";
import { Link } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { GcpResourceTable, type GcpColumn } from "../console/index.js";
import { GcpDialog } from "../console/GcpDialog.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { createGCSBucket, deleteGCSBucket, fetchGCSBuckets, type GCSBucket } from "../api.js";
import { useProject } from "../console/project.js";

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

// DeleteBucketDialog is shared by the list's per-row action and the bucket
// detail page's header action — the same real storage.buckets.delete
// operation (DELETE /storage/v1/b/{bucket}, answered 204 No Content) either
// way.
export function DeleteBucketDialog({
  name,
  onClose,
  onDeleted,
}: {
  name: string;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const remove = useMutation({
    mutationFn: () => deleteGCSBucket(name),
    onSuccess: onDeleted,
  });
  return (
    <GcpDialog title="Delete bucket?" testId="gcs-delete-dialog" onClose={onClose}>
      <p>
        Deleting <strong>{name}</strong> permanently removes the bucket and every object it holds.
        This can't be undone.
      </p>
      {remove.isError ? (
        <div className="gc-message gc-message-error" role="alert">
          <strong>Couldn't delete the bucket.</strong>{" "}
          {remove.error instanceof Error ? remove.error.message : "The API did not respond."}
        </div>
      ) : null}
      <div className="gc-dialog-actions">
        <button type="button" className="gc-button-text" onClick={onClose}>Cancel</button>
        <button
          type="button"
          className="gc-button-primary"
          data-testid="gcs-delete-confirm"
          disabled={remove.isPending}
          onClick={() => remove.mutate()}
        >
          {remove.isPending ? "Deleting…" : "Delete"}
        </button>
      </div>
    </GcpDialog>
  );
}

// Cloud Storage's bucket-name contract: 3–63 characters, lowercase letters,
// digits, hyphens, underscores and dots, starting and ending with a letter
// or digit — checked before submitting so the dialog explains the rule the
// way the real Create bucket page does.
const BUCKET_NAME_PATTERN = /^[a-z0-9][a-z0-9._-]{1,61}[a-z0-9]$/;

export function CreateBucketDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: () => void;
}) {
  const { project } = useProject();
  const [name, setName] = useState("");

  const create = useMutation({
    mutationFn: () => createGCSBucket(project, name),
    onSuccess: onCreated,
  });

  const valid = BUCKET_NAME_PATTERN.test(name);

  return (
    <GcpDialog title="Create a bucket" testId="gcs-create-dialog" onClose={onClose}>
      <label className="gc-field">
        Name your bucket
        <input
          type="text"
          value={name}
          data-testid="gcs-create-name"
          onChange={(event) => setName(event.target.value)}
        />
        <p className="gc-field-hint">
          3–63 characters. Lowercase letters, numbers, dashes (-), underscores (_) and dots (.); must start and
          end with a letter or number.
        </p>
      </label>
      {create.isError ? (
        <div className="gc-message gc-message-error" role="alert">
          <strong>Couldn't create the bucket.</strong>{" "}
          {create.error instanceof Error ? create.error.message : "The API did not respond."}
        </div>
      ) : null}
      <div className="gc-dialog-actions">
        <button type="button" className="gc-button-text" onClick={onClose}>Cancel</button>
        <button
          type="button"
          className="gc-button-primary"
          data-testid="gcs-create-submit"
          disabled={!valid || create.isPending}
          onClick={() => create.mutate()}
        >
          {create.isPending ? "Creating…" : "Create"}
        </button>
      </div>
    </GcpDialog>
  );
}

export function GCSBucketsPage() {
  const { project } = useProject();
  const queryClient = useQueryClient();
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);

  const refresh = () => void queryClient.invalidateQueries({ queryKey: ["gcs-buckets-real", project] });

  const columnsWithActions: GcpColumn<GCSBucket>[] = [
    ...columns,
    {
      id: "actions",
      header: "Actions",
      cell: (row) => {
        const name = shortName(row.name);
        return (
          <button
            type="button"
            className="gc-button-text"
            data-testid={`gcs-delete-${name}`}
            aria-label={`Delete ${name}`}
            onClick={() => setDeleting(name)}
          >
            Delete
          </button>
        );
      },
      value: () => "",
    },
  ];

  return (
    <>
      <GcpResourceTable<GCSBucket>
        title="Cloud Storage"
        description="Buckets hold your objects — durable, scalable storage for any amount of data."
        actions={[
          { label: "Create bucket", icon: "add", primary: true, testId: "gcs-create-bucket", onSelect: () => setCreating(true) },
        ]}
        columns={columnsWithActions}
        queryKey={["gcs-buckets-real", project]}
        queryFn={() => fetchGCSBuckets(project)}
        filterPlaceholder="Filter buckets"
        resourceNoun="buckets"
        empty={{
          headline: "Store any amount of data",
          description: "Create a bucket to store and serve objects with Cloud Storage.",
          primaryLabel: "Create bucket",
          onPrimary: () => setCreating(true),
        }}
        rowKey={(row) => row.name}
      />
      {creating ? (
        <CreateBucketDialog
          onClose={() => setCreating(false)}
          onCreated={() => {
            setCreating(false);
            refresh();
          }}
        />
      ) : null}
      {deleting ? (
        <DeleteBucketDialog
          name={deleting}
          onClose={() => setDeleting(null)}
          onDeleted={() => {
            setDeleting(null);
            refresh();
          }}
        />
      ) : null}
    </>
  );
}
