import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { GcpResourceDetail, GcpStatus } from "../console/index.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { fetchCloudFunction, type CloudFunction } from "../api.js";
import { DeleteFunctionDialog } from "./CloudFunctionsPage.js";
import { useProject } from "../console/project.js";

export function CloudFunctionDetailPage() {
  const { name = "" } = useParams();
  const { project } = useProject();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [deleting, setDeleting] = useState(false);
  return (
    <>
      <GcpResourceDetail<CloudFunction>
        title={shortName(name)}
        description="Cloud Run function"
        backTo="/ui/functions"
        backLabel="Cloud Run functions"
        queryKey={["cloud-function", project, name]}
        queryFn={() => fetchCloudFunction(project, name)}
        // Deletion addresses the function by the id in the route, not the
        // loaded resource, so the action is available (and testable) even
        // before the read settles.
        actions={[{ label: "Delete", testId: "function-delete", onSelect: () => setDeleting(true) }]}
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
      {deleting ? (
        <DeleteFunctionDialog
          project={project}
          functionId={name}
          onClose={() => setDeleting(false)}
          onDeleted={() => {
            void queryClient.invalidateQueries({ queryKey: ["cloud-functions-real", project] });
            navigate("/ui/functions");
          }}
        />
      ) : null}
    </>
  );
}
