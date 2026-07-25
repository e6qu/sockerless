import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import Table from "@cloudscape-design/components/table";
import Header from "@cloudscape-design/components/header";
import {
  AwsButton,
  AwsContainer,
  AwsEmptyState,
  AwsErrorAlert,
  AwsKeyValue,
  AwsPageHeader,
  AwsStatus,
  AwsTabs,
  type AwsTab,
} from "../console/index.js";
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
              <AwsKeyValue
                items={[
                  { label: "Handler", value: config.handler || "–" },
                  { label: "Memory", value: `${config.memorySize} MB` },
                  { label: "Timeout", value: `${config.timeout} s` },
                  { label: "Execution role", value: config.role || "–" },
                  ...(config.vpcConfig
                    ? [
                        { label: "VPC", value: config.vpcConfig.vpcId || "–" },
                        { label: "Subnets", value: config.vpcConfig.subnetIds.join(", ") || "–" },
                        { label: "Security groups", value: config.vpcConfig.securityGroupIds.join(", ") || "–" },
                      ]
                    : []),
                ]}
              />
              <div style={{ marginTop: 20 }}>
                <Header variant="h3">Environment variables</Header>
                {config.environment.length === 0 ? (
                  <AwsEmptyState title="No environment variables" description="This function defines no environment variables." />
                ) : (
                  <Table
                    variant="embedded"
                    ariaLabels={{ tableLabel: "Environment variables" }}
                    items={config.environment}
                    columnDefinitions={[
                      { id: "name", header: "Key", cell: (entry) => <code>{entry.name}</code> },
                      { id: "value", header: "Value", cell: (entry) => <code>{entry.value}</code> },
                    ]}
                  />
                )}
              </div>
            </div>
          ),
        },
        {
          id: "code",
          label: "Code",
          content: (
            <div data-testid="lambda-function-code">
              <AwsKeyValue
                items={[
                  { label: "Package type", value: config.packageType || "Zip" },
                  { label: "Architectures", value: config.architectures.join(", ") || "–" },
                  detail.data?.code.imageUri
                    ? { label: "Container image", value: <code>{detail.data.code.imageUri}</code> }
                    : { label: "Code size", value: formatBytes(config.codeSize) },
                  { label: "Code SHA-256", value: <code>{config.codeSha256 || "–"}</code> },
                  { label: "Version", value: config.version || "–" },
                ]}
              />
            </div>
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
          <AwsErrorAlert testId="lambda-function-error">
            <strong>Could not load the function.</strong>{" "}
            {detail.error instanceof Error ? detail.error.message : "The request failed."}
          </AwsErrorAlert>
        ) : detail.isLoading ? (
          <AwsEmptyState title="Loading function…" loading />
        ) : config ? (
          <>
            <div data-testid="lambda-function-summary">
              <AwsKeyValue
                items={[
                  { label: "State", value: <AwsStatus status={config.state} /> },
                  {
                    label: "Last update status",
                    value: config.lastUpdateStatus ? <AwsStatus status={config.lastUpdateStatus} /> : "–",
                  },
                  { label: "Runtime", value: config.runtime || "–" },
                  { label: "ARN", value: config.arn },
                  { label: "Last modified", value: formatTimestamp(config.lastModified) },
                ]}
              />
            </div>
            <div style={{ marginTop: 20 }}>
              <AwsTabs ariaLabel="Function detail" tabs={tabs} />
            </div>
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
