import { useState } from "react";
import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { AzureCommandBar, AzureEssentials, AzureStatus } from "../portal/AzurePortal.js";
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
          <div className="az-message az-message-error" role="alert" data-testid="storage-account-error">
            <strong>Could not load this storage account.</strong>{" "}
            {account.error instanceof Error ? account.error.message : "Azure Resource Manager did not respond."}
          </div>
        ) : account.isLoading || !account.data ? (
          <div className="az-empty" role="status">
            Loading the storage account…
          </div>
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
              <h2>Containers</h2>
              <table className="az-table" data-testid="storage-containers">
                <thead>
                  <tr>
                    <th>Name</th>
                  </tr>
                </thead>
                <tbody>
                  {containers.isError ? (
                    <tr>
                      <td className="az-table-state">
                        <div className="az-message az-message-error" role="alert">
                          <strong>Could not reach the blob service.</strong>{" "}
                          {containers.error instanceof Error ? containers.error.message : "The storage account did not respond."}
                        </div>
                      </td>
                    </tr>
                  ) : containers.isLoading ? (
                    <tr>
                      <td className="az-table-state">
                        <div className="az-empty" role="status">
                          Loading containers…
                        </div>
                      </td>
                    </tr>
                  ) : (containers.data ?? []).length === 0 ? (
                    <tr>
                      <td className="az-table-state">
                        <div className="az-empty">
                          <strong>No containers to display</strong>
                          <p>Blob containers created in this account appear here.</p>
                        </div>
                      </td>
                    </tr>
                  ) : (
                    (containers.data ?? []).map((container) => (
                      <tr key={container.name} className={selectedContainer === container.name ? "az-row-selected" : undefined}>
                        <td>
                          <button
                            type="button"
                            className="az-command"
                            data-testid="storage-container-row"
                            aria-pressed={selectedContainer === container.name}
                            onClick={() => setSelectedContainer((current) => (current === container.name ? null : container.name))}
                          >
                            {container.name}
                          </button>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </section>

            {selectedContainer ? (
              <section className="az-blade-section" aria-label={`Blobs in ${selectedContainer}`}>
                <h2>Blobs — {selectedContainer}</h2>
                <table className="az-table" data-testid="storage-blobs">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Size</th>
                      <th>Last modified</th>
                    </tr>
                  </thead>
                  <tbody>
                    {blobs.isError ? (
                      <tr>
                        <td className="az-table-state" colSpan={3}>
                          <div className="az-message az-message-error" role="alert">
                            <strong>Could not load blobs.</strong>{" "}
                            {blobs.error instanceof Error ? blobs.error.message : "The storage account did not respond."}
                          </div>
                        </td>
                      </tr>
                    ) : blobs.isLoading ? (
                      <tr>
                        <td className="az-table-state" colSpan={3}>
                          <div className="az-empty" role="status">
                            Loading blobs…
                          </div>
                        </td>
                      </tr>
                    ) : (blobs.data ?? []).length === 0 ? (
                      <tr>
                        <td className="az-table-state" colSpan={3}>
                          <div className="az-empty">
                            <strong>No blobs to display</strong>
                          </div>
                        </td>
                      </tr>
                    ) : (
                      (blobs.data ?? []).map((blob) => (
                        <tr key={blob.name}>
                          <td>{blob.name}</td>
                          <td>{blob.contentLength.toLocaleString()} B</td>
                          <td>{blob.lastModified || "—"}</td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </section>
            ) : null}
          </>
        )}
      </div>
    </>
  );
}
