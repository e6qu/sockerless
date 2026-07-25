import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { NavLink } from "react-router";
import { useTheme } from "@sockerless/ui-core/hooks";
import { AwsIcon } from "./icons.js";
import { filterNavGroups, type NavGroup } from "./serviceCatalog.js";

/**
 * The console shell: a dark global header, a breadcrumb trail, and a grouped
 * service navigation beside the working area. Every AWS console page is
 * assembled this way, so an operator recognises where they are before reading
 * anything.
 */

export type { NavGroup, NavService } from "./serviceCatalog.js";

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

/**
 * The "Services" mega-menu: the real AWS console's global, from-anywhere way
 * to reach any service, distinct from the side navigation beside it (which
 * this simulator keeps as the current-section affordance — see AwsApp.tsx).
 * The trigger is a plain menu-button (`aria-haspopup="dialog"` /
 * `aria-expanded`); the panel it discloses is a labelled dialog docked to the
 * header's own bottom edge and spanning its full width, the way the real
 * console's flyout does, reusing the exact NAV_GROUPS catalog the side
 * navigation renders rather than a second copy of it.
 *
 * Keyboard and focus behaviour mirrors AwsModal (focus enters the panel,
 * lands on the search field so typing narrows the catalog immediately, Tab
 * is trapped inside while open, Escape and an outside pointer close it and
 * return focus to the trigger) — the same contract every disclosure in this
 * console holds, sized for a menu instead of a form.
 */
export function AwsServicesMenu({ groups }: { groups: NavGroup[] }) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const panelId = useId();
  const titleId = useId();
  const buttonRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    searchRef.current?.focus();

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.stopPropagation();
        setOpen(false);
        buttonRef.current?.focus();
        return;
      }
      if (event.key !== "Tab") return;
      const panel = panelRef.current;
      if (!panel) return;
      const focusables = Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
      if (focusables.length === 0) {
        event.preventDefault();
        return;
      }
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }
    function onPointerDown(event: MouseEvent) {
      const target = event.target as Node;
      if (panelRef.current?.contains(target) || buttonRef.current?.contains(target)) return;
      setOpen(false);
    }
    document.addEventListener("keydown", onKeyDown);
    document.addEventListener("mousedown", onPointerDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      document.removeEventListener("mousedown", onPointerDown);
    };
  }, [open]);

  // A fresh search every time the menu opens, rather than remembering the
  // last query, matches the real console's flyout and keeps the full catalog
  // as the first thing an operator who re-opens it sees.
  useEffect(() => {
    if (!open) setQuery("");
  }, [open]);

  const filtered = filterNavGroups(groups, query);

  return (
    <div className="aws-services-menu">
      <button
        ref={buttonRef}
        type="button"
        className="aws-services-trigger"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        onClick={() => setOpen((current) => !current)}
      >
        Services
        <AwsIcon name="caret" size={10} />
      </button>
      {open && (
        <div
          ref={panelRef}
          id={panelId}
          role="dialog"
          aria-labelledby={titleId}
          className="aws-services-panel"
          tabIndex={-1}
        >
          <h2 id={titleId} className="aws-services-title">All services</h2>
          <div className="aws-services-search">
            <AwsIcon name="search" size={16} />
            <input
              ref={searchRef}
              type="search"
              aria-label="Search services"
              placeholder="Find services"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
          </div>
          {filtered.length === 0 ? (
            <p className="aws-services-empty">No services match &quot;{query.trim()}&quot;.</p>
          ) : (
            <div className="aws-services-columns">
              {filtered.map((group) => (
                <div className="aws-services-column" key={group.label}>
                  <div className="aws-services-column-label">{group.label}</div>
                  <ul>
                    {group.items.map((item) => {
                      const supported = item.supported !== false;
                      return (
                        <li key={item.to}>
                          <NavLink
                            to={item.to}
                            end={item.to === "/ui/"}
                            className={[
                              "aws-services-link",
                              !supported && "aws-services-link-unsupported",
                            ]
                              .filter(Boolean)
                              .join(" ")}
                            aria-label={supported ? undefined : `${item.label}, not supported in this simulator`}
                            onClick={() => setOpen(false)}
                          >
                            <span className="aws-services-link-label">{item.label}</span>
                            {!supported && <AwsNotSupportedBadge serviceName={item.label} decorative />}
                          </NavLink>
                        </li>
                      );
                    })}
                  </ul>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export function AwsHeader({
  region,
  account,
  navExpanded,
  onToggleNav,
  navId,
  services,
}: {
  region: string;
  account: ReactNode;
  navExpanded: boolean;
  onToggleNav: () => void;
  navId: string;
  services: NavGroup[];
}) {
  return (
    <header className="aws-header">
      <div className="aws-header-left">
        <button
          type="button"
          className="aws-icon-button"
          aria-label={navExpanded ? "Close navigation" : "Open navigation"}
          aria-expanded={navExpanded}
          aria-controls={navId}
          onClick={onToggleNav}
        >
          <AwsIcon name="menu" size={16} />
        </button>
        <span className="aws-logo" aria-hidden>aws</span>
        <span className="aws-header-title">Simulator Console</span>
        <AwsServicesMenu groups={services} />
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

export function AwsSideNavigation({
  groups,
  serviceName,
  id,
  expanded,
}: {
  groups: NavGroup[];
  serviceName: string;
  id: string;
  expanded: boolean;
}) {
  return (
    <nav aria-label="Service" className="aws-sidenav" id={id} data-collapsed={expanded ? undefined : "true"}>
      <div className="aws-sidenav-title">{serviceName}</div>
      {groups.map((group) => (
        <div className="aws-sidenav-group" key={group.label}>
          <div className="aws-sidenav-group-label">{group.label}</div>
          <ul>
            {group.items.map((item) => {
              const supported = item.supported !== false;
              return (
                <li key={item.to}>
                  {/* react-router's NavLink sets aria-current="page" on the
                   * rendered anchor automatically when the route matches, so
                   * the current service is conveyed to assistive tech as well
                   * as by the bold/underline treatment below. An unsupported
                   * service's accessible name states that up front, in the
                   * link itself, rather than relying on a screen reader
                   * reaching the decorative pill inside it. */}
                  <NavLink
                    to={item.to}
                    end={item.to === "/ui/"}
                    aria-label={supported ? undefined : `${item.label}, not supported in this simulator`}
                  >
                    {({ isActive }) => (
                      <span
                        className={[
                          "aws-sidenav-link",
                          isActive && "aws-sidenav-link-active",
                          !supported && "aws-sidenav-link-unsupported",
                        ]
                          .filter(Boolean)
                          .join(" ")}
                      >
                        <span className="aws-sidenav-link-label">{item.label}</span>
                        {!supported && <AwsNotSupportedBadge serviceName={item.label} decorative />}
                      </span>
                    )}
                  </NavLink>
                </li>
              );
            })}
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
 *
 * Every page in this console renders exactly one of these, so it is also the
 * one place that sets the browser tab's title — the real AWS console updates
 * it per page (an operator with a dozen console tabs open depends on this to
 * tell them apart), and a single console-wide "Simulator Console" title
 * would not.
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
  useEffect(() => {
    document.title = `${title} - Simulator Console`;
  }, [title]);
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
 * A Cloudscape badge: a small pill that states a fact about the thing beside
 * it. Cloudscape never encodes that fact in colour alone — the label itself
 * carries the meaning — which is why every caller passes readable text, not
 * an icon or a dot.
 */
export type AwsBadgeColor = "blue" | "green" | "red" | "grey";

export function AwsBadge({
  children,
  color = "grey",
  ...rest
}: { children: ReactNode; color?: AwsBadgeColor } & React.HTMLAttributes<HTMLSpanElement>) {
  return (
    <span className={`aws-badge aws-badge-${color}`} {...rest}>
      {children}
    </span>
  );
}

/**
 * The pill the navigation and the "not supported" page use for a service
 * sockerless has not implemented. In the side navigation the enclosing link
 * already states "…, not supported in this simulator" as its own accessible
 * name (see AwsSideNavigation), so the badge there is `decorative` — hidden
 * from assistive tech to avoid announcing the same fact twice. Used on its
 * own, on the "not supported" page itself, it carries its own accessible name
 * naming the service.
 */
export function AwsNotSupportedBadge({
  serviceName,
  decorative = false,
}: {
  serviceName: string;
  decorative?: boolean;
}) {
  return (
    <AwsBadge
      color="grey"
      aria-hidden={decorative || undefined}
      aria-label={decorative ? undefined : `${serviceName} is not supported in this simulator`}
    >
      Not supported
    </AwsBadge>
  );
}

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * A Cloudscape-style modal dialog. When `onDismiss` is provided the dialog can
 * be dismissed from its close control, the overlay, or the Escape key; when it
 * is omitted the dialog closes only through its own footer actions — the
 * shape a one-time disclosure needs, where an accidental dismissal would lose
 * material the operator can never see again.
 *
 * Focus moves into the dialog when it opens (a page-level `autoFocus` field
 * takes priority; the dialog itself is the fallback), Tab is trapped inside
 * it for as long as it is open, and focus returns to whatever opened it on
 * close — the keyboard-operability contract every Cloudscape dialog holds.
 */
export function AwsModal({
  title,
  onDismiss,
  footer,
  children,
}: {
  title: string;
  onDismiss?: () => void;
  footer: ReactNode;
  children: ReactNode;
}) {
  const titleId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const onDismissRef = useRef(onDismiss);
  onDismissRef.current = onDismiss;
  // Captured during render, before this component's own children — including
  // a page's autoFocus field — reach the DOM. useEffect runs after commit, by
  // which point autoFocus has already moved document.activeElement onto the
  // dialog's own input; capturing there would restore focus to the dialog's
  // own field instead of what opened it.
  const previouslyFocusedRef = useRef<HTMLElement | null>(
    typeof document === "undefined" ? null : (document.activeElement as HTMLElement | null),
  );

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;

    if (!dialog.contains(document.activeElement)) {
      const focusable = dialog.querySelector<HTMLElement>(FOCUSABLE_SELECTOR);
      (focusable ?? dialog).focus();
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        if (!onDismissRef.current) return;
        event.stopPropagation();
        onDismissRef.current();
        return;
      }
      if (event.key !== "Tab" || !dialog) return;
      const focusables = Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
      if (focusables.length === 0) {
        event.preventDefault();
        return;
      }
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }
    dialog.addEventListener("keydown", onKeyDown);
    return () => {
      dialog.removeEventListener("keydown", onKeyDown);
      previouslyFocusedRef.current?.focus?.();
    };
  }, []);

  return (
    <div className="aws-modal-overlay" role="presentation" onClick={onDismiss}>
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="aws-modal"
        tabIndex={-1}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="aws-modal-header">
          <h2 id={titleId}>{title}</h2>
          {onDismiss && (
            <button type="button" className="aws-modal-close" aria-label="Close" onClick={onDismiss}>
              <AwsIcon name="close" size={16} />
            </button>
          )}
        </div>
        <div className="aws-modal-content">{children}</div>
        <div className="aws-modal-footer">{footer}</div>
      </div>
    </div>
  );
}

/**
 * Copy-to-clipboard with spoken and visible feedback, the control Cloudscape
 * places beside every credential and identifier. Failure is announced rather
 * than swallowed: an operator who believes a secret is on the clipboard when it
 * is not will paste something else into a credentials file.
 */
export function AwsCopyButton({ value, label }: { value: string; label: string }) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");
  return (
    <button
      type="button"
      className="aws-copy"
      aria-label={label}
      title={label}
      onClick={() => {
        navigator.clipboard.writeText(value).then(
          () => setState("copied"),
          () => setState("failed"),
        );
      }}
    >
      <AwsIcon name="copy" size={14} />
      <span role="status">{state === "copied" ? "Copied" : state === "failed" ? "Copy failed" : "Copy"}</span>
    </button>
  );
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
