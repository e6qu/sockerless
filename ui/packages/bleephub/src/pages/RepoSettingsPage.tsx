import { useEffect, useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  addRepoDeployKey,
  createPagesSite,
  deletePagesSite,
  cancelRepoInvitation,
  deleteRepoDeployKey,
  fetchPagesBuilds,
  fetchPagesDeploymentStatus,
  fetchPagesHealth,
  fetchPagesSite,
  fetchRepoBranches,
  fetchRepoCollaborators,
  fetchRepoDeployKeys,
  fetchRepoDetail,
  fetchRepoInvitations,
  fetchRepoTopics,
  inviteRepoCollaborator,
  removeRepoCollaborator,
  renameBranch,
  requestPagesBuild,
  setRepoFlag,
  setRepoInteractionLimit,
  transferRepo,
  updatePagesSite,
  updateRepo,
  updateRepoTopics,
} from "../api.js";
import type {
  BleephubRepo,
  GithubCollaborator,
  GithubDeployKey,
  GithubPagesBuild,
  GithubPagesSite,
  GithubRepoInvitation,
} from "../types.js";
import { RepoHeader } from "../components/Shell.js";
import { PageTitle, Button, Box, Tabs, FormLabel, ErrorBanner } from "../components/ui.js";

type SettingsTab = "general" | "collaborators" | "deploy-keys" | "pages" | "security" | "interaction" | "transfer" | "rename";

export function RepoSettingsPage() {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  const [tab, setTab] = useState<SettingsTab>("general");

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["repo", owner, repo],
    queryFn: () => fetchRepoDetail(owner, repo),
    enabled: !!owner && !!repo,
  });

  if (isLoading) return <Spinner label={`loading ${owner}/${repo}`} />;
  if (isError || !data)
    return <InlineError title={`Failed to load ${owner}/${repo}`} detail={String(error)} />;

  const tabs = [
    { key: "general" as const, label: "General" },
    { key: "collaborators" as const, label: "Collaborators" },
    { key: "deploy-keys" as const, label: "Deploy keys" },
    { key: "pages" as const, label: "Pages" },
    { key: "security" as const, label: "Security" },
    { key: "interaction" as const, label: "Interaction limits" },
    { key: "transfer" as const, label: "Transfer" },
    { key: "rename" as const, label: "Rename branch" },
  ];

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="settings" />
      <PageTitle title="Settings" />
      <Tabs items={tabs} active={tab} onChange={setTab} />
      {tab === "general" && <GeneralSettingsTab owner={owner} repo={repo} repoData={data} />}
      {tab === "collaborators" && <CollaboratorsTab owner={owner} repo={repo} />}
      {tab === "deploy-keys" && <DeployKeysTab owner={owner} repo={repo} />}
      {tab === "pages" && <PagesTab owner={owner} repo={repo} />}
      {tab === "security" && <SecurityTab owner={owner} repo={repo} />}
      {tab === "interaction" && <InteractionTab owner={owner} repo={repo} />}
      {tab === "transfer" && <TransferTab owner={owner} repo={repo} />}
      {tab === "rename" && <RenameBranchTab owner={owner} repo={repo} />}
    </div>
  );
}

function GeneralSettingsTab({ owner, repo, repoData }: { owner: string; repo: string; repoData: BleephubRepo }) {
  const queryClient = useQueryClient();

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

  return (
    <>
      <RepoSettingsForm repo={repoData} onSave={(payload) => mutation.mutate(payload)} />
      <BranchProtectionCard owner={owner} repo={repo} />
      <SecretsAndVariablesCard owner={owner} repo={repo} />
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
    </>
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
        header={<span style={{ fontWeight: 600 }}>Repository settings</span>
        }
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

function SecretsAndVariablesCard({ owner, repo }: { owner: string; repo: string }) {
  return (
    <Box header={<span style={{ fontWeight: 600 }}>Secrets and variables</span>} className="mt-4">
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "1rem" }}>
        <span style={{ fontSize: "0.9rem" }}>Manage Actions secrets and variables across repository, environment, and organization scopes.</span>
        <Link to={`/ui/repos/${owner}/${repo}/settings/secrets`}>
          <Button variant="secondary" size="sm">Manage secrets and variables</Button>
        </Link>
      </div>
    </Box>
  );
}

function CollaboratorsTab({ owner, repo }: { owner: string; repo: string }) {
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [username, setUsername] = useState("");
  const [permission, setPermission] = useState("push");

  const collaboratorsQuery = useQuery({
    queryKey: ["repo-collaborators", owner, repo],
    queryFn: () => fetchRepoCollaborators(owner, repo),
    enabled: !!owner && !!repo,
  });
  const invitationsQuery = useQuery({
    queryKey: ["repo-invitations", owner, repo],
    queryFn: () => fetchRepoInvitations(owner, repo),
    enabled: !!owner && !!repo,
  });

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["repo-collaborators", owner, repo] });
    queryClient.invalidateQueries({ queryKey: ["repo-invitations", owner, repo] });
  };

  const inviteMut = useMutation({
    mutationFn: () => inviteRepoCollaborator(owner, repo, username.trim(), permission),
    onSuccess: (invitation) => {
      setError(null);
      setNotice(
        invitation
          ? `Invitation sent to ${invitation.invitee?.login ?? username.trim()}.`
          : `Updated ${username.trim()}'s permission.`,
      );
      setUsername("");
      invalidate();
    },
    onError: (err: Error) => {
      setNotice(null);
      setError(err.message);
    },
  });

  const removeMut = useMutation({
    mutationFn: (login: string) => removeRepoCollaborator(owner, repo, login),
    onSuccess: () => {
      setError(null);
      setNotice(null);
      invalidate();
    },
    onError: (err: Error) => setError(err.message),
  });

  const cancelMut = useMutation({
    mutationFn: (invitationId: number) => cancelRepoInvitation(owner, repo, invitationId),
    onSuccess: () => {
      setError(null);
      setNotice(null);
      invalidate();
    },
    onError: (err: Error) => setError(err.message),
  });

  if (collaboratorsQuery.isLoading || invitationsQuery.isLoading)
    return <Spinner label="loading collaborators" />;
  if (collaboratorsQuery.isError)
    return <InlineError title="Failed to load collaborators" detail={String(collaboratorsQuery.error)} />;
  if (invitationsQuery.isError)
    return <InlineError title="Failed to load invitations" detail={String(invitationsQuery.error)} />;

  const collaborators = collaboratorsQuery.data ?? [];
  const invitations = invitationsQuery.data ?? [];

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      {notice && <div style={{ color: "var(--gh-open)", fontSize: "0.85rem" }}>{notice}</div>}
      <Box header={<span style={{ fontWeight: 600 }}>Invite a collaborator</span>}>
        <div style={{ padding: "1rem", display: "flex", flexWrap: "wrap", gap: "0.75rem", alignItems: "center" }}>
          <input
            type="text"
            aria-label="Username to invite"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="username"
            className="flex-1"
            style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem", minWidth: "12rem" }}
          />
          <select
            aria-label="Role"
            value={permission}
            onChange={(e) => setPermission(e.target.value)}
            style={{ fontSize: "0.9rem", padding: "0.4rem 0.5rem" }}
          >
            <option value="pull">Read</option>
            <option value="push">Write</option>
            <option value="admin">Admin</option>
          </select>
          <Button
            variant="primary"
            onClick={() => {
              setError(null);
              setNotice(null);
              inviteMut.mutate();
            }}
            disabled={inviteMut.isPending || !username.trim()}
          >
            Invite
          </Button>
        </div>
      </Box>
      <Box header={<span style={{ fontWeight: 600 }}>Collaborators</span>}>
        {collaborators.length === 0 ? (
          <div style={{ padding: "1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
            No collaborators.
          </div>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {collaborators.map((c: GithubCollaborator) => (
              <li
                key={c.login}
                className="flex items-center justify-between gap-4"
                style={{ padding: "0.6rem 1rem", borderBottom: "1px solid var(--color-border)" }}
              >
                <div>
                  <span style={{ fontWeight: 500 }}>{c.login}</span>
                  <span style={{ marginLeft: "0.5rem", fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
                    {c.role_name}
                  </span>
                </div>
                {c.login !== owner && (
                  <Button
                    size="sm"
                    variant="danger"
                    onClick={() => {
                      if (confirm(`Remove ${c.login} from ${owner}/${repo}?`)) {
                        removeMut.mutate(c.login);
                      }
                    }}
                    disabled={removeMut.isPending}
                  >
                    remove
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
      </Box>
      <Box header={<span style={{ fontWeight: 600 }}>Pending invitations</span>}>
        {invitations.length === 0 ? (
          <div style={{ padding: "1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
            No pending invitations.
          </div>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {invitations.map((inv: GithubRepoInvitation) => (
              <li
                key={inv.id}
                className="flex items-center justify-between gap-4"
                style={{ padding: "0.6rem 1rem", borderBottom: "1px solid var(--color-border)" }}
              >
                <div>
                  <span style={{ fontWeight: 500 }}>{inv.invitee?.login ?? "unknown"}</span>
                  <span style={{ marginLeft: "0.5rem", fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>
                    {inv.permissions} · invited by {inv.inviter?.login ?? "unknown"} on{" "}
                    {new Date(inv.created_at).toLocaleDateString()}
                  </span>
                </div>
                <Button
                  size="sm"
                  variant="danger"
                  onClick={() => cancelMut.mutate(inv.id)}
                  disabled={cancelMut.isPending}
                >
                  cancel
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Box>
    </div>
  );
}

function DeployKeysTab({ owner, repo }: { owner: string; repo: string }) {
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [title, setTitle] = useState("");
  const [key, setKey] = useState("");
  const [readOnly, setReadOnly] = useState(false);

  const query = useQuery({
    queryKey: ["repo-deploy-keys", owner, repo],
    queryFn: () => fetchRepoDeployKeys(owner, repo),
    enabled: !!owner && !!repo,
  });

  const addMut = useMutation({
    mutationFn: () => addRepoDeployKey(owner, repo, title.trim(), key.trim(), readOnly),
    onSuccess: () => {
      setError(null);
      setTitle("");
      setKey("");
      setReadOnly(false);
      queryClient.invalidateQueries({ queryKey: ["repo-deploy-keys", owner, repo] });
    },
    onError: (err: Error) => setError(err.message),
  });

  const deleteMut = useMutation({
    mutationFn: (keyId: number) => deleteRepoDeployKey(owner, repo, keyId),
    onSuccess: () => {
      setError(null);
      queryClient.invalidateQueries({ queryKey: ["repo-deploy-keys", owner, repo] });
    },
    onError: (err: Error) => setError(err.message),
  });

  if (query.isLoading) return <Spinner label="loading deploy keys" />;
  if (query.isError) return <InlineError title="Failed to load deploy keys" />;

  const keys = query.data ?? [];

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      {error && <ErrorBanner>{error}</ErrorBanner>}
      <Box header={<span style={{ fontWeight: 600 }}>Add deploy key</span>}>
        <div style={{ padding: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
          <FormLabel id="deploy-key-title">Title</FormLabel>
          <input
            id="deploy-key-title"
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            className="w-full"
          />
          <FormLabel id="deploy-key-key">Key</FormLabel>
          <textarea
            id="deploy-key-key"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            rows={4}
            className="w-full"
            style={{ fontFamily: "var(--font-mono)", fontSize: "0.8rem" }}
          />
          <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
            <input type="checkbox" checked={readOnly} onChange={(e) => setReadOnly(e.target.checked)} />
            Read-only
          </label>
          <div className="flex justify-end">
            <Button
              variant="primary"
              onClick={() => {
                setError(null);
                addMut.mutate();
              }}
              disabled={addMut.isPending || !title.trim() || !key.trim()}
            >
              Add key
            </Button>
          </div>
        </div>
      </Box>
      <Box header={<span style={{ fontWeight: 600 }}>Deploy keys</span>}>
        <div style={{ padding: "0" }}>
          {keys.length === 0 ? (
            <div style={{ padding: "1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
              No deploy keys.
            </div>
          ) : (
            <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
              {keys.map((k: GithubDeployKey) => (
                <li
                  key={k.id}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    padding: "0.6rem 1rem",
                    borderBottom: "1px solid var(--color-border)",
                    gap: "1rem",
                  }}
                >
                  <div style={{ minWidth: 0 }}>
                    <div style={{ fontWeight: 500 }}>{k.title}</div>
                    <div
                      style={{
                        color: "var(--color-fg-muted)",
                        fontSize: "0.8rem",
                        fontFamily: "var(--font-mono)",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                      }}
                    >
                      {k.key}
                    </div>
                    <div style={{ color: "var(--color-fg-muted)", fontSize: "0.75rem" }}>
                      {k.read_only ? "read-only" : "read/write"} · {k.verified ? "verified" : "unverified"}
                    </div>
                  </div>
                  <Button
                    size="sm"
                    variant="danger"
                    onClick={() => {
                      if (confirm(`Delete deploy key "${k.title}"?`)) {
                        deleteMut.mutate(k.id);
                      }
                    }}
                    disabled={deleteMut.isPending}
                  >
                    delete
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </Box>
    </div>
  );
}

function SecurityTab({ owner, repo }: { owner: string; repo: string }) {
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  type FlagKey = "automated_security_fixes" | "private_vulnerability_reporting" | "vulnerability_alerts";
  const [flags, setFlags] = useState<Record<FlagKey, boolean>>({
    automated_security_fixes: false,
    private_vulnerability_reporting: false,
    vulnerability_alerts: false,
  });

  const mutation = useMutation({
    mutationFn: ({ flag, enabled }: { flag: FlagKey; enabled: boolean }) => setRepoFlag(owner, repo, flag, enabled),
    onSuccess: (_, vars) => {
      setError(null);
      setSuccess(`Updated ${vars.flag.replace(/_/g, " ")}.`);
      setFlags((prev) => ({ ...prev, [vars.flag]: vars.enabled }));
    },
    onError: (err: Error) => {
      setSuccess(null);
      setError(err.message);
    },
  });

  const toggle = (flag: FlagKey) => {
    const enabled = !flags[flag];
    setError(null);
    setSuccess(null);
    mutation.mutate({ flag, enabled });
  };

  return (
    <Box header={<span style={{ fontWeight: 600 }}>Security settings</span>}>
      <div style={{ padding: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
        {error && <ErrorBanner>{error}</ErrorBanner>}
        {success && <div style={{ color: "var(--gh-open)" }}>{success}</div>}
        <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
          <input
            type="checkbox"
            checked={flags.automated_security_fixes}
            onChange={() => toggle("automated_security_fixes")}
            disabled={mutation.isPending}
          />
          Automated security fixes
        </label>
        <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
          <input
            type="checkbox"
            checked={flags.private_vulnerability_reporting}
            onChange={() => toggle("private_vulnerability_reporting")}
            disabled={mutation.isPending}
          />
          Private vulnerability reporting
        </label>
        <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
          <input
            type="checkbox"
            checked={flags.vulnerability_alerts}
            onChange={() => toggle("vulnerability_alerts")}
            disabled={mutation.isPending}
          />
          Vulnerability alerts
        </label>
      </div>
    </Box>
  );
}

function InteractionTab({ owner, repo }: { owner: string; repo: string }) {
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [limit, setLimit] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => setRepoInteractionLimit(owner, repo, limit),
    onSuccess: () => {
      setError(null);
      setSuccess(limit === null ? "Interaction limit cleared." : `Interaction limit set to ${limit}.`);
      queryClient.invalidateQueries({ queryKey: ["repo-interaction-limit", owner, repo] });
    },
    onError: (err: Error) => {
      setSuccess(null);
      setError(err.message);
    },
  });

  return (
    <Box header={<span style={{ fontWeight: 600 }}>Interaction limits</span>}>
      <div style={{ padding: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
        {error && <ErrorBanner>{error}</ErrorBanner>}
        {success && <div style={{ color: "var(--gh-open)" }}>{success}</div>}
        <FormLabel id="interaction-limit">Limit</FormLabel>
        <select
          id="interaction-limit"
          value={limit ?? ""}
          onChange={(e) => setLimit(e.target.value || null)}
          className="w-full"
        >
          <option value="">No limit</option>
          <option value="existing_users">Existing users</option>
          <option value="contributors_only">Contributors only</option>
          <option value="collaborators_only">Collaborators only</option>
        </select>
        <div className="flex justify-end gap-2">
          <Button
            variant="ghost"
            onClick={() => {
              setError(null);
              setSuccess(null);
              setLimit(null);
              mutation.mutate();
            }}
            disabled={mutation.isPending}
          >
            Clear limit
          </Button>
          <Button
            variant="primary"
            onClick={() => {
              setError(null);
              setSuccess(null);
              mutation.mutate();
            }}
            disabled={mutation.isPending}
          >
            Set limit
          </Button>
        </div>
      </div>
    </Box>
  );
}

function TransferTab({ owner, repo }: { owner: string; repo: string }) {
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [newOwner, setNewOwner] = useState("");

  const mutation = useMutation({
    mutationFn: () => transferRepo(owner, repo, newOwner.trim()),
    onSuccess: () => {
      setError(null);
      setSuccess(`Repository transferred to ${newOwner.trim()}.`);
      setNewOwner("");
    },
    onError: (err: Error) => {
      setSuccess(null);
      setError(err.message);
    },
  });

  return (
    <Box header={<span style={{ fontWeight: 600 }}>Transfer repository</span>}>
      <div style={{ padding: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
        {error && <ErrorBanner>{error}</ErrorBanner>}
        {success && <div style={{ color: "var(--gh-open)" }}>{success}</div>}
        <p style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
          Transferring removes you as owner and moves the repository to the target owner or organization.
        </p>
        <FormLabel id="transfer-owner">New owner login</FormLabel>
        <input
          id="transfer-owner"
          type="text"
          value={newOwner}
          onChange={(e) => setNewOwner(e.target.value)}
          placeholder="owner or org"
          className="w-full"
        />
        <div className="flex justify-end">
          <Button
            variant="primary"
            onClick={() => {
              setError(null);
              setSuccess(null);
              mutation.mutate();
            }}
            disabled={mutation.isPending || !newOwner.trim()}
          >
            Transfer
          </Button>
        </div>
      </div>
    </Box>
  );
}

function RenameBranchTab({ owner, repo }: { owner: string; repo: string }) {
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [branch, setBranch] = useState("");
  const [newName, setNewName] = useState("");

  const branchesQuery = useQuery({
    queryKey: ["repo-branches", owner, repo],
    queryFn: () => fetchRepoBranches(owner, repo),
    enabled: !!owner && !!repo,
  });

  const mutation = useMutation({
    mutationFn: () => renameBranch(owner, repo, branch.trim(), newName.trim()),
    onSuccess: () => {
      setError(null);
      setSuccess(`Branch ${branch.trim()} renamed to ${newName.trim()}.`);
      setBranch("");
      setNewName("");
    },
    onError: (err: Error) => {
      setSuccess(null);
      setError(err.message);
    },
  });

  return (
    <Box header={<span style={{ fontWeight: 600 }}>Rename branch</span>}>
      <div style={{ padding: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
        {error && <ErrorBanner>{error}</ErrorBanner>}
        {success && <div style={{ color: "var(--gh-open)" }}>{success}</div>}
        <FormLabel id="rename-branch-old">Branch to rename</FormLabel>
        {branchesQuery.isLoading ? (
          <Spinner label="loading branches" />
        ) : (
          <select
            id="rename-branch-old"
            value={branch}
            onChange={(e) => setBranch(e.target.value)}
            className="w-full"
          >
            <option value="">Select branch…</option>
            {(branchesQuery.data ?? []).map((b) => (
              <option key={b.name} value={b.name}>{b.name}</option>
            ))}
          </select>
        )}
        <FormLabel id="rename-branch-new">New name</FormLabel>
        <input
          id="rename-branch-new"
          type="text"
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          placeholder="new-branch-name"
          className="w-full"
        />
        <div className="flex justify-end">
          <Button
            variant="primary"
            onClick={() => {
              setError(null);
              setSuccess(null);
              mutation.mutate();
            }}
            disabled={mutation.isPending || !branch.trim() || !newName.trim()}
          >
            Rename branch
          </Button>
        </div>
      </div>
    </Box>
  );
}

// ─── GitHub Pages panel ──────────────────────────────────────────────────

function PagesTab({ owner, repo }: { owner: string; repo: string }) {
  const queryClient = useQueryClient();
  const siteQ = useQuery({
    queryKey: ["pages-site", owner, repo],
    queryFn: () => fetchPagesSite(owner, repo),
    enabled: !!owner && !!repo,
  });

  if (siteQ.isLoading) return <Spinner label="loading Pages site" />;
  if (siteQ.isError)
    return <InlineError title="Failed to load Pages site" detail={String(siteQ.error)} />;

  const site = siteQ.data ?? null;
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["pages-site", owner, repo] });
    void queryClient.invalidateQueries({ queryKey: ["pages-builds", owner, repo] });
    void queryClient.invalidateQueries({ queryKey: ["pages-health", owner, repo] });
  };

  if (site === null) return <PagesEnableForm owner={owner} repo={repo} onEnabled={invalidate} />;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      <PagesSiteCard owner={owner} repo={repo} site={site} onChanged={invalidate} />
      <PagesBuildsCard owner={owner} repo={repo} />
      <PagesHealthCard owner={owner} repo={repo} hasCustomDomain={!!site.cname} />
      <PagesDeploymentLookupCard owner={owner} repo={repo} />
    </div>
  );
}

function PagesEnableForm({
  owner,
  repo,
  onEnabled,
}: {
  owner: string;
  repo: string;
  onEnabled: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [branch, setBranch] = useState("");
  const [path, setPath] = useState("/");
  const [buildType, setBuildType] = useState<"legacy" | "workflow">("legacy");

  const enableMut = useMutation({
    mutationFn: () =>
      createPagesSite(owner, repo, {
        build_type: buildType,
        ...(buildType === "legacy"
          ? { source: { branch: branch.trim(), path: path.trim() || "/" } }
          : branch.trim()
            ? { source: { branch: branch.trim(), path: path.trim() || "/" } }
            : {}),
      }),
    onSuccess: () => {
      setError(null);
      onEnabled();
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Box header={<span style={{ fontWeight: 600 }}>GitHub Pages</span>}>
      <div style={{ padding: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
        {error && <ErrorBanner>{error}</ErrorBanner>}
        <p style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
          Pages is not enabled for this repository.
        </p>
        <FormLabel id="pages-build-type">Build type</FormLabel>
        <select
          id="pages-build-type"
          value={buildType}
          onChange={(e) => setBuildType(e.target.value as "legacy" | "workflow")}
          className="w-full"
        >
          <option value="legacy">Deploy from a branch (legacy)</option>
          <option value="workflow">GitHub Actions workflow</option>
        </select>
        <FormLabel id="pages-source-branch">Source branch</FormLabel>
        <input
          id="pages-source-branch"
          type="text"
          value={branch}
          onChange={(e) => setBranch(e.target.value)}
          placeholder={buildType === "workflow" ? "optional for workflow builds" : "main"}
          className="w-full"
        />
        <FormLabel id="pages-source-path">Source path</FormLabel>
        <input
          id="pages-source-path"
          type="text"
          value={path}
          onChange={(e) => setPath(e.target.value)}
          className="w-full"
        />
        <div className="flex justify-end">
          <Button
            variant="primary"
            onClick={() => {
              setError(null);
              enableMut.mutate();
            }}
            disabled={enableMut.isPending || (buildType === "legacy" && !branch.trim())}
          >
            Enable Pages
          </Button>
        </div>
      </div>
    </Box>
  );
}

function PagesSiteCard({
  owner,
  repo,
  site,
  onChanged,
}: {
  owner: string;
  repo: string;
  site: GithubPagesSite;
  onChanged: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [cname, setCname] = useState(site.cname);
  const [httpsEnforced, setHttpsEnforced] = useState(site.https_enforced);

  const updateMut = useMutation({
    mutationFn: () =>
      updatePagesSite(owner, repo, {
        cname: cname.trim() || null,
        https_enforced: httpsEnforced,
      }),
    onSuccess: () => {
      setError(null);
      setSuccess("Pages settings saved.");
      onChanged();
    },
    onError: (err: Error) => {
      setSuccess(null);
      setError(err.message);
    },
  });

  const disableMut = useMutation({
    mutationFn: () => deletePagesSite(owner, repo),
    onSuccess: () => {
      setError(null);
      onChanged();
    },
    onError: (err: Error) => {
      setSuccess(null);
      setError(err.message);
    },
  });

  return (
    <Box header={<span style={{ fontWeight: 600 }}>GitHub Pages</span>}>
      <div style={{ padding: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
        {error && <ErrorBanner>{error}</ErrorBanner>}
        {success && <div style={{ color: "var(--gh-open)" }}>{success}</div>}
        <div style={{ fontSize: "0.85rem" }}>
          Status: <span className="font-mono">{site.status}</span>
          {" · "}
          Site:{" "}
          <a href={site.html_url} style={{ color: "var(--color-accent)" }}>
            {site.html_url}
          </a>
        </div>
        <div style={{ fontSize: "0.8rem", color: "var(--color-fg-muted)" }}>
          Build type: {site.build_type ?? "legacy"}
          {site.source?.branch ? ` · source: ${site.source.branch} at ${site.source.path ?? "/"}` : ""}
          {" · "}
          {site.public ? "public" : "private"} site
        </div>
        <FormLabel id="pages-cname">Custom domain (CNAME)</FormLabel>
        <input
          id="pages-cname"
          type="text"
          value={cname}
          onChange={(e) => setCname(e.target.value)}
          placeholder="www.example.com"
          className="w-full"
        />
        <label style={{ display: "flex", alignItems: "center", gap: "0.4rem", fontSize: "0.85rem" }}>
          <input
            type="checkbox"
            checked={httpsEnforced}
            onChange={(e) => setHttpsEnforced(e.target.checked)}
          />
          Enforce HTTPS
        </label>
        <div className="flex justify-end gap-2">
          <Button
            variant="danger"
            onClick={() => {
              if (confirm("Disable GitHub Pages for this repository?")) {
                setError(null);
                setSuccess(null);
                disableMut.mutate();
              }
            }}
            disabled={disableMut.isPending}
          >
            Disable Pages
          </Button>
          <Button
            variant="primary"
            onClick={() => {
              setError(null);
              setSuccess(null);
              updateMut.mutate();
            }}
            disabled={updateMut.isPending}
          >
            Save Pages settings
          </Button>
        </div>
      </div>
    </Box>
  );
}

function PagesBuildsCard({ owner, repo }: { owner: string; repo: string }) {
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);

  const buildsQ = useQuery({
    queryKey: ["pages-builds", owner, repo],
    queryFn: () => fetchPagesBuilds(owner, repo),
  });

  const requestMut = useMutation({
    mutationFn: () => requestPagesBuild(owner, repo),
    onSuccess: () => {
      setError(null);
      void queryClient.invalidateQueries({ queryKey: ["pages-builds", owner, repo] });
    },
    onError: (err: Error) => setError(err.message),
  });

  return (
    <Box
      header={
        <div className="flex w-full items-center justify-between">
          <span style={{ fontWeight: 600 }}>Builds</span>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => {
              setError(null);
              requestMut.mutate();
            }}
            disabled={requestMut.isPending}
          >
            Request build
          </Button>
        </div>
      }
    >
      <div style={{ padding: "0" }}>
        {error && (
          <div style={{ padding: "0.75rem 1rem" }}>
            <ErrorBanner>{error}</ErrorBanner>
          </div>
        )}
        {buildsQ.isLoading ? (
          <div style={{ padding: "1rem" }}>
            <Spinner label="loading builds" />
          </div>
        ) : buildsQ.isError ? (
          <div style={{ padding: "1rem" }}>
            <InlineError title="Failed to load Pages builds" detail={String(buildsQ.error)} />
          </div>
        ) : (buildsQ.data ?? []).length === 0 ? (
          <div style={{ padding: "1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
            No builds yet.
          </div>
        ) : (
          <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {(buildsQ.data ?? []).map((b: GithubPagesBuild, i: number) => (
              <li
                key={b.url}
                className="flex items-center gap-3"
                style={{
                  padding: "0.6rem 1rem",
                  borderBottom:
                    i < (buildsQ.data ?? []).length - 1 ? "1px solid var(--color-border)" : "none",
                }}
              >
                <span
                  className="font-mono"
                  style={{
                    fontSize: "0.74rem",
                    color: b.status === "built" ? "var(--gh-open)" : "var(--color-fg-muted)",
                    border: "1px solid var(--color-border)",
                    borderRadius: "999px",
                    padding: "0.05rem 0.5rem",
                    flexShrink: 0,
                  }}
                >
                  {b.status}
                </span>
                <div className="min-w-0 flex-1" style={{ fontSize: "0.8rem" }}>
                  <span className="font-mono">{b.commit.slice(0, 7)}</span>
                  {b.pusher ? ` · by ${b.pusher.login}` : ""} ·{" "}
                  {new Date(b.created_at).toLocaleString()}
                  {b.error?.message ? (
                    <span style={{ color: "var(--color-danger-fg)" }}> · {b.error.message}</span>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Box>
  );
}

function PagesHealthCard({
  owner,
  repo,
  hasCustomDomain,
}: {
  owner: string;
  repo: string;
  hasCustomDomain: boolean;
}) {
  const healthQ = useQuery({
    queryKey: ["pages-health", owner, repo],
    queryFn: () => fetchPagesHealth(owner, repo),
    enabled: hasCustomDomain,
    retry: false,
  });

  return (
    <Box header={<span style={{ fontWeight: 600 }}>Custom domain health check</span>}>
      <div style={{ padding: "1rem", fontSize: "0.85rem" }}>
        {!hasCustomDomain ? (
          <span style={{ color: "var(--color-fg-muted)" }}>
            No custom domain configured — set a CNAME above to run the health check.
          </span>
        ) : healthQ.isLoading ? (
          <Spinner label="running health check" />
        ) : healthQ.isError ? (
          <InlineError title="Health check failed" detail={String(healthQ.error)} />
        ) : healthQ.data?.domain == null ? (
          <span style={{ color: "var(--color-fg-muted)" }}>No domain checks reported.</span>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: "0.3rem" }}>
            <div>
              <span className="font-mono">{healthQ.data.domain.host}</span> —{" "}
              {healthQ.data.domain.is_valid ? (
                <span style={{ color: "var(--gh-open)" }}>healthy</span>
              ) : (
                <span style={{ color: "var(--color-danger-fg)" }}>
                  unhealthy{healthQ.data.domain.reason ? ` (${healthQ.data.domain.reason})` : ""}
                </span>
              )}
            </div>
            <ul style={{ listStyle: "none", margin: 0, padding: 0, color: "var(--color-fg-muted)", fontSize: "0.8rem" }}>
              <li>DNS resolves: {healthQ.data.domain.dns_resolves ? "yes" : "no"}</li>
              <li>Valid domain: {healthQ.data.domain.is_valid_domain ? "yes" : "no"}</li>
              <li>Apex domain: {healthQ.data.domain.is_apex_domain ? "yes" : "no"}</li>
              <li>Pages domain: {healthQ.data.domain.is_pages_domain ? "yes" : "no"}</li>
              <li>Enforces HTTPS: {healthQ.data.domain.enforces_https ? "yes" : "no"}</li>
            </ul>
          </div>
        )}
      </div>
    </Box>
  );
}

function PagesDeploymentLookupCard({ owner, repo }: { owner: string; repo: string }) {
  const [error, setError] = useState<string | null>(null);
  const [deploymentId, setDeploymentId] = useState("");
  const [result, setResult] = useState<{ id: number; status: string } | null>(null);

  const lookupMut = useMutation({
    mutationFn: (id: number) => fetchPagesDeploymentStatus(owner, repo, id),
    onSuccess: (data, id) => {
      setError(null);
      setResult({ id, status: data.status });
    },
    onError: (err: Error) => {
      setResult(null);
      setError(err.message);
    },
  });

  return (
    <Box header={<span style={{ fontWeight: 600 }}>Pages deployment status</span>}>
      <div style={{ padding: "1rem", display: "flex", flexDirection: "column", gap: "0.75rem" }}>
        {error && <ErrorBanner>{error}</ErrorBanner>}
        <p style={{ fontSize: "0.8rem", color: "var(--color-fg-muted)" }}>
          Look up the status of a Pages deployment created via POST /pages/deployments
          (e.g. by the actions/deploy-pages workflow step).
        </p>
        <FormLabel id="pages-deployment-id">Deployment ID</FormLabel>
        <div className="flex gap-2">
          <input
            id="pages-deployment-id"
            type="text"
            inputMode="numeric"
            value={deploymentId}
            onChange={(e) => setDeploymentId(e.target.value)}
            placeholder="e.g. 1"
            style={{ flex: 1 }}
          />
          <Button
            variant="secondary"
            onClick={() => {
              const id = parseInt(deploymentId.trim(), 10);
              if (Number.isNaN(id)) {
                setResult(null);
                setError("Deployment ID must be a number.");
                return;
              }
              setError(null);
              lookupMut.mutate(id);
            }}
            disabled={lookupMut.isPending || !deploymentId.trim()}
          >
            Check status
          </Button>
        </div>
        {result && (
          <div style={{ fontSize: "0.85rem" }}>
            Deployment <span className="font-mono">#{result.id}</span>:{" "}
            <span className="font-mono">{result.status}</span>
          </div>
        )}
      </div>
    </Box>
  );
}
