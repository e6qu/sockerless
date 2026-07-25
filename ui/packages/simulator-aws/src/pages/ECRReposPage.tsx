import { useState } from "react";
import { NavLink, useNavigate } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AwsButton, AwsModal, AwsResourceTable, type AwsColumn } from "../console/index.js";
import { formatEpoch } from "../console/format.js";
import { deleteECRRepo, fetchECRRepos, type ECRRepo } from "../api.js";

// Amazon Elastic Container Registry — Repositories. The list and Delete both
// go through the real ECR awsjson1.1 API (DescribeRepositories for the
// table, DeleteRepository for the header action) with the operator's
// federated credentials.

const columns: AwsColumn<ECRRepo>[] = [
  {
    id: "name",
    header: "Repository name",
    cell: (row) => <NavLink to={`/ui/ecr/${encodeURIComponent(row.name)}`}>{row.name}</NavLink>,
    value: (row) => row.name,
  },
  { id: "uri", header: "URI", cell: (row) => row.uri, value: (row) => row.uri },
  { id: "createdAt", header: "Created at", cell: (row) => formatEpoch(row.createdAt), value: (row) => String(row.createdAt) },
];

export function DeleteReposModal({
  repos,
  onClose,
  clearSelection,
}: {
  repos: ECRRepo[];
  onClose: () => void;
  clearSelection: () => void;
}) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    // DeleteRepository is per-repository on the wire; a failure part-way
    // surfaces as the real API error, with the already-deleted repositories
    // gone from the refreshed list.
    mutationFn: async () => {
      for (const repo of repos) {
        await deleteECRRepo(repo.name);
      }
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["ecr-repos"] }),
    onSuccess: () => {
      clearSelection();
      onClose();
    },
  });
  return (
    <AwsModal
      title={repos.length === 1 ? `Delete ${repos[0].name}?` : `Delete ${repos.length} repositories?`}
      onDismiss={onClose}
      footer={
        <>
          <AwsButton onClick={onClose}>Cancel</AwsButton>
          <AwsButton
            variant="primary"
            data-testid="ecr-delete-repo-confirm"
            disabled={remove.isPending}
            onClick={() => remove.mutate()}
          >
            {remove.isPending ? "Deleting…" : "Delete"}
          </AwsButton>
        </>
      }
    >
      <p>Deleting a repository is permanent and deletes every image it holds.</p>
      <ul>
        {repos.map((repo) => (
          <li key={repo.name}>
            <code>{repo.name}</code>
          </li>
        ))}
      </ul>
      {remove.isError && (
        <div className="aws-flash aws-flash-error" role="alert">
          <strong>Could not delete.</strong>{" "}
          {remove.error instanceof Error ? remove.error.message : "The request failed."}
        </div>
      )}
    </AwsModal>
  );
}

export function ECRReposPage() {
  const navigate = useNavigate();
  const [deleting, setDeleting] = useState<{ repos: ECRRepo[]; clearSelection: () => void } | null>(null);
  return (
    <>
      <AwsResourceTable<ECRRepo>
        title="Repositories"
        description="Private repositories in this account and Region."
        columns={columns}
        queryKey={["ecr-repos"]}
        queryFn={fetchECRRepos}
        filterPlaceholder="Find repositories"
        emptyTitle="No repositories"
        emptyDescription="No private repositories exist in this account and Region."
        rowKey={(row) => row.name}
        tableTestId="ecr-repos-table"
        rowTestId={(row) => `ecr-repo-row-${row.name}`}
        actions={({ selected, clearSelection, refetch, isFetching }) => (
          <>
            <AwsButton
              data-testid="ecr-view-repo"
              disabled={selected.length !== 1}
              onClick={() => navigate(`/ui/ecr/${encodeURIComponent(selected[0].name)}`)}
            >
              View details
            </AwsButton>
            <AwsButton
              data-testid="ecr-delete-repo"
              disabled={selected.length === 0}
              onClick={() => setDeleting({ repos: selected, clearSelection })}
            >
              Delete
            </AwsButton>
            <AwsButton onClick={refetch} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
          </>
        )}
      />
      {deleting && (
        <DeleteReposModal repos={deleting.repos} clearSelection={deleting.clearSelection} onClose={() => setDeleting(null)} />
      )}
    </>
  );
}
