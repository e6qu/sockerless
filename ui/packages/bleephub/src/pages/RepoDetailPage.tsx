import { useState } from "react";
import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchRepoDetail,
  fetchRepoBranches,
  fetchRepoCommits,
  fetchRepoIssues,
  fetchRepoPRs,
  fetchWebhooks,
  fetchSecrets,
  fetchEnvironments,
  fetchReleases,
} from "../api.js";
import type {
  GithubCommit,
  GithubWebhook,
  GithubSecret,
  GithubEnvironment,
  GithubRelease,
} from "../types.js";
import { RepoHeader } from "../components/Shell.js";
import { Box, Blankslate, CodeBlock } from "../components/ui.js";
import { BranchIcon, TagIcon, LockIcon, CommentIcon } from "../components/octicons.js";

type SubTab = "code" | "commits" | "releases" | "webhooks" | "secrets" | "environments";

const SUB_TABS: { key: SubTab; label: string }[] = [
  { key: "code", label: "Code" },
  { key: "commits", label: "Commits" },
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
  const { data: commits = [], isLoading: commitsLoading } = useQuery({
    queryKey: ["commits", owner, repo],
    queryFn: () => fetchRepoCommits(owner, repo),
    enabled: tab === "commits" || tab === "code",
  });
  const { data: issues = [] } = useQuery({
    queryKey: ["issues", owner, repo, "open"],
    queryFn: () => fetchRepoIssues(owner, repo, "open"),
    enabled: !!owner && !!repo,
  });
  const { data: prs = [] } = useQuery({
    queryKey: ["prs", owner, repo, "open"],
    queryFn: () => fetchRepoPRs(owner, repo, "open"),
    enabled: !!owner && !!repo,
  });
  const { data: webhooks = [] } = useQuery({
    queryKey: ["webhooks", owner, repo],
    queryFn: () => fetchWebhooks(owner, repo),
    enabled: tab === "webhooks" && !!owner && !!repo,
  });
  const { data: secrets = [] } = useQuery({
    queryKey: ["secrets", owner, repo],
    queryFn: () => fetchSecrets(owner, repo),
    enabled: tab === "secrets" && !!owner && !!repo,
  });
  const { data: environments = [] } = useQuery({
    queryKey: ["environments", owner, repo],
    queryFn: () => fetchEnvironments(owner, repo),
    enabled: tab === "environments" && !!owner && !!repo,
  });
  const { data: releases = [] } = useQuery({
    queryKey: ["releases", owner, repo],
    queryFn: () => fetchReleases(owner, repo),
    enabled: tab === "releases" && !!owner && !!repo,
  });

  if (isLoading) return <Spinner label={`loading ${owner}/${repo}`} />;
  if (isError || !repoData)
    return <InlineError title={`Failed to load ${owner}/${repo}`} detail={String(error)} />;

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="code" issueCount={issues.length} prCount={prs.length} />

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
      </div>

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
        <CodeView owner={owner} repo={repo} commits={commits} loading={commitsLoading} branch={repoData.default_branch} />
      )}
      {tab === "commits" && <CommitsList commits={commits} loading={commitsLoading} />}
      {tab === "releases" && <ReleasesList releases={releases} />}
      {tab === "webhooks" && <WebhooksList hooks={webhooks} />}
      {tab === "secrets" && <SecretsList secrets={secrets} />}
      {tab === "environments" && <EnvironmentsList environments={environments} />}
    </div>
  );
}

function CodeView({
  owner,
  repo,
  commits,
  loading,
  branch,
}: {
  owner: string;
  repo: string;
  commits: GithubCommit[];
  loading: boolean;
  branch: string;
}) {
  if (loading) return <Spinner label="loading code" />;
  if (commits.length === 0) {
    return (
      <Blankslate title="This repository is empty">
        <p className="mb-3">Push an existing repository from the command line:</p>
        <CodeBlock>
          {`git remote add origin http://localhost:5555/${owner}/${repo}.git\ngit push -u origin ${branch}`}
        </CodeBlock>
      </Blankslate>
    );
  }
  const head = commits[0];
  return (
    <Box
      header={
        <span>
          <span style={{ color: "var(--color-fg)", fontWeight: 600 }}>{head.commit.author.name}</span>{" "}
          {head.commit.message.split("\n")[0]} ·{" "}
          <span className="font-mono">{head.sha.slice(0, 7)}</span> ·{" "}
          {new Date(head.commit.author.date).toLocaleDateString()}
        </span>
      }
    >
      <div style={{ padding: "1rem", fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
        {commits.length} commit{commits.length !== 1 ? "s" : ""} on{" "}
        <span className="inline-flex items-center gap-1">
          <BranchIcon size={14} /> {branch}
        </span>
      </div>
    </Box>
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

function WebhooksList({ hooks }: { hooks: GithubWebhook[] }) {
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
              published {new Date(r.published_at).toLocaleDateString()}
            </div>
          </div>
        </div>
      ))}
    </Box>
  );
}
