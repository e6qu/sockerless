import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { InlineError, Spinner } from "@sockerless/ui-core/components";
import { fetchOrgProfile, fetchOrgReposPage, fetchOrgMembers } from "../api.js";
import type { BleephubRepo } from "../types.js";
import { OrgHeader } from "../components/Shell.js";
import { Avatar } from "../components/Avatar.js";
import { Box, SectionLabel, Blankslate } from "../components/ui.js";
import { RepoIcon, PeopleIcon, GlobeIcon, LockIcon } from "../components/octicons.js";

export function OrgOverviewPage() {
  const { org = "" } = useParams<{ org: string }>();

  const profile = useQuery({
    queryKey: ["org-profile", org],
    queryFn: () => fetchOrgProfile(org),
  });
  const repos = useQuery({
    queryKey: ["org-overview-repos", org],
    queryFn: () => fetchOrgReposPage(org, { sort: "updated" }),
  });
  const members = useQuery({
    queryKey: ["org-overview-members", org],
    queryFn: () => fetchOrgMembers(org),
  });

  if (profile.isLoading) return <Spinner label="loading organization" />;
  if (profile.isError || !profile.data) {
    return <InlineError title="Failed to load organization" detail={String(profile.error)} />;
  }
  const p = profile.data;
  const previewRepos = repos.data?.items.slice(0, 6) ?? [];

  return (
    <div>
      <OrgHeader org={org} active="overview" />

      <div className="grid gap-6 md:grid-cols-[260px_1fr]">
        <aside className="flex flex-col gap-3">
          <div className="flex items-center gap-3">
            <Avatar login={p.login} src={p.avatar_url} size={64} square />
            <div className="min-w-0">
              <div style={{ fontSize: "1.25rem", fontWeight: 600, lineHeight: 1.2 }}>
                {p.name || p.login}
              </div>
              <div style={{ color: "var(--color-fg-muted)", fontSize: "0.9rem" }}>{p.login}</div>
            </div>
          </div>
          {p.description && (
            <p style={{ fontSize: "0.9rem", color: "var(--color-fg)" }}>{p.description}</p>
          )}
          <dl className="flex flex-col gap-1.5" style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
            <MetaRow icon={<PeopleIcon size={15} />}>
              <Link to={`/ui/orgs/${org}/people`} style={{ color: "var(--color-accent)", textDecoration: "none" }}>
                {members.data ? `${members.data.length} member${members.data.length === 1 ? "" : "s"}` : "Members"}
              </Link>
            </MetaRow>
            <MetaRow icon={<RepoIcon size={15} />}>
              <Link to={`/ui/orgs/${org}/repos`} style={{ color: "var(--color-accent)", textDecoration: "none" }}>
                {p.public_repos} repositor{p.public_repos === 1 ? "y" : "ies"}
              </Link>
            </MetaRow>
            {p.location && <MetaRow>{p.location}</MetaRow>}
            {p.blog && (
              <MetaRow icon={<GlobeIcon size={15} />}>
                <a href={p.blog} style={{ color: "var(--color-accent)", textDecoration: "none" }} rel="noreferrer">
                  {p.blog}
                </a>
              </MetaRow>
            )}
            {p.email && <MetaRow>{p.email}</MetaRow>}
          </dl>
        </aside>

        <section>
          <SectionLabel>
            <span className="inline-flex items-center gap-2">
              Repositories
              <Link
                to={`/ui/orgs/${org}/repos`}
                style={{ fontSize: "0.82rem", fontWeight: 500, color: "var(--color-accent)", textDecoration: "none" }}
              >
                View all
              </Link>
            </span>
          </SectionLabel>
          {repos.isLoading && <Spinner label="loading repositories" />}
          {repos.isError && <InlineError title="Failed to load repositories" detail={String(repos.error)} />}
          {repos.data &&
            (previewRepos.length === 0 ? (
              <Blankslate icon={<RepoIcon size={28} />} title="No repositories yet">
                This organization has no repositories.
              </Blankslate>
            ) : (
              <div className="grid gap-3 sm:grid-cols-2">
                {previewRepos.map((repo) => (
                  <RepoPreviewCard key={repo.id} repo={repo} />
                ))}
              </div>
            ))}
        </section>
      </div>
    </div>
  );
}

function MetaRow({ icon, children }: { icon?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-2">
      {icon && <span style={{ color: "var(--color-fg-subtle)" }}>{icon}</span>}
      <span>{children}</span>
    </div>
  );
}

function RepoPreviewCard({ repo }: { repo: BleephubRepo }) {
  const [owner, name] = repo.full_name.split("/");
  return (
    <Box style={{ padding: "0.85rem 1rem" }}>
      <div className="flex items-center gap-2">
        <Link
          to={`/ui/repos/${owner}/${name}`}
          style={{ color: "var(--color-accent)", fontWeight: 600, fontSize: "0.95rem", textDecoration: "none" }}
        >
          {repo.name}
        </Link>
        {repo.private && <LockIcon size={13} style={{ color: "var(--color-fg-muted)" }} />}
      </div>
      {repo.description && (
        <p className="mt-1" style={{ fontSize: "0.82rem", color: "var(--color-fg-muted)" }}>
          {repo.description}
        </p>
      )}
      <div className="mt-2" style={{ fontSize: "0.75rem", color: "var(--color-fg-muted)" }}>
        Updated {new Date(repo.updated_at).toLocaleDateString()}
      </div>
    </Box>
  );
}
