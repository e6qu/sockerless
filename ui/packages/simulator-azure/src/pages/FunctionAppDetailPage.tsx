import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { Table, TableHeader, TableRow, TableHeaderCell, TableBody, TableCell, Text } from "@fluentui/react-components";
import { AzureCommandBar, AzureEssentials, AzureStatus, AzureErrorMessage, AzureEmptyState } from "../portal/AzurePortal.js";
import { AzureTableErrorRow, AzureTableLoadingRow, AzureTableEmptyRow } from "../portal/AzureTable.js";
import { resourceGroupOf, locationLabel } from "../portal/format.js";
import { fetchFunctionApp, fetchFunctionAppSettings, fetchFunctions } from "../api.js";

// The Function App blade: the real Microsoft.Web/sites Essentials, the
// site's application settings (read through the real `/list` action, since
// Microsoft.Web serves current app-setting values only that way), and the
// deployed functions — all real Azure Resource Manager reads.
export function FunctionAppDetailPage() {
  const { name = "" } = useParams();

  const site = useQuery({ queryKey: ["fn-site", name], queryFn: () => fetchFunctionApp(name) });
  const siteId = site.data?.id;
  const settings = useQuery({
    queryKey: ["fn-site-settings", siteId],
    queryFn: () => fetchFunctionAppSettings(siteId!),
    enabled: Boolean(siteId),
  });
  const functions = useQuery({
    queryKey: ["fn-site-functions", siteId],
    queryFn: () => fetchFunctions(siteId!),
    enabled: Boolean(siteId),
  });

  return (
    <>
      <AzureCommandBar
        commands={[
          {
            label: "Refresh",
            icon: "refresh",
            onSelect: () => {
              void site.refetch();
              void settings.refetch();
              void functions.refetch();
            },
            disabled: site.isFetching,
          },
          { label: "Feedback", icon: "feedback" },
        ]}
      />
      <div className="az-main" data-testid="fn-site-detail">
        {site.isError ? (
          <AzureErrorMessage testid="fn-site-error">
            <strong>Could not load this Function App.</strong>{" "}
            {site.error instanceof Error ? site.error.message : "Azure Resource Manager did not respond."}
          </AzureErrorMessage>
        ) : site.isLoading || !site.data ? (
          <AzureEmptyState title="Loading the Function App…" loading />
        ) : (
          <>
            <AzureEssentials
              properties={[
                { label: "Resource group", value: resourceGroupOf(site.data.id) },
                { label: "Location", value: locationLabel(site.data.location) },
                { label: "Kind", value: site.data.kind || "—" },
                { label: "Status", value: <AzureStatus status={site.data.state || (site.data.enabled ? "Running" : "Stopped")} /> },
                { label: "Default domain", value: site.data.defaultHostName ? <code>{site.data.defaultHostName}</code> : "—" },
                { label: "HTTPS only", value: site.data.httpsOnly ? "Enabled" : "Disabled" },
              ]}
            />

            <section className="az-blade-section" aria-label="Application settings">
              <Text as="h2" weight="semibold" block>
                Application settings
              </Text>
              <Table aria-label="Application settings" size="small" data-testid="fn-site-settings">
                <TableHeader>
                  <TableRow>
                    <TableHeaderCell>Name</TableHeaderCell>
                    <TableHeaderCell>Value</TableHeaderCell>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {settings.isError ? (
                    <AzureTableErrorRow colSpan={2}>
                      <strong>Could not load application settings.</strong>{" "}
                      {settings.error instanceof Error ? settings.error.message : "Azure Resource Manager did not respond."}
                    </AzureTableErrorRow>
                  ) : settings.isLoading ? (
                    <AzureTableLoadingRow colSpan={2} label="Loading application settings…" />
                  ) : Object.keys(settings.data ?? {}).length === 0 ? (
                    <AzureTableEmptyRow colSpan={2} title="No application settings" />
                  ) : (
                    Object.entries(settings.data ?? {}).map(([key, value]) => (
                      <TableRow key={key}>
                        <TableCell>
                          <code>{key}</code>
                        </TableCell>
                        <TableCell>
                          <code>{value}</code>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </section>

            <section className="az-blade-section" aria-label="Functions">
              <Text as="h2" weight="semibold" block>
                Functions
              </Text>
              <Table aria-label="Functions" size="small" data-testid="fn-site-functions">
                <TableHeader>
                  <TableRow>
                    <TableHeaderCell>Name</TableHeaderCell>
                    <TableHeaderCell>Language</TableHeaderCell>
                    <TableHeaderCell>Status</TableHeaderCell>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {functions.isError ? (
                    <AzureTableErrorRow colSpan={3}>
                      <strong>Could not load functions.</strong>{" "}
                      {functions.error instanceof Error ? functions.error.message : "Azure Resource Manager did not respond."}
                    </AzureTableErrorRow>
                  ) : functions.isLoading ? (
                    <AzureTableLoadingRow colSpan={3} label="Loading functions…" />
                  ) : (functions.data ?? []).length === 0 ? (
                    <AzureTableEmptyRow
                      colSpan={3}
                      title="No functions to display"
                      description="Functions deployed to this app appear here."
                    />
                  ) : (
                    (functions.data ?? []).map((fn) => (
                      <TableRow key={fn.name}>
                        <TableCell>{fn.name}</TableCell>
                        <TableCell>{fn.language || "—"}</TableCell>
                        <TableCell>
                          <AzureStatus status={fn.isDisabled ? "Disabled" : "Enabled"} />
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </section>
          </>
        )}
      </div>
    </>
  );
}
