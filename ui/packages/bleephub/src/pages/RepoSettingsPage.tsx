import { useEffect, useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import { fetchRepoDetail, updateRepo, fetchRepoTopics, updateRepoTopics } from "../api.js";
import type { BleephubRepo } from "../types.js";
import { RepoHeader } from "../components/Shell.js";
import { PageTitle, Button, Box } from "../components/ui.js";

export function RepoSettingsPage() {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  const queryClient = useQueryClient();

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["repo", owner, repo],
    queryFn: () => fetchRepoDetail(owner, repo),
    enabled: !!owner && !!repo,
  });

  const mutation = useMutation({
    mutationFn: (payload: Partial<BleephubRepo>) => updateRepo(owner, repo, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["repo", owner, repo] });
    },
  });

  const topicsQuery = useQuery({
    queryKey: ["repo-topics", owner, repo],
    queryFn: () => fetchRepoTopics(owner, repo),
    enabled: !!owner && !!repo,
  });

  const topicsMutation = useMutation({
    mutationFn: (names: string[]) => updateRepoTopics(owner, repo, names),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["repo-topics", owner, repo] });
      queryClient.invalidateQueries({ queryKey: ["repo", owner, repo] });
    },
  });

  if (isLoading) return <Spinner label={`loading ${owner}/${repo}`} />;
  if (isError || !data)
    return <InlineError title={`Failed to load ${owner}/${repo}`} detail={String(error)} />;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="settings" />
      <PageTitle title="General" />
      <RepoSettingsForm repo={data} onSave={(payload) => mutation.mutate(payload)} />
      <BranchProtectionCard owner={owner} repo={repo} />
      <RepoTopicsForm
        topics={topicsQuery.data?.names ?? []}
        isLoading={topicsQuery.isLoading}
        onSave={(names) => topicsMutation.mutate(names)}
      />
      {mutation.isError && (
        <div className="mt-4" style={{ color: "var(--color-danger-fg)" }}>
          {mutation.error instanceof Error ? mutation.error.message : String(mutation.error)}
        </div>
      )}
      {mutation.isSuccess && (
        <div className="mt-4" style={{ color: "var(--gh-open)" }}>Settings saved.</div>
      )}
      {topicsMutation.isError && (
        <div className="mt-4" style={{ color: "var(--color-danger-fg)" }}>
          {topicsMutation.error instanceof Error ? topicsMutation.error.message : String(topicsMutation.error)}
        </div>
      )}
      {topicsMutation.isSuccess && (
        <div className="mt-4" style={{ color: "var(--gh-open)" }}>Topics saved.</div>
      )}
    </div>
  );
}

function RepoTopicsForm({
  topics,
  isLoading,
  onSave,
}: {
  topics: string[];
  isLoading: boolean;
  onSave: (names: string[]) => void;
}) {
  const [value, setValue] = useState(topics.join(", "));
  useEffect(() => {
    setValue(topics.join(", "));
  }, [topics]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const names = value
      .split(",")
      .map((t) => t.trim())
      .filter((t) => t.length > 0 && t.length <= 50 && !t.includes(" ") && !t.includes("/") && !t.includes("\\") && !t.includes(":"));
    onSave(names.slice(0, 20));
  };

  return (
    <form onSubmit={handleSubmit} className="mt-4">
      <Box header={<span style={{ fontWeight: 600 }}>Topics</span>}>
        <div style={{ display: "flex", flexDirection: "column", gap: "1rem", padding: "1rem" }}>
          <label style={{ display: "flex", flexDirection: "column", gap: "0.25rem" }}>
            <span style={{ fontSize: "0.85rem", fontWeight: 500 }}>Topics (comma separated)</span>
            <input
              type="text"
              value={value}
              disabled={isLoading}
              onChange={(e) => setValue(e.target.value)}
              placeholder="e.g. go, ci, bleephub"
              style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
            />
            <span style={{ fontSize: "0.75rem", color: "var(--color-fg-muted)" }}>
              Up to 20 topics, max 50 chars, no spaces or / \ :.
            </span>
          </label>
          <div className="flex justify-end">
            <Button type="submit" variant="primary">Save topics</Button>
          </div>
        </div>
      </Box>
    </form>
  );
}

function RepoSettingsForm({
  repo,
  onSave,
}: {
  repo: BleephubRepo;
  onSave: (payload: Partial<BleephubRepo>) => void;
}) {
  const [description, setDescription] = useState(repo.description ?? "");
  const [homepage, setHomepage] = useState(repo.homepage ?? "");
  const [defaultBranch, setDefaultBranch] = useState(repo.default_branch);
  const [private_, setPrivate] = useState(repo.private);
  const [hasIssues, setHasIssues] = useState(repo.has_issues);
  const [hasProjects, setHasProjects] = useState(repo.has_projects);
  const [hasWiki, setHasWiki] = useState(repo.has_wiki);
  const [hasPullRequests, setHasPullRequests] = useState(repo.has_pull_requests);
  const [allowSquashMerge, setAllowSquashMerge] = useState(repo.allow_squash_merge);
  const [allowMergeCommit, setAllowMergeCommit] = useState(repo.allow_merge_commit);
  const [allowRebaseMerge, setAllowRebaseMerge] = useState(repo.allow_rebase_merge);
  const [allowAutoMerge, setAllowAutoMerge] = useState(repo.allow_auto_merge);
  const [deleteBranchOnMerge, setDeleteBranchOnMerge] = useState(repo.delete_branch_on_merge);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSave({
      description: description.trim(),
      homepage: homepage.trim() || null,
      default_branch: defaultBranch.trim(),
      private: private_,
      visibility: private_ ? "private" : "public",
      has_issues: hasIssues,
      has_projects: hasProjects,
      has_wiki: hasWiki,
      has_pull_requests: hasPullRequests,
      allow_squash_merge: allowSquashMerge,
      allow_merge_commit: allowMergeCommit,
      allow_rebase_merge: allowRebaseMerge,
      allow_auto_merge: allowAutoMerge,
      delete_branch_on_merge: deleteBranchOnMerge,
    });
  };

  return (
    <form onSubmit={handleSubmit}>
      <Box
        header={<span style={{ fontWeight: 600 }}>Repository settings</span>}
      >
        <div style={{ display: "flex", flexDirection: "column", gap: "1rem", padding: "1rem" }}>
          <label style={{ display: "flex", flexDirection: "column", gap: "0.25rem" }}>
            <span style={{ fontSize: "0.85rem", fontWeight: 500 }}>Description</span>
            <input
              type="text"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Short description of this repository"
              style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
            />
          </label>

          <label style={{ display: "flex", flexDirection: "column", gap: "0.25rem" }}>
            <span style={{ fontSize: "0.85rem", fontWeight: 500 }}>Website</span>
            <input
              type="text"
              value={homepage}
              onChange={(e) => setHomepage(e.target.value)}
              placeholder="https://example.com"
              style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
            />
          </label>

          <label style={{ display: "flex", flexDirection: "column", gap: "0.25rem" }}>
            <span style={{ fontSize: "0.85rem", fontWeight: 500 }}>Default branch</span>
            <input
              type="text"
              value={defaultBranch}
              onChange={(e) => setDefaultBranch(e.target.value)}
              style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
            />
          </label>

          <fieldset style={{ border: "none", padding: 0, margin: 0, display: "flex", gap: "1rem" }}>
            <legend style={{ fontSize: "0.85rem", fontWeight: 500, marginBottom: "0.5rem" }}>Visibility</legend>
            <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
              <input
                type="radio"
                name="visibility"
                checked={!private_}
                onChange={() => setPrivate(false)}
              />
              Public
            </label>
            <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
              <input
                type="radio"
                name="visibility"
                checked={private_}
                onChange={() => setPrivate(true)}
              />
              Private
            </label>
          </fieldset>

          <fieldset style={{ border: "none", padding: 0, margin: 0 }}>
            <legend style={{ fontSize: "0.85rem", fontWeight: 500, marginBottom: "0.5rem" }}>Features</legend>
            <div style={{ display: "flex", flexDirection: "column", gap: "0.4rem" }}>
              <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
                <input type="checkbox" checked={hasIssues} onChange={(e) => setHasIssues(e.target.checked)} />
                Issues
              </label>
              <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
                <input type="checkbox" checked={hasProjects} onChange={(e) => setHasProjects(e.target.checked)} />
                Projects
              </label>
              <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
                <input type="checkbox" checked={hasWiki} onChange={(e) => setHasWiki(e.target.checked)} />
                Wiki
              </label>
              <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
                <input type="checkbox" checked={hasPullRequests} onChange={(e) => setHasPullRequests(e.target.checked)} />
                Pull requests
              </label>
            </div>
          </fieldset>

          <fieldset style={{ border: "none", padding: 0, margin: 0 }}>
            <legend style={{ fontSize: "0.85rem", fontWeight: 500, marginBottom: "0.5rem" }}>Merge button</legend>
            <div style={{ display: "flex", flexDirection: "column", gap: "0.4rem" }}>
              <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
                <input type="checkbox" checked={allowSquashMerge} onChange={(e) => setAllowSquashMerge(e.target.checked)} />
                Allow squash merging
              </label>
              <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
                <input type="checkbox" checked={allowMergeCommit} onChange={(e) => setAllowMergeCommit(e.target.checked)} />
                Allow merge commits
              </label>
              <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
                <input type="checkbox" checked={allowRebaseMerge} onChange={(e) => setAllowRebaseMerge(e.target.checked)} />
                Allow rebase merging
              </label>
              <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
                <input type="checkbox" checked={allowAutoMerge} onChange={(e) => setAllowAutoMerge(e.target.checked)} />
                Allow auto-merge
              </label>
              <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
                <input type="checkbox" checked={deleteBranchOnMerge} onChange={(e) => setDeleteBranchOnMerge(e.target.checked)} />
                Automatically delete head branches
              </label>
            </div>
          </fieldset>

          <div className="flex justify-end">
            <Button type="submit" variant="primary">Save changes</Button>
          </div>
        </div>
      </Box>
    </form>
  );
}

function BranchProtectionCard({ owner, repo }: { owner: string; repo: string }) {
  return (
    <Box header={<span style={{ fontWeight: 600 }}>Branch protection</span>} className="mt-4">
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "1rem" }}>
        <span style={{ fontSize: "0.9rem" }}>Define merge constraints and required status checks.</span>
        <Link to={`/ui/repos/${owner}/${repo}/settings/branch-protection`}>
          <Button variant="secondary" size="sm">Manage branch protection</Button>
        </Link>
      </div>
    </Box>
  );
}
