import { type ReactNode } from "react";
import { NavLink } from "react-router";
import { useTheme } from "@sockerless/ui-core/hooks";
import { AwsIcon } from "./icons.js";

/**
 * The console shell: a dark global header, a breadcrumb trail, and a grouped
 * service navigation beside the working area. Every AWS console page is
 * assembled this way, so an operator recognises where they are before reading
 * anything.
 */

export interface NavGroup {
  /** Section heading. Cloudscape groups services rather than listing them flat. */
  label: string;
  items: { label: string; to: string }[];
}

/**
 * The console keeps its theme control in the header, so this one lives there
 * too. The glyph is decorative; the accessible name states the action, and it
 * changes with the state so a screen reader announces what the press will do.
 */
function AwsThemeToggle() {
  const { theme, toggle } = useTheme();
  const isDark = theme === "dark";
  const label = isDark ? "Switch to light theme" : "Switch to dark theme";
  return (
    <button type="button" className="aws-icon-button" onClick={toggle} aria-label={label} title={label}>
      <AwsIcon name={isDark ? "sun" : "moon"} size={16} />
    </button>
  );
}

export function AwsHeader({ region, account }: { region: string; account: ReactNode }) {
  return (
    <header className="aws-header">
      <div className="aws-header-left">
        <button type="button" className="aws-icon-button" aria-label="Open menu">
          <AwsIcon name="menu" size={16} />
        </button>
        <span className="aws-logo" aria-hidden>aws</span>
        <span className="aws-header-title">Simulator Console</span>
      </div>
      <div className="aws-header-search">
        <AwsIcon name="search" size={16} />
        <input type="search" aria-label="Search" placeholder="Search [Option+S]" />
      </div>
      <div className="aws-header-right">
        <button type="button" className="aws-icon-button" aria-label="Notifications">
          <AwsIcon name="notification" size={16} />
        </button>
        <button type="button" className="aws-icon-button" aria-label="Settings">
          <AwsIcon name="settings" size={16} />
        </button>
        <button type="button" className="aws-icon-button" aria-label="Support">
          <AwsIcon name="help" size={16} />
        </button>
        <span className="aws-header-region">{region}</span>
        {account}
        <AwsThemeToggle />
      </div>
    </header>
  );
}

export function AwsBreadcrumbs({ trail }: { trail: { label: string; to?: string }[] }) {
  return (
    <nav aria-label="Breadcrumbs" className="aws-breadcrumbs">
      <ol>
        {trail.map((crumb, index) => (
          <li key={crumb.label}>
            {crumb.to && index < trail.length - 1 ? (
              <NavLink to={crumb.to}>{crumb.label}</NavLink>
            ) : (
              <span aria-current={index === trail.length - 1 ? "page" : undefined}>{crumb.label}</span>
            )}
          </li>
        ))}
      </ol>
    </nav>
  );
}

export function AwsSideNavigation({ groups, serviceName }: { groups: NavGroup[]; serviceName: string }) {
  return (
    <nav aria-label="Service" className="aws-sidenav">
      <div className="aws-sidenav-title">{serviceName}</div>
      {groups.map((group) => (
        <div className="aws-sidenav-group" key={group.label}>
          <div className="aws-sidenav-group-label">{group.label}</div>
          <ul>
            {group.items.map((item) => (
              <li key={item.to}>
                <NavLink to={item.to} end={item.to === "/ui/"}>
                  {({ isActive }) => (
                    <span className={isActive ? "aws-sidenav-link aws-sidenav-link-active" : "aws-sidenav-link"}>
                      {item.label}
                    </span>
                  )}
                </NavLink>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </nav>
  );
}

/**
 * `Resources (count)` with an information link, the pattern every Cloudscape
 * list page uses. The count belongs beside the title rather than under it,
 * because that is how an operator confirms a filter did what they expected.
 */
export function AwsPageHeader({
  title,
  count,
  description,
  actions,
}: {
  title: string;
  count?: number;
  description?: string;
  actions?: ReactNode;
}) {
  return (
    <div className="aws-page-header">
      <div>
        <h1>
          {title}
          {count !== undefined && <span className="aws-page-count"> ({count})</span>}
        </h1>
        {description && <p className="aws-page-description">{description}</p>}
      </div>
      {actions && <div className="aws-page-actions">{actions}</div>}
    </div>
  );
}

export function AwsButton({
  children,
  variant = "normal",
  ...rest
}: { children: ReactNode; variant?: "normal" | "primary" } & React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button type="button" className={variant === "primary" ? "aws-button aws-button-primary" : "aws-button"} {...rest}>
      {children}
    </button>
  );
}

export function AwsContainer({ children }: { children: ReactNode }) {
  return <section className="aws-container">{children}</section>;
}

/**
 * Status is an icon and a word. Cloudscape never relies on colour alone, so
 * the glyph carries the meaning for anyone who cannot distinguish the hue.
 */
export type AwsStatusKind = "success" | "error" | "warning" | "inactive";

/**
 * Where the caller knows the meaning it passes `kind`, because inferring it
 * from wording is guesswork that fails quietly. The inference below exists
 * only for values that arrive from the service as free text.
 */
export function AwsStatus({ status, kind: explicitKind }: { status: string; kind?: AwsStatusKind }) {
  const value = status.toLowerCase();
  // Failure terms are tested first and matched on whole words. Substring tests
  // are the trap here: "unavailable" contains "available", and "inactive"
  // contains "active", so a looser check reports a green tick for a failure.
  const words = value.split(/[^a-z]+/).filter(Boolean);
  const has = (...candidates: string[]) => candidates.some((candidate) => words.includes(candidate));
  const kind = has("unavailable", "stopped", "stopping", "failed", "failure", "error", "deactivated", "inactive", "deleted")
    ? "error"
    : has("pending", "provisioning", "creating", "updating", "deleting")
      ? "warning"
      : has("running", "active", "activated", "available", "succeeded", "success", "ok", "healthy")
        ? "success"
        : "inactive";
  const resolved = explicitKind ?? kind;
  return (
    <span className={`aws-status aws-status-${resolved}`}>
      {resolved === "success" ? (
        <AwsIcon name="status_positive" size={14} />
      ) : resolved === "error" ? (
        <AwsIcon name="status_negative" size={14} />
      ) : resolved === "warning" ? (
        <AwsIcon name="status_pending" size={14} />
      ) : (
        <span aria-hidden>–</span>
      )}
      {status}
    </span>
  );
}
