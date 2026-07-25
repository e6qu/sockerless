import { useState } from "react";
import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { AzureCommandBar, AzureEssentials, AzureStatus } from "../portal/AzurePortal.js";
import { resourceGroupOf, locationLabel } from "../portal/format.js";
import { fetchACRRegistry, fetchACRRepositories, fetchACRTags } from "../api.js";

// The Container registry blade: the real Microsoft.ContainerRegistry
// Essentials, and its repositories/tags read from the registry's own data
// plane (the ACR REST convenience API, `/acr/v1/_catalog` and
// `/acr/v1/{repo}/_tags`) using admin credentials minted through the real
// `listCredentials` ARM action — the same admin-user flow
// `az acr repository list` and `docker login` use, not a synthetic reader.
export function ACRRegistryDetailPage() {
  const { name = "" } = useParams();
  const [selectedRepo, setSelectedRepo] = useState<string | null>(null);

  const registry = useQuery({ queryKey: ["acr-registry", name], queryFn: () => fetchACRRegistry(name) });
  const repositories = useQuery({
    queryKey: ["acr-repositories", registry.data?.id],
    queryFn: () => fetchACRRepositories(registry.data!),
    enabled: Boolean(registry.data),
  });
  const tags = useQuery({
    queryKey: ["acr-tags", registry.data?.id, selectedRepo],
    queryFn: () => fetchACRTags(registry.data!, selectedRepo!),
    enabled: Boolean(registry.data) && Boolean(selectedRepo),
  });

  return (
    <>
      <AzureCommandBar
        commands={[
          {
            label: "Refresh",
            icon: "refresh",
            onSelect: () => {
              void registry.refetch();
              void repositories.refetch();
              if (selectedRepo) void tags.refetch();
            },
            disabled: registry.isFetching,
          },
          { label: "Feedback", icon: "feedback" },
        ]}
      />
      <div className="az-main" data-testid="acr-registry-detail">
        {registry.isError ? (
          <div className="az-message az-message-error" role="alert" data-testid="acr-registry-error">
            <strong>Could not load this container registry.</strong>{" "}
            {registry.error instanceof Error ? registry.error.message : "Azure Resource Manager did not respond."}
          </div>
        ) : registry.isLoading || !registry.data ? (
          <div className="az-empty" role="status">
            Loading the registry…
          </div>
        ) : (
          <>
            <AzureEssentials
              properties={[
                { label: "Resource group", value: resourceGroupOf(registry.data.id) },
                { label: "Location", value: locationLabel(registry.data.location) },
                { label: "Login server", value: <code>{registry.data.loginServer || "—"}</code> },
                { label: "SKU", value: registry.data.skuTier || registry.data.skuName || "—" },
                { label: "Admin user", value: registry.data.adminUserEnabled ? "Enabled" : "Disabled" },
                { label: "Provisioning state", value: <AzureStatus status={registry.data.provisioningState || "Unknown"} /> },
              ]}
            />

            <section className="az-blade-section" aria-label="Repositories">
              <h2>Repositories</h2>
              {!registry.data.adminUserEnabled ? (
                <div className="az-message az-message-warning" role="status" data-testid="acr-admin-disabled">
                  Enable the admin user on this registry to browse its repositories from this console.
                </div>
              ) : (
                <div className="az-table-tools">
                  <table className="az-table" data-testid="acr-repositories">
                    <thead>
                      <tr>
                        <th>Repository</th>
                      </tr>
                    </thead>
                    <tbody>
                      {repositories.isError ? (
                        <tr>
                          <td className="az-table-state">
                            <div className="az-message az-message-error" role="alert">
                              <strong>Could not reach the registry's repository catalog.</strong>{" "}
                              {repositories.error instanceof Error ? repositories.error.message : "The registry did not respond."}
                            </div>
                          </td>
                        </tr>
                      ) : repositories.isLoading ? (
                        <tr>
                          <td className="az-table-state">
                            <div className="az-empty" role="status">
                              Loading repositories…
                            </div>
                          </td>
                        </tr>
                      ) : (repositories.data ?? []).length === 0 ? (
                        <tr>
                          <td className="az-table-state">
                            <div className="az-empty">
                              <strong>No repositories to display</strong>
                              <p>Images pushed to this registry appear here.</p>
                            </div>
                          </td>
                        </tr>
                      ) : (
                        (repositories.data ?? []).map((repo) => (
                          <tr key={repo} className={selectedRepo === repo ? "az-row-selected" : undefined}>
                            <td>
                              <button
                                type="button"
                                className="az-command"
                                data-testid="acr-repository-row"
                                aria-pressed={selectedRepo === repo}
                                onClick={() => setSelectedRepo((current) => (current === repo ? null : repo))}
                              >
                                {repo}
                              </button>
                            </td>
                          </tr>
                        ))
                      )}
                    </tbody>
                  </table>
                </div>
              )}
            </section>

            {selectedRepo ? (
              <section className="az-blade-section" aria-label={`Tags for ${selectedRepo}`}>
                <h2>Tags — {selectedRepo}</h2>
                <table className="az-table" data-testid="acr-tags">
                  <thead>
                    <tr>
                      <th>Tag</th>
                      <th>Digest</th>
                    </tr>
                  </thead>
                  <tbody>
                    {tags.isError ? (
                      <tr>
                        <td className="az-table-state" colSpan={2}>
                          <div className="az-message az-message-error" role="alert">
                            <strong>Could not load tags.</strong>{" "}
                            {tags.error instanceof Error ? tags.error.message : "The registry did not respond."}
                          </div>
                        </td>
                      </tr>
                    ) : tags.isLoading ? (
                      <tr>
                        <td className="az-table-state" colSpan={2}>
                          <div className="az-empty" role="status">
                            Loading tags…
                          </div>
                        </td>
                      </tr>
                    ) : (tags.data ?? []).length === 0 ? (
                      <tr>
                        <td className="az-table-state" colSpan={2}>
                          <div className="az-empty">
                            <strong>No tags to display</strong>
                          </div>
                        </td>
                      </tr>
                    ) : (
                      (tags.data ?? []).map((tag) => (
                        <tr key={tag.name}>
                          <td>{tag.name}</td>
                          <td>
                            <code>{tag.digest}</code>
                          </td>
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
