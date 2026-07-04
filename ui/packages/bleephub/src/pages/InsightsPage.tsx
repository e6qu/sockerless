import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchRepoContributors,
  fetchCommunityProfile,
  fetchCommitActivity,
  fetchTrafficViews,
  fetchTrafficClones,
  fetchTrafficPopularPaths,
  fetchTrafficPopularReferrers,
} from "../api.js";
import type { GithubCommunityProfile, GithubTrafficBucket } from "../types.js";
import { RepoHeader } from "../components/Shell.js";
import { useOpenCounts } from "../hooks/useOpenCounts.js";
import { Box, Blankslate, SectionLabel, StatCard } from "../components/ui.js";
import { GraphIcon, PeopleIcon, CheckCircleIcon, XCircleIcon } from "../components/octicons.js";

export function InsightsPage() {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  const counts = useOpenCounts(owner, repo);

  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="insights" {...counts} />
      <div className="flex flex-col gap-6">
        <CommunityProfileSection owner={owner} repo={repo} />
        <ContributorsSection owner={owner} repo={repo} />
        <CommitActivitySection owner={owner} repo={repo} />
        <TrafficSection owner={owner} repo={repo} />
        <PopularContentSection owner={owner} repo={repo} />
      </div>
    </div>
  );
}

const COMMUNITY_CHECKS: { key: keyof GithubCommunityProfile["files"]; label: string }[] = [
  { key: "readme", label: "README" },
  { key: "license", label: "License" },
  { key: "contributing", label: "Contributing guidelines" },
  { key: "code_of_conduct_file", label: "Code of conduct" },
  { key: "issue_template", label: "Issue template" },
  { key: "pull_request_template", label: "Pull request template" },
];

function CommunityProfileSection({ owner, repo }: { owner: string; repo: string }) {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["community-profile", owner, repo],
    queryFn: () => fetchCommunityProfile(owner, repo),
  });

  return (
    <section>
      <SectionLabel>Community profile</SectionLabel>
      {isLoading && <Spinner label="loading community profile" />}
      {isError && <InlineError title="Failed to load community profile" detail={String(error)} />}
      {data && (
        <div className="grid gap-3 sm:grid-cols-[12rem_1fr]">
          <StatCard title="Health score" value={`${data.health_percentage}%`} emphasized />
          <Box>
            <ul className="grid gap-x-6 sm:grid-cols-2" style={{ listStyle: "none", margin: 0, padding: "0.75rem 1rem" }}>
              <li className="flex items-center gap-2 py-1" style={{ fontSize: "0.85rem" }}>
                {data.description ? (
                  <CheckCircleIcon size={15} style={{ color: "var(--gh-open)" }} />
                ) : (
                  <XCircleIcon size={15} style={{ color: "var(--color-fg-subtle)" }} />
                )}
                Description
              </li>
              {COMMUNITY_CHECKS.map((check) => (
                <li key={check.key} className="flex items-center gap-2 py-1" style={{ fontSize: "0.85rem" }}>
                  {data.files[check.key] ? (
                    <CheckCircleIcon size={15} style={{ color: "var(--gh-open)" }} />
                  ) : (
                    <XCircleIcon size={15} style={{ color: "var(--color-fg-subtle)" }} />
                  )}
                  {check.label}
                </li>
              ))}
            </ul>
          </Box>
        </div>
      )}
    </section>
  );
}

function ContributorsSection({ owner, repo }: { owner: string; repo: string }) {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["contributors", owner, repo],
    queryFn: () => fetchRepoContributors(owner, repo),
  });

  return (
    <section>
      <SectionLabel>Contributors</SectionLabel>
      {isLoading && <Spinner label="loading contributors" />}
      {isError && <InlineError title="Failed to load contributors" detail={String(error)} />}
      {data &&
        (data.length === 0 ? (
          <Blankslate icon={<PeopleIcon size={26} />} title="No contributors yet">
            Contributors appear once the default branch has commits.
          </Blankslate>
        ) : (
          <Box>
            {data.map((c, i) => (
              <div
                key={c.login ?? `${c.name}<${c.email}>`}
                className="flex items-center justify-between gap-3"
                style={{
                  padding: "0.6rem 1rem",
                  borderBottom: i < data.length - 1 ? "1px solid var(--color-border)" : "none",
                }}
              >
                <span style={{ fontWeight: 500, fontSize: "0.9rem" }}>
                  {c.login ? `@${c.login}` : `${c.name} <${c.email}>`}
                  {c.type === "Anonymous" && (
                    <span style={{ marginLeft: "0.5rem", color: "var(--color-fg-muted)", fontWeight: 400, fontSize: "0.78rem" }}>
                      anonymous
                    </span>
                  )}
                </span>
                <span className="tabular-nums" style={{ color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
                  {c.contributions} commit{c.contributions === 1 ? "" : "s"}
                </span>
              </div>
            ))}
          </Box>
        ))}
    </section>
  );
}

function CommitActivitySection({ owner, repo }: { owner: string; repo: string }) {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["commit-activity", owner, repo],
    queryFn: () => fetchCommitActivity(owner, repo),
  });

  const total = data?.reduce((sum, w) => sum + w.total, 0) ?? 0;
  const max = data?.reduce((m, w) => Math.max(m, w.total), 0) ?? 0;

  return (
    <section>
      <SectionLabel>Commit activity (last 52 weeks)</SectionLabel>
      {isLoading && <Spinner label="loading commit activity" />}
      {isError && <InlineError title="Failed to load commit activity" detail={String(error)} />}
      {data &&
        (total === 0 ? (
          <Blankslate icon={<GraphIcon size={26} />} title="No commits in the last year" />
        ) : (
          <Box>
            <div style={{ padding: "1rem" }}>
              <div style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)", marginBottom: "0.6rem" }}>
                {total} commit{total === 1 ? "" : "s"} on the default branch
              </div>
              <div className="flex items-end gap-px" style={{ height: "4rem" }} role="img" aria-label={`Weekly commit counts, most recent week last; peak week has ${max} commits`}>
                {data.map((week) => (
                  <div
                    key={week.week}
                    title={`Week of ${new Date(week.week * 1000).toLocaleDateString()}: ${week.total} commit${week.total === 1 ? "" : "s"}`}
                    style={{
                      flex: 1,
                      minWidth: "2px",
                      height: `${max > 0 ? Math.max((week.total / max) * 100, week.total > 0 ? 4 : 0) : 0}%`,
                      background: week.total > 0 ? "var(--color-accent)" : "transparent",
                      borderRadius: "1px 1px 0 0",
                    }}
                  />
                ))}
              </div>
            </div>
          </Box>
        ))}
    </section>
  );
}

function TrafficBucketList({ buckets, noun }: { buckets: GithubTrafficBucket[]; noun: string }) {
  if (buckets.length === 0) {
    return (
      <div style={{ padding: "0.75rem 1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
        No {noun} in the last 14 days.
      </div>
    );
  }
  return (
    <div>
      {buckets.map((b, i) => (
        <div
          key={b.timestamp}
          className="flex items-center justify-between gap-3"
          style={{
            padding: "0.5rem 1rem",
            fontSize: "0.85rem",
            borderBottom: i < buckets.length - 1 ? "1px solid var(--color-border)" : "none",
          }}
        >
          <span>{new Date(b.timestamp).toLocaleDateString()}</span>
          <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
            {b.count} ({b.uniques} unique)
          </span>
        </div>
      ))}
    </div>
  );
}

function TrafficSection({ owner, repo }: { owner: string; repo: string }) {
  const views = useQuery({
    queryKey: ["traffic-views", owner, repo],
    queryFn: () => fetchTrafficViews(owner, repo),
  });
  const clones = useQuery({
    queryKey: ["traffic-clones", owner, repo],
    queryFn: () => fetchTrafficClones(owner, repo),
  });

  return (
    <section>
      <SectionLabel>Traffic (last 14 days)</SectionLabel>
      {(views.isLoading || clones.isLoading) && <Spinner label="loading traffic" />}
      {views.isError && <InlineError title="Failed to load view traffic" detail={String(views.error)} />}
      {clones.isError && <InlineError title="Failed to load clone traffic" detail={String(clones.error)} />}
      {views.data && clones.data && (
        <>
          <div className="mb-3 grid gap-3 sm:grid-cols-4">
            <StatCard title="Views" value={views.data.count} />
            <StatCard title="Unique visitors" value={views.data.uniques} />
            <StatCard title="Clones" value={clones.data.count} />
            <StatCard title="Unique cloners" value={clones.data.uniques} />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <Box header={<span style={{ fontWeight: 600 }}>Views by day</span>}>
              <TrafficBucketList buckets={views.data.views} noun="views" />
            </Box>
            <Box header={<span style={{ fontWeight: 600 }}>Clones by day</span>}>
              <TrafficBucketList buckets={clones.data.clones} noun="clones" />
            </Box>
          </div>
        </>
      )}
    </section>
  );
}

function PopularContentSection({ owner, repo }: { owner: string; repo: string }) {
  const paths = useQuery({
    queryKey: ["traffic-paths", owner, repo],
    queryFn: () => fetchTrafficPopularPaths(owner, repo),
  });
  const referrers = useQuery({
    queryKey: ["traffic-referrers", owner, repo],
    queryFn: () => fetchTrafficPopularReferrers(owner, repo),
  });

  return (
    <section>
      <SectionLabel>Popular content</SectionLabel>
      {(paths.isLoading || referrers.isLoading) && <Spinner label="loading popular content" />}
      {paths.isError && <InlineError title="Failed to load popular paths" detail={String(paths.error)} />}
      {referrers.isError && (
        <InlineError title="Failed to load referrers" detail={String(referrers.error)} />
      )}
      {paths.data && referrers.data && (
        <div className="grid gap-3 sm:grid-cols-2">
          <Box header={<span style={{ fontWeight: 600 }}>Popular paths</span>}>
            {paths.data.length === 0 ? (
              <div style={{ padding: "0.75rem 1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
                No path traffic recorded.
              </div>
            ) : (
              paths.data.map((p, i) => (
                <div
                  key={p.path}
                  className="flex items-center justify-between gap-3"
                  style={{
                    padding: "0.5rem 1rem",
                    fontSize: "0.85rem",
                    borderBottom: i < paths.data.length - 1 ? "1px solid var(--color-border)" : "none",
                  }}
                >
                  <span className="min-w-0 truncate">{p.path}</span>
                  <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
                    {p.count} ({p.uniques} unique)
                  </span>
                </div>
              ))
            )}
          </Box>
          <Box header={<span style={{ fontWeight: 600 }}>Referring sites</span>}>
            {referrers.data.length === 0 ? (
              <div style={{ padding: "0.75rem 1rem", color: "var(--color-fg-muted)", fontSize: "0.85rem" }}>
                No referrer traffic recorded.
              </div>
            ) : (
              referrers.data.map((r, i) => (
                <div
                  key={r.referrer}
                  className="flex items-center justify-between gap-3"
                  style={{
                    padding: "0.5rem 1rem",
                    fontSize: "0.85rem",
                    borderBottom: i < referrers.data.length - 1 ? "1px solid var(--color-border)" : "none",
                  }}
                >
                  <span className="min-w-0 truncate">{r.referrer}</span>
                  <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
                    {r.count} ({r.uniques} unique)
                  </span>
                </div>
              ))
            )}
          </Box>
        </div>
      )}
    </section>
  );
}
