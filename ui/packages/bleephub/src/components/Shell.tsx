import { type ReactNode } from "react";
import { NavLink, Link, useLocation } from "react-router";
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
} from "./octicons.js";
import { Counter } from "./ui.js";
import { AppHeader } from "./AppHeader.js";

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
  const onSecurity = active === "security" || location.pathname.startsWith(`${base}/security/`);
  return (
    <div className="mb-5">
      <div className="mb-3 flex items-center gap-1.5" style={{ fontSize: "1.15rem" }}>
        <RepoIcon size={18} style={{ color: "var(--color-fg-muted)" }} />
        <Link to="/ui/repos" style={{ color: "var(--color-accent)", textDecoration: "none" }}>
          {owner}
        </Link>
        <span style={{ color: "var(--color-fg-muted)" }}>/</span>
        <Link to={base} style={{ color: "var(--color-accent)", fontWeight: 600, textDecoration: "none" }}>
          {repo}
        </Link>
      </div>
      <nav
        aria-label="Repository"
        className="flex flex-wrap items-center gap-1"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
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
