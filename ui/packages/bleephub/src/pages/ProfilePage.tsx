import { useMemo, useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { InlineError, Spinner } from "@sockerless/ui-core/components";
import {
  fetchUserProfile,
  fetchUserReposByLoginPage,
  fetchUserOrgsByLogin,
} from "../api.js";
import type { BleephubRepo, GithubOrgSummary, GithubUserProfile } from "../types.js";
import { Avatar } from "../components/Avatar.js";
import { Box, SectionLabel, Blankslate, Button } from "../components/ui.js";
import {
  RepoIcon,
  BranchIcon,
  PeopleIcon,
  GlobeIcon,
  OrganizationIcon,
  LockIcon,
} from "../components/octicons.js";

export function ProfilePage() {
  const { login = "" } = useParams<{ login: string }>();

  const profile = useQuery({
    queryKey: ["user-profile", login],
    queryFn: () => fetchUserProfile(login),
  });
  const orgs = useQuery({
    queryKey: ["user-orgs", login],
    queryFn: () => fetchUserOrgsByLogin(login),
  });

  if (profile.isLoading) return <Spinner label="loading profile" />;
  if (profile.isError || !profile.data) {
    return <InlineError title="Failed to load profile" detail={String(profile.error)} />;
  }

  return (
    <div className="grid gap-6 md:grid-cols-[296px_1fr]">
      <ProfileSidebar profile={profile.data} orgs={orgs.data} />
      <ProfileRepos login={login} />
    </div>
  );
}

function ProfileSidebar({
  profile,
  orgs,
}: {
  profile: GithubUserProfile;
  orgs?: GithubOrgSummary[];
}) {
  const p = profile;
  return (
    <aside className="flex flex-col gap-3">
      <Avatar login={p.login} src={p.avatar_url} size={200} />
      <div>
        <div style={{ fontSize: "1.5rem", fontWeight: 600, lineHeight: 1.2 }}>{p.name || p.login}</div>
        <div style={{ fontSize: "1.15rem", color: "var(--color-fg-muted)", fontWeight: 300 }}>{p.login}</div>
      </div>
      {p.bio && <p style={{ fontSize: "0.9rem", color: "var(--color-fg)" }}>{p.bio}</p>}
      <div className="flex items-center gap-2" style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
        <PeopleIcon size={15} />
        <span>
          <strong style={{ color: "var(--color-fg)" }}>{p.followers}</strong> followers
        </span>
        <span>·</span>
        <span>
          <strong style={{ color: "var(--color-fg)" }}>{p.following}</strong> following
        </span>
      </div>
      <dl className="flex flex-col gap-1.5" style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
        {p.company && <MetaRow>{p.company}</MetaRow>}
        {p.location && <MetaRow>{p.location}</MetaRow>}
        {p.email && <MetaRow>{p.email}</MetaRow>}
        {p.blog && (
          <MetaRow icon={<GlobeIcon size={15} />}>
            <a href={p.blog} style={{ color: "var(--color-accent)", textDecoration: "none" }} rel="noreferrer">
              {p.blog}
            </a>
          </MetaRow>
        )}
        {p.twitter_username && <MetaRow>@{p.twitter_username}</MetaRow>}
      </dl>
      {orgs && orgs.length > 0 && (
        <div>
          <SectionLabel>Organizations</SectionLabel>
          <div className="flex flex-wrap gap-2">
            {orgs.map((o) => (
              <Link key={o.id} to={`/ui/orgs/${o.login}`} title={o.login}>
                <Avatar login={o.login} src={o.avatar_url} size={32} square />
              </Link>
            ))}
          </div>
        </div>
      )}
      <div style={{ fontSize: "0.78rem", color: "var(--color-fg-subtle)" }}>
        Joined {new Date(p.created_at).toLocaleDateString()}
      </div>
    </aside>
  );
}

function MetaRow({ icon, children }: { icon?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-2">
      {icon && <span style={{ color: "var(--color-fg-subtle)" }}>{icon}</span>}
      <span className="min-w-0 break-words">{children}</span>
    </div>
  );
}

function ProfileRepos({ login }: { login: string }) {
  const [filter, setFilter] = useState("");
  const [pageUrl, setPageUrl] = useState<string | undefined>(undefined);
  const [pageStack, setPageStack] = useState<string[]>([]);

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["user-profile-repos", login, pageUrl ?? "first"],
    queryFn: () => fetchUserReposByLoginPage(login, { sort: "updated" }, pageUrl),
  });

  const filtered = useMemo(() => {
    if (!data) return [];
    const q = filter.trim().toLowerCase();
    if (!q) return data.items;
    return data.items.filter(
      (r) => r.name.toLowerCase().includes(q) || (r.description ?? "").toLowerCase().includes(q),
    );
  }, [data, filter]);

  const goNext = () => {
    if (!data?.nextUrl) return;
    setPageStack((s) => [...s, pageUrl ?? ""]);
    setPageUrl(data.nextUrl);
  };
  const goPrev = () => {
    setPageStack((s) => {
      const prev = s[s.length - 1];
      setPageUrl(prev || undefined);
      return s.slice(0, -1);
    });
  };

  return (
    <section>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <SectionLabel>Repositories</SectionLabel>
        <input
          type="search"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Find a repository…"
          aria-label="Find a repository"
          style={{ fontSize: "0.82rem", minWidth: "14rem" }}
        />
      </div>

      {isLoading && <Spinner label="loading repositories" />}
      {isError && <InlineError title="Failed to load repositories" detail={String(error)} />}
      {data &&
        (data.items.length === 0 ? (
          <Blankslate icon={<RepoIcon size={28} />} title="No repositories">
            This user has no repositories.
          </Blankslate>
        ) : filtered.length === 0 ? (
          <Blankslate icon={<RepoIcon size={28} />} title="No matches">
            No repository matches “{filter}”.
          </Blankslate>
        ) : (
          <ul style={{ borderTop: "1px solid var(--color-border)" }}>
            {filtered.map((repo) => (
              <ProfileRepoRow key={repo.id} repo={repo} />
            ))}
          </ul>
        ))}

      {(pageStack.length > 0 || data?.nextUrl) && (
        <div className="mt-4 flex items-center gap-2">
          <Button onClick={goPrev} disabled={pageStack.length === 0}>
            Previous
          </Button>
          <Button onClick={goNext} disabled={!data?.nextUrl}>
            Next
          </Button>
        </div>
      )}
    </section>
  );
}

function ProfileRepoRow({ repo }: { repo: BleephubRepo }) {
  const [owner, name] = repo.full_name.split("/");
  return (
    <li style={{ padding: "0.9rem 0", borderBottom: "1px solid var(--color-border)" }}>
      <div className="flex items-center gap-2">
        <Link
          to={`/ui/repos/${owner}/${name}`}
          style={{ color: "var(--color-accent)", fontWeight: 600, fontSize: "1rem", textDecoration: "none" }}
        >
          {repo.name}
        </Link>
        <span
          className="inline-flex items-center gap-1"
          style={{
            fontSize: "0.68rem",
            fontWeight: 500,
            color: "var(--color-fg-muted)",
            border: "1px solid var(--color-border)",
            borderRadius: "2rem",
            padding: "0.05rem 0.5rem",
            textTransform: "capitalize",
          }}
        >
          {repo.private && <LockIcon size={11} />}
          {repo.private ? "Private" : repo.visibility || "Public"}
        </span>
        {repo.organization && (
          <span className="inline-flex items-center gap-1" style={{ fontSize: "0.72rem", color: "var(--color-fg-subtle)" }}>
            <OrganizationIcon size={12} /> {repo.organization.login}
          </span>
        )}
      </div>
      {repo.description && (
        <p className="mt-1" style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)", maxWidth: "44rem" }}>
          {repo.description}
        </p>
      )}
      <div
        className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1"
        style={{ fontSize: "0.76rem", color: "var(--color-fg-muted)" }}
      >
        <span className="inline-flex items-center gap-1">
          <BranchIcon size={13} /> {repo.default_branch}
        </span>
        <span>Updated {new Date(repo.updated_at).toLocaleDateString()}</span>
      </div>
    </li>
  );
}
