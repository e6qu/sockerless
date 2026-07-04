import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { deleteRepoContent } from "../api.js";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  fetchRepoDetail,
  fetchRepoBranches,
  fetchRepoCommits,
  fetchRepoContents,
  fetchRepoLanguages,
  fetchRepoReadme,
  fetchRepoSocialCounts,
  fetchRepoTags,
  fetchWebhooks,
  fetchSecrets,
  fetchEnvironments,
  fetchReleases,
} from "../api.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import type {
  GithubBranch,
  GithubCommit,
  GithubContentItem,
  GithubTag,
  GithubWebhook,
  GithubSecret,
  GithubEnvironment,
  GithubRelease,
} from "../types.js";
import { RepoHeader } from "../components/Shell.js";
import { Box, Blankslate, CodeBlock } from "../components/ui.js";
import {
  BranchIcon,
  TagIcon,
  LockIcon,
  CommentIcon,
  FileIcon,
  DirectoryIcon,
  StarIcon,
} from "../components/octicons.js";

type SubTab = "code" | "commits" | "branches" | "tags" | "releases" | "webhooks" | "secrets" | "environments";

const SUB_TABS: { key: SubTab; label: string }[] = [
  { key: "code", label: "Code" },
  { key: "commits", label: "Commits" },
  { key: "branches", label: "Branches" },
  { key: "tags", label: "Tags" },
  { key: "releases", label: "Releases" },
  { key: "webhooks", label: "Webhooks" },
  { key: "secrets", label: "Secrets" },
  { key: "environments", label: "Environments" },
];

export function RepoDetailPage() {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  const [tab, setTab] = useState<SubTab>("code");

  const { data: repoData, isLoading, isError, error } = useQuery({
    queryKey: ["repo", owner, repo],
    queryFn: () => fetchRepoDetail(owner, repo),
    enabled: !!owner && !!repo,
  });
  const { data: branches = [] } = useQuery({
    queryKey: ["branches", owner, repo],
    queryFn: () => fetchRepoBranches(owner, repo),
    enabled: !!owner && !!repo,
  });
  const {
    data: commits = [],
    isLoading: commitsLoading,
    isError: commitsError,
    error: commitsErr,
  } = useQuery({
    queryKey: ["commits", owner, repo],
    queryFn: () => fetchRepoCommits(owner, repo),
    enabled: tab === "commits" || tab === "code",
  });
  const counts = useOpenCounts(owner, repo);
  const { data: webhooks = [], isError: webhooksError, error: webhooksErr } = useQuery({
    queryKey: ["webhooks", owner, repo],
    queryFn: () => fetchWebhooks(owner, repo),
    enabled: tab === "webhooks" && !!owner && !!repo,
  });
  const { data: secrets = [], isError: secretsError, error: secretsErr } = useQuery({
    queryKey: ["secrets", owner, repo],
    queryFn: () => fetchSecrets(owner, repo),
    enabled: tab === "secrets" && !!owner && !!repo,
  });
  const { data: environments = [], isError: environmentsError, error: environmentsErr } = useQuery({
    queryKey: ["environments", owner, repo],
    queryFn: () => fetchEnvironments(owner, repo),
    enabled: tab === "environments" && !!owner && !!repo,
  });
  const { data: releases = [], isError: releasesError, error: releasesErr } = useQuery({
    queryKey: ["releases", owner, repo],
    queryFn: () => fetchReleases(owner, repo),
    enabled: tab === "releases" && !!owner && !!repo,
  });
  const { data: tags = [], isError: tagsError, error: tagsErr } = useQuery({
    queryKey: ["repo-tags", owner, repo],
    queryFn: () => fetchRepoTags(owner, repo),
    enabled: tab === "tags" && !!owner && !!repo,
  });
  const { data: socialCounts } = useQuery({
    queryKey: ["repo-social-counts", owner, repo],
    queryFn: () => fetchRepoSocialCounts(owner, repo),
    enabled: !!owner && !!repo,
  });
  const { data: languages } = useQuery({
    queryKey: ["repo-languages", owner, repo],
    queryFn: () => fetchRepoLanguages(owner, repo),
    enabled: !!owner && !!repo,
  });

  if (isLoading) return <Spinner label={`loading ${owner}/${repo}`} />;
  if (isError || !repoData)
    return <InlineError title={`Failed to load ${owner}/${repo}`} detail={String(error)} />;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="code" {...counts} />

      {/* About line */}
      <div
        className="mb-4 flex flex-wrap items-center gap-x-4 gap-y-1"
        style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}
      >
        <span>{repoData.description || "No description provided."}</span>
        <span className="inline-flex items-center gap-1">
          <BranchIcon size={14} />{" "}
          {branches.length > 0 ? branches.map((b) => b.name).join(", ") : repoData.default_branch}
        </span>
        <Link
          to={`/ui/repos/${owner}/${repo}/packages`}
          style={{ color: "var(--color-accent)", textDecoration: "none" }}
        >
          Packages
        </Link>
        <Link
          to={`/ui/repos/${owner}/${repo}/codespaces`}
          style={{ color: "var(--color-accent)", textDecoration: "none" }}
        >
          Codespaces
        </Link>
      </div>

      {/* Social counters (star/watch/fork) linking to their list views. */}
      {socialCounts && (
        <div
          className="mb-4 flex flex-wrap items-center gap-x-4 gap-y-1"
          style={{ fontSize: "0.85rem" }}
        >
          <Link
            to={`/ui/repos/${owner}/${repo}/stargazers`}
            className="inline-flex items-center gap-1"
            style={{ color: "var(--color-accent)", textDecoration: "none" }}
          >
            <StarIcon size={14} /> {socialCounts.stargazers_count}{" "}
            {socialCounts.stargazers_count === 1 ? "star" : "stars"}
          </Link>
          <Link
            to={`/ui/repos/${owner}/${repo}/watchers`}
            style={{ color: "var(--color-accent)", textDecoration: "none" }}
          >
            {socialCounts.subscribers_count}{" "}
            {socialCounts.subscribers_count === 1 ? "watcher" : "watchers"}
          </Link>
          <Link
            to={`/ui/repos/${owner}/${repo}/forks`}
            style={{ color: "var(--color-accent)", textDecoration: "none" }}
          >
            {socialCounts.forks_count} {socialCounts.forks_count === 1 ? "fork" : "forks"}
          </Link>
        </div>
      )}

      {languages && Object.keys(languages).length > 0 && <LanguagesBar languages={languages} />}

      {/* Secondary (repo-admin) tab strip */}
      <div
        className="mb-4 flex flex-wrap gap-1"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        {SUB_TABS.map((t) => (
          <button
            key={t.key}
            type="button"
            onClick={() => setTab(t.key)}
            style={{
              padding: "0.4rem 0.7rem",
              marginBottom: "-1px",
              fontSize: "0.84rem",
              fontWeight: tab === t.key ? 600 : 500,
              color: tab === t.key ? "var(--color-fg)" : "var(--color-fg-muted)",
              background: "transparent",
              border: "none",
              borderBottom: `2px solid ${tab === t.key ? "var(--color-accent)" : "transparent"}`,
            }}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === "code" && (
        commitsError ? (
          <InlineError title="Failed to load repository contents" detail={String(commitsErr)} />
        ) : (
          <CodeView
            owner={owner}
            repo={repo}
            commits={commits}
            loading={commitsLoading}
            branches={branches.map((b) => b.name)}
            defaultBranch={repoData.default_branch}
          />
        )
      )}
      {tab === "commits" &&
        (commitsError ? (
          <InlineError title="Failed to load commits" detail={String(commitsErr)} />
        ) : (
          <CommitsList commits={commits} loading={commitsLoading} />
        ))}
      {tab === "branches" && (
        <BranchesList branches={branches} defaultBranch={repoData.default_branch} />
      )}
      {tab === "tags" &&
        (tagsError ? (
          <InlineError title="Failed to load tags" detail={String(tagsErr)} />
        ) : (
          <TagsList tags={tags} />
        ))}
      {tab === "releases" &&
        (releasesError ? (
          <InlineError title="Failed to load releases" detail={String(releasesErr)} />
        ) : (
          <ReleasesList releases={releases} />
        ))}
      {tab === "webhooks" &&
        (webhooksError ? (
          <InlineError title="Failed to load webhooks" detail={String(webhooksErr)} />
        ) : (
          <WebhooksList owner={owner} repo={repo} hooks={webhooks} />
        ))}
      {tab === "secrets" &&
        (secretsError ? (
          <InlineError title="Failed to load secrets" detail={String(secretsErr)} />
        ) : (
          <SecretsList secrets={secrets} />
        ))}
      {tab === "environments" &&
        (environmentsError ? (
          <InlineError title="Failed to load environments" detail={String(environmentsErr)} />
        ) : (
          <>
            <div className="mb-3" style={{ fontSize: "0.85rem" }}>
              <Link
                to={`/ui/repos/${owner}/${repo}/deployments`}
                style={{ color: "var(--color-accent)", textDecoration: "none" }}
              >
                View deployments, protection rules, and pending approvals →
              </Link>
            </div>
            <EnvironmentsList environments={environments} />
          </>
        ))}
    </div>
  );
}

function CodeView({
  owner,
  repo,
  commits,
  loading,
  branches,
  defaultBranch,
}: {
  owner: string;
  repo: string;
  commits: GithubCommit[];
  loading: boolean;
  branches: string[];
  defaultBranch: string;
}) {
  const [branch, setBranch] = useState(defaultBranch);
  const [path, setPath] = useState("");

  useEffect(() => {
    setBranch(defaultBranch);
  }, [defaultBranch]);

  const {
    data: items,
    isLoading: itemsLoading,
    isError: itemsError,
    error: itemsErr,
  } = useQuery({
    queryKey: ["contents", owner, repo, path, branch],
    queryFn: () => fetchRepoContents(owner, repo, path, branch),
    enabled: commits.length > 0,
  });

  const {
    data: readme,
    isLoading: readmeLoading,
    isError: readmeError,
  } = useQuery({
    queryKey: ["readme", owner, repo, branch],
    queryFn: () => fetchRepoReadme(owner, repo, branch),
    enabled: commits.length > 0 && path === "",
  });

  if (loading || itemsLoading || readmeLoading) return <Spinner label="loading code" />;
  if (commits.length === 0) {
    return <EmptyRepoSetup owner={owner} repo={repo} defaultBranch={defaultBranch} />;
  }
  if (itemsError) return <InlineError title="Failed to load files" detail={String(itemsErr)} />;

  const fileList = Array.isArray(items) ? items : [];

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      <div className="flex items-center gap-2">
        <select
          aria-label="Branch"
          value={branch}
          onChange={(e) => setBranch(e.target.value)}
          style={{ fontSize: "0.85rem", padding: "0.35rem 0.5rem" }}
        >
          {branches.map((b) => (
            <option key={b} value={b}>
              {b}
            </option>
          ))}
        </select>
        {path && (
          <button
            type="button"
            onClick={() => setPath(path.split("/").slice(0, -1).join("/"))}
            style={{ fontSize: "0.85rem", color: "var(--color-accent)", background: "transparent", border: "none" }}
          >
            ..
          </button>
        )}
        <span style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>{path}</span>
      </div>

      {fileList.length > 0 && (
        <Box>
          {fileList.map((item, i) => (
            <FileRow
              key={item.sha}
              owner={owner}
              repo={repo}
              basePath={path}
              item={item}
              isLast={i === fileList.length - 1}
              onClick={() => {
                if (item.type === "dir") {
                  setPath(path ? `${path}/${item.name}` : item.name);
                }
              }}
            />
          ))}
        </Box>
      )}

      {readmeError ? null : readme ? (
        <Box
          header={
            <span style={{ fontSize: "0.9rem", fontWeight: 600 }}>
              {readme.name}
            </span>
          }
        >
          <div
            style={{ padding: "1rem", fontSize: "0.9rem" }}
            className="markdown-body"
          >
            <Markdown remarkPlugins={[remarkGfm]}>
              {decodeBase64(readme.content)}
            </Markdown>
          </div>
        </Box>
      ) : null}
    </div>
  );
}

function FileRow({
  owner,
  repo,
  basePath,
  item,
  isLast,
  onClick,
}: {
  owner: string;
  repo: string;
  basePath: string;
  item: GithubContentItem;
  isLast: boolean;
  onClick: () => void;
}) {
  const queryClient = useQueryClient();
  const isDir = item.type === "dir";
  const deleteMutation = useMutation({
    mutationFn: () =>
      deleteRepoContent(owner, repo, item.path, item.sha, `Delete ${item.name} via web`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["contents", owner, repo, basePath] });
      queryClient.invalidateQueries({ queryKey: ["readme", owner, repo] });
      queryClient.invalidateQueries({ queryKey: ["commits", owner, repo] });
    },
  });
  return (
    <div
      role={isDir ? "button" : undefined}
      onClick={isDir ? onClick : undefined}
      className="flex items-center gap-2"
      style={{
        padding: "0.55rem 1rem",
        borderBottom: isLast ? "none" : "1px solid var(--color-border)",
        cursor: isDir ? "pointer" : "default",
        fontSize: "0.85rem",
      }}
    >
      <span style={{ color: "var(--color-accent)", display: "flex" }}>
        {isDir ? <DirectoryIcon size={16} /> : <FileIcon size={16} />}
      </span>
      <span style={{ color: "var(--color-accent)", fontWeight: 500, flex: 1 }}>{item.name}</span>
      {!isDir && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            if (window.confirm(`Delete ${item.name}?`)) {
              deleteMutation.mutate();
            }
          }}
          disabled={deleteMutation.isPending}
          style={{
            fontSize: "0.75rem",
            color: "var(--color-danger-fg)",
            background: "transparent",
            border: "none",
            cursor: "pointer",
          }}
        >
          {deleteMutation.isPending ? "Deleting..." : "Delete"}
        </button>
      )}
    </div>
  );
}

function EmptyRepoSetup({
  owner,
  repo,
  defaultBranch,
}: {
  owner: string;
  repo: string;
  defaultBranch: string;
}) {
  const origin = typeof window !== "undefined" ? window.location.origin : "";
  const [activeTab, setActiveTab] = useState<"https" | "ssh" | "gh">("https");
  const tabs: { key: "https" | "ssh" | "gh"; label: string }[] = [
    { key: "https", label: "HTTPS" },
    { key: "ssh", label: "SSH" },
    { key: "gh", label: "GitHub CLI" },
  ];

  const snippets: Record<typeof activeTab, string> = {
    https: `git remote add origin ${origin}/${owner}/${repo}.git\ngit branch -M ${defaultBranch}\ngit push -u origin ${defaultBranch}`,
    ssh: `git remote add origin git@bleephub.local:${owner}/${repo}.git\ngit branch -M ${defaultBranch}\ngit push -u origin ${defaultBranch}`,
    gh: `gh repo clone ${owner}/${repo}\ncd ${repo}`,
  };

  return (
    <Blankslate title="This repository is empty">
      <p className="mb-3">Get started by creating a new file or cloning an existing repository.</p>

      <div
        className="mb-3 flex gap-1"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        {tabs.map((t) => (
          <button
            key={t.key}
            type="button"
            onClick={() => setActiveTab(t.key)}
            style={{
              padding: "0.4rem 0.7rem",
              marginBottom: "-1px",
              fontSize: "0.84rem",
              fontWeight: activeTab === t.key ? 600 : 500,
              color: activeTab === t.key ? "var(--color-fg)" : "var(--color-fg-muted)",
              background: "transparent",
              border: "none",
              borderBottom: `2px solid ${activeTab === t.key ? "var(--color-accent)" : "transparent"}`,
            }}
          >
            {t.label}
          </button>
        ))}
      </div>
      <CodeBlock>{snippets[activeTab]}</CodeBlock>
    </Blankslate>
  );
}

function CommitsList({ commits, loading }: { commits: GithubCommit[]; loading: boolean }) {
  if (loading) return <Spinner label="loading commits" />;
  if (commits.length === 0) return <Blankslate title="No commits yet" />;
  return (
    <Box>
      {commits.map((c, i) => (
        <div
          key={c.sha}
          className="flex items-center gap-3"
          style={{
            padding: "0.65rem 1rem",
            borderBottom: i < commits.length - 1 ? "1px solid var(--color-border)" : "none",
          }}
        >
          <div className="min-w-0 flex-1">
            <div
              style={{
                fontSize: "0.88rem",
                color: "var(--color-fg)",
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
              }}
            >
              {c.commit.message.split("\n")[0]}
            </div>
            <div className="mt-0.5" style={{ fontSize: "0.76rem", color: "var(--color-fg-muted)" }}>
              {c.commit.author.name} · {new Date(c.commit.author.date).toLocaleDateString()}
            </div>
          </div>
          <span
            className="font-mono"
            style={{
              fontSize: "0.74rem",
              color: "var(--color-fg-muted)",
              background: "var(--color-bg-subtle)",
              border: "1px solid var(--color-border)",
              padding: "0.1rem 0.4rem",
              borderRadius: "var(--radius-sm)",
            }}
          >
            {c.sha.slice(0, 7)}
          </span>
        </div>
      ))}
    </Box>
  );
}

function WebhooksList({
  owner,
  repo,
  hooks,
}: {
  owner: string;
  repo: string;
  hooks: GithubWebhook[];
}) {
  if (hooks.length === 0) return <Blankslate icon={<CommentIcon size={26} />} title="No webhooks configured" />;
  return (
    <Box>
      {hooks.map((h, i) => (
        <div
          key={h.id}
          className="flex items-center gap-3"
          style={{
            padding: "0.7rem 1rem",
            borderBottom: i < hooks.length - 1 ? "1px solid var(--color-border)" : "none",
          }}
        >
          <span
            aria-hidden
            style={{
              width: 8,
              height: 8,
              borderRadius: "999px",
              background: h.active ? "var(--gh-open)" : "var(--color-fg-subtle)",
              flexShrink: 0,
            }}
          />
          <div className="min-w-0 flex-1">
            <div style={{ fontSize: "0.88rem", fontWeight: 500, color: "var(--color-fg)" }}>
              {h.name}{" "}
              <span style={{ color: "var(--color-fg-subtle)", fontWeight: 400 }}>#{h.id}</span>
            </div>
            <div className="font-mono" style={{ fontSize: "0.74rem", color: "var(--color-fg-muted)" }}>
              {h.config?.url || "no url"} · events: {h.events?.join(", ") || "none"}
            </div>
          </div>
          <Link
            to={`/ui/repos/${owner}/${repo}/hooks/${h.id}/deliveries`}
            style={{ color: "var(--color-accent)", fontSize: "0.8rem", textDecoration: "none", flexShrink: 0 }}
          >
            Deliveries
          </Link>
        </div>
      ))}
    </Box>
  );
}

function SecretsList({ secrets }: { secrets: GithubSecret[] }) {
  if (secrets.length === 0) return <Blankslate icon={<LockIcon size={26} />} title="No secrets configured" />;
  return (
    <Box>
      {secrets.map((s, i) => (
        <div
          key={s.name}
          className="flex items-center gap-2 font-mono"
          style={{
            padding: "0.65rem 1rem",
            fontSize: "0.85rem",
            color: "var(--color-fg)",
            borderBottom: i < secrets.length - 1 ? "1px solid var(--color-border)" : "none",
          }}
        >
          <LockIcon size={14} style={{ color: "var(--color-fg-muted)" }} /> {s.name}
        </div>
      ))}
    </Box>
  );
}

function EnvironmentsList({ environments }: { environments: GithubEnvironment[] }) {
  if (environments.length === 0) return <Blankslate title="No environments" />;
  return (
    <Box>
      {environments.map((e, i) => (
        <div
          key={e.name}
          style={{
            padding: "0.65rem 1rem",
            fontSize: "0.85rem",
            color: "var(--color-fg)",
            borderBottom: i < environments.length - 1 ? "1px solid var(--color-border)" : "none",
          }}
        >
          {e.name}
        </div>
      ))}
    </Box>
  );
}

function ReleasesList({ releases }: { releases: GithubRelease[] }) {
  if (releases.length === 0) return <Blankslate icon={<TagIcon size={26} />} title="No releases" />;
  return (
    <Box>
      {releases.map((r, i) => (
        <div
          key={r.id}
          className="flex items-center gap-3"
          style={{
            padding: "0.7rem 1rem",
            borderBottom: i < releases.length - 1 ? "1px solid var(--color-border)" : "none",
          }}
        >
          <span
            className="inline-flex items-center gap-1 font-mono"
            style={{
              fontSize: "0.74rem",
              color: "var(--color-accent)",
              background: "var(--color-accent-soft)",
              padding: "0.1rem 0.45rem",
              borderRadius: "var(--radius-sm)",
            }}
          >
            <TagIcon size={12} /> {r.tag_name}
          </span>
          <div className="min-w-0 flex-1">
            <div style={{ fontSize: "0.88rem", fontWeight: 500, color: "var(--color-fg)" }}>
              {r.name || r.tag_name}
            </div>
            <div style={{ fontSize: "0.76rem", color: "var(--color-fg-muted)" }}>
              {r.published_at === null
                ? "draft"
                : `published ${new Date(r.published_at).toLocaleDateString()}`}
            </div>
          </div>
        </div>
      ))}
    </Box>
  );
}

/** Fixed palette cycled across languages, largest share first. */
const LANGUAGE_BAR_COLORS = ["#3572A5", "#F1E05A", "#E34C26", "#563D7C", "#00ADD8", "#B07219", "#701516", "#178600"];

function LanguagesBar({ languages }: { languages: Record<string, number> }) {
  const entries = Object.entries(languages);
  const total = entries.reduce((sum, [, bytes]) => sum + bytes, 0);
  if (total === 0) return null;
  return (
    <div className="mb-4">
      <div
        className="flex overflow-hidden"
        style={{ height: 8, borderRadius: "var(--radius-md)", border: "1px solid var(--color-border)" }}
      >
        {entries.map(([lang, bytes], i) => (
          <span
            key={lang}
            title={`${lang} ${((bytes / total) * 100).toFixed(1)}%`}
            style={{
              width: `${(bytes / total) * 100}%`,
              background: LANGUAGE_BAR_COLORS[i % LANGUAGE_BAR_COLORS.length],
            }}
          />
        ))}
      </div>
      <div className="mt-1.5 flex flex-wrap gap-x-4 gap-y-1" style={{ fontSize: "0.78rem" }}>
        {entries.map(([lang, bytes], i) => (
          <span key={lang} className="inline-flex items-center gap-1.5">
            <span
              aria-hidden
              style={{
                width: 8,
                height: 8,
                borderRadius: "999px",
                background: LANGUAGE_BAR_COLORS[i % LANGUAGE_BAR_COLORS.length],
              }}
            />
            <span style={{ fontWeight: 500 }}>{lang}</span>
            <span style={{ color: "var(--color-fg-muted)" }}>
              {((bytes / total) * 100).toFixed(1)}%
            </span>
          </span>
        ))}
      </div>
    </div>
  );
}

function BranchesList({ branches, defaultBranch }: { branches: GithubBranch[]; defaultBranch: string }) {
  if (branches.length === 0) return <Blankslate icon={<BranchIcon size={26} />} title="No branches" />;
  return (
    <Box>
      {branches.map((b, i) => (
        <div
          key={b.name}
          className="flex items-center gap-3"
          style={{
            padding: "0.65rem 1rem",
            borderBottom: i < branches.length - 1 ? "1px solid var(--color-border)" : "none",
          }}
        >
          <BranchIcon size={14} style={{ color: "var(--color-fg-muted)" }} />
          <span className="font-mono" style={{ fontSize: "0.85rem", fontWeight: 500, flex: 1 }}>
            {b.name}
            {b.name === defaultBranch && (
              <span
                style={{
                  marginLeft: "0.6rem",
                  fontSize: "0.72rem",
                  fontWeight: 600,
                  color: "var(--color-accent)",
                  border: "1px solid var(--color-accent)",
                  borderRadius: "2rem",
                  padding: "0.05rem 0.5rem",
                }}
              >
                default
              </span>
            )}
          </span>
          <span className="font-mono" style={{ fontSize: "0.74rem", color: "var(--color-fg-muted)" }}>
            {b.commit.sha.slice(0, 7)}
          </span>
        </div>
      ))}
    </Box>
  );
}

function TagsList({ tags }: { tags: GithubTag[] }) {
  if (tags.length === 0) return <Blankslate icon={<TagIcon size={26} />} title="No tags" />;
  return (
    <Box>
      {tags.map((t, i) => (
        <div
          key={t.name}
          className="flex items-center gap-3"
          style={{
            padding: "0.65rem 1rem",
            borderBottom: i < tags.length - 1 ? "1px solid var(--color-border)" : "none",
          }}
        >
          <TagIcon size={14} style={{ color: "var(--color-fg-muted)" }} />
          <span className="font-mono" style={{ fontSize: "0.85rem", fontWeight: 500, flex: 1 }}>
            {t.name}
          </span>
          <span className="font-mono" style={{ fontSize: "0.74rem", color: "var(--color-fg-muted)" }}>
            {t.commit.sha.slice(0, 7)}
          </span>
          <a
            href={t.zipball_url}
            style={{ fontSize: "0.78rem", color: "var(--color-accent)", textDecoration: "none" }}
          >
            zip
          </a>
          <a
            href={t.tarball_url}
            style={{ fontSize: "0.78rem", color: "var(--color-accent)", textDecoration: "none" }}
          >
            tar.gz
          </a>
        </div>
      ))}
    </Box>
  );
}

function decodeBase64(s: string): string {
  try {
    if (typeof window !== "undefined" && window.atob) {
      return window.atob(s);
    }
    return Buffer.from(s, "base64").toString("utf-8");
  } catch {
    return "";
  }
}
