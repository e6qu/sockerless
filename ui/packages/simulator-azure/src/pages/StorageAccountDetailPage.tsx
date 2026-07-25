import { useState } from "react";
import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { Table, TableHeader, TableRow, TableHeaderCell, TableBody, TableCell, Button, Text } from "@fluentui/react-components";
import { AzureCommandBar, AzureEssentials, AzureStatus, AzureErrorMessage, AzureEmptyState } from "../portal/AzurePortal.js";
import { AzureTableErrorRow, AzureTableLoadingRow, AzureTableEmptyRow } from "../portal/AzureTable.js";
import { resourceGroupOf, locationLabel } from "../portal/format.js";
import { fetchStorageAccount, fetchBlobContainers, fetchBlobs } from "../api.js";

// The Storage account blade: the real Microsoft.Storage Essentials, and its
// blob containers/blobs read from the account's own blob data plane —
// authenticated with an account SAS minted through the real
// `ListAccountSas` ARM action, the same credential path the portal's own
// Storage Browser uses.
export function StorageAccountDetailPage() {
  const { name = "" } = useParams();
  const [selectedContainer, setSelectedContainer] = useState<string | null>(null);

  const account = useQuery({ queryKey: ["storage-account", name], queryFn: () => fetchStorageAccount(name) });
  const containers = useQuery({
    queryKey: ["storage-containers", account.data?.id],
    queryFn: () => fetchBlobContainers(account.data!),
    enabled: Boolean(account.data),
  });
  const blobs = useQuery({
    queryKey: ["storage-blobs", account.data?.id, selectedContainer],
    queryFn: () => fetchBlobs(account.data!, selectedContainer!),
    enabled: Boolean(account.data) && Boolean(selectedContainer),
  });

  return (
    <>
      <AzureCommandBar
        commands={[
          {
            label: "Refresh",
            icon: "refresh",
            onSelect: () => {
              void account.refetch();
              void containers.refetch();
              if (selectedContainer) void blobs.refetch();
            },
            disabled: account.isFetching,
          },
          { label: "Feedback", icon: "feedback" },
        ]}
      />
      <div className="az-main" data-testid="storage-account-detail">
        {account.isError ? (
          <AzureErrorMessage testid="storage-account-error">
            <strong>Could not load this storage account.</strong>{" "}
            {account.error instanceof Error ? account.error.message : "Azure Resource Manager did not respond."}
          </AzureErrorMessage>
        ) : account.isLoading || !account.data ? (
          <AzureEmptyState title="Loading the storage account…" loading />
        ) : (
          <>
            <AzureEssentials
              properties={[
                { label: "Resource group", value: resourceGroupOf(account.data.id) },
                { label: "Location", value: locationLabel(account.data.location) },
                { label: "Kind", value: account.data.kind || "—" },
                { label: "Replication", value: account.data.skuName || "—" },
                { label: "Access tier", value: account.data.accessTier || "—" },
                { label: "Provisioning state", value: <AzureStatus status={account.data.provisioningState || "Unknown"} /> },
                { label: "Blob service endpoint", value: account.data.blobEndpoint ? <code>{account.data.blobEndpoint}</code> : "—" },
              ]}
            />

            <section className="az-blade-section" aria-label="Containers">
              <Text as="h2" weight="semibold" block>
                Containers
              </Text>
              <Table aria-label="Containers" size="small" data-testid="storage-containers">
                <TableHeader>
                  <TableRow>
                    <TableHeaderCell>Name</TableHeaderCell>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {containers.isError ? (
                    <AzureTableErrorRow colSpan={1}>
                      <strong>Could not reach the blob service.</strong>{" "}
                      {containers.error instanceof Error ? containers.error.message : "The storage account did not respond."}
                    </AzureTableErrorRow>
                  ) : containers.isLoading ? (
                    <AzureTableLoadingRow colSpan={1} label="Loading containers…" />
                  ) : (containers.data ?? []).length === 0 ? (
                    <AzureTableEmptyRow
                      colSpan={1}
                      title="No containers to display"
                      description="Blob containers created in this account appear here."
                    />
                  ) : (
                    (containers.data ?? []).map((container) => (
                      <TableRow key={container.name} appearance={selectedContainer === container.name ? "neutral" : undefined}>
                        <TableCell>
                          <Button
                            appearance="subtle"
                            data-testid="storage-container-row"
                            aria-pressed={selectedContainer === container.name}
                            onClick={() => setSelectedContainer((current) => (current === container.name ? null : container.name))}
                          >
                            {container.name}
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </section>

            {selectedContainer ? (
              <section className="az-blade-section" aria-label={`Blobs in ${selectedContainer}`}>
                <Text as="h2" weight="semibold" block>
                  Blobs — {selectedContainer}
                </Text>
                <Table aria-label={`Blobs in ${selectedContainer}`} size="small" data-testid="storage-blobs">
                  <TableHeader>
                    <TableRow>
                      <TableHeaderCell>Name</TableHeaderCell>
                      <TableHeaderCell>Size</TableHeaderCell>
                      <TableHeaderCell>Last modified</TableHeaderCell>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {blobs.isError ? (
                      <AzureTableErrorRow colSpan={3}>
                        <strong>Could not load blobs.</strong>{" "}
                        {blobs.error instanceof Error ? blobs.error.message : "The storage account did not respond."}
                      </AzureTableErrorRow>
                    ) : blobs.isLoading ? (
                      <AzureTableLoadingRow colSpan={3} label="Loading blobs…" />
                    ) : (blobs.data ?? []).length === 0 ? (
                      <AzureTableEmptyRow colSpan={3} title="No blobs to display" />
                    ) : (
                      (blobs.data ?? []).map((blob) => (
                        <TableRow key={blob.name}>
                          <TableCell>{blob.name}</TableCell>
                          <TableCell>{blob.contentLength.toLocaleString()} B</TableCell>
                          <TableCell>{blob.lastModified || "—"}</TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </section>
            ) : null}
          </>
        )}
      </div>
    </>
  );
}
