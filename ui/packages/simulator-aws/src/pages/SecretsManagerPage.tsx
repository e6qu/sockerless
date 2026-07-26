import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import Input from "@cloudscape-design/components/input";
import FormField from "@cloudscape-design/components/form-field";
import SpaceBetween from "@cloudscape-design/components/space-between";
import { AwsButton, AwsErrorAlert, AwsModal, AwsResourceTable, type AwsColumn } from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import { createSecret, deleteSecret, fetchSecrets, type Secret } from "../api.js";

// AWS Secrets Manager — Secrets. ListSecrets, CreateSecret, and DeleteSecret on
// the real Secrets Manager API (X-Amz-Target secretsmanager.<Op>).

const columns: AwsColumn<Secret>[] = [
  { id: "name", header: "Secret name", cell: (row) => row.name, value: (row) => row.name },
  { id: "description", header: "Description", cell: (row) => row.description || "–", value: (row) => row.description },
  {
    id: "rotationEnabled",
    header: "Rotation",
    cell: (row) => (row.rotationEnabled ? "Enabled" : "Disabled"),
    value: (row) => (row.rotationEnabled ? "Enabled" : "Disabled"),
  },
  {
    id: "lastChangedDate",
    header: "Last changed",
    cell: (row) => formatEpoch(row.lastChangedDate),
    value: (row) => String(row.lastChangedDate),
  },
  {
    id: "createdDate",
    header: "Created",
    cell: (row) => formatEpoch(row.createdDate),
    value: (row) => String(row.createdDate),
  },
];

function CreateSecretModal({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [secretValue, setSecretValue] = useState("");
  const create = useMutation({
    mutationFn: () => createSecret(name.trim(), secretValue, description.trim()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["secrets"] });
      onClose();
    },
  });
  const valid = name.trim().length > 0 && secretValue.length > 0;
  return (
    <AwsModal
      title="Store a new secret"
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="secrets-create-secret-submit"
            disabled={!valid || create.isPending}
            onClick={() => create.mutate()}
          >
            {create.isPending ? "Storing…" : "Store"}
          </AwsButton>
        </>
      }
    >
      <SpaceBetween size="m">
        <p>The secret value is encrypted with the service-managed key unless a customer managed key is chosen later.</p>
        <FormField label="Secret name">
          <Input
            value={name}
            onChange={(event) => setName(event.detail.value)}
            nativeInputAttributes={{ "data-testid": "secrets-secret-name-input" }}
          />
        </FormField>
        <FormField label="Secret value">
          <Input
            type="password"
            value={secretValue}
            onChange={(event) => setSecretValue(event.detail.value)}
            nativeInputAttributes={{ "data-testid": "secrets-secret-value-input" }}
          />
        </FormField>
        <FormField label="Description - optional">
          <Input
            value={description}
            onChange={(event) => setDescription(event.detail.value)}
            nativeInputAttributes={{ "data-testid": "secrets-secret-description-input" }}
          />
        </FormField>
        {create.isError && (
          <AwsErrorAlert>
            <strong>Could not store the secret.</strong>{" "}
            {create.error instanceof Error ? create.error.message : "The request failed."}
          </AwsErrorAlert>
        )}
      </SpaceBetween>
    </AwsModal>
  );
}

function DeleteSecretsModal({
  secrets,
  onClose,
  clearSelection,
}: {
  secrets: Secret[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: async () => {
      for (const secret of secrets) {
        await deleteSecret(secret.arn || secret.name);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["secrets"] }),
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  return (
    <AwsModal
      title={secrets.length === 1 ? `Delete ${secrets[0].name}?` : `Delete ${secrets.length} secrets?`}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="secrets-delete-secret-confirm"
            disabled={remove.isPending}
            onClick={() => remove.mutate()}
          >
            {remove.isPending ? "Scheduling…" : "Schedule deletion"}
          </AwsButton>
        </>
      }
    >
      <p>Secrets Manager schedules the deletion after its recovery window rather than deleting immediately, so the secret can be restored until then.</p>
      <ul>
        {secrets.map((secret) => (
          <li key={secret.arn || secret.name}>
            <code>{secret.name}</code>
          </li>
        ))}
      </ul>
      {remove.isError && (
        <AwsErrorAlert>
          <strong>Could not schedule the deletion.</strong>{" "}
          {remove.error instanceof Error ? remove.error.message : "The request failed."}
        </AwsErrorAlert>
      )}
    </AwsModal>
  );
}

export function SecretsManagerPage() {
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<{ secrets: Secret[]; clearSelection: () => void } | null>(null);
  return (
    <>
      <AwsResourceTable<Secret>
        title="Secrets"
        description="Secrets Manager secrets in this account and Region."
        columns={columns}
        queryKey={["secrets"]}
        queryFn={fetchSecrets}
        filterPlaceholder="Find secrets"
        emptyTitle="No secrets"
        emptyDescription="No Secrets Manager secrets exist in this account and Region."
        rowKey={(row) => row.arn || row.name}
        tableTestId="secrets-table"
        errorTestId="secrets-error"
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="secrets-delete-secret"
              disabled={selected.length === 0}
              onClick={() => setDeleting({ secrets: selected, clearSelection })}
            >
              Delete
            </AwsButton>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
            <AwsButton variant="primary" data-testid="secrets-create-secret" onClick={() => setCreating(true)}>
              Store a new secret
            </AwsButton>
          </>
        )}
      />
      {creating && <CreateSecretModal onClose={() => setCreating(false)} />}
      {deleting && (
        <DeleteSecretsModal
          secrets={deleting.secrets}
          clearSelection={deleting.clearSelection}
          onClose={() => setDeleting(null)}
        />
      )}
    </>
  );
}
