import { useEffect, useRef, useState, type ReactNode, type CSSProperties, type FormEvent } from "react";
import { Link, NavLink, useNavigate } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { useTheme } from "@sockerless/ui-core/hooks";
import {
  Mark,
  ThreeBarsIcon,
  SearchIcon,
  PlusIcon,
  TriangleDownIcon,
  NotificationBellIcon,
  IssueOpenedIcon,
  PullRequestIcon,
  SunIcon,
  MoonIcon,
  SignOutIcon,
  RepoIcon,
  GistIcon,
  PackageIcon,
  CodespaceIcon,
  MigrationIcon,
  OrganizationIcon,
  KeyIcon,
  ServerIcon,
  PeopleIcon,
  TeamIcon,
  GlobeIcon,
  AuditLogIcon,
  GraphIcon,
} from "./octicons.js";
import { clearToken, fetchCurrentUser, fetchNotifications } from "../api.js";

/**
 * GitHub-faithful global header: hamburger → global-nav drawer, brand, a
 * search box, a "create" menu, Issues / Pull requests quick links, the
 * notifications bell, and an avatar dropdown. It mirrors github.com's chrome
 * so the app's information architecture matches a user's GitHub muscle memory;
 * only the visual styling is bleephub's own.
 */

// ─── click-outside dropdown menu ────────────────────────────────────────────

function useDismiss(open: boolean, close: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) close();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    document.addEventListener("mousedown", onClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, close]);
  return ref;
}

function HeaderMenu({
  label,
  trigger,
  align = "right",
  children,
}: {
  label: string;
  trigger: ReactNode;
  align?: "left" | "right";
  children: (close: () => void) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const close = () => setOpen(false);
  const ref = useDismiss(open, close);
  return (
    <div ref={ref} style={{ position: "relative" }}>
      <button
        type="button"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className="inline-flex items-center gap-1"
        style={{
          background: "transparent",
          color: "var(--color-fg-muted)",
          border: "1px solid var(--color-border)",
          borderRadius: "var(--radius-md)",
          padding: "0.3rem 0.5rem",
          cursor: "pointer",
        }}
      >
        {trigger}
      </button>
      {open && (
        <div
          role="menu"
          style={{
            position: "absolute",
            top: "calc(100% + 6px)",
            [align]: 0,
            minWidth: 220,
            background: "var(--color-bg)",
            border: "1px solid var(--color-border)",
            borderRadius: "var(--radius-md)",
            boxShadow: "0 8px 24px rgba(0,0,0,0.18)",
            zIndex: 60,
            padding: "0.35rem",
          }}
        >
          {children(close)}
        </div>
      )}
    </div>
  );
}

function MenuLink({ to, icon, children, onClick }: { to: string; icon?: ReactNode; children: ReactNode; onClick: () => void }) {
  return (
    <Link
      to={to}
      role="menuitem"
      onClick={onClick}
      className="flex items-center gap-2"
      style={{
        textDecoration: "none",
        color: "var(--color-fg)",
        fontSize: "0.85rem",
        padding: "0.4rem 0.5rem",
        borderRadius: "var(--radius-sm)",
      }}
    >
      {icon}
      {children}
    </Link>
  );
}

function MenuButton({ icon, children, onClick }: { icon?: ReactNode; children: ReactNode; onClick: () => void }) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      className="flex w-full items-center gap-2"
      style={{
        background: "transparent",
        border: "none",
        color: "var(--color-fg)",
        fontSize: "0.85rem",
        padding: "0.4rem 0.5rem",
        borderRadius: "var(--radius-sm)",
        cursor: "pointer",
        textAlign: "left",
      }}
    >
      {icon}
      {children}
    </button>
  );
}

function MenuSeparator() {
  return <div style={{ height: 1, background: "var(--color-border)", margin: "0.35rem 0" }} />;
}

// ─── global-nav drawer (hamburger) ──────────────────────────────────────────

type DrawerItem = { label: string; to: string; icon: ReactNode; end?: boolean };

const GITHUB_NAV: DrawerItem[] = [
  { label: "Dashboard", to: "/ui/", icon: <RepoIcon size={16} />, end: true },
  { label: "Issues", to: "/ui/search?type=issues&q=is%3Aissue", icon: <IssueOpenedIcon size={16} /> },
  { label: "Pull requests", to: "/ui/search?type=issues&q=is%3Apr", icon: <PullRequestIcon size={16} /> },
  { label: "Repositories", to: "/ui/repos", icon: <RepoIcon size={16} /> },
  { label: "Gists", to: "/ui/gists", icon: <GistIcon size={16} /> },
  { label: "Packages", to: "/ui/packages", icon: <PackageIcon size={16} /> },
  { label: "Codespaces", to: "/ui/codespaces", icon: <CodespaceIcon size={16} /> },
  { label: "Migrations", to: "/ui/migrations", icon: <MigrationIcon size={16} /> },
  { label: "Notifications", to: "/ui/notifications", icon: <NotificationBellIcon size={16} /> },
  { label: "Explore", to: "/ui/search", icon: <SearchIcon size={16} /> },
];

// Bleephub service administration surfaces that map to public GitHub or GitHub
// Enterprise Server routes stay grouped away from the repository/product nav.
const OPS_NAV: DrawerItem[] = [
  { label: "System status", to: "/ui/admin", icon: <GraphIcon size={16} />, end: true },
  { label: "Workflow runs", to: "/ui/workflows", icon: <RepoIcon size={16} /> },
  { label: "Runners", to: "/ui/runners", icon: <ServerIcon size={16} /> },
  { label: "Metrics", to: "/ui/metrics", icon: <GraphIcon size={16} /> },
  { label: "GitHub Apps", to: "/ui/apps", icon: <KeyIcon size={16} /> },
  { label: "OAuth Apps", to: "/ui/oauth", icon: <KeyIcon size={16} /> },
  { label: "Users", to: "/ui/admin/users", icon: <PeopleIcon size={16} /> },
  { label: "Organizations", to: "/ui/admin/orgs", icon: <OrganizationIcon size={16} /> },
  { label: "Teams", to: "/ui/admin/teams", icon: <TeamIcon size={16} /> },
  { label: "Enterprise", to: "/ui/admin/enterprise", icon: <GlobeIcon size={16} /> },
  { label: "Audit log", to: "/ui/admin/audit-log", icon: <AuditLogIcon size={16} /> },
];

function DrawerSection({ title, items, onNavigate }: { title: string; items: DrawerItem[]; onNavigate: () => void }) {
  return (
    <div style={{ padding: "0.5rem 0" }}>
      <div
        style={{
          fontSize: "0.72rem",
          fontWeight: 600,
          textTransform: "uppercase",
          letterSpacing: "0.04em",
          color: "var(--color-fg-muted)",
          padding: "0.25rem 0.75rem",
        }}
      >
        {title}
      </div>
      {items.map((it) => (
        <NavLink
          key={it.to}
          to={it.to}
          end={it.end}
          onClick={onNavigate}
          style={{ textDecoration: "none" }}
        >
          {({ isActive }) => (
            <span
              className="flex items-center gap-2.5"
              style={{
                padding: "0.45rem 0.75rem",
                fontSize: "0.9rem",
                color: isActive ? "var(--color-fg)" : "var(--color-fg-muted)",
                fontWeight: isActive ? 600 : 500,
                borderLeft: `2px solid ${isActive ? "var(--color-accent)" : "transparent"}`,
                background: isActive ? "color-mix(in srgb, var(--color-fg-muted) 10%, transparent)" : "transparent",
              }}
            >
              {it.icon}
              {it.label}
            </span>
          )}
        </NavLink>
      ))}
    </div>
  );
}

function GlobalNavDrawer({ open, onClose }: { open: boolean; onClose: () => void }) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, onClose]);
  if (!open) return null;
  return (
    <>
      <div
        onClick={onClose}
        style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.35)", zIndex: 70 }}
      />
      <nav
        aria-label="Global"
        style={{
          position: "fixed",
          top: 0,
          left: 0,
          bottom: 0,
          width: 300,
          maxWidth: "85vw",
          background: "var(--color-bg)",
          borderRight: "1px solid var(--color-border)",
          boxShadow: "2px 0 16px rgba(0,0,0,0.18)",
          zIndex: 71,
          overflowY: "auto",
        }}
      >
        <div className="flex items-center gap-2" style={{ padding: "0.9rem 0.9rem 0.5rem" }}>
          <Mark size={22} />
          <span style={{ fontWeight: 600 }}>bleephub</span>
        </div>
        <DrawerSection title="GitHub" items={GITHUB_NAV} onNavigate={onClose} />
        <div style={{ height: 1, background: "var(--color-border)" }} />
        <DrawerSection title="Operations" items={OPS_NAV} onNavigate={onClose} />
      </nav>
    </>
  );
}

// ─── header ─────────────────────────────────────────────────────────────────

function iconButtonStyle(): CSSProperties {
  return {
    background: "transparent",
    color: "var(--color-fg-muted)",
    border: "1px solid var(--color-border)",
    borderRadius: "var(--radius-md)",
    height: 32,
    width: 32,
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    position: "relative",
  };
}

function Avatar({ login, url, size = 24 }: { login: string; url?: string; size?: number }) {
  if (url) {
    return <img src={url} alt="" width={size} height={size} style={{ borderRadius: "50%", display: "block" }} />;
  }
  const initials = login.slice(0, 2).toUpperCase();
  return (
    <span
      aria-hidden
      className="inline-flex items-center justify-center"
      style={{
        width: size,
        height: size,
        borderRadius: "50%",
        background: "var(--color-accent)",
        color: "var(--color-accent-fg)",
        fontSize: size * 0.42,
        fontWeight: 600,
      }}
    >
      {initials}
    </span>
  );
}

export function AppHeader() {
  const navigate = useNavigate();
  const { theme, toggle } = useTheme("light");
  const isDark = theme === "dark";
  const [drawer, setDrawer] = useState(false);
  const [q, setQ] = useState("");

  const { data: user } = useQuery({ queryKey: ["current-user"], queryFn: fetchCurrentUser, staleTime: 60_000 });
  const { data: notifications } = useQuery({
    queryKey: ["notifications", "header"],
    queryFn: fetchNotifications,
    refetchInterval: 30_000,
  });
  const unread = notifications?.filter((n) => n.unread !== false).length ?? 0;
  const login = user?.login ?? "";

  const submitSearch = (e: FormEvent) => {
    e.preventDefault();
    const term = q.trim();
    navigate(term ? `/ui/search?q=${encodeURIComponent(term)}` : "/ui/search");
  };

  return (
    <>
      <GlobalNavDrawer open={drawer} onClose={() => setDrawer(false)} />
      <header style={{ background: "var(--color-bg-subtle)", borderBottom: "1px solid var(--color-border)" }}>
        <div className="mx-auto flex max-w-[1280px] items-center gap-3 px-4 py-2.5">
          <button type="button" aria-label="Open global navigation" onClick={() => setDrawer(true)} style={iconButtonStyle()}>
            <ThreeBarsIcon size={16} />
          </button>

          <Link to="/ui/" className="inline-flex items-center gap-2" style={{ textDecoration: "none", color: "var(--color-fg)" }}>
            <Mark size={24} />
            <span style={{ fontWeight: 600, fontSize: "0.95rem" }} className="hidden sm:inline">
              bleephub
            </span>
          </Link>

          <form onSubmit={submitSearch} className="flex flex-1 items-center" style={{ maxWidth: 480 }}>
            <div
              className="flex w-full items-center gap-2"
              style={{
                border: "1px solid var(--color-border)",
                borderRadius: "var(--radius-md)",
                background: "var(--color-bg)",
                padding: "0.3rem 0.55rem",
              }}
            >
              <SearchIcon size={14} style={{ color: "var(--color-fg-muted)" }} />
              <input
                type="search"
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder="Search or jump to…"
                aria-label="Search"
                style={{
                  flex: 1,
                  border: "none",
                  outline: "none",
                  background: "transparent",
                  color: "var(--color-fg)",
                  fontSize: "0.85rem",
                }}
              />
            </div>
          </form>

          <div className="flex items-center gap-2">
            {/* create menu */}
            <HeaderMenu label="Create new…" trigger={<><PlusIcon size={14} /><TriangleDownIcon size={12} /></>}>
              {(close) => (
                <>
                  <MenuLink to="/ui/repos" icon={<RepoIcon size={16} />} onClick={close}>New repository</MenuLink>
                  <MenuLink to="/ui/gists" icon={<GistIcon size={16} />} onClick={close}>New gist</MenuLink>
                  <MenuLink to="/ui/admin/orgs" icon={<OrganizationIcon size={16} />} onClick={close}>New organization</MenuLink>
                </>
              )}
            </HeaderMenu>

            <Link to="/ui/search?type=issues&q=is%3Aissue" aria-label="Issues" title="Issues" style={iconButtonStyle()}>
              <IssueOpenedIcon size={16} />
            </Link>
            <Link to="/ui/search?type=issues&q=is%3Apr" aria-label="Pull requests" title="Pull requests" style={iconButtonStyle()}>
              <PullRequestIcon size={16} />
            </Link>

            <Link to="/ui/notifications" aria-label={unread ? `Notifications (${unread} unread)` : "Notifications"} title="Notifications" style={iconButtonStyle()}>
              <NotificationBellIcon size={16} />
              {unread > 0 && (
                <span
                  aria-hidden
                  style={{
                    position: "absolute",
                    top: -4,
                    right: -4,
                    minWidth: 16,
                    height: 16,
                    padding: "0 4px",
                    borderRadius: 8,
                    background: "var(--color-accent)",
                    color: "var(--color-accent-fg)",
                    fontSize: "0.62rem",
                    fontWeight: 700,
                    display: "inline-flex",
                    alignItems: "center",
                    justifyContent: "center",
                  }}
                >
                  {unread > 99 ? "99+" : unread}
                </span>
              )}
            </Link>

            {/* avatar menu */}
            <HeaderMenu label="Open user menu" align="right" trigger={<><Avatar login={login} url={user?.avatar_url} /><TriangleDownIcon size={12} /></>}>
              {(close) => (
                <>
                  <div style={{ padding: "0.35rem 0.5rem", fontSize: "0.8rem", color: "var(--color-fg-muted)" }}>
                    Signed in as <strong style={{ color: "var(--color-fg)" }}>{login || "…"}</strong>
                  </div>
                  <MenuSeparator />
                  {login && <MenuLink to={`/ui/${login}`} icon={<PeopleIcon size={16} />} onClick={close}>Your profile</MenuLink>}
                  <MenuLink to="/ui/repos" icon={<RepoIcon size={16} />} onClick={close}>Your repositories</MenuLink>
                  <MenuLink to="/ui/gists" icon={<GistIcon size={16} />} onClick={close}>Your gists</MenuLink>
                  <MenuLink to="/ui/packages" icon={<PackageIcon size={16} />} onClick={close}>Your packages</MenuLink>
                  <MenuLink to="/ui/codespaces" icon={<CodespaceIcon size={16} />} onClick={close}>Your codespaces</MenuLink>
                  <MenuSeparator />
                  <MenuLink to="/ui/account" icon={<KeyIcon size={16} />} onClick={close}>Settings</MenuLink>
                  <MenuLink to="/ui/admin" icon={<GraphIcon size={16} />} onClick={close}>Operations</MenuLink>
                  <MenuSeparator />
                  <MenuButton icon={isDark ? <SunIcon size={16} /> : <MoonIcon size={16} />} onClick={() => { toggle(); close(); }}>
                    {isDark ? "Light theme" : "Dark theme"}
                  </MenuButton>
                  <MenuButton
                    icon={<SignOutIcon size={16} />}
                    onClick={() => {
                      clearToken();
                      window.location.href = "/ui/login";
                    }}
                  >
                    Sign out
                  </MenuButton>
                </>
              )}
            </HeaderMenu>
          </div>
        </div>
      </header>
    </>
  );
}
