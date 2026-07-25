import { useState } from "react";
import { useNavigate, useParams } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AwsButton,
  AwsContainer,
  AwsCopyButton,
  AwsPageHeader,
  AwsResourceTable,
  type AwsColumn,
} from "../console/index.js";
import { formatBytes, formatEpoch } from "../console/format.js";
import { fetchECRImages, fetchECRRepo, type ECRImage, type ECRRepo } from "../api.js";
import { DeleteReposModal } from "./ECRReposPage.js";

// Amazon Elastic Container Registry — Repository detail. Reads the real ECR
// awsjson1.1 API (DescribeRepositories filtered to one name, since there is
// no singular DescribeRepository operation, and DescribeImages for the image
// list) with the operator's federated credentials, and reuses the
// Repositories list page's real DeleteRepository action. Cloudscape's own
// guidance is not to introduce tabs when the content already groups into one
// meaningful section, so this page is a properties block plus the image
// sub-resource table — the same shape as the IAM user's Security credentials
// page.

const imageColumns: AwsColumn<ECRImage>[] = [
  {
    id: "tags",
    header: "Image tag",
    cell: (row) => (row.tags.length > 0 ? row.tags.join(", ") : <em>untagged</em>),
    value: (row) => row.tags.join(" "),
  },
  { id: "digest", header: "Digest", cell: (row) => <code>{row.digest}</code>, value: (row) => row.digest },
  { id: "pushedAt", header: "Pushed at", cell: (row) => formatEpoch(row.pushedAt), value: (row) => String(row.pushedAt) },
  { id: "size", header: "Size", cell: (row) => formatBytes(row.sizeBytes), value: (row) => String(row.sizeBytes) },
];

export function ECRRepositoryDetailPage() {
  const { name = "" } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [deleting, setDeleting] = useState(false);
  const repo = useQuery({ queryKey: ["ecr-repo", name], queryFn: () => fetchECRRepo(name) });

  const asECRRepo: ECRRepo | null = repo.data ?? null;

  return (
    <>
      <AwsPageHeader
        title={name}
        description="Repository in Amazon Elastic Container Registry."
        actions={
          <AwsButton data-testid="ecr-repo-delete" disabled={!repo.isSuccess} onClick={() => setDeleting(true)}>
            Delete
          </AwsButton>
        }
      />
      <AwsContainer>
        {repo.isError ? (
          <div className="aws-flash aws-flash-error" role="alert" data-testid="ecr-repo-error">
            <strong>Could not load the repository.</strong>{" "}
            {repo.error instanceof Error ? repo.error.message : "The request failed."}
          </div>
        ) : repo.isLoading ? (
          <div className="aws-empty" role="status">
            Loading repository…
          </div>
        ) : repo.data ? (
          <dl className="aws-key-value" data-testid="ecr-repo-summary">
            <dt>URI</dt>
            <dd>
              <code>{repo.data.uri}</code>
              <AwsCopyButton value={repo.data.uri} label="Copy repository URI" />
            </dd>
            <dt>Created at</dt>
            <dd>{formatEpoch(repo.data.createdAt)}</dd>
          </dl>
        ) : null}
      </AwsContainer>
      <AwsResourceTable<ECRImage>
        title="Images"
        description="Images pushed to this repository."
        columns={imageColumns}
        queryKey={["ecr-images", name]}
        queryFn={() => fetchECRImages(name)}
        filterPlaceholder="Find images"
        emptyTitle="No images"
        emptyDescription="No images have been pushed to this repository."
        rowKey={(row) => row.digest}
        tableTestId="ecr-images-table"
        rowTestId={(row) => `ecr-image-row-${row.digest}`}
        actions={({ refetch, isFetching }) => (
          <AwsButton onClick={refetch} disabled={isFetching}>
            {isFetching ? "Refreshing…" : "Refresh"}
          </AwsButton>
        )}
      />
      {deleting && asECRRepo && (
        <DeleteReposModal
          repos={[asECRRepo]}
          clearSelection={() => navigate("/ui/ecr")}
          onClose={() => {
            setDeleting(false);
            void queryClient.invalidateQueries({ queryKey: ["ecr-repo", name] });
          }}
        />
      )}
    </>
  );
}
