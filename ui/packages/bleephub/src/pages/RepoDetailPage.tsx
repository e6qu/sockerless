import { useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { PageHeading, Spinner, StatusBadge, InlineError } from "@sockerless/ui-core/components";
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
import type { GithubIssue, GithubPR, GithubCommit, GithubWebhook, GithubSecret, GithubEnvironment, GithubRelease } from "../types.js";
import { rowHoverProps } from "../components/RowHover.js";
import { LabelPills } from "../components/LabelPills.js";

export function RepoDetailPage() {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  const [tab, setTab] = useState<"code" | "issues" | "pulls" | "commits" | "webhooks" | "secrets" | "environments" | "releases">("code");

  const { data: repoData, isLoading: repoLoading, isError: repoError, error: repoErr } = useQuery({
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

  if (repoLoading) return <Spinner label={`loading ${owner}/${repo}`} />;
  if (repoError || !repoData) return <InlineError title={`Failed to load ${owner}/${repo}`} detail={String(repoErr)} />;

  const tabStyle = (active: boolean): React.CSSProperties => ({
    padding: "0.4rem 0.85rem",
    fontSize: "0.82rem",
    fontFamily: "var(--font-mono)",
    background: active ? "var(--color-accent-soft)" : "transparent",
    color: active ? "var(--color-accent)" : "var(--color-fg-muted)",
    border: "1px solid",
    borderColor: active ? "var(--color-accent)" : "var(--color-border)",
    borderRadius: "var(--radius-sm)",
    cursor: "pointer",
    marginRight: "0.35rem",
  });

  return (
    <div>
      <div style={{ marginBottom: "0.25rem" }}>
        <Link
          to="/ui/repos"
          style={{ color: "var(--color-fg-muted)", fontSize: "0.8rem", fontFamily: "var(--font-mono)" }}
        >
          ← Repositories
        </Link>
      </div>
      <PageHeading
        kicker={`${repoData.visibility} repo`}
        title={<>{owner} / {repo}</>}
        meta={repoData.description || "no description"}
        actions={
          <StatusBadge status={repoData.visibility} />
        }
      />

      {/* Repo meta bar */}
      <div
        style={{
          display: "flex",
          gap: "1.5rem",
          marginBottom: "1.25rem",
          fontSize: "0.8rem",
          fontFamily: "var(--font-mono)",
          color: "var(--color-fg-muted)",
          flexWrap: "wrap",
        }}
      >
        <span>
          <span style={{ color: "var(--color-accent)" }}>⎇</span>{" "}
          {branches.length > 0 ? branches.map((b) => b.name).join(", ") : repoData.default_branch}
        </span>
        <Link
          to={`/ui/repos/${owner}/${repo}/issues`}
          style={{ color: "var(--color-fg-muted)", textDecoration: "none" }}
        >
          ○ {issues.length} open issues
        </Link>
        <Link
          to={`/ui/repos/${owner}/${repo}/pulls`}
          style={{ color: "var(--color-fg-muted)", textDecoration: "none" }}
        >
          ↯ {prs.length} open PRs
        </Link>
        <span>updated {new Date(repoData.updated_at).toLocaleDateString()}</span>
      </div>

      {/* Tabs */}
      <div style={{ marginBottom: "1.25rem" }}>
        {(["code", "issues", "pulls", "commits", "webhooks", "secrets", "environments", "releases"] as const).map((t) => (
          <button key={t} style={tabStyle(tab === t)} onClick={() => setTab(t)}>
            {t === "code" ? "Code" : t === "issues" ? `Issues (${issues.length})` : t === "pulls" ? `PRs (${prs.length})` : t.charAt(0).toUpperCase() + t.slice(1) + (t === "webhooks" ? ` (${webhooks.length})` : t === "secrets" ? ` (${secrets.length})` : t === "environments" ? ` (${environments.length})` : t === "releases" ? ` (${releases.length})` : "")}
          </button>
        ))}
      </div>

      {tab === "code" && (
        <div>
          {commits.length === 0 && !commitsLoading ? (
            <div
              style={{
                padding: "2.5rem",
                textAlign: "center",
                border: "1px dashed var(--color-border)",
                borderRadius: "var(--radius-md)",
                color: "var(--color-fg-muted)",
                fontFamily: "var(--font-mono)",
                fontSize: "0.85rem",
              }}
            >
              <div style={{ fontSize: "2rem", marginBottom: "0.5rem" }}>⌥</div>
              Empty repository. Push to get started:
              <pre
                style={{
                  marginTop: "1rem",
                  padding: "0.75rem 1rem",
                  background: "var(--color-bg-subtle)",
                  border: "1px solid var(--color-border)",
                  borderRadius: "var(--radius-sm)",
                  textAlign: "left",
                  display: "inline-block",
                  fontSize: "0.78rem",
                  color: "var(--color-fg)",
                }}
              >
                {`git remote add origin http://localhost:5555/${owner}/${repo}.git\ngit push -u origin main`}
              </pre>
            </div>
          ) : (
            <div>
              <div
                style={{
                  border: "1px solid var(--color-border)",
                  borderRadius: "var(--radius-md)",
                  overflow: "hidden",
                }}
              >
                <div
                  style={{
                    padding: "0.6rem 0.85rem",
                    background: "var(--color-bg-subtle)",
                    borderBottom: "1px solid var(--color-border)",
                    fontSize: "0.78rem",
                    fontFamily: "var(--font-mono)",
                    color: "var(--color-fg-muted)",
                  }}
                >
                  Latest commit: {commits[0]?.commit.message.split("\n")[0]} ·{" "}
                  {commits[0]?.commit.author.name} ·{" "}
                  {commits[0] && new Date(commits[0].commit.author.date).toLocaleDateString()}
                </div>
                <div style={{ padding: "1rem", color: "var(--color-fg-muted)", fontFamily: "var(--font-mono)", fontSize: "0.82rem" }}>
                  {commits.length} commit{commits.length !== 1 ? "s" : ""} on {repoData.default_branch}
                </div>
              </div>
            </div>
          )}
        </div>
      )}

      {tab === "issues" && (
        <IssuesList owner={owner} repo={repo} issues={issues} />
      )}

      {tab === "pulls" && (
        <PRsList owner={owner} repo={repo} prs={prs} />
      )}

      {tab === "commits" && (
        <CommitsList commits={commits} isLoading={commitsLoading} />
      )}

      {tab === "webhooks" && (
        <WebhooksList hooks={webhooks} owner={owner} repo={repo} />
      )}

      {tab === "secrets" && (
        <SecretsList secrets={secrets} />
      )}

      {tab === "environments" && (
        <EnvironmentsList environments={environments} />
      )}

      {tab === "releases" && (
        <ReleasesList releases={releases} />
      )}
    </div>
  );
}

/** Shared bordered list for repo-scoped items (issues or PRs). */
function RepoItemList<T extends { id: number }>({
  items,
  emptyMessage,
  toPath,
  renderIcon,
  renderContent,
}: {
  items: T[];
  emptyMessage: string;
  toPath: (item: T) => string;
  renderIcon: (item: T) => React.ReactNode;
  renderContent: (item: T) => React.ReactNode;
}) {
  if (items.length === 0) {
    return (
      <div style={{ color: "var(--color-fg-muted)", fontFamily: "var(--font-mono)", fontSize: "0.85rem", padding: "2rem 0" }}>
        {emptyMessage}
      </div>
    );
  }
  return (
    <div style={{ border: "1px solid var(--color-border)", borderRadius: "var(--radius-md)", overflow: "hidden" }}>
      {items.map((item, i) => (
        <Link
          key={item.id}
          to={toPath(item)}
          style={{
            display: "flex",
            alignItems: "flex-start",
            gap: "0.75rem",
            padding: "0.85rem 1rem",
            borderBottom: i < items.length - 1 ? "1px solid var(--color-border)" : "none",
            textDecoration: "none",
            background: "var(--color-surface-raised)",
            transition: "background 0.1s",
          }}
          {...rowHoverProps}
        >
          {renderIcon(item)}
          {renderContent(item)}
        </Link>
      ))}
    </div>
  );
}

function IssuesList({ owner, repo, issues }: { owner: string; repo: string; issues: GithubIssue[] }) {
  return (
    <RepoItemList
      items={issues}
      emptyMessage="No open issues."
      toPath={(issue) => `/ui/repos/${owner}/${repo}/issues/${issue.number}`}
      renderIcon={() => <span style={{ color: "var(--color-status-ok)", marginTop: "0.1rem" }}>○</span>}
      renderContent={(issue) => (
        <>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontWeight: 500, color: "var(--color-fg)", fontSize: "0.9rem" }}>{issue.title}</div>
            <div style={{ fontSize: "0.75rem", color: "var(--color-fg-muted)", fontFamily: "var(--font-mono)", marginTop: "0.2rem" }}>
              #{issue.number} opened by {issue.user?.login} · {issue.comments} comments
            </div>
          </div>
          <LabelPills labels={issue.labels} />
        </>
      )}
    />
  );
}

function PRsList({ owner, repo, prs }: { owner: string; repo: string; prs: GithubPR[] }) {
  return (
    <RepoItemList
      items={prs}
      emptyMessage="No open pull requests."
      toPath={(pr) => `/ui/repos/${owner}/${repo}/pulls/${pr.number}`}
      renderIcon={() => <span style={{ color: "var(--color-status-ok)", marginTop: "0.1rem" }}>↯</span>}
      renderContent={(pr) => (
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontWeight: 500, color: "var(--color-fg)", fontSize: "0.9rem" }}>
            {pr.title}{pr.draft && <span style={{ color: "var(--color-fg-subtle)", fontSize: "0.75rem" }}> [draft]</span>}
          </div>
          <div style={{ fontSize: "0.75rem", color: "var(--color-fg-muted)", fontFamily: "var(--font-mono)", marginTop: "0.2rem" }}>
            #{pr.number} · {pr.head.ref} → {pr.base.ref} · opened by {pr.user?.login}
          </div>
        </div>
      )}
    />
  );
}

function CommitsList({
  commits,
  isLoading,
}: {
  commits: GithubCommit[];
  isLoading: boolean;
}) {
  if (isLoading) return <Spinner label="loading commits" />;
  if (commits.length === 0) return (
    <div style={{ color: "var(--color-fg-muted)", fontFamily: "var(--font-mono)", fontSize: "0.85rem", padding: "2rem 0" }}>
      No commits yet.
    </div>
  );
  return (
    <div style={{ border: "1px solid var(--color-border)", borderRadius: "var(--radius-md)", overflow: "hidden" }}>
      {commits.map((c, i) => (
        <div
          key={c.sha}
          style={{
            display: "flex",
            alignItems: "center",
            gap: "0.75rem",
            padding: "0.7rem 1rem",
            borderBottom: i < commits.length - 1 ? "1px solid var(--color-border)" : "none",
            background: "var(--color-surface-raised)",
          }}
        >
          <span
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: "0.72rem",
              color: "var(--color-accent)",
              background: "var(--color-accent-soft)",
              padding: "0.1rem 0.4rem",
              borderRadius: "var(--radius-sm)",
              whiteSpace: "nowrap",
            }}
          >
            {c.sha.slice(0, 7)}
          </span>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div
              style={{
                fontSize: "0.85rem",
                color: "var(--color-fg)",
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
              }}
            >
              {c.commit.message.split("\n")[0]}
            </div>
            <div style={{ fontSize: "0.72rem", color: "var(--color-fg-subtle)", fontFamily: "var(--font-mono)", marginTop: "0.1rem" }}>
              {c.commit.author.name} · {new Date(c.commit.author.date).toLocaleDateString()}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

function WebhooksList({ hooks, owner, repo }: { hooks: GithubWebhook[]; owner: string; repo: string }) {
  if (hooks.length === 0) return (
    <div style={{ color: "var(--color-fg-muted)", fontFamily: "var(--font-mono)", fontSize: "0.85rem", padding: "2rem 0" }}>
      No webhooks configured.
    </div>
  );
  return (
    <div style={{ border: "1px solid var(--color-border)", borderRadius: "var(--radius-md)", overflow: "hidden" }}>
      {hooks.map((h, i) => (
        <div
          key={h.id}
          style={{
            display: "flex",
            alignItems: "center",
            gap: "0.75rem",
            padding: "0.85rem 1rem",
            borderBottom: i < hooks.length - 1 ? "1px solid var(--color-border)" : "none",
            background: "var(--color-surface-raised)",
          }}
        >
          <span style={{ color: h.active ? "var(--color-status-ok)" : "var(--color-fg-subtle)", fontFamily: "var(--font-mono)", fontSize: "0.85rem" }}>
            {h.active ? "●" : "○"}
          </span>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontWeight: 500, color: "var(--color-fg)", fontSize: "0.9rem" }}>
              {h.name} <span style={{ color: "var(--color-fg-subtle)", fontSize: "0.75rem" }}>#{h.id}</span>
            </div>
            <div style={{ fontSize: "0.72rem", color: "var(--color-fg-subtle)", fontFamily: "var(--font-mono)" }}>
              {h.config?.url || "no url"} · events: {h.events?.join(", ") || "none"}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

function SecretsList({ secrets }: { secrets: GithubSecret[] }) {
  if (secrets.length === 0) return (
    <div style={{ color: "var(--color-fg-muted)", fontFamily: "var(--font-mono)", fontSize: "0.85rem", padding: "2rem 0" }}>
      No secrets configured.
    </div>
  );
  return (
    <div style={{ border: "1px solid var(--color-border)", borderRadius: "var(--radius-md)", overflow: "hidden" }}>
      {secrets.map((s, i) => (
        <div
          key={s.name}
          style={{
            padding: "0.7rem 1rem",
            borderBottom: i < secrets.length - 1 ? "1px solid var(--color-border)" : "none",
            fontFamily: "var(--font-mono)",
            fontSize: "0.85rem",
            color: "var(--color-fg)",
          }}
        >
          🔒 {s.name}
        </div>
      ))}
    </div>
  );
}

function EnvironmentsList({ environments }: { environments: GithubEnvironment[] }) {
  if (environments.length === 0) return (
    <div style={{ color: "var(--color-fg-muted)", fontFamily: "var(--font-mono)", fontSize: "0.85rem", padding: "2rem 0" }}>
      No environments.
    </div>
  );
  return (
    <div style={{ border: "1px solid var(--color-border)", borderRadius: "var(--radius-md)", overflow: "hidden" }}>
      {environments.map((e, i) => (
        <div
          key={e.name}
          style={{
            padding: "0.7rem 1rem",
            borderBottom: i < environments.length - 1 ? "1px solid var(--color-border)" : "none",
            fontFamily: "var(--font-mono)",
            fontSize: "0.85rem",
            color: "var(--color-fg)",
          }}
        >
          {e.name}
        </div>
      ))}
    </div>
  );
}

function ReleasesList({ releases }: { releases: GithubRelease[] }) {
  if (releases.length === 0) return (
    <div style={{ color: "var(--color-fg-muted)", fontFamily: "var(--font-mono)", fontSize: "0.85rem", padding: "2rem 0" }}>
      No releases.
    </div>
  );
  return (
    <div style={{ border: "1px solid var(--color-border)", borderRadius: "var(--radius-md)", overflow: "hidden" }}>
      {releases.map((r, i) => (
        <div
          key={r.id}
          style={{
            display: "flex",
            alignItems: "center",
            gap: "0.75rem",
            padding: "0.85rem 1rem",
            borderBottom: i < releases.length - 1 ? "1px solid var(--color-border)" : "none",
            background: "var(--color-surface-raised)",
          }}
        >
          <span
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: "0.72rem",
              color: "var(--color-accent)",
              background: "var(--color-accent-soft)",
              padding: "0.1rem 0.4rem",
              borderRadius: "var(--radius-sm)",
              whiteSpace: "nowrap",
            }}
          >
            {r.tag_name}
          </span>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontWeight: 500, color: "var(--color-fg)", fontSize: "0.9rem" }}>
              {r.name || r.tag_name}
            </div>
            <div style={{ fontSize: "0.72rem", color: "var(--color-fg-subtle)", fontFamily: "var(--font-mono)" }}>
              published {new Date(r.published_at).toLocaleDateString()}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}
