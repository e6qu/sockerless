import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import Input from "@cloudscape-design/components/input";
import FormField from "@cloudscape-design/components/form-field";
import { AwsButton, AwsErrorAlert, AwsModal, AwsResourceTable, type AwsColumn } from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import { createSQSQueue, deleteSQSQueue, fetchSQSQueues, type SQSQueue } from "../api.js";

// Amazon Simple Queue Service (SQS) — Queues. ListQueues answers URLs only, so
// the table's message counts and creation time come from GetQueueAttributes,
// the same pair the real console's Queues page reads. CreateQueue and
// DeleteQueue back the header actions.

const columns: AwsColumn<SQSQueue>[] = [
  { id: "name", header: "Name", cell: (row) => row.name, value: (row) => row.name },
  { id: "type", header: "Type", cell: (row) => (row.name.endsWith(".fifo") ? "FIFO" : "Standard"), value: (row) => row.name },
  {
    id: "messages",
    header: "Messages available",
    cell: (row) => String(row.approximateNumberOfMessages),
    value: (row) => String(row.approximateNumberOfMessages),
  },
  {
    id: "inFlight",
    header: "Messages in flight",
    cell: (row) => String(row.approximateNumberOfMessagesNotVisible),
    value: (row) => String(row.approximateNumberOfMessagesNotVisible),
  },
  {
    id: "visibilityTimeout",
    header: "Visibility timeout",
    cell: (row) => `${row.visibilityTimeout} seconds`,
    value: (row) => String(row.visibilityTimeout),
  },
  {
    id: "created",
    header: "Created",
    cell: (row) => formatEpoch(row.createdTimestamp),
    value: (row) => String(row.createdTimestamp),
  },
];

// The queue-name shape real SQS enforces on CreateQueue: up to 80 characters of
// letters, numbers, hyphens, and underscores (a FIFO queue's name additionally
// ends in `.fifo`).
const SQS_QUEUE_NAME_PATTERN = /^[A-Za-z0-9_-]{1,75}(\.fifo)?$/;

function CreateQueueModal({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const create = useMutation({
    mutationFn: () => createSQSQueue(name.trim()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["sqs-queues"] });
      onClose();
    },
  });
  const valid = SQS_QUEUE_NAME_PATTERN.test(name.trim());
  return (
    <AwsModal
      title="Create queue"
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="sqs-create-queue-submit"
            disabled={!valid || create.isPending}
            onClick={() => create.mutate()}
          >
            {create.isPending ? "Creating…" : "Create queue"}
          </AwsButton>
        </>
      }
    >
      <p>A queue keeps its own defaults for visibility timeout, retention, and delivery delay unless they are changed later.</p>
      <FormField
        label="Name"
        constraintText="Up to 80 characters. Letters, numbers, hyphens, and underscores. A FIFO queue's name ends in .fifo."
      >
        <Input
          value={name}
          onChange={(event) => setName(event.detail.value)}
          nativeInputAttributes={{ "data-testid": "sqs-queue-name-input" }}
        />
      </FormField>
      {create.isError && (
        <AwsErrorAlert>
          <strong>Could not create the queue.</strong>{" "}
          {create.error instanceof Error ? create.error.message : "The request failed."}
        </AwsErrorAlert>
      )}
    </AwsModal>
  );
}

function DeleteQueuesModal({
  queues,
  onClose,
  clearSelection,
}: {
  queues: SQSQueue[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: async () => {
      for (const queue of queues) {
        await deleteSQSQueue(queue.queueUrl);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["sqs-queues"] }),
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  return (
    <AwsModal
      title={queues.length === 1 ? `Delete ${queues[0].name}?` : `Delete ${queues.length} queues?`}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="sqs-delete-queue-confirm"
            disabled={remove.isPending}
            onClick={() => remove.mutate()}
          >
            {remove.isPending ? "Deleting…" : "Delete"}
          </AwsButton>
        </>
      }
    >
      <p>Deleting a queue is permanent and discards every message still in it.</p>
      <ul>
        {queues.map((queue) => (
          <li key={queue.queueUrl}>
            <code>{queue.name}</code>
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

export function SQSPage() {
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<{ queues: SQSQueue[]; clearSelection: () => void } | null>(null);
  return (
    <>
      <AwsResourceTable<SQSQueue>
        title="Queues"
        description="Amazon SQS queues in this account and Region."
        columns={columns}
        queryKey={["sqs-queues"]}
        queryFn={fetchSQSQueues}
        filterPlaceholder="Find queues"
        emptyTitle="No queues"
        emptyDescription="No Amazon SQS queues exist in this account and Region."
        rowKey={(row) => row.queueUrl}
        tableTestId="sqs-table"
        errorTestId="sqs-error"
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="sqs-delete-queue"
              disabled={selected.length === 0}
              onClick={() => setDeleting({ queues: selected, clearSelection })}
            >
              Delete
            </AwsButton>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
            <AwsButton variant="primary" data-testid="sqs-create-queue" onClick={() => setCreating(true)}>
              Create queue
            </AwsButton>
          </>
        )}
      />
      {creating && <CreateQueueModal onClose={() => setCreating(false)} />}
      {deleting && (
        <DeleteQueuesModal queues={deleting.queues} clearSelection={deleting.clearSelection} onClose={() => setDeleting(null)} />
      )}
    </>
  );
}
