import { useEffect, useState, type ReactNode } from "react";
import { NavLink } from "react-router";
import { Icon, type IconName } from "./icons.js";

export interface GcpNavItem {
  label: string;
  to: string;
  icon: IconName;
}

function GcpThemeToggle() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
  }, [dark]);
  const label = dark ? "Switch to light theme" : "Switch to dark theme";
  return (
    <button type="button" className="gc-icon-button" onClick={() => setDark((on) => !on)} aria-label={label} title={label}>
      <Icon name={dark ? "light_mode" : "dark_mode"} />
    </button>
  );
}

export function GcpHeader({ project, account }: { project: string; account: ReactNode }) {
  return (
    <header className="gc-header">
      <div className="gc-header-left">
        <button type="button" className="gc-icon-button" aria-label="Main menu">
          <Icon name="menu" />
        </button>
        <span className="gc-wordmark">Google Cloud</span>
        <span className="gc-project-chip">
          <span aria-hidden className="gc-project-dot" />
          {project}
        </span>
      </div>
      <div className="gc-header-search">
        <Icon name="search" size="1.25em" style={{ color: "var(--gc-fg-secondary)" }} />
        <input
          type="search"
          aria-label="Search resources, docs, products, and more"
          placeholder="Search (/) for resources, docs, products, and more"
        />
      </div>
      <div className="gc-header-right">
        <button type="button" className="gc-icon-button" aria-label="Gemini">
          <Icon name="auto_awesome" />
        </button>
        <button type="button" className="gc-icon-button" aria-label="Activate Cloud Shell">
          <Icon name="terminal" />
        </button>
        <button type="button" className="gc-icon-button" aria-label="Notifications">
          <Icon name="notifications" />
        </button>
        <button type="button" className="gc-icon-button" aria-label="Support">
          <Icon name="help" />
        </button>
        <button type="button" className="gc-icon-button" aria-label="Settings and utilities">
          <Icon name="more_vert" />
        </button>
        <GcpThemeToggle />
        {account}
      </div>
    </header>
  );
}

/** The console leads a product with the product name and icon, then a flat
 *  navigation whose active item is a filled pill anchored to the left edge. */
export function GcpProductNav({ product, items }: { product: string; items: GcpNavItem[] }) {
  return (
    <nav aria-label="Product" className="gc-product-nav">
      <div className="gc-product-title">
        <Icon name="deployed_code" size="1.5em" style={{ color: "var(--gc-primary)" }} />
        {product}
      </div>
      <ul>
        {items.map((item) => (
          <li key={item.to}>
            <NavLink
              to={item.to}
              end={item.to === "/ui/"}
              className={({ isActive }) => (isActive ? "gc-nav-link gc-nav-link-active" : "gc-nav-link")}
            >
              <Icon name={item.icon} size="1.25em" />
              <span>{item.label}</span>
            </NavLink>
          </li>
        ))}
      </ul>
      <div className="gc-nav-footer">
        <a href="#" className="gc-nav-link" onClick={(event) => event.preventDefault()}>
          <Icon name="list_alt" size="1.25em" />
          <span>Release Notes</span>
        </a>
      </div>
    </nav>
  );
}

export interface GcpPageAction {
  label: string;
  icon?: IconName;
  onSelect?: () => void;
  disabled?: boolean;
  primary?: boolean;
}

/** The console puts inline text actions beside the page title rather than in a
 *  button group, describes the resource in a sentence beneath, and pins a
 *  refresh at the right. */
export function GcpPageHeader({
  title,
  description,
  actions,
  onRefresh,
  refreshing,
}: {
  title: string;
  description?: string;
  actions?: GcpPageAction[];
  onRefresh?: () => void;
  refreshing?: boolean;
}) {
  return (
    <div className="gc-page-header">
      <div className="gc-page-header-row">
        <h1>{title}</h1>
        <div className="gc-page-actions">
          {actions?.map((action) => (
            <button
              key={action.label}
              type="button"
              className={action.primary ? "gc-text-action gc-text-action-primary" : "gc-text-action"}
              onClick={action.onSelect}
              disabled={action.disabled}
            >
              {action.icon ? <Icon name={action.icon} size="1.25em" /> : null}
              {action.label}
            </button>
          ))}
        </div>
        {onRefresh ? (
          <button type="button" className="gc-refresh" onClick={onRefresh} disabled={refreshing}>
            <Icon name="refresh" size="1.25em" />
            {refreshing ? "Refreshing" : "Refresh"}
          </button>
        ) : null}
      </div>
      {description ? <p className="gc-page-description">{description}</p> : null}
    </div>
  );
}

export type GcpStatusKind = "success" | "error" | "warning" | "inactive";

export function GcpStatus({ status, kind: explicitKind }: { status: string; kind?: GcpStatusKind }) {
  // Matched on whole words. A substring test reports success for failure
  // states, because "unavailable" contains "available" and "inactive" contains
  // "active", and a tick beside a failed resource stops an operator looking any
  // further.
  const words = status.toLowerCase().split(/[^a-z]+/).filter(Boolean);
  const has = (...candidates: string[]) => candidates.some((candidate) => words.includes(candidate));
  // The failure and warning sets cover both resource states and Cloud Logging
  // severities, so a log entry's severity reads the same way a resource's does.
  const derived: GcpStatusKind = has(
    "unavailable",
    "stopped",
    "failed",
    "failure",
    "error",
    "deleting",
    "inactive",
    "unknown",
    "critical",
    "alert",
    "emergency",
  )
    ? "error"
    : has("pending", "provisioning", "creating", "deploying", "updating", "warning")
      ? "warning"
      : has("running", "active", "available", "succeeded", "success", "ok", "healthy", "ready", "enabled")
        ? "success"
        : "inactive";
  const kind = explicitKind ?? derived;
  const glyph = kind === "success" ? "✔" : kind === "error" ? "✖" : kind === "warning" ? "⧗" : "•";
  return (
    <span className={`gc-status gc-status-${kind}`}>
      <span aria-hidden>{glyph}</span>
      {status}
    </span>
  );
}
