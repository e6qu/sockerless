import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import Input from "@cloudscape-design/components/input";
import FormField from "@cloudscape-design/components/form-field";
import { AwsButton, AwsErrorAlert, AwsModal, AwsResourceTable, type AwsColumn } from "../console/index.js";
import {
  createSNSTopic,
  deleteSNSTopic,
  fetchSNSSubscriptions,
  fetchSNSTopics,
  type SNSSubscription,
  type SNSTopic,
} from "../api.js";

// Amazon Simple Notification Service (SNS) — Topics and Subscriptions.
// ListTopics, CreateTopic, DeleteTopic, and ListSubscriptions on the real SNS
// Query API (Version 2010-03-31).

const topicColumns: AwsColumn<SNSTopic>[] = [
  { id: "name", header: "Name", cell: (row) => row.name, value: (row) => row.name },
  { id: "arn", header: "ARN", cell: (row) => row.arn, value: (row) => row.arn },
];

const subscriptionColumns: AwsColumn<SNSSubscription>[] = [
  { id: "arn", header: "Subscription ARN", cell: (row) => row.subscriptionArn, value: (row) => row.subscriptionArn },
  { id: "protocol", header: "Protocol", cell: (row) => row.protocol, value: (row) => row.protocol },
  { id: "endpoint", header: "Endpoint", cell: (row) => row.endpoint, value: (row) => row.endpoint },
  { id: "topicArn", header: "Topic ARN", cell: (row) => row.topicArn, value: (row) => row.topicArn },
];

// The topic-name shape real SNS enforces on CreateTopic: up to 256 characters
// of letters, numbers, hyphens, and underscores.
const SNS_TOPIC_NAME_PATTERN = /^[A-Za-z0-9_-]+$/;

function CreateTopicModal({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const create = useMutation({
    mutationFn: () => createSNSTopic(name.trim()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["sns-topics"] });
      onClose();
    },
  });
  const trimmed = name.trim();
  const valid = trimmed.length > 0 && trimmed.length <= 256 && SNS_TOPIC_NAME_PATTERN.test(trimmed);
  return (
    <AwsModal
      title="Create topic"
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="sns-create-topic-submit"
            disabled={!valid || create.isPending}
            onClick={() => create.mutate()}
          >
            {create.isPending ? "Creating…" : "Create topic"}
          </AwsButton>
        </>
      }
    >
      <p>A standard topic fans a published message out to every subscription attached to it.</p>
      <FormField label="Name" constraintText="Up to 256 characters. Letters, numbers, hyphens, and underscores.">
        <Input
          value={name}
          onChange={(event) => setName(event.detail.value)}
          nativeInputAttributes={{ "data-testid": "sns-topic-name-input" }}
        />
      </FormField>
      {create.isError && (
        <AwsErrorAlert>
          <strong>Could not create the topic.</strong>{" "}
          {create.error instanceof Error ? create.error.message : "The request failed."}
        </AwsErrorAlert>
      )}
    </AwsModal>
  );
}

function DeleteTopicsModal({
  topics,
  onClose,
  clearSelection,
}: {
  topics: SNSTopic[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: async () => {
      for (const topic of topics) {
        await deleteSNSTopic(topic.arn);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["sns-topics"] }),
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  return (
    <AwsModal
      title={topics.length === 1 ? `Delete ${topics[0].name}?` : `Delete ${topics.length} topics?`}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="sns-delete-topic-confirm"
            disabled={remove.isPending}
            onClick={() => remove.mutate()}
          >
            {remove.isPending ? "Deleting…" : "Delete"}
          </AwsButton>
        </>
      }
    >
      <p>Deleting a topic also deletes every subscription attached to it.</p>
      <ul>
        {topics.map((topic) => (
          <li key={topic.arn}>
            <code>{topic.name}</code>
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

export function SNSPage() {
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<{ topics: SNSTopic[]; clearSelection: () => void } | null>(null);
  return (
    <>
      <AwsResourceTable<SNSTopic>
        title="Topics"
        description="Amazon SNS topics in this account and Region."
        columns={topicColumns}
        queryKey={["sns-topics"]}
        queryFn={fetchSNSTopics}
        filterPlaceholder="Find topics"
        emptyTitle="No topics"
        emptyDescription="No Amazon SNS topics exist in this account and Region."
        rowKey={(row) => row.arn}
        tableTestId="sns-topics-table"
        errorTestId="sns-topics-error"
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="sns-delete-topic"
              disabled={selected.length === 0}
              onClick={() => setDeleting({ topics: selected, clearSelection })}
            >
              Delete
            </AwsButton>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
            <AwsButton variant="primary" data-testid="sns-create-topic" onClick={() => setCreating(true)}>
              Create topic
            </AwsButton>
          </>
        )}
      />
      <AwsResourceTable<SNSSubscription>
        title="Subscriptions"
        headingVariant="h2"
        description="The endpoints subscribed to topics in this account and Region."
        columns={subscriptionColumns}
        queryKey={["sns-subscriptions"]}
        queryFn={fetchSNSSubscriptions}
        filterPlaceholder="Find subscriptions"
        emptyTitle="No subscriptions"
        emptyDescription="No subscriptions exist in this account and Region."
        rowKey={(row) => row.subscriptionArn}
        tableTestId="sns-subscriptions-table"
        errorTestId="sns-subscriptions-error"
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
      {creating && <CreateTopicModal onClose={() => setCreating(false)} />}
      {deleting && (
        <DeleteTopicsModal topics={deleting.topics} clearSelection={deleting.clearSelection} onClose={() => setDeleting(null)} />
      )}
    </>
  );
}
