import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Table from "@cloudscape-design/components/table";
import Header from "@cloudscape-design/components/header";
import SpaceBetween from "@cloudscape-design/components/space-between";
import FormField from "@cloudscape-design/components/form-field";
import Input from "@cloudscape-design/components/input";
import Select from "@cloudscape-design/components/select";
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
import { formatBytes, formatEpoch, formatTimestamp } from "../console/format.js";
import {
  fetchLambdaAliases,
  fetchLambdaConcurrency,
  fetchLambdaEventSourceMappings,
  fetchLambdaFunctionDetail,
  fetchLambdaFunctionUrls,
  fetchLambdaVersions,
  fetchCWLogEvents,
  fetchCWLogStreams,
  createLambdaAlias,
  createLambdaFunctionUrl,
  deleteLambdaConcurrency,
  deleteLambdaFunctionUrl,
  invokeLambdaFunction,
  publishLambdaVersion,
  putLambdaConcurrency,
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

function PublishVersionModal({ name, onClose }: { name: string; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [description, setDescription] = useState("");
  const publish = useMutation({
    mutationFn: () => publishLambdaVersion(name, description),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["lambda-versions", name] });
      onClose();
    },
  });
  return (
    <AwsModal
      title="Publish new version"
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton variant="primary" disabled={publish.isPending} onClick={() => publish.mutate()}>
            {publish.isPending ? "Publishing…" : "Publish"}
          </AwsButton>
        </>
      }
    >
      <SpaceBetween size="m">
        <p>Publish an immutable snapshot of the current code and configuration.</p>
        <FormField label="Version description">
          <Input value={description} onChange={(event) => setDescription(event.detail.value)} />
        </FormField>
        {publish.isError && <AwsErrorAlert>{publish.error instanceof Error ? publish.error.message : "Publish failed."}</AwsErrorAlert>}
      </SpaceBetween>
    </AwsModal>
  );
}

function CreateAliasModal({
  name,
  versions,
  onClose,
}: {
  name: string;
  versions: { version: string }[];
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const published = versions.filter((version) => version.version !== "$LATEST");
  const [alias, setAlias] = useState("");
  const [version, setVersion] = useState(published.at(-1)?.version ?? "");
  const [description, setDescription] = useState("");
  const create = useMutation({
    mutationFn: () => createLambdaAlias(name, alias, version, description),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["lambda-aliases", name] });
      onClose();
    },
  });
  const valid = /^[a-zA-Z0-9-_]{1,128}$/.test(alias) && !/^\d+$/.test(alias) && version !== "";
  return (
    <AwsModal
      title="Create alias"
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton variant="primary" disabled={!valid || create.isPending} onClick={() => create.mutate()}>
            {create.isPending ? "Creating…" : "Create"}
          </AwsButton>
        </>
      }
    >
      <SpaceBetween size="m">
        <FormField label="Alias name">
          <Input value={alias} onChange={(event) => setAlias(event.detail.value)} />
        </FormField>
        <FormField label="Function version">
          <Select
            selectedOption={version ? { label: version, value: version } : null}
            options={published.map((item) => ({ label: item.version, value: item.version }))}
            onChange={(event) => setVersion(event.detail.selectedOption.value ?? "")}
            placeholder="Choose a published version"
          />
        </FormField>
        <FormField label="Description">
          <Input value={description} onChange={(event) => setDescription(event.detail.value)} />
        </FormField>
        {published.length === 0 && <AwsErrorAlert>Publish a function version before creating an alias.</AwsErrorAlert>}
        {create.isError && <AwsErrorAlert>{create.error instanceof Error ? create.error.message : "Create failed."}</AwsErrorAlert>}
      </SpaceBetween>
    </AwsModal>
  );
}

function ConcurrencyModal({
  name,
  current,
  onClose,
}: {
  name: string;
  current?: number;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [value, setValue] = useState(current === undefined ? "" : String(current));
  const parsed = Number(value);
  const valid = Number.isInteger(parsed) && parsed >= 0;
  const save = useMutation({
    mutationFn: () => putLambdaConcurrency(name, parsed),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["lambda-concurrency", name] });
      onClose();
    },
  });
  const remove = useMutation({
    mutationFn: () => deleteLambdaConcurrency(name),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["lambda-concurrency", name] });
      onClose();
    },
  });
  return (
    <AwsModal
      title="Edit reserved concurrency"
      onDismiss={onClose}
      footer={
        <>
          {current !== undefined && (
            <AwsButton disabled={remove.isPending || save.isPending} onClick={() => remove.mutate()}>
              Use unreserved concurrency
            </AwsButton>
          )}
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton variant="primary" disabled={!valid || save.isPending || remove.isPending} onClick={() => save.mutate()}>
            {save.isPending ? "Saving…" : "Save"}
          </AwsButton>
        </>
      }
    >
      <SpaceBetween size="m">
        <FormField label="Reserved concurrent executions" constraintText="Use 0 to throttle all invocations.">
          <Input type="number" value={value} onChange={(event) => setValue(event.detail.value)} />
        </FormField>
        {(save.isError || remove.isError) && (
          <AwsErrorAlert>
            {save.error instanceof Error
              ? save.error.message
              : remove.error instanceof Error
                ? remove.error.message
                : "Update failed."}
          </AwsErrorAlert>
        )}
      </SpaceBetween>
    </AwsModal>
  );
}

function FunctionUrlModal({ name, onClose }: { name: string; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [authType, setAuthType] = useState<"AWS_IAM" | "NONE">("AWS_IAM");
  const [invokeMode, setInvokeMode] = useState<"BUFFERED" | "RESPONSE_STREAM">("BUFFERED");
  const create = useMutation({
    mutationFn: () => createLambdaFunctionUrl(name, authType, invokeMode),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["lambda-function-urls", name] });
      onClose();
    },
  });
  return (
    <AwsModal
      title="Create function URL"
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton variant="primary" disabled={create.isPending} onClick={() => create.mutate()}>
            {create.isPending ? "Creating…" : "Create"}
          </AwsButton>
        </>
      }
    >
      <SpaceBetween size="m">
        <FormField label="Auth type">
          <Select
            selectedOption={{ label: authType === "AWS_IAM" ? "AWS IAM" : "None", value: authType }}
            options={[{ label: "AWS IAM", value: "AWS_IAM" }, { label: "None", value: "NONE" }]}
            onChange={(event) => setAuthType(event.detail.selectedOption.value as "AWS_IAM" | "NONE")}
          />
        </FormField>
        <FormField label="Invoke mode">
          <Select
            selectedOption={{ label: invokeMode === "BUFFERED" ? "Buffered" : "Response stream", value: invokeMode }}
            options={[
              { label: "Buffered", value: "BUFFERED" },
              { label: "Response stream", value: "RESPONSE_STREAM" },
            ]}
            onChange={(event) => setInvokeMode(event.detail.selectedOption.value as "BUFFERED" | "RESPONSE_STREAM")}
          />
        </FormField>
        {create.isError && <AwsErrorAlert>{create.error instanceof Error ? create.error.message : "Create failed."}</AwsErrorAlert>}
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
  const [publishing, setPublishing] = useState(false);
  const [creatingAlias, setCreatingAlias] = useState(false);
  const [editingConcurrency, setEditingConcurrency] = useState(false);
  const [creatingFunctionUrl, setCreatingFunctionUrl] = useState(false);
  const [taggingArn, setTaggingArn] = useState<string | null>(null);
  const detail = useQuery({ queryKey: ["lambda-function", name], queryFn: () => fetchLambdaFunctionDetail(name) });
  const aliases = useQuery({ queryKey: ["lambda-aliases", name], queryFn: () => fetchLambdaAliases(name) });
  const versions = useQuery({ queryKey: ["lambda-versions", name], queryFn: () => fetchLambdaVersions(name) });
  const eventSources = useQuery({
    queryKey: ["lambda-event-source-mappings", name],
    queryFn: () => fetchLambdaEventSourceMappings(name),
  });
  const concurrency = useQuery({
    queryKey: ["lambda-concurrency", name],
    queryFn: () => fetchLambdaConcurrency(name),
  });
  const functionUrls = useQuery({
    queryKey: ["lambda-function-urls", name],
    queryFn: () => fetchLambdaFunctionUrls(name),
  });
  const recentLogs = useQuery({
    queryKey: ["lambda-recent-logs", name],
    queryFn: async () => {
      const streams = await fetchCWLogStreams(`/aws/lambda/${name}`);
      const latest = [...streams].sort((a, b) => b.lastEventTimestamp - a.lastEventTimestamp)[0];
      if (!latest) return [];
      return fetchCWLogEvents(`/aws/lambda/${name}`, latest.name);
    },
  });
  const removeFunctionUrl = useMutation({
    mutationFn: () => deleteLambdaFunctionUrl(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["lambda-function-urls", name] }),
  });
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
          id: "overview",
          label: "Function overview",
          content: (
            <div className="aws-lambda-overview" data-testid="lambda-function-overview">
              <div className="aws-lambda-trigger-column">
                <Header variant="h3">Triggers</Header>
                {eventSources.isLoading ? (
                  <AwsEmptyState title="Loading triggers…" loading />
                ) : eventSources.data?.length ? (
                  eventSources.data.map((source) => (
                    <div className="aws-lambda-resource-card" key={source.uuid}>
                      <strong>{source.eventSourceArn.split(":")[2] || "Event source"}</strong>
                      <small>{source.eventSourceArn}</small>
                    </div>
                  ))
                ) : (
                  <AwsEmptyState title="No triggers" description="No event source mappings invoke this function." />
                )}
              </div>
              <div className="aws-lambda-function-node">
                <span className="aws-lambda-icon">λ</span>
                <strong>{config.name}</strong>
                <small>{config.packageType === "Image" ? "Container image" : config.runtime}</small>
              </div>
              <div className="aws-lambda-destination-column">
                <Header variant="h3">Destinations</Header>
                <AwsEmptyState
                  title="No destinations"
                  description="No asynchronous invocation destinations are configured."
                />
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
                  { label: "Runtime", value: config.runtime || "–" },
                  { label: "Handler", value: config.handler || "–" },
                  { label: "Architectures", value: config.architectures.join(", ") || "–" },
                  detail.data?.code.imageUri
                    ? { label: "Container image", value: <code>{detail.data.code.imageUri}</code> }
                    : { label: "Code size", value: formatBytes(config.codeSize) },
                  { label: "Code SHA-256", value: <code>{config.codeSha256 || "–"}</code> },
                  { label: "Version", value: config.version || "–" },
                  ...(config.imageConfig
                    ? [
                        { label: "Entry point", value: config.imageConfig.entryPoint.join(" ") || "Image default" },
                        { label: "Command", value: config.imageConfig.command.join(" ") || "Image default" },
                        { label: "Working directory", value: config.imageConfig.workingDirectory || "Image default" },
                      ]
                    : []),
                ]}
              />
            </div>
          ),
        },
        {
          id: "test",
          label: "Test",
          content: (
            <SpaceBetween size="m">
              <Header
                variant="h3"
                actions={
                  <AwsButton variant="primary" onClick={() => setTesting(true)}>
                    Test function
                  </AwsButton>
                }
              >
                Test event
              </Header>
              <p>Invoke the published function synchronously with a JSON event and inspect its real response payload.</p>
            </SpaceBetween>
          ),
        },
        {
          id: "monitor",
          label: "Monitor",
          content: (
            <SpaceBetween size="m">
              <Header
                variant="h3"
                actions={
                  <SpaceBetween direction="horizontal" size="xs">
                    <AwsButton onClick={() => setEditingConcurrency(true)}>Edit concurrency</AwsButton>
                    <AwsButton
                      onClick={() => navigate(`/ui/logs/${encodeURIComponent(`/aws/lambda/${config.name}`)}`)}
                    >
                      View CloudWatch logs
                    </AwsButton>
                  </SpaceBetween>
                }
              >
                Monitoring
              </Header>
              <AwsKeyValue
                items={[
                  { label: "Log group", value: <code>/aws/lambda/{config.name}</code> },
                  {
                    label: "Reserved concurrency",
                    value:
                      concurrency.data?.reservedConcurrentExecutions === undefined
                        ? "Unreserved account concurrency"
                        : concurrency.data.reservedConcurrentExecutions,
                  },
                  { label: "Last update", value: <AwsStatus status={config.lastUpdateStatus || config.state} /> },
                ]}
              />
              {recentLogs.isError && (
                <AwsErrorAlert>
                  {recentLogs.error instanceof Error ? recentLogs.error.message : "Could not load CloudWatch Logs."}
                </AwsErrorAlert>
              )}
              <Table
                variant="embedded"
                loading={recentLogs.isLoading}
                loadingText="Loading recent logs"
                items={recentLogs.data ?? []}
                ariaLabels={{ tableLabel: "Recent Lambda log events" }}
                header={<Header variant="h3">Recent invocations</Header>}
                empty={<AwsEmptyState title="No log events" description="Invoke this function to produce runtime logs." />}
                columnDefinitions={[
                  { id: "timestamp", header: "Timestamp", cell: (event) => formatEpoch(event.timestamp / 1000) },
                  { id: "message", header: "Message", cell: (event) => <pre className="aws-lambda-log-message">{event.message}</pre> },
                ]}
              />
            </SpaceBetween>
          ),
        },
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
                <Header variant="h3">Layers</Header>
                {config.layers.length === 0 ? (
                  <AwsEmptyState title="No layers" description="This function has no Lambda layers." />
                ) : (
                  <Table
                    variant="embedded"
                    ariaLabels={{ tableLabel: "Lambda layers" }}
                    items={config.layers}
                    columnDefinitions={[
                      { id: "arn", header: "Layer version ARN", cell: (layer) => <code>{layer.arn}</code> },
                      { id: "size", header: "Code size", cell: (layer) => formatBytes(layer.codeSize) },
                    ]}
                  />
                )}
              </div>
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
          id: "aliases",
          label: `Aliases (${aliases.data?.length ?? 0})`,
          content: (
            <Table
              variant="embedded"
              loading={aliases.isLoading}
              loadingText="Loading aliases"
              items={aliases.data ?? []}
              ariaLabels={{ tableLabel: "Function aliases" }}
              header={
                <Header
                  variant="h3"
                  actions={<AwsButton onClick={() => setCreatingAlias(true)}>Create alias</AwsButton>}
                >
                  Aliases
                </Header>
              }
              empty={<AwsEmptyState title="No aliases" description="This function has no aliases." />}
              columnDefinitions={[
                { id: "name", header: "Name", cell: (alias) => alias.name },
                { id: "version", header: "Function version", cell: (alias) => alias.functionVersion },
                { id: "description", header: "Description", cell: (alias) => alias.description || "–" },
                { id: "arn", header: "ARN", cell: (alias) => alias.arn },
              ]}
            />
          ),
        },
        {
          id: "versions",
          label: `Versions (${versions.data?.length ?? 0})`,
          content: (
            <Table
              variant="embedded"
              loading={versions.isLoading}
              loadingText="Loading versions"
              items={versions.data ?? []}
              ariaLabels={{ tableLabel: "Function versions" }}
              header={
                <Header
                  variant="h3"
                  actions={<AwsButton onClick={() => setPublishing(true)}>Publish new version</AwsButton>}
                >
                  Versions
                </Header>
              }
              empty={<AwsEmptyState title="No versions" description="This function has no published versions." />}
              columnDefinitions={[
                { id: "version", header: "Version", cell: (version) => version.version },
                { id: "runtime", header: "Runtime", cell: (version) => version.runtime || "–" },
                { id: "size", header: "Code size", cell: (version) => formatBytes(version.codeSize) },
                { id: "modified", header: "Last modified", cell: (version) => formatTimestamp(version.lastModified) },
              ]}
            />
          ),
        },
        {
          id: "event-sources",
          label: `Event source mappings (${eventSources.data?.length ?? 0})`,
          content: (
            <Table
              variant="embedded"
              loading={eventSources.isLoading}
              loadingText="Loading event source mappings"
              items={eventSources.data ?? []}
              ariaLabels={{ tableLabel: "Event source mappings" }}
              header={<Header variant="h3">Event source mappings</Header>}
              empty={<AwsEmptyState title="No event source mappings" />}
              columnDefinitions={[
                { id: "source", header: "Event source", cell: (mapping) => mapping.eventSourceArn },
                { id: "state", header: "State", cell: (mapping) => <AwsStatus status={mapping.state} /> },
                { id: "batch", header: "Batch size", cell: (mapping) => mapping.batchSize },
                { id: "uuid", header: "UUID", cell: (mapping) => mapping.uuid },
              ]}
            />
          ),
        },
        {
          id: "function-url",
          label: "Function URL",
          content: (
            <SpaceBetween size="m">
              {removeFunctionUrl.isError && (
                <AwsErrorAlert>
                  {removeFunctionUrl.error instanceof Error
                    ? removeFunctionUrl.error.message
                    : "Could not delete the function URL."}
                </AwsErrorAlert>
              )}
              <Table
                variant="embedded"
                loading={functionUrls.isLoading}
                loadingText="Loading function URLs"
                items={functionUrls.data ?? []}
                ariaLabels={{ tableLabel: "Function URL configurations" }}
                header={
                  <Header
                    variant="h3"
                    actions={
                      functionUrls.data?.length ? (
                        <AwsButton disabled={removeFunctionUrl.isPending} onClick={() => removeFunctionUrl.mutate()}>
                          Delete
                        </AwsButton>
                      ) : (
                        <AwsButton onClick={() => setCreatingFunctionUrl(true)}>Create function URL</AwsButton>
                      )
                    }
                  >
                    Function URL
                  </Header>
                }
                empty={<AwsEmptyState title="No function URL" description="This function has no function URL configuration." />}
                columnDefinitions={[
                  { id: "url", header: "Function URL", cell: (url) => url.functionUrl },
                  { id: "auth", header: "Auth type", cell: (url) => url.authType },
                  { id: "mode", header: "Invoke mode", cell: (url) => url.invokeMode || "BUFFERED" },
                  { id: "modified", header: "Last modified", cell: (url) => formatTimestamp(url.lastModifiedTime) },
                ]}
              />
            </SpaceBetween>
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
            <AwsButton disabled={!config} onClick={() => setPublishing(true)}>
              Publish version
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
      {publishing && config && <PublishVersionModal name={name} onClose={() => setPublishing(false)} />}
      {creatingAlias && config && (
        <CreateAliasModal name={name} versions={versions.data ?? []} onClose={() => setCreatingAlias(false)} />
      )}
      {editingConcurrency && config && (
        <ConcurrencyModal
          name={name}
          current={concurrency.data?.reservedConcurrentExecutions}
          onClose={() => setEditingConcurrency(false)}
        />
      )}
      {creatingFunctionUrl && config && (
        <FunctionUrlModal name={name} onClose={() => setCreatingFunctionUrl(false)} />
      )}
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
