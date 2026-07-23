import { useEffect, useState, type ReactNode } from "react";
import { NavLink } from "react-router";

export interface GcpNavItem {
  label: string;
  to: string;
}

function GcpThemeToggle() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
  }, [dark]);
  const label = dark ? "Switch to light theme" : "Switch to dark theme";
  return (
    <button type="button" className="gc-icon-button" onClick={() => setDark((on) => !on)} aria-label={label} title={label}>
      <span aria-hidden>{dark ? "☀" : "☾"}</span>
    </button>
  );
}

export function GcpHeader({ project, account }: { project: string; account: ReactNode }) {
  return (
    <header className="gc-header">
      <div className="gc-header-left">
        <span className="gc-hamburger" aria-hidden>☰</span>
        <span className="gc-wordmark">Google Cloud</span>
        <span className="gc-project-chip">
          <span aria-hidden className="gc-project-dot">⬖</span>
          {project}
        </span>
      </div>
      <div className="gc-header-search">
        <span aria-hidden className="gc-search-icon">⌕</span>
        <input
          type="search"
          aria-label="Search resources, docs, products, and more"
          placeholder="Search (/) for resources, docs, products, and more"
        />
      </div>
      <div className="gc-header-right">
        {account}
        <GcpThemeToggle />
      </div>
    </header>
  );
}

/** The console leads a product with the product name and icon, then a flat
 *  navigation whose active item is a filled pill. */
export function GcpProductNav({ product, items }: { product: string; items: GcpNavItem[] }) {
  return (
    <nav aria-label="Product" className="gc-product-nav">
      <div className="gc-product-title">
        <span aria-hidden className="gc-product-glyph">▸</span>
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
              {item.label}
            </NavLink>
          </li>
        ))}
      </ul>
    </nav>
  );
}

export interface GcpPageAction {
  label: string;
  glyph?: string;
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
              {action.glyph ? <span aria-hidden className="gc-action-glyph">{action.glyph}</span> : null}
              {action.label}
            </button>
          ))}
        </div>
        {onRefresh ? (
          <button type="button" className="gc-refresh" onClick={onRefresh} disabled={refreshing}>
            <span aria-hidden>↻</span>
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
  const derived: GcpStatusKind = has(
    "unavailable",
    "stopped",
    "failed",
    "failure",
    "error",
    "deleting",
    "inactive",
    "unknown",
  )
    ? "error"
    : has("pending", "provisioning", "creating", "deploying", "updating")
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
