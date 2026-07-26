import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Table from "@cloudscape-design/components/table";
import Header from "@cloudscape-design/components/header";
import SpaceBetween from "@cloudscape-design/components/space-between";
import FormField from "@cloudscape-design/components/form-field";
import Input from "@cloudscape-design/components/input";
import Textarea from "@cloudscape-design/components/textarea";
import Box from "@cloudscape-design/components/box";
import StatusIndicator from "@cloudscape-design/components/status-indicator";
import {
  AwsButton,
  AwsContainer,
  AwsEmptyState,
  AwsErrorAlert,
  AwsKeyValue,
  AwsModal,
  AwsPageHeader,
  AwsStatus,
  AwsTabs,
  KeyValueEditor,
  removedKeys,
  rowsAreValid,
  rowsToTags,
  TagsEditorModal,
  type AwsTab,
  type KeyValueRow,
} from "../console/index.js";
import { formatBytes, formatTimestamp } from "../console/format.js";
import {
  fetchLambdaFunctionDetail,
  invokeLambdaFunction,
  tagLambdaResource,
  untagLambdaResource,
  updateLambdaFunctionConfiguration,
  type LambdaFunction,
  type LambdaFunctionDetail,
  type LambdaInvokeResult,
} from "../api.js";
import { DeleteFunctionsModal } from "./LambdaFunctionsPage.js";

// AWS Lambda — Function detail. Reads the real Lambda REST-JSON GetFunction
// operation (GET /2015-03-31/functions/{name}, which answers Configuration,
// Code, and Tags in one call) with the operator's federated credentials, and
// drives the real update surface the console offers for a function:
// UpdateFunctionConfiguration (memory, timeout, description, environment),
// Invoke (the "Test" action), TagResource/UntagResource, and DeleteFunction.

function EditConfigurationModal({
  name,
  config,
  onClose,
}: {
  name: string;
  config: LambdaFunctionDetail;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [memory, setMemory] = useState(String(config.memorySize));
  const [timeout, setTimeout] = useState(String(config.timeout));
  const [description, setDescription] = useState(config.description);
  const [env, setEnv] = useState<KeyValueRow[]>(config.environment.map((e) => ({ key: e.name, value: e.value })));

  const memoryValue = Number(memory);
  const timeoutValue = Number(timeout);
  const memoryValid = Number.isInteger(memoryValue) && memoryValue >= 128 && memoryValue <= 10240;
  const timeoutValid = Number.isInteger(timeoutValue) && timeoutValue >= 1 && timeoutValue <= 900;
  const envValid = rowsAreValid(env);
  const valid = memoryValid && timeoutValid && envValid;

  const update = useMutation({
    mutationFn: () =>
      updateLambdaFunctionConfiguration(name, {
        memorySize: memoryValue,
        timeout: timeoutValue,
        description,
        environment: Object.entries(rowsToTags(env)).map(([name, value]) => ({ name, value })),
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["lambda-function", name] });
      onClose();
    },
  });

  return (
    <AwsModal
      title="Edit basic settings"
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="lambda-edit-config-save"
            disabled={!valid || update.isPending}
            onClick={() => update.mutate()}
          >
            {update.isPending ? "Saving…" : "Save"}
          </AwsButton>
        </>
      }
    >
      <SpaceBetween size="m">
        <FormField
          label="Memory"
          constraintText="128–10240 MB."
          errorText={memory && !memoryValid ? "Enter a whole number of MB between 128 and 10240." : undefined}
        >
          <Input
            type="number"
            value={memory}
            onChange={(event) => setMemory(event.detail.value)}
            nativeInputAttributes={{ "data-testid": "lambda-edit-config-memory" }}
          />
        </FormField>
        <FormField
          label="Timeout"
          constraintText="1–900 seconds."
          errorText={timeout && !timeoutValid ? "Enter a whole number of seconds between 1 and 900." : undefined}
        >
          <Input
            type="number"
            value={timeout}
            onChange={(event) => setTimeout(event.detail.value)}
            nativeInputAttributes={{ "data-testid": "lambda-edit-config-timeout" }}
          />
        </FormField>
        <FormField label="Description">
          <Input
            value={description}
            onChange={(event) => setDescription(event.detail.value)}
            nativeInputAttributes={{ "data-testid": "lambda-edit-config-description" }}
          />
        </FormField>
        <FormField label="Environment variables" description="Key/value pairs passed to the function at runtime.">
          <KeyValueEditor
            rows={env}
            onChange={setEnv}
            keyLabel="Key"
            valueLabel="Value"
            addLabel="Add environment variable"
            emptyText="No environment variables."
            testIdPrefix="lambda-edit-config-env"
          />
        </FormField>
        {update.isError && (
          <AwsErrorAlert>
            <strong>Could not save the configuration.</strong>{" "}
            {update.error instanceof Error ? update.error.message : "The request failed."}
          </AwsErrorAlert>
        )}
      </SpaceBetween>
    </AwsModal>
  );
}

function TestFunctionModal({ name, onClose }: { name: string; onClose: () => void }) {
  const [payload, setPayload] = useState("{}");
  const [result, setResult] = useState<LambdaInvokeResult | null>(null);
  const invoke = useMutation({
    mutationFn: () => invokeLambdaFunction(name, payload),
    onSuccess: (data) => setResult(data),
  });
  return (
    <AwsModal
      title={`Test ${name}`}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Close</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="lambda-test-invoke"
            disabled={invoke.isPending}
            onClick={() => invoke.mutate()}
          >
            {invoke.isPending ? "Invoking…" : "Test"}
          </AwsButton>
        </>
      }
    >
      <SpaceBetween size="m">
        <FormField label="Event JSON" description="The payload passed to the function on this invocation.">
          <Textarea
            value={payload}
            onChange={(event) => setPayload(event.detail.value)}
            rows={6}
            ariaLabel="Event JSON payload"
          />
        </FormField>
        {invoke.isError && (
          <AwsErrorAlert>
            <strong>Could not invoke the function.</strong>{" "}
            {invoke.error instanceof Error ? invoke.error.message : "The request failed."}
          </AwsErrorAlert>
        )}
        {result && (
          <div data-testid="lambda-test-result">
            <Box variant="awsui-key-label">Execution result</Box>
            <Box margin={{ bottom: "xs" }}>
              {result.functionError ? (
                <StatusIndicator type="error">Failed ({result.functionError})</StatusIndicator>
              ) : (
                <StatusIndicator type="success">Succeeded</StatusIndicator>
              )}
            </Box>
            <FormField label="Response payload">
              <Textarea value={result.payload} readOnly rows={6} ariaLabel="Response payload" />
            </FormField>
          </div>
        )}
      </SpaceBetween>
    </AwsModal>
  );
}

export function LambdaFunctionDetailPage() {
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [deleting, setDeleting] = useState(false);
  const [editing, setEditing] = useState(false);
  const [testing, setTesting] = useState(false);
  const [taggingArn, setTaggingArn] = useState<string | null>(null);
  const detail = useQuery({ queryKey: ["lambda-function", name], queryFn: () => fetchLambdaFunctionDetail(name) });
  const config = detail.data?.configuration;
  const tags = detail.data?.tags ?? {};

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
        {
          id: "tags",
          label: "Tags",
          content: (
            <div data-testid="lambda-function-tags">
              <SpaceBetween size="m">
                <Header
                  variant="h3"
                  actions={
                    <AwsButton data-testid="lambda-function-manage-tags" onClick={() => setTaggingArn(config.arn)}>
                      Manage tags
                    </AwsButton>
                  }
                >
                  Tags
                </Header>
                {Object.keys(tags).length === 0 ? (
                  <AwsEmptyState title="No tags" description="This function has no tags." />
                ) : (
                  <Table
                    variant="embedded"
                    ariaLabels={{ tableLabel: "Tags" }}
                    items={Object.entries(tags).map(([key, value]) => ({ key, value }))}
                    columnDefinitions={[
                      { id: "key", header: "Key", cell: (entry) => <code>{entry.key}</code> },
                      { id: "value", header: "Value", cell: (entry) => <code>{entry.value}</code> },
                    ]}
                  />
                )}
              </SpaceBetween>
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
          <SpaceBetween direction="horizontal" size="xs">
            <AwsButton data-testid="lambda-function-test" disabled={!config} onClick={() => setTesting(true)}>
              Test
            </AwsButton>
            <AwsButton data-testid="lambda-function-edit" disabled={!config} onClick={() => setEditing(true)}>
              Edit
            </AwsButton>
            <AwsButton data-testid="lambda-function-delete" disabled={!config} onClick={() => setDeleting(true)}>
              Delete
            </AwsButton>
          </SpaceBetween>
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
      {editing && config && <EditConfigurationModal name={name} config={config} onClose={() => setEditing(false)} />}
      {testing && config && <TestFunctionModal name={name} onClose={() => setTesting(false)} />}
      {taggingArn && (
        <TagsEditorModal
          title="Manage tags"
          intro={`Tags applied to the ${name} function.`}
          initialTags={tags}
          testIdPrefix="lambda-function"
          onClose={() => setTaggingArn(null)}
          onSaved={() => queryClient.invalidateQueries({ queryKey: ["lambda-function", name] })}
          save={async (next) => {
            const remove = removedKeys(tags, next);
            if (Object.keys(next).length > 0) await tagLambdaResource(taggingArn, next);
            if (remove.length > 0) await untagLambdaResource(taggingArn, remove);
          }}
        />
      )}
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
