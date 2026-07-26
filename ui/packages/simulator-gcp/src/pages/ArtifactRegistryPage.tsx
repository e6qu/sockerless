import { useState } from "react";
import { Link } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { GcpResourceTable, type GcpColumn } from "../console/index.js";
import { GcpDialog } from "../console/GcpDialog.js";
import { shortName, formatTimestamp } from "../console/format.js";
import { CONSOLE_REGION, createARRepository, fetchARRepos, waitArOperation, type ARRepo } from "../api.js";
import { useProject } from "../console/project.js";

const columns: GcpColumn<ARRepo>[] = [
  {
    id: "name",
    header: "Name",
    cell: (row) => <Link className="gc-cell-link" to={`/ui/ar/${shortName(row.name)}`}>{shortName(row.name)}</Link>,
    value: (row) => shortName(row.name),
  },
  { id: "format", header: "Format", cell: (row) => row.format ?? "—", value: (row) => row.format ?? "" },
  { id: "mode", header: "Mode", cell: (row) => row.mode ?? "Standard", value: (row) => row.mode ?? "" },
  { id: "createTime", header: "Created", cell: (row) => formatTimestamp(row.createTime ?? ""), value: (row) => row.createTime ?? "" },
];

// Artifact Registry's repository-ID contract: up to 63 characters of
// lowercase letters, numbers and hyphens, starting with a letter — checked
// before submitting so the dialog explains the rule the real Create
// repository page does.
const REPO_ID_PATTERN = /^[a-z][a-z0-9-]{0,62}$/;

const FORMATS = ["DOCKER", "MAVEN", "NPM", "PYTHON", "APT", "YUM", "GO", "GENERIC"] as const;

export function CreateRepoDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: () => void;
}) {
  const { project } = useProject();
  const [repositoryId, setRepositoryId] = useState("");
  const [format, setFormat] = useState<(typeof FORMATS)[number]>("DOCKER");

  const create = useMutation({
    mutationFn: async () =>
      waitArOperation(await createARRepository(project, CONSOLE_REGION, repositoryId, format)),
    onSuccess: onCreated,
  });

  const valid = REPO_ID_PATTERN.test(repositoryId);

  return (
    <GcpDialog title="Create repository" testId="ar-create-dialog" onClose={onClose}>
      <label className="gc-field">
        Name
        <input
          type="text"
          value={repositoryId}
          data-testid="ar-create-id"
          onChange={(event) => setRepositoryId(event.target.value)}
        />
        <p className="gc-field-hint">
          Up to 63 lowercase letters, numbers or hyphens; must start with a letter.
        </p>
      </label>
      <label className="gc-field">
        Format
        <select
          value={format}
          data-testid="ar-create-format"
          onChange={(event) => setFormat(event.target.value as (typeof FORMATS)[number])}
        >
          {FORMATS.map((candidate) => (
            <option key={candidate} value={candidate}>
              {candidate}
            </option>
          ))}
        </select>
      </label>
      {create.isError ? (
        <div className="gc-message gc-message-error" role="alert">
          <strong>Couldn't create the repository.</strong>{" "}
          {create.error instanceof Error ? create.error.message : "The API did not respond."}
        </div>
      ) : null}
      <div className="gc-dialog-actions">
        <button type="button" className="gc-button-text" onClick={onClose}>Cancel</button>
        <button
          type="button"
          className="gc-button-primary"
          data-testid="ar-create-submit"
          disabled={!valid || create.isPending}
          onClick={() => create.mutate()}
        >
          {create.isPending ? "Creating…" : "Create"}
        </button>
      </div>
    </GcpDialog>
  );
}

export function ArtifactRegistryPage() {
  const { project } = useProject();
  const queryClient = useQueryClient();
  const [creating, setCreating] = useState(false);

  return (
    <>
      <GcpResourceTable<ARRepo>
        title="Artifact Registry"
        description="Store and manage your build artifacts — container images and language packages — in one place."
        actions={[
          { label: "Create repository", icon: "add", primary: true, testId: "ar-create-repo", onSelect: () => setCreating(true) },
        ]}
        columns={columns}
        queryKey={["ar-repos-real", project]}
        queryFn={() => fetchARRepos(project)}
        filterPlaceholder="Filter repositories"
        resourceNoun="repositories"
        empty={{
          headline: "Store your build artifacts",
          description: "Create a repository to store container images and language packages.",
          primaryLabel: "Create repository",
          onPrimary: () => setCreating(true),
        }}
        rowKey={(row) => row.name}
      />
      {creating ? (
        <CreateRepoDialog
          onClose={() => setCreating(false)}
          onCreated={() => {
            setCreating(false);
            void queryClient.invalidateQueries({ queryKey: ["ar-repos-real", project] });
          }}
        />
      ) : null}
    </>
  );
}
