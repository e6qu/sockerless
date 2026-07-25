import { useParams } from "react-router";
import { GcpResourceDetail, GcpStatus } from "../console/index.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { fetchCloudFunction, type CloudFunction } from "../api.js";
import { useProject } from "../console/project.js";

export function CloudFunctionDetailPage() {
  const { name = "" } = useParams();
  const { project } = useProject();
  return (
    <GcpResourceDetail<CloudFunction>
      title={shortName(name)}
      description="Cloud Run function"
      backTo="/ui/functions"
      backLabel="Cloud Run functions"
      queryKey={["cloud-function", project, name]}
      queryFn={() => fetchCloudFunction(project, name)}
      properties={(fn) => [
        { label: "State", value: <GcpStatus status={fn.state ?? "UNKNOWN"} /> },
        { label: "Environment", value: fn.environment ?? "—" },
        { label: "Runtime", value: fn.buildConfig?.runtime ?? "—" },
        { label: "Entry point", value: fn.buildConfig?.entryPoint ?? "—" },
        { label: "URL", value: fn.serviceConfig?.uri ?? "—" },
        { label: "Created", value: formatTimestamp(fn.createTime ?? "") },
        { label: "Updated", value: formatTimestamp(fn.updateTime ?? "") },
        { label: "Description", value: fn.description ?? "—" },
      ]}
      extra={(fn) => {
        const service = fn.serviceConfig;
        const envVars = Object.entries(service?.environmentVariables ?? {});
        return (
          <>
            <h2 className="gc-detail-heading">Configuration</h2>
            <dl className="gc-detail-grid">
              {[
                { label: "Memory allocated", value: service?.availableMemory ?? "—" },
                { label: "CPU", value: service?.availableCpu ?? "—" },
                { label: "Timeout", value: service?.timeoutSeconds ? `${service.timeoutSeconds} seconds` : "—" },
                { label: "Minimum instances", value: String(service?.minInstanceCount ?? 0) },
                { label: "Maximum instances", value: service?.maxInstanceCount ? String(service.maxInstanceCount) : "—" },
                { label: "Ingress settings", value: service?.ingressSettings ?? "—" },
                { label: "Underlying Cloud Run service", value: service?.service ? shortName(service.service) : "—" },
              ].map((property) => (
                <div className="gc-detail-pair" key={property.label}>
                  <dt>{property.label}</dt>
                  <dd>{property.value}</dd>
                </div>
              ))}
            </dl>

            <h2 className="gc-detail-heading">Runtime environment variables</h2>
            {envVars.length === 0 ? (
              <p className="gc-page-description">No environment variables are configured for this function.</p>
            ) : (
              <div className="gc-table-wrap">
                <table className="gc-table" data-testid="function-env-vars-table">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Value</th>
                    </tr>
                  </thead>
                  <tbody>
                    {envVars.map(([key, value]) => (
                      <tr key={key}>
                        <td>{key}</td>
                        <td>{value}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        );
      }}
    />
  );
}
