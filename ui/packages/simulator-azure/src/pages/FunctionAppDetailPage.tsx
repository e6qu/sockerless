import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { AzureCommandBar, AzureEssentials, AzureStatus } from "../portal/AzurePortal.js";
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
          <div className="az-message az-message-error" role="alert" data-testid="fn-site-error">
            <strong>Could not load this Function App.</strong>{" "}
            {site.error instanceof Error ? site.error.message : "Azure Resource Manager did not respond."}
          </div>
        ) : site.isLoading || !site.data ? (
          <div className="az-empty" role="status">
            Loading the Function App…
          </div>
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
              <h2>Application settings</h2>
              <table className="az-table" data-testid="fn-site-settings">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Value</th>
                  </tr>
                </thead>
                <tbody>
                  {settings.isError ? (
                    <tr>
                      <td className="az-table-state" colSpan={2}>
                        <div className="az-message az-message-error" role="alert">
                          <strong>Could not load application settings.</strong>{" "}
                          {settings.error instanceof Error ? settings.error.message : "Azure Resource Manager did not respond."}
                        </div>
                      </td>
                    </tr>
                  ) : settings.isLoading ? (
                    <tr>
                      <td className="az-table-state" colSpan={2}>
                        <div className="az-empty" role="status">
                          Loading application settings…
                        </div>
                      </td>
                    </tr>
                  ) : Object.keys(settings.data ?? {}).length === 0 ? (
                    <tr>
                      <td className="az-table-state" colSpan={2}>
                        <div className="az-empty">
                          <strong>No application settings</strong>
                        </div>
                      </td>
                    </tr>
                  ) : (
                    Object.entries(settings.data ?? {}).map(([key, value]) => (
                      <tr key={key}>
                        <td>
                          <code>{key}</code>
                        </td>
                        <td>
                          <code>{value}</code>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </section>

            <section className="az-blade-section" aria-label="Functions">
              <h2>Functions</h2>
              <table className="az-table" data-testid="fn-site-functions">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Language</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {functions.isError ? (
                    <tr>
                      <td className="az-table-state" colSpan={3}>
                        <div className="az-message az-message-error" role="alert">
                          <strong>Could not load functions.</strong>{" "}
                          {functions.error instanceof Error ? functions.error.message : "Azure Resource Manager did not respond."}
                        </div>
                      </td>
                    </tr>
                  ) : functions.isLoading ? (
                    <tr>
                      <td className="az-table-state" colSpan={3}>
                        <div className="az-empty" role="status">
                          Loading functions…
                        </div>
                      </td>
                    </tr>
                  ) : (functions.data ?? []).length === 0 ? (
                    <tr>
                      <td className="az-table-state" colSpan={3}>
                        <div className="az-empty">
                          <strong>No functions to display</strong>
                          <p>Functions deployed to this app appear here.</p>
                        </div>
                      </td>
                    </tr>
                  ) : (
                    (functions.data ?? []).map((fn) => (
                      <tr key={fn.name}>
                        <td>{fn.name}</td>
                        <td>{fn.language || "—"}</td>
                        <td>
                          <AzureStatus status={fn.isDisabled ? "Disabled" : "Enabled"} />
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </section>
          </>
        )}
      </div>
    </>
  );
}
