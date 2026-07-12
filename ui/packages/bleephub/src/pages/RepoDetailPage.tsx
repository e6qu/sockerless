import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
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
  fetchRepoTopics,
  fetchWebhooks,
  fetchSecrets,
  fetchEnvironments,
  fetchReleases,
  fetchPackages,
} from "../api.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import { decodeContentsBase64 } from "../utils/workflowDispatch.js";
import { relativeTimeFromNow } from "../utils/format.js";
import type {
  BleephubRepo,
  GithubBranch,
  GithubCommit,
  GithubContentItem,
  GithubTag,
  GithubWebhook,
  GithubSecret,
  GithubEnvironment,
  GithubRelease,
  GithubRepoSocialCounts,
} from "../types.js";
import { RepoHeader } from "../components/Shell.js";
import { Box, Blankslate, CodeBlock, SectionLabel } from "../components/ui.js";
import {
  BranchIcon,
  TagIcon,
  LockIcon,
  CommentIcon,
  FileIcon,
  DirectoryIcon,
  StarIcon,
  EyeIcon,
  RepoForkedIcon,
  GlobeIcon,
  CodeIcon,
  CopyIcon,
  CheckIcon,
  ChevronDownIcon,
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

      {/* GitHub's two-column Code page: file browser + README on the left,
          the About sidebar (description, topics, releases, packages,
          languages, social counts) on the right. */}
      {tab === "code" && (
        commitsError ? (
          <InlineError title="Failed to load repository contents" detail={String(commitsErr)} />
        ) : (
          <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_296px]">
            <div className="min-w-0">
              <CodeView
                owner={owner}
                repo={repo}
                commits={commits}
                loading={commitsLoading}
                branches={branches.map((b) => b.name)}
                defaultBranch={repoData.default_branch}
              />
            </div>
            <AboutSidebar
              owner={owner}
              repo={repo}
              repoData={repoData}
              languages={languages}
              socialCounts={socialCounts}
            />
          </div>
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
          <ReleasesList owner={owner} repo={repo} releases={releases} />
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
    // Decode here so a corrupt base64 payload surfaces as readmeError
    // instead of throwing mid-render.
    queryFn: async () => {
      const file = await fetchRepoReadme(owner, repo, branch);
      return { name: file.name, text: decodeContentsBase64(file.content) };
    },
    enabled: commits.length > 0 && path === "",
  });

  if (loading || itemsLoading || readmeLoading) return <Spinner label="loading code" />;
  if (commits.length === 0) {
    return <EmptyRepoSetup owner={owner} repo={repo} defaultBranch={defaultBranch} />;
  }
  if (itemsError) return <InlineError title="Failed to load files" detail={String(itemsErr)} />;

  const fileList = Array.isArray(items) ? items : [];
  // Only the repository root shows the "latest commit" banner — it is the
  // repo's most recent commit, not a per-directory one, so surfacing it in a
  // sub-tree would misattribute it. No per-file commit data is fabricated.
  const latestCommit = path === "" ? commits[0] : undefined;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      <div className="flex flex-wrap items-center gap-2">
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
        <span style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)", flex: 1 }}>{path}</span>
        <CloneButton owner={owner} repo={repo} />
      </div>

      {fileList.length > 0 && (
        <Box
          header={
            latestCommit ? (
              <LatestCommitBanner owner={owner} repo={repo} commit={latestCommit} total={commits.length} />
            ) : undefined
          }
        >
          {fileList.map((item, i) => (
            <FileRow
              key={item.sha}
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
            style={{ padding: "1.5rem", fontSize: "0.9rem" }}
            className="markdown-body"
          >
            <Markdown remarkPlugins={[remarkGfm]}>
              {readme.text}
            </Markdown>
          </div>
        </Box>
      ) : null}
    </div>
  );
}

/** GitHub's "latest commit" strip at the top of the file listing. */
function LatestCommitBanner({
  owner,
  repo,
  commit,
  total,
}: {
  owner: string;
  repo: string;
  commit: GithubCommit;
  total: number;
}) {
  return (
    <div className="flex w-full min-w-0 items-center gap-2">
      <span
        className="min-w-0 flex-1 truncate"
        style={{ color: "var(--color-fg)" }}
        title={commit.commit.message}
      >
        <span style={{ fontWeight: 600 }}>{commit.commit.author.name}</span>{" "}
        {commit.commit.message.split("\n")[0]}
      </span>
      <span className="font-mono" style={{ color: "var(--color-fg-muted)" }}>
        {commit.sha.slice(0, 7)}
      </span>
      <span style={{ color: "var(--color-fg-muted)" }}>
        · {relativeTimeFromNow(commit.commit.author.date)}
      </span>
      <Link
        to={`/ui/repos/${owner}/${repo}`}
        onClick={(e) => e.preventDefault()}
        className="inline-flex items-center gap-1"
        style={{ color: "var(--color-fg-muted)", textDecoration: "none", whiteSpace: "nowrap" }}
      >
        <CommentIcon size={14} /> {total} {total === 1 ? "commit" : "commits"}
      </Link>
    </div>
  );
}

/** GitHub's green "Code" clone dropdown — HTTPS clone URL with a copy button. */
function CloneButton({ owner, repo }: { owner: string; repo: string }) {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);
  const origin = typeof window !== "undefined" ? window.location.origin : "";
  const cloneUrl = `${origin}/${owner}/${repo}.git`;

  useEffect(() => {
    if (!open) return;
    const onDocClick = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDocClick);
    return () => document.removeEventListener("mousedown", onDocClick);
  }, [open]);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(cloneUrl);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard is unavailable (insecure context / denied permission). The
      // URL stays selectable in the field so the user can copy manually.
      setCopied(false);
    }
  };

  return (
    <div ref={wrapRef} style={{ position: "relative" }}>
      <button
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className="inline-flex items-center gap-1.5"
        style={{
          background: "var(--gh-open-solid)",
          color: "#ffffff",
          border: "1px solid color-mix(in srgb, #000 12%, var(--gh-open-solid))",
          borderRadius: "var(--radius-md)",
          padding: "0.34rem 0.7rem",
          fontSize: "0.82rem",
          fontWeight: 600,
        }}
      >
        <CodeIcon size={15} /> Code <ChevronDownIcon size={14} />
      </button>
      {open && (
        <div
          role="dialog"
          aria-label="Clone this repository"
          style={{
            position: "absolute",
            top: "calc(100% + 6px)",
            right: 0,
            zIndex: 20,
            width: 320,
            padding: "0.85rem",
            background: "var(--color-surface-raised)",
            border: "1px solid var(--color-border)",
            borderRadius: "var(--radius-md)",
            boxShadow: "0 8px 24px rgba(31,35,40,0.2)",
          }}
        >
          <div style={{ fontSize: "0.82rem", fontWeight: 600, marginBottom: "0.4rem" }}>Clone</div>
          <div style={{ fontSize: "0.72rem", color: "var(--color-fg-muted)", marginBottom: "0.5rem" }}>
            HTTPS
          </div>
          <div className="flex items-center gap-1.5">
            <input
              type="text"
              readOnly
              value={cloneUrl}
              aria-label="HTTPS clone URL"
              onFocus={(e) => e.currentTarget.select()}
              style={{
                flex: 1,
                minWidth: 0,
                fontSize: "0.78rem",
                fontFamily: "var(--font-mono)",
                padding: "0.35rem 0.5rem",
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-sm)",
                background: "var(--color-bg-subtle)",
                color: "var(--color-fg)",
              }}
            />
            <button
              type="button"
              onClick={copy}
              aria-label="Copy clone URL"
              title="Copy clone URL"
              className="inline-flex items-center justify-center"
              style={{
                flexShrink: 0,
                width: 30,
                height: 30,
                background: "var(--color-bg-subtle)",
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-sm)",
                color: copied ? "var(--color-status-ok)" : "var(--color-fg-muted)",
                cursor: "pointer",
              }}
            >
              {copied ? <CheckIcon size={15} /> : <CopyIcon size={15} />}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

/** GitHub's right-hand "About" column on the repo Code page. */
function AboutSidebar({
  owner,
  repo,
  repoData,
  languages,
  socialCounts,
}: {
  owner: string;
  repo: string;
  repoData: BleephubRepo;
  languages: Record<string, number> | undefined;
  socialCounts: GithubRepoSocialCounts | undefined;
}) {
  const { data: topics, isError: topicsError } = useQuery({
    queryKey: ["repo-topics", owner, repo],
    queryFn: () => fetchRepoTopics(owner, repo),
    enabled: !!owner && !!repo,
  });
  const { data: releases, isError: releasesError } = useQuery({
    queryKey: ["releases", owner, repo],
    queryFn: () => fetchReleases(owner, repo),
    enabled: !!owner && !!repo,
  });
  const { data: packages, isError: packagesError } = useQuery({
    queryKey: ["repo-packages", owner, repo],
    queryFn: () => fetchPackages({ kind: "repo", owner, repo }),
    enabled: !!owner && !!repo,
  });

  const base = `/ui/repos/${owner}/${repo}`;
  const topicNames = topics?.names ?? [];
  const divider = { border: "none", borderTop: "1px solid var(--color-border)", margin: 0 } as const;
  const mutedLink = { color: "var(--color-fg-muted)", textDecoration: "none" } as const;

  return (
    <aside className="flex min-w-0 flex-col gap-4" style={{ fontSize: "0.85rem" }} aria-label="About">
      <section>
        <SectionLabel>About</SectionLabel>
        <p
          className="mb-2"
          style={{ color: repoData.description ? "var(--color-fg)" : "var(--color-fg-muted)" }}
        >
          {repoData.description || "No description provided."}
        </p>
        {repoData.homepage && (
          <a
            href={repoData.homepage}
            target="_blank"
            rel="noreferrer noopener"
            className="mb-2 flex items-center gap-1.5"
            style={{ color: "var(--color-accent)", textDecoration: "none", fontWeight: 600 }}
          >
            <GlobeIcon size={15} />
            <span className="truncate">{repoData.homepage.replace(/^https?:\/\//, "")}</span>
          </a>
        )}
        {topicsError ? (
          <InlineError title="Failed to load topics" />
        ) : topicNames.length > 0 ? (
          <div className="mb-1 mt-1 flex flex-wrap gap-1.5">
            {topicNames.map((t) => (
              <Link
                key={t}
                to={`/ui/search?q=${encodeURIComponent(`topic:${t}`)}`}
                style={{
                  display: "inline-block",
                  padding: "0.1rem 0.6rem",
                  fontSize: "0.75rem",
                  fontWeight: 500,
                  color: "var(--color-accent)",
                  background: "var(--color-accent-soft)",
                  borderRadius: "2rem",
                  textDecoration: "none",
                }}
              >
                {t}
              </Link>
            ))}
          </div>
        ) : null}
        {socialCounts && (
          <div className="mt-2 flex flex-col gap-1.5">
            <Link to={`${base}/stargazers`} className="inline-flex items-center gap-1.5" style={mutedLink}>
              <StarIcon size={15} /> {socialCounts.stargazers_count}{" "}
              {socialCounts.stargazers_count === 1 ? "star" : "stars"}
            </Link>
            <Link to={`${base}/watchers`} className="inline-flex items-center gap-1.5" style={mutedLink}>
              <EyeIcon size={15} /> {socialCounts.subscribers_count}{" "}
              {socialCounts.subscribers_count === 1 ? "watcher" : "watchers"}
            </Link>
            <Link to={`${base}/forks`} className="inline-flex items-center gap-1.5" style={mutedLink}>
              <RepoForkedIcon size={15} /> {socialCounts.forks_count}{" "}
              {socialCounts.forks_count === 1 ? "fork" : "forks"}
            </Link>
          </div>
        )}
      </section>

      <hr style={divider} />

      <section>
        <SectionLabel>Releases</SectionLabel>
        {releasesError ? (
          <InlineError title="Failed to load releases" />
        ) : releases && releases.length > 0 ? (
          <div className="flex flex-col gap-1">
            <span className="inline-flex items-center gap-1.5" style={{ fontWeight: 600 }}>
              <TagIcon size={15} style={{ color: "var(--color-status-ok)" }} />
              {releases[0].name || releases[0].tag_name}
              <span
                style={{
                  fontSize: "0.68rem",
                  fontWeight: 600,
                  color: "#ffffff",
                  background: "var(--gh-open-solid)",
                  borderRadius: "2rem",
                  padding: "0.05rem 0.5rem",
                }}
              >
                Latest
              </span>
            </span>
            {releases.length > 1 && (
              <Link to={base} onClick={(e) => e.preventDefault()} style={mutedLink}>
                + {releases.length - 1} {releases.length - 1 === 1 ? "release" : "releases"}
              </Link>
            )}
          </div>
        ) : (
          <span style={{ color: "var(--color-fg-muted)" }}>No releases published</span>
        )}
      </section>

      <hr style={divider} />

      <section>
        <SectionLabel>Packages</SectionLabel>
        {packagesError ? (
          <InlineError title="Failed to load packages" />
        ) : packages && packages.length > 0 ? (
          <Link
            to={`/ui/repos/${owner}/${repo}/packages`}
            style={{ color: "var(--color-accent)", textDecoration: "none" }}
          >
            {packages.length} {packages.length === 1 ? "package" : "packages"}
          </Link>
        ) : (
          <span style={{ color: "var(--color-fg-muted)" }}>No packages published</span>
        )}
      </section>

      {languages && Object.keys(languages).length > 0 && (
        <>
          <hr style={divider} />
          <section>
            <SectionLabel>Languages</SectionLabel>
            <LanguagesBar languages={languages} />
          </section>
        </>
      )}
    </aside>
  );
}

function FileRow({
  item,
  isLast,
  onClick,
}: {
  item: GithubContentItem;
  isLast: boolean;
  onClick: () => void;
}) {
  const isDir = item.type === "dir";
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
      <span style={{ color: isDir ? "var(--color-accent)" : "var(--color-fg-muted)", display: "flex" }}>
        {isDir ? <DirectoryIcon size={16} /> : <FileIcon size={16} />}
      </span>
      <span style={{ color: "var(--color-fg)", fontWeight: 400, flex: 1 }}>{item.name}</span>
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

function ReleasesList({ owner, repo, releases }: { owner: string; repo: string; releases: GithubRelease[] }) {
  if (releases.length === 0) return (
    <Blankslate icon={<TagIcon size={26} />} title="No releases">
      <Link to={`/ui/repos/${owner}/${repo}/releases/new`}>Create the first release</Link>
    </Blankslate>
  );
  return (
    <div className="flex flex-col gap-3">
      <div className="flex justify-end">
        <Link to={`/ui/repos/${owner}/${repo}/releases`} style={{ color: "var(--color-accent)", fontSize: "0.82rem" }}>
          Manage releases and assets
        </Link>
      </div>
      <Box>{releases.map((r, i) => (
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
          <Link className="min-w-0 flex-1" to={`/ui/repos/${owner}/${repo}/releases/${r.id}`} style={{ color: "inherit", textDecoration: "none" }}>
            <div style={{ fontSize: "0.88rem", fontWeight: 500, color: "var(--color-fg)" }}>
              {r.name || r.tag_name}
            </div>
            <div style={{ fontSize: "0.76rem", color: "var(--color-fg-muted)" }}>
              {r.published_at === null
                ? "draft"
                : `published ${new Date(r.published_at).toLocaleDateString()}`}
            </div>
          </Link>
        </div>
      ))}</Box>
    </div>
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
