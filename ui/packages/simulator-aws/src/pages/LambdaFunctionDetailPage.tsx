import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { AwsButton, AwsContainer, AwsPageHeader, AwsStatus, AwsTabs, type AwsTab } from "../console/index.js";
import { formatBytes, formatTimestamp } from "../console/format.js";
import { fetchLambdaFunctionDetail, type LambdaFunction } from "../api.js";
import { DeleteFunctionsModal } from "./LambdaFunctionsPage.js";

// AWS Lambda — Function detail. Reads the real Lambda REST-JSON GetFunction
// operation (GET /2015-03-31/functions/{name}, which answers Configuration,
// Code, and Tags in one call) with the operator's federated credentials, and
// reuses the Functions list page's real DeleteFunction action.

export function LambdaFunctionDetailPage() {
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const [deleting, setDeleting] = useState(false);
  const detail = useQuery({ queryKey: ["lambda-function", name], queryFn: () => fetchLambdaFunctionDetail(name) });
  const config = detail.data?.configuration;

  const asLambdaFunction: LambdaFunction | null = config
    ? {
        name: config.name,
        runtime: config.runtime,
        state: config.state,
        memorySize: config.memorySize,
        timeout: config.timeout,
        lastModified: config.lastModified,
      }
    : null;

  const tabs: AwsTab[] = config
    ? [
        {
          id: "configuration",
          label: "Configuration",
          content: (
            <div data-testid="lambda-function-configuration">
              <dl className="aws-key-value">
                <dt>Handler</dt>
                <dd>{config.handler || "–"}</dd>
                <dt>Memory</dt>
                <dd>{config.memorySize} MB</dd>
                <dt>Timeout</dt>
                <dd>{config.timeout} s</dd>
                <dt>Execution role</dt>
                <dd>{config.role || "–"}</dd>
                {config.vpcConfig && (
                  <>
                    <dt>VPC</dt>
                    <dd>{config.vpcConfig.vpcId || "–"}</dd>
                    <dt>Subnets</dt>
                    <dd>{config.vpcConfig.subnetIds.join(", ") || "–"}</dd>
                    <dt>Security groups</dt>
                    <dd>{config.vpcConfig.securityGroupIds.join(", ") || "–"}</dd>
                  </>
                )}
              </dl>
              <div className="aws-detail-section">
                <h3 className="aws-subheading">Environment variables</h3>
                {config.environment.length === 0 ? (
                  <div className="aws-empty">
                    <strong>No environment variables</strong>
                    <p>This function defines no environment variables.</p>
                  </div>
                ) : (
                  <table className="aws-table" aria-label="Environment variables">
                    <thead>
                      <tr>
                        <th scope="col">Key</th>
                        <th scope="col">Value</th>
                      </tr>
                    </thead>
                    <tbody>
                      {config.environment.map((entry) => (
                        <tr key={entry.name}>
                          <td>
                            <code>{entry.name}</code>
                          </td>
                          <td>
                            <code>{entry.value}</code>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            </div>
          ),
        },
        {
          id: "code",
          label: "Code",
          content: (
            <dl className="aws-key-value" data-testid="lambda-function-code">
              <dt>Package type</dt>
              <dd>{config.packageType || "Zip"}</dd>
              <dt>Architectures</dt>
              <dd>{config.architectures.join(", ") || "–"}</dd>
              {detail.data?.code.imageUri ? (
                <>
                  <dt>Container image</dt>
                  <dd>
                    <code>{detail.data.code.imageUri}</code>
                  </dd>
                </>
              ) : (
                <>
                  <dt>Code size</dt>
                  <dd>{formatBytes(config.codeSize)}</dd>
                </>
              )}
              <dt>Code SHA-256</dt>
              <dd>
                <code>{config.codeSha256 || "–"}</code>
              </dd>
              <dt>Version</dt>
              <dd>{config.version || "–"}</dd>
            </dl>
          ),
        },
      ]
    : [];

  return (
    <>
      <AwsPageHeader
        title={name}
        description={config?.description || "Function in AWS Lambda."}
        actions={
          <AwsButton data-testid="lambda-function-delete" disabled={!config} onClick={() => setDeleting(true)}>
            Delete
          </AwsButton>
        }
      />
      <AwsContainer>
        {detail.isError ? (
          <div className="aws-flash aws-flash-error" role="alert" data-testid="lambda-function-error">
            <strong>Could not load the function.</strong>{" "}
            {detail.error instanceof Error ? detail.error.message : "The request failed."}
          </div>
        ) : detail.isLoading ? (
          <div className="aws-empty" role="status">
            Loading function…
          </div>
        ) : config ? (
          <>
            <dl className="aws-key-value" data-testid="lambda-function-summary">
              <dt>State</dt>
              <dd>
                <AwsStatus status={config.state} />
              </dd>
              <dt>Last update status</dt>
              <dd>{config.lastUpdateStatus ? <AwsStatus status={config.lastUpdateStatus} /> : "–"}</dd>
              <dt>Runtime</dt>
              <dd>{config.runtime || "–"}</dd>
              <dt>ARN</dt>
              <dd>{config.arn}</dd>
              <dt>Last modified</dt>
              <dd>{formatTimestamp(config.lastModified)}</dd>
            </dl>
            <AwsTabs ariaLabel="Function detail" tabs={tabs} />
          </>
        ) : null}
      </AwsContainer>
      {deleting && asLambdaFunction && (
        <DeleteFunctionsModal
          functions={[asLambdaFunction]}
          clearSelection={() => navigate("/ui/lambda")}
          onClose={() => setDeleting(false)}
        />
      )}
    </>
  );
}
