import { useState } from "react";
import { Link, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchRepoForksPage,
  fetchRepoStargazers,
  fetchRepoSubscribersPage,
  type Page,
} from "../api.js";
import type { BleephubRepo, GithubAccount } from "../types.js";
import { RepoHeader } from "../components/Shell.js";
import { PageTitle, Box, Blankslate, Button } from "../components/ui.js";
import { StarIcon, RepoIcon } from "../components/octicons.js";

export type RepoSocialKind = "stargazers" | "watchers" | "forks";

const TITLES: Record<RepoSocialKind, string> = {
  stargazers: "Stargazers",
  watchers: "Watchers",
  forks: "Forks",
};

/** Simple list views behind a repo's star/watch/fork counters. */
export function RepoSocialPage({ kind }: { kind: RepoSocialKind }) {
  const { owner = "", repo = "" } = useParams<{ owner: string; repo: string }>();
  return (
    <div>
      <RepoHeader owner={owner} repo={repo} active="code" />
      <PageTitle title={TITLES[kind]} meta={`${owner}/${repo}`} />
      {kind === "stargazers" && <StargazersList owner={owner} repo={repo} />}
      {kind === "watchers" && <WatchersList owner={owner} repo={repo} />}
      {kind === "forks" && <ForksList owner={owner} repo={repo} />}
    </div>
  );
}

function UserRows({ users }: { users: GithubAccount[] }) {
  return (
    <Box>
      {users.map((u, i) => (
        <div
          key={u.login}
          className="flex items-center gap-3"
          style={{
            padding: "0.65rem 1rem",
            fontSize: "0.88rem",
            borderBottom: i < users.length - 1 ? "1px solid var(--color-border)" : "none",
          }}
        >
          <span style={{ fontWeight: 500 }}>{u.login}</span>
          <span style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}>{u.type}</span>
        </div>
      ))}
    </Box>
  );
}

function StargazersList({ owner, repo }: { owner: string; repo: string }) {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["repo-stargazers", owner, repo],
    queryFn: () => fetchRepoStargazers(owner, repo),
    enabled: !!owner && !!repo,
  });
  if (isLoading) return <Spinner label="loading stargazers" />;
  if (isError || !data)
    return <InlineError title="Failed to load stargazers" detail={String(error)} />;
  if (data.length === 0)
    return (
      <Blankslate icon={<StarIcon size={26} />} title="No stargazers yet">
        <p>Nobody has starred this repository.</p>
      </Blankslate>
    );
  return <UserRows users={data} />;
}

/** Link-paginated user list accumulated across "Load more" clicks. */
function PagedUserList({
  label,
  emptyTitle,
  firstPage,
  fetchNext,
}: {
  label: string;
  emptyTitle: string;
  firstPage: () => Promise<Page<GithubAccount>>;
  fetchNext: (pageUrl: string) => Promise<Page<GithubAccount>>;
}) {
  const [extra, setExtra] = useState<GithubAccount[]>([]);
  const [nextUrl, setNextUrl] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState<string | null>(null);

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["repo-social-page", label],
    queryFn: async () => {
      const page = await firstPage();
      setExtra([]);
      setNextUrl(page.nextUrl);
      return page;
    },
  });

  if (isLoading) return <Spinner label={`loading ${label}`} />;
  if (isError || !data)
    return <InlineError title={`Failed to load ${label}`} detail={String(error)} />;

  const users = [...data.items, ...extra];
  if (users.length === 0) return <Blankslate title={emptyTitle} />;

  const loadMore = async () => {
    if (!nextUrl) return;
    setLoadingMore(true);
    setMoreError(null);
    try {
      const page = await fetchNext(nextUrl);
      setExtra((prev) => [...prev, ...page.items]);
      setNextUrl(page.nextUrl);
    } catch (err) {
      setMoreError(String(err));
    } finally {
      setLoadingMore(false);
    }
  };

  return (
    <div>
      {moreError && <InlineError title={`Failed to load more ${label}`} detail={moreError} />}
      <UserRows users={users} />
      {nextUrl && (
        <div className="mt-3 flex justify-center">
          <Button size="sm" variant="secondary" onClick={loadMore} disabled={loadingMore}>
            {loadingMore ? "Loading…" : "Load more"}
          </Button>
        </div>
      )}
    </div>
  );
}

function WatchersList({ owner, repo }: { owner: string; repo: string }) {
  return (
    <PagedUserList
      label={`watchers ${owner}/${repo}`}
      emptyTitle="No watchers yet"
      firstPage={() => fetchRepoSubscribersPage(owner, repo)}
      fetchNext={(pageUrl) => fetchRepoSubscribersPage(owner, repo, pageUrl)}
    />
  );
}

function ForksList({ owner, repo }: { owner: string; repo: string }) {
  const [extra, setExtra] = useState<BleephubRepo[]>([]);
  const [nextUrl, setNextUrl] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState<string | null>(null);

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["repo-forks", owner, repo],
    queryFn: async () => {
      const page = await fetchRepoForksPage(owner, repo);
      setExtra([]);
      setNextUrl(page.nextUrl);
      return page;
    },
    enabled: !!owner && !!repo,
  });

  if (isLoading) return <Spinner label="loading forks" />;
  if (isError || !data) return <InlineError title="Failed to load forks" detail={String(error)} />;

  const forks = [...data.items, ...extra];
  if (forks.length === 0)
    return (
      <Blankslate icon={<RepoIcon size={26} />} title="No forks yet">
        <p>Nobody has forked this repository.</p>
      </Blankslate>
    );

  const loadMore = async () => {
    if (!nextUrl) return;
    setLoadingMore(true);
    setMoreError(null);
    try {
      const page = await fetchRepoForksPage(owner, repo, nextUrl);
      setExtra((prev) => [...prev, ...page.items]);
      setNextUrl(page.nextUrl);
    } catch (err) {
      setMoreError(String(err));
    } finally {
      setLoadingMore(false);
    }
  };

  return (
    <div>
      {moreError && <InlineError title="Failed to load more forks" detail={moreError} />}
      <Box>
        {forks.map((f, i) => (
          <div
            key={f.full_name}
            className="flex items-center gap-3"
            style={{
              padding: "0.65rem 1rem",
              fontSize: "0.88rem",
              borderBottom: i < forks.length - 1 ? "1px solid var(--color-border)" : "none",
            }}
          >
            <RepoIcon size={14} style={{ color: "var(--color-fg-muted)" }} />
            <Link
              to={`/ui/repos/${f.full_name}`}
              style={{ color: "var(--color-accent)", fontWeight: 500, textDecoration: "none", flex: 1 }}
            >
              {f.full_name}
            </Link>
            <span style={{ fontSize: "0.76rem", color: "var(--color-fg-muted)" }}>
              updated {new Date(f.updated_at).toLocaleDateString()}
            </span>
          </div>
        ))}
      </Box>
      {nextUrl && (
        <div className="mt-3 flex justify-center">
          <Button size="sm" variant="secondary" onClick={loadMore} disabled={loadingMore}>
            {loadingMore ? "Loading…" : "Load more"}
          </Button>
        </div>
      )}
    </div>
  );
}
