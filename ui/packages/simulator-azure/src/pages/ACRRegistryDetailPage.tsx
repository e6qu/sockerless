import { useState } from "react";
import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { Table, TableHeader, TableRow, TableHeaderCell, TableBody, TableCell, Button, Text } from "@fluentui/react-components";
import {
  AzureCommandBar,
  AzureEssentials,
  AzureStatus,
  AzureErrorMessage,
  AzureEmptyState,
  AzureWarningMessage,
} from "../portal/AzurePortal.js";
import { AzureTableErrorRow, AzureTableLoadingRow, AzureTableEmptyRow } from "../portal/AzureTable.js";
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
          <AzureErrorMessage testid="acr-registry-error">
            <strong>Could not load this container registry.</strong>{" "}
            {registry.error instanceof Error ? registry.error.message : "Azure Resource Manager did not respond."}
          </AzureErrorMessage>
        ) : registry.isLoading || !registry.data ? (
          <AzureEmptyState title="Loading the registry…" loading />
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
              <Text as="h2" weight="semibold" block>
                Repositories
              </Text>
              {!registry.data.adminUserEnabled ? (
                <AzureWarningMessage testid="acr-admin-disabled">
                  Enable the admin user on this registry to browse its repositories from this console.
                </AzureWarningMessage>
              ) : (
                <Table aria-label="Repositories" size="small" data-testid="acr-repositories">
                  <TableHeader>
                    <TableRow>
                      <TableHeaderCell>Repository</TableHeaderCell>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {repositories.isError ? (
                      <AzureTableErrorRow colSpan={1}>
                        <strong>Could not reach the registry's repository catalog.</strong>{" "}
                        {repositories.error instanceof Error ? repositories.error.message : "The registry did not respond."}
                      </AzureTableErrorRow>
                    ) : repositories.isLoading ? (
                      <AzureTableLoadingRow colSpan={1} label="Loading repositories…" />
                    ) : (repositories.data ?? []).length === 0 ? (
                      <AzureTableEmptyRow
                        colSpan={1}
                        title="No repositories to display"
                        description="Images pushed to this registry appear here."
                      />
                    ) : (
                      (repositories.data ?? []).map((repo) => (
                        <TableRow key={repo} appearance={selectedRepo === repo ? "neutral" : undefined}>
                          <TableCell>
                            <Button
                              appearance="subtle"
                              data-testid="acr-repository-row"
                              aria-pressed={selectedRepo === repo}
                              onClick={() => setSelectedRepo((current) => (current === repo ? null : repo))}
                            >
                              {repo}
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              )}
            </section>

            {selectedRepo ? (
              <section className="az-blade-section" aria-label={`Tags for ${selectedRepo}`}>
                <Text as="h2" weight="semibold" block>
                  Tags — {selectedRepo}
                </Text>
                <Table aria-label={`Tags for ${selectedRepo}`} size="small" data-testid="acr-tags">
                  <TableHeader>
                    <TableRow>
                      <TableHeaderCell>Tag</TableHeaderCell>
                      <TableHeaderCell>Digest</TableHeaderCell>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {tags.isError ? (
                      <AzureTableErrorRow colSpan={2}>
                        <strong>Could not load tags.</strong>{" "}
                        {tags.error instanceof Error ? tags.error.message : "The registry did not respond."}
                      </AzureTableErrorRow>
                    ) : tags.isLoading ? (
                      <AzureTableLoadingRow colSpan={2} label="Loading tags…" />
                    ) : (tags.data ?? []).length === 0 ? (
                      <AzureTableEmptyRow colSpan={2} title="No tags to display" />
                    ) : (
                      (tags.data ?? []).map((tag) => (
                        <TableRow key={tag.name}>
                          <TableCell>{tag.name}</TableCell>
                          <TableCell>
                            <code>{tag.digest}</code>
                          </TableCell>
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
