import { type ReactNode } from "react";
import { NavLink, Link, useLocation } from "react-router";
import { useTheme } from "@sockerless/ui-core/hooks";
import {
  Mark,
  SunIcon,
  MoonIcon,
  SignOutIcon,
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
} from "./octicons.js";
import { Counter } from "./ui.js";
import { clearToken } from "../api.js";

export interface NavItem {
  label: string;
  to: string;
  end?: boolean;
}

const PRIMARY_NAV: NavItem[] = [
  { label: "Overview", to: "/ui/", end: true },
  { label: "Repos", to: "/ui/repos" },
  { label: "Gists", to: "/ui/gists" },
  { label: "Runners", to: "/ui/runners" },
  { label: "Apps", to: "/ui/apps" },
  { label: "OAuth", to: "/ui/oauth" },
  { label: "Metrics", to: "/ui/metrics" },
];

const ADMIN_NAV: NavItem[] = [
  { label: "Users", to: "/ui/admin/users" },
  { label: "Orgs", to: "/ui/admin/orgs" },
  { label: "Teams", to: "/ui/admin/teams" },
  { label: "Audit log", to: "/ui/admin/audit-log" },
  { label: "Storage", to: "/ui/admin/storage" },
];

function navIcon(label: string) {
  switch (label) {
    case "Overview":
      return null;
    case "Repos":
      return <RepoIcon size={14} />;
    case "Gists":
      return <GistIcon size={14} />;
    case "Runners":
      return null;
    case "Apps":
      return null;
    case "OAuth":
      return null;
    case "Metrics":
      return null;
    case "Users":
      return <PeopleIcon size={14} />;
    case "Orgs":
      return <OrganizationIcon size={14} />;
    case "Teams":
      return <TeamIcon size={14} />;
    case "Audit log":
      return <AuditLogIcon size={14} />;
    case "Storage":
      return <ServerIcon size={14} />;
    default:
      return null;
  }
}

function HeaderNavLink({ item }: { item: NavItem }) {
  return (
    <NavLink to={item.to} end={item.end} style={{ textDecoration: "none" }}>
      {({ isActive }) => (
        <span
          className="inline-flex items-center gap-1.5"
          style={{
            padding: "0.3rem 0.6rem",
            borderRadius: "var(--radius-md)",
            fontSize: "0.85rem",
            fontWeight: isActive ? 600 : 500,
            color: isActive ? "var(--color-fg)" : "var(--color-fg-muted)",
            background: isActive ? "color-mix(in srgb, var(--color-fg-muted) 12%, transparent)" : "transparent",
          }}
        >
          {navIcon(item.label)}
          {item.label}
        </span>
      )}
    </NavLink>
  );
}

function ThemeButton() {
  const { theme, toggle } = useTheme("light");
  const isDark = theme === "dark";
  return (
    <button
      type="button"
      onClick={toggle}
      aria-label={isDark ? "Switch to light theme" : "Switch to dark theme"}
      title={isDark ? "Switch to light theme" : "Switch to dark theme"}
      className="inline-flex h-8 w-8 items-center justify-center"
      style={{
        background: "transparent",
        color: "var(--color-fg-muted)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-md)",
      }}
    >
      {isDark ? <SunIcon /> : <MoonIcon />}
    </button>
  );
}

/** Global top header: brand, primary nav, theme toggle, sign-out. */
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
      <header
        style={{
          background: "var(--color-bg-subtle)",
          borderBottom: "1px solid var(--color-border)",
        }}
      >
        <div className="mx-auto flex max-w-[1280px] flex-wrap items-center gap-x-3 gap-y-2 px-4 py-2.5">
          <Link
            to="/ui/"
            className="inline-flex items-center gap-2"
            style={{ textDecoration: "none", color: "var(--color-fg)" }}
          >
            <Mark size={24} />
            <span style={{ fontWeight: 600, fontSize: "0.95rem" }}>bleephub</span>
          </Link>
          <nav aria-label="Primary" className="flex flex-1 flex-wrap items-center gap-0.5">
            {PRIMARY_NAV.map((item) => (
              <HeaderNavLink key={item.to} item={item} />
            ))}
          </nav>
          <nav aria-label="Admin" className="flex flex-wrap items-center gap-0.5 border-l pl-3" style={{ borderColor: "var(--color-border)" }}>
            {ADMIN_NAV.map((item) => (
              <HeaderNavLink key={item.to} item={item} />
            ))}
          </nav>
          <div className="flex items-center gap-2">
            <ThemeButton />
            <button
              type="button"
              onClick={() => {
                clearToken();
                window.location.href = "/ui/login";
              }}
              className="inline-flex items-center gap-1.5"
              style={{
                background: "transparent",
                color: "var(--color-fg-muted)",
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-md)",
                padding: "0.3rem 0.6rem",
                fontSize: "0.82rem",
                fontWeight: 500,
              }}
            >
              <SignOutIcon /> Sign out
            </button>
          </div>
        </div>
      </header>
      <main id="main-content" tabIndex={-1} className="mx-auto max-w-[1280px] px-4 py-6">
        {children}
      </main>
    </div>
  );
}

// ─── Repo context header + tabs ────────────────────────────────────────

export type RepoTab = "code" | "issues" | "pulls" | "actions" | "projects-classic" | "security" | "settings";

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
        </nav>
      )}
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
