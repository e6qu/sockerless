import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import FormField from "@cloudscape-design/components/form-field";
import Input from "@cloudscape-design/components/input";
import Select from "@cloudscape-design/components/select";
import SpaceBetween from "@cloudscape-design/components/space-between";
import Textarea from "@cloudscape-design/components/textarea";
import { AwsButton, AwsErrorAlert, AwsModal, AwsResourceTable, type AwsColumn } from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import { createAmplifyApp, fetchAmplifyApps, type AmplifyApp } from "../api.js";

// AWS Amplify — Apps. ListApps on the real Amplify REST-JSON API (GET /apps).

const columns: AwsColumn<AmplifyApp>[] = [
  { id: "name", header: "App name", cell: (row) => row.name, value: (row) => row.name },
  { id: "appId", header: "App ID", cell: (row) => row.appId, value: (row) => row.appId },
  { id: "platform", header: "Platform", cell: (row) => row.platform || "–", value: (row) => row.platform },
  {
    id: "defaultDomain",
    header: "Default domain",
    cell: (row) => row.defaultDomain || "–",
    value: (row) => row.defaultDomain,
  },
  { id: "repository", header: "Repository", cell: (row) => row.repository || "–", value: (row) => row.repository },
  {
    id: "repositoryCloneMethod",
    header: "Repository access",
    cell: (row) => row.repositoryCloneMethod || "Manual deploy",
    value: (row) => row.repositoryCloneMethod,
  },
  {
    id: "createTime",
    header: "Created",
    cell: (row) => formatEpoch(row.createTime),
    value: (row) => String(row.createTime),
  },
];

function CreateAmplifyAppModal({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [repository, setRepository] = useState("");
  const [accessToken, setAccessToken] = useState("");
  const [platform, setPlatform] = useState<"WEB" | "WEB_COMPUTE">("WEB");
  const [buildSpec, setBuildSpec] = useState("");
  const valid = name.trim().length > 0 && (!repository || /^https?:\/\//.test(repository));
  const create = useMutation({
    mutationFn: () => createAmplifyApp({ name, repository, accessToken, platform, buildSpec }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["amplify-apps"] });
      onClose();
    },
  });
  return (
    <AwsModal
      title="Create Amplify app"
      size="max"
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="amplify-create-app-submit"
            disabled={!valid || create.isPending}
            onClick={() => create.mutate()}
          >
            {create.isPending ? "Creating…" : "Create app"}
          </AwsButton>
        </>
      }
    >
      <SpaceBetween size="l">
        <FormField label="App name">
          <Input value={name} onChange={(event) => setName(event.detail.value)} />
        </FormField>
        <FormField
          label="Repository URL"
          description="An HTTPS Git repository. Leave blank when the app will use manual deployments."
        >
          <Input value={repository} onChange={(event) => setRepository(event.detail.value)} placeholder="https://github.com/example/site" />
        </FormField>
        <FormField
          label="Repository access token"
          description="Used to establish a private repository connection. The token is write-only and is not returned by Amplify."
        >
          <Input value={accessToken} type="password" onChange={(event) => setAccessToken(event.detail.value)} />
        </FormField>
        <FormField label="Platform">
          <Select
            selectedOption={{ label: platform === "WEB" ? "Web" : "Web compute", value: platform }}
            options={[
              { label: "Web", value: "WEB", description: "Static web hosting" },
              { label: "Web compute", value: "WEB_COMPUTE", description: "Server-side rendered hosting" },
            ]}
            onChange={(event) => setPlatform(event.detail.selectedOption.value as "WEB" | "WEB_COMPUTE")}
          />
        </FormField>
        <FormField
          label="Build specification"
          description="Optional. Leave blank to use amplify.yml from the repository. The managed build image includes Node.js and Python."
        >
          <Textarea
            value={buildSpec}
            onChange={(event) => setBuildSpec(event.detail.value)}
            rows={16}
            spellcheck={false}
            ariaLabel="AWS Amplify build specification"
          />
        </FormField>
        {create.isError && (
          <AwsErrorAlert>
            <strong>Could not create the app.</strong>{" "}
            {create.error instanceof Error ? create.error.message : "The request failed."}
          </AwsErrorAlert>
        )}
      </SpaceBetween>
    </AwsModal>
  );
}

export function AmplifyPage() {
  const [creating, setCreating] = useState(false);
  return (
    <>
      <AwsResourceTable<AmplifyApp>
        title="Apps"
        description="AWS Amplify apps in this account and Region."
        columns={columns}
        queryKey={["amplify-apps"]}
        queryFn={fetchAmplifyApps}
        filterPlaceholder="Find apps"
        emptyTitle="No apps"
        emptyDescription="No AWS Amplify apps exist in this account and Region."
        rowKey={(row) => row.appId}
        tableTestId="amplify-table"
        errorTestId="amplify-error"
        actions={({ refetch, isFetching }) => (
          <>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
            <AwsButton variant="primary" data-testid="amplify-create-app" onClick={() => setCreating(true)}>
              Create app
            </AwsButton>
          </>
        )}
      />
      {creating && <CreateAmplifyAppModal onClose={() => setCreating(false)} />}
    </>
  );
}
