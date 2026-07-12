import { type ReactNode, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { NavLink, Link, useLocation, useNavigate } from "react-router";
import { useTheme } from "@sockerless/ui-core/hooks";
import {
  RepoIcon,
  CommentIcon,
  PullRequestIcon,
  PlayIcon,
  GearIcon,
  PeopleIcon,
  OrganizationIcon,
  TeamIcon,
  AuditLogIcon,
  ServerIcon,
  GistIcon,
  ProjectIcon,
  LockIcon,
  MigrationIcon,
  CodespaceIcon,
  PackageIcon,
  DiscussionIcon,
  NotificationBellIcon,
  GraphIcon,
  GlobeIcon,
  WebhookIcon,
  SearchIcon,
  KeyIcon,
  StarIcon,
  EyeIcon,
  RepoForkedIcon,
} from "./octicons.js";
import { Counter } from "./ui.js";
import { AppHeader } from "./AppHeader.js";
import {
  fetchRepoSocialCounts,
  fetchRepoViewerState,
  fetchAuthenticatedUserOrgs,
  fetchCurrentUser,
  forkRepo,
  setRepoSubscription,
  starRepo,
  unstarRepo,
} from "../api.js";

/**
 * App chrome: the GitHub-faithful global header ({@link AppHeader}) above the
 * routed page content. The header owns the brand, global search, create menu,
 * Issues / Pull requests, notifications, and the user menu — bleephub's
 * server-operational surfaces live in the header's "Operations" drawer section,
 * not in the primary GitHub-shaped nav.
 */
export function BleephubShell({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-screen" style={{ background: "var(--color-bg)", color: "var(--color-fg)" }}>
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only"
        style={{
          position: "absolute",
          top: 8,
          left: 8,
          padding: "0.4rem 0.75rem",
          background: "var(--color-accent)",
          color: "var(--color-accent-fg)",
          fontSize: "0.8rem",
          zIndex: 100,
          borderRadius: "var(--radius-md)",
        }}
      >
        Skip to main content
      </a>
      <AppHeader />
      <main id="main-content" tabIndex={-1} className="mx-auto max-w-[1280px] px-4 py-6">
        {children}
      </main>
    </div>
  );
}

// ─── Repo context header + tabs ────────────────────────────────────────

export type RepoTab = "code" | "issues" | "pulls" | "actions" | "projects-classic" | "discussions" | "insights" | "security" | "settings";

/**
 * Repo context bar: "owner / repo" breadcrumb above the GitHub-style tab
 * row (Code / Issues / Pull requests). Rendered by the repo pages, which
 * already hold the open-issue / open-PR counts shown on the tab badges.
 */
export function RepoHeader({
  owner,
  repo,
  active,
  issueCount,
  prCount,
}: {
  owner: string;
  repo: string;
  active: RepoTab;
  /** number when exact; "N+" when the server reports further pages. */
  issueCount?: number | string;
  prCount?: number | string;
}) {
  const base = `/ui/repos/${owner}/${repo}`;
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const onSecurity = active === "security" || location.pathname.startsWith(`${base}/security/`);
  const [forkOpen, setForkOpen] = useState(false);
  const [forkOwner, setForkOwner] = useState("");
  const socialKey = ["repo-social-counts", owner, repo] as const;
  const viewerKey = ["repo-viewer", owner, repo] as const;
  const social = useQuery({ queryKey: socialKey, queryFn: () => fetchRepoSocialCounts(owner, repo) });
  const viewer = useQuery({ queryKey: viewerKey, queryFn: () => fetchRepoViewerState(owner, repo) });
  const currentUser = useQuery({ queryKey: ["current-user"], queryFn: fetchCurrentUser, staleTime: 60_000 });
  const organizations = useQuery({
    queryKey: ["viewer-organizations"],
    queryFn: fetchAuthenticatedUserOrgs,
    staleTime: 60_000,
    enabled: forkOpen,
  });
  useEffect(() => {
    if (!forkOwner && currentUser.data?.login) setForkOwner(currentUser.data.login);
  }, [currentUser.data?.login, forkOwner]);
  useEffect(() => {
    if (!forkOpen) return;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setForkOpen(false);
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [forkOpen]);

  const refreshSocial = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: socialKey }),
      queryClient.invalidateQueries({ queryKey: viewerKey }),
    ]);
  };
  const starMutation = useMutation({
    mutationFn: () => viewer.data?.starred ? unstarRepo(owner, repo) : starRepo(owner, repo),
    onSuccess: refreshSocial,
  });
  const watchMutation = useMutation({
    mutationFn: () => setRepoSubscription(owner, repo, !viewer.data?.subscribed),
    onSuccess: refreshSocial,
  });
  const forkMutation = useMutation({
    mutationFn: () => forkRepo(owner, repo, forkOwner && forkOwner !== currentUser.data?.login ? forkOwner : undefined),
    onSuccess: (created) => {
      setForkOpen(false);
      navigate(`/ui/repos/${created.full_name}`);
    },
  });
  const actionError = starMutation.error ?? watchMutation.error ?? forkMutation.error;

  return (
    <div className="repo-context mb-6">
      <div className="repo-context-inner">
        <div className="repo-title-row">
          <div className="flex min-w-0 items-center gap-1.5" style={{ fontSize: "1.15rem" }}>
            <RepoIcon size={18} style={{ color: "var(--color-fg-muted)" }} />
            <Link to={`/ui/${owner}`} style={{ color: "var(--color-accent)", textDecoration: "none" }}>
              {owner}
            </Link>
            <span style={{ color: "var(--color-fg-muted)" }}>/</span>
            <Link to={base} style={{ color: "var(--color-accent)", fontWeight: 600, textDecoration: "none" }}>
              {repo}
            </Link>
          </div>
          <div className="repo-actions" aria-label="Repository actions">
            <RepoAction
              icon={<EyeIcon size={15} />}
              label={viewer.data?.subscribed ? "Unwatch" : "Watch"}
              count={social.data?.subscribers_count}
              busy={watchMutation.isPending}
              onClick={() => watchMutation.mutate()}
              tone="watch"
            />
            <RepoAction
              icon={<RepoForkedIcon size={15} />}
              label="Fork"
              count={social.data?.forks_count}
              busy={forkMutation.isPending}
              onClick={() => setForkOpen(true)}
              tone="fork"
            />
            <RepoAction
              icon={<StarIcon size={15} />}
              label={viewer.data?.starred ? "Unstar" : "Star"}
              count={social.data?.stargazers_count}
              busy={starMutation.isPending}
              onClick={() => starMutation.mutate()}
              active={viewer.data?.starred}
              tone="star"
            />
          </div>
        </div>
        {actionError && (
          <div role="alert" className="repo-action-error">
            Repository action failed: {String(actionError)}
          </div>
        )}
        {forkOpen && (
          <div className="repo-fork-backdrop" role="presentation" onMouseDown={() => setForkOpen(false)}>
            <div
              role="dialog"
              aria-modal="true"
              aria-labelledby="fork-repository-title"
              className="repo-fork-dialog"
              onMouseDown={(event) => event.stopPropagation()}
            >
              <h2 id="fork-repository-title">Create a new fork</h2>
              <p>
                Choose an owner for the real fork of <strong>{owner}/{repo}</strong>.
              </p>
              <label htmlFor="fork-owner">Owner</label>
              <select id="fork-owner" value={forkOwner} onChange={(event) => setForkOwner(event.target.value)}>
                {currentUser.data && <option value={currentUser.data.login}>{currentUser.data.login}</option>}
                {(organizations.data ?? []).map((organization) => (
                  <option key={organization.id} value={organization.login}>{organization.login}</option>
                ))}
              </select>
              {forkOwner === owner && (
                <div className="repo-fork-warning">Choose a different owner; a fork cannot share the source owner.</div>
              )}
              {forkMutation.error && <div className="repo-fork-warning">{String(forkMutation.error)}</div>}
              <div className="repo-fork-actions">
                <button type="button" onClick={() => setForkOpen(false)}>Cancel</button>
                <button
                  type="button"
                  className="primary"
                  disabled={!forkOwner || forkOwner === owner || forkMutation.isPending}
                  onClick={() => forkMutation.mutate()}
                >
                  {forkMutation.isPending ? "Creating fork…" : "Create fork"}
                </button>
              </div>
            </div>
          </div>
        )}
        <nav aria-label="Repository" className="repo-tabs flex items-center gap-1">
        <RepoTabLink to={base} icon={<RepoIcon size={15} />} label="Code" active={active === "code"} />
        <RepoTabLink
          to={`${base}/issues`}
          icon={<CommentIcon size={15} />}
          label="Issues"
          count={issueCount}
          active={active === "issues"}
        />
        <RepoTabLink
          to={`${base}/pulls`}
          icon={<PullRequestIcon size={15} />}
          label="Pull requests"
          count={prCount}
          active={active === "pulls"}
        />
        <RepoTabLink
          to={`${base}/discussions`}
          icon={<DiscussionIcon size={15} />}
          label="Discussions"
          active={active === "discussions"}
        />
        <RepoTabLink
          to={`${base}/actions`}
          icon={<PlayIcon size={15} />}
          label="Actions"
          active={active === "actions"}
        />
        <RepoTabLink
          to={`${base}/projects-classic`}
          icon={<ProjectIcon size={15} />}
          label="Projects"
          active={active === "projects-classic"}
        />
        <RepoTabLink
          to={`${base}/insights`}
          icon={<GraphIcon size={15} />}
          label="Insights"
          active={active === "insights"}
        />
        <RepoTabLink
          to={`${base}/security/secret-scanning`}
          icon={<LockIcon size={15} />}
          label="Security"
          active={onSecurity}
        />
        <RepoTabLink
          to={`${base}/settings`}
          icon={<GearIcon size={15} />}
          label="Settings"
          active={active === "settings"}
        />
        </nav>
      {onSecurity && (
        <nav
          aria-label="Security"
          className="mt-2 flex flex-wrap items-center gap-2"
          style={{ fontSize: "0.85rem", borderBottom: "1px solid var(--color-border)", paddingBottom: "0.5rem" }}
        >
          <RepoTabLink
            to={`${base}/security/secret-scanning`}
            label="Secret scanning"
            active={location.pathname === `${base}/security/secret-scanning`}
          />
          <RepoTabLink
            to={`${base}/security/code-scanning`}
            label="Code scanning"
            active={location.pathname === `${base}/security/code-scanning`}
          />
          <RepoTabLink
            to={`${base}/security/dependabot`}
            label="Dependabot"
            active={location.pathname === `${base}/security/dependabot`}
          />
          <RepoTabLink
            to={`${base}/security/advisories`}
            label="Advisories"
            active={location.pathname === `${base}/security/advisories`}
          />
        </nav>
        )}
      </div>
    </div>
  );
}

function RepoAction({
  icon,
  label,
  count,
  busy,
  active = false,
  tone,
  onClick,
}: {
  icon: ReactNode;
  label: string;
  count?: number;
  busy: boolean;
  active?: boolean;
  tone: "watch" | "fork" | "star";
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className={`repo-action-button tone-${tone}${active ? " is-active" : ""}`}
      disabled={busy}
      aria-busy={busy}
      onClick={onClick}
    >
      {icon}
      <span>{busy ? "Working…" : label}</span>
      {count != null && <Counter>{count}</Counter>}
    </button>
  );
}

export type OrgTab =
  | "overview"
  | "repos"
  | "packages"
  | "people"
  | "teams"
  | "rulesets"
  | "governance"
  | "copilot"
  | "hooks";

/**
 * Org context bar: organization login breadcrumb with org-level tabs.
 * The tab set mirrors GitHub's org navigation (Overview, Repositories,
 * Packages, People, Teams …) with the bleephub-specific governance
 * surfaces (Rulesets, Governance, Webhooks, Copilot) appended. The
 * breadcrumb login links to the org's Overview landing page.
 */
export function OrgHeader({ org, active }: { org: string; active: OrgTab }) {
  const base = `/ui/orgs/${org}`;
  return (
    <div className="mb-5">
      <div className="mb-3 flex items-center gap-1.5" style={{ fontSize: "1.15rem" }}>
        <OrganizationIcon size={18} style={{ color: "var(--color-fg-muted)" }} />
        <Link to={base} style={{ color: "var(--color-accent)", fontWeight: 600, textDecoration: "none" }}>
          {org}
        </Link>
      </div>
      <nav
        aria-label="Organization"
        className="flex flex-wrap items-center gap-1"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <RepoTabLink to={base} icon={<OrganizationIcon size={15} />} label="Overview" active={active === "overview"} />
        <RepoTabLink to={`${base}/repos`} icon={<RepoIcon size={15} />} label="Repositories" active={active === "repos"} />
        <RepoTabLink to={`${base}/packages`} icon={<PackageIcon size={15} />} label="Packages" active={active === "packages"} />
        <RepoTabLink to={`${base}/people`} icon={<PeopleIcon size={15} />} label="People" active={active === "people"} />
        <RepoTabLink to={`${base}/teams`} icon={<TeamIcon size={15} />} label="Teams" active={active === "teams"} />
        <RepoTabLink to={`${base}/rulesets`} icon={<GearIcon size={15} />} label="Rulesets" active={active === "rulesets"} />
        <RepoTabLink to={`${base}/governance`} icon={<PeopleIcon size={15} />} label="Governance" active={active === "governance"} />
        <RepoTabLink to={`${base}/hooks`} icon={<WebhookIcon size={15} />} label="Webhooks" active={active === "hooks"} />
        <RepoTabLink to={`${base}/copilot`} icon={<CommentIcon size={15} />} label="Copilot" active={active === "copilot"} />
      </nav>
    </div>
  );
}

function RepoTabLink({
  to,
  icon,
  label,
  count,
  active,
}: {
  to: string;
  icon?: ReactNode;
  label: string;
  count?: number | string;
  active: boolean;
}) {
  return (
    <Link
      to={to}
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: "0.4rem",
        padding: "0.5rem 0.7rem",
        marginBottom: "-1px",
        fontSize: "0.86rem",
        fontWeight: active ? 600 : 500,
        color: active ? "var(--color-fg)" : "var(--color-fg-muted)",
        borderBottom: `2px solid ${active ? "var(--color-accent)" : "transparent"}`,
        textDecoration: "none",
      }}
    >
      {icon && <span style={{ color: active ? "var(--color-fg-muted)" : "var(--color-fg-subtle)" }}>{icon}</span>}
      {label}
      {count != null && <Counter>{count}</Counter>}
    </Link>
  );
}
