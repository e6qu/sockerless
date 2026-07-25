import { useEffect, useRef, useState, type ReactNode } from "react";
import { NavLink } from "react-router";
import { AzureIcon, type AzureIconName } from "./icons.js";

export interface ServiceMenuItem {
  label: string;
  to: string;
  /** False marks a service the real Azure portal offers that this simulator
   *  does not implement. The item still links — to a small honest
   *  explanation — rather than disappearing or turning into a dead end that
   *  looks clickable but isn't; see AzureNotSupportedBadge. */
  supported?: boolean;
}

export interface ServiceMenuGroup {
  label: string;
  items: ServiceMenuItem[];
}

/** A command bar entry. The portal shows unavailable commands greyed rather
 *  than hidden, so the set of things a resource supports stays visible. */
export interface PortalCommand {
  label: string;
  icon: AzureIconName;
  onSelect?: () => void;
  disabled?: boolean;
  /** A structural-test hook rendered as data-testid on the command button. */
  testid?: string;
}

function AzureThemeToggle() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
  }, [dark]);
  const label = dark ? "Switch to light theme" : "Switch to dark theme";
  return (
    <button
      type="button"
      className="az-icon-button"
      data-testid="az-theme-toggle"
      onClick={() => setDark((on) => !on)}
      aria-label={label}
      title={label}
    >
      <AzureIcon name={dark ? "sun" : "moon"} size={18} />
    </button>
  );
}

/** A header icon button that discloses a small panel rather than acting
 *  directly — the shape every header control that has no real backing
 *  feature in this simulator takes (Cloud Shell, Notifications, Help). The
 *  panel says plainly what it is and, where one exists, what the faithful
 *  alternative is, rather than presenting a dead icon that looks live. Focus
 *  moves into the panel on open, Escape and an outside pointer close it and
 *  return focus to the trigger — the same contract a real Fluent callout
 *  offers. */
function AzureHeaderPopover({
  icon,
  label,
  testid,
  children,
}: {
  icon: AzureIconName;
  label: string;
  testid: string;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    panelRef.current?.focus();
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "Escape") return;
      setOpen(false);
      buttonRef.current?.focus();
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

  return (
    <div className="az-header-popover">
      <button
        ref={buttonRef}
        type="button"
        className="az-icon-button"
        data-testid={testid}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        aria-label={label}
        title={label}
      >
        <AzureIcon name={icon} size={18} />
      </button>
      {open ? (
        <div ref={panelRef} className="az-header-panel" role="dialog" aria-label={label} tabIndex={-1}>
          {children}
        </div>
      ) : null}
    </div>
  );
}

export function AzureHeader({ account }: { account: ReactNode }) {
  return (
    <header className="az-header">
      <div className="az-header-brand">
        <button type="button" className="az-hamburger" aria-label="Show portal menu">
          <AzureIcon name="menu" size={18} />
        </button>
        <span className="az-wordmark">Microsoft Azure</span>
      </div>
      <div className="az-header-search">
        <AzureIcon name="search" size={16} />
        <input
          type="search"
          aria-label="Search resources, services, and docs"
          placeholder="Search resources, services, and docs (G+/)"
        />
      </div>
      <div className="az-header-right">
        {/* The real Azure portal header carries Cloud Shell, Notifications,
         * and Help alongside the account and Settings controls. This
         * simulator has no Cloud Shell runtime, no operation queue to
         * notify from, and no support surface — each stays a real, focusable
         * control rather than disappearing, disclosing that honestly instead
         * of faking the feature. Settings is the theme toggle below. */}
        <AzureHeaderPopover icon="cloud_shell" label="Cloud Shell" testid="az-cloud-shell">
          <h2>Cloud Shell</h2>
          <p>
            Azure Cloud Shell isn&rsquo;t available in this simulator. Use the <code>az</code> CLI installed locally,
            pointed at this deployment&rsquo;s endpoint, for an equivalent browser-free shell.
          </p>
        </AzureHeaderPopover>
        <AzureHeaderPopover icon="notifications" label="Notifications" testid="az-notifications">
          <h2>Notifications</h2>
          <p>
            This simulator doesn&rsquo;t emit portal notifications — there&rsquo;s no background operation queue to
            report on. Command results and errors surface inline on each blade instead.
          </p>
        </AzureHeaderPopover>
        <AzureHeaderPopover icon="help" label="Help and support" testid="az-help">
          <h2>Help + support</h2>
          <p>
            Help and support isn&rsquo;t available in this simulator. See the real{" "}
            <a href="https://learn.microsoft.com/azure/" target="_blank" rel="noreferrer">
              Azure documentation
            </a>{" "}
            for the services it simulates.
          </p>
        </AzureHeaderPopover>
        {account}
        <AzureThemeToggle />
      </div>
    </header>
  );
}

export function AzureBreadcrumbs({ trail }: { trail: { label: string; to?: string }[] }) {
  return (
    <nav aria-label="Breadcrumbs" className="az-breadcrumbs">
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

/** The portal titles a working pane with the resource, its type beneath, and
 *  the directory it belongs to — not with the service's name alone. It sits
 *  above the service menu and the main pane, spanning both, so it is its own
 *  landmark region — named from the same heading a sighted operator reads —
 *  rather than content orphaned outside every landmark on the page. */
export function AzureResourceTitle({ name, kind, directory }: { name: string; kind: string; directory: string }) {
  return (
    <div className="az-resource-title" role="region" aria-labelledby="az-resource-title-heading">
      <h1 id="az-resource-title-heading">{name}</h1>
      <p>
        <span>{kind}</span>
        <span className="az-directory">Directory: {directory}</span>
      </p>
    </div>
  );
}

export function AzureCommandBar({ commands }: { commands: PortalCommand[] }) {
  return (
    <div className="az-command-bar" role="toolbar" aria-label="Commands">
      {commands.map((command) => (
        <button
          key={command.label}
          type="button"
          className="az-command"
          onClick={command.onSelect}
          disabled={command.disabled}
          data-testid={command.testid}
        >
          <span className="az-command-glyph"><AzureIcon name={command.icon} size={16} /></span>
          {command.label}
        </button>
      ))}
    </div>
  );
}

/** A Fluent-style pill marking a service the real Azure portal offers that
 *  this simulator does not implement. State never rests on colour alone: the
 *  badge carries its own icon and the words "Not supported" as real text, so
 *  it reads the same to a screen reader as to a sighted operator. */
export function AzureNotSupportedBadge() {
  return (
    <span className="az-badge az-badge-unsupported">
      <AzureIcon name="not_supported" size={12} />
      Not supported
    </span>
  );
}

function ServiceMenuLink({ item, end }: { item: ServiceMenuItem; end?: boolean }) {
  const unsupported = item.supported === false;
  return (
    <NavLink
      to={item.to}
      end={end}
      // aria-label replaces the link's accessible name outright, so an
      // unsupported service announces why it's there rather than just that
      // it's there — "not supported in this simulator" reads the same for a
      // screen-reader operator as the visible badge reads for a sighted one.
      aria-label={unsupported ? `${item.label}, not supported in this simulator` : undefined}
      className={({ isActive }) =>
        [
          "az-service-link",
          isActive ? "az-service-link-active" : "",
          unsupported ? "az-service-link-unsupported" : "",
        ]
          .filter(Boolean)
          .join(" ")
      }
    >
      <span className="az-service-label">{item.label}</span>
      {unsupported ? <AzureNotSupportedBadge /> : null}
    </NavLink>
  );
}

/** The service menu carries its own search and expandable groups, so a long
 *  menu stays navigable without leaving the resource. It lists the real
 *  Azure catalog — including services this simulator does not implement,
 *  marked rather than hidden — so the menu reads as the real portal's
 *  information architecture. */
export function AzureServiceMenu({ groups, flat }: { groups: ServiceMenuGroup[]; flat: ServiceMenuItem[] }) {
  const [filter, setFilter] = useState("");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const needle = filter.trim().toLowerCase();
  const matches = (item: ServiceMenuItem) => !needle || item.label.toLowerCase().includes(needle);

  return (
    <nav aria-label="Service" className="az-service-menu">
      <input
        type="search"
        className="az-service-search"
        value={filter}
        aria-label="Search the service menu"
        placeholder="Search"
        onChange={(event) => setFilter(event.target.value)}
      />
      <ul className="az-service-flat">
        {flat.filter(matches).map((item) => (
          <li key={item.to}>
            <ServiceMenuLink item={item} end={item.to === "/ui/"} />
          </li>
        ))}
      </ul>
      {groups.map((group) => {
        const visible = group.items.filter(matches);
        if (visible.length === 0) return null;
        // A search narrows to what matched, so a group is opened to show its
        // matches even when the operator had collapsed it.
        const open = needle !== "" || !collapsed.has(group.label);
        return (
          <div className="az-service-group" key={group.label}>
            <button
              type="button"
              className="az-service-group-toggle"
              aria-expanded={open}
              onClick={() =>
                setCollapsed((current) => {
                  const next = new Set(current);
                  if (next.has(group.label)) next.delete(group.label);
                  else next.add(group.label);
                  return next;
                })
              }
            >
              <span className="az-chevron"><AzureIcon name={open ? "chevron_down" : "chevron_right"} size={14} /></span>
              {group.label}
            </button>
            {open ? (
              <ul>
                {visible.map((item) => (
                  <li key={item.to}>
                    <ServiceMenuLink item={item} />
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
        );
      })}
    </nav>
  );
}

export interface EssentialsProperty {
  label: string;
  value: ReactNode;
}

/** Essentials presents a resource's properties as a two-column key/value grid
 *  under a collapsible header, which is how the portal leads every resource
 *  page before its tabs. */
export function AzureEssentials({ properties }: { properties: EssentialsProperty[] }) {
  const [open, setOpen] = useState(true);
  return (
    <section className="az-essentials" aria-label="Essentials">
      <button type="button" className="az-essentials-toggle" aria-expanded={open} onClick={() => setOpen((on) => !on)}>
        <span className="az-chevron"><AzureIcon name={open ? "chevron_down" : "chevron_right"} size={14} /></span>
        Essentials
      </button>
      {open ? (
        <dl className="az-essentials-grid">
          {properties.map((property) => (
            <div className="az-essentials-pair" key={property.label}>
              <dt>{property.label}</dt>
              <dd>{property.value}</dd>
            </div>
          ))}
        </dl>
      ) : null}
    </section>
  );
}

export type AzureStatusKind = "success" | "error" | "warning" | "inactive";

export function AzureStatus({ status, kind: explicitKind }: { status: string; kind?: AzureStatusKind }) {
  // Matched on whole words. A substring test reports success for failure
  // states, because "unavailable" contains "available" and "inactive" contains
  // "active", and a tick beside a failed resource stops an operator looking
  // any further.
  const words = status.toLowerCase().split(/[^a-z]+/).filter(Boolean);
  const has = (...candidates: string[]) => candidates.some((candidate) => words.includes(candidate));
  const derived: AzureStatusKind = has(
    "unavailable",
    "stopped",
    "stopping",
    "failed",
    "failure",
    "error",
    "deleted",
    "inactive",
    "canceled",
    "cancelled",
  )
    ? "error"
    : has("pending", "provisioning", "creating", "updating", "deleting", "starting")
      ? "warning"
      : has("running", "active", "available", "succeeded", "success", "ok", "healthy", "ready")
        ? "success"
        : "inactive";
  const kind = explicitKind ?? derived;
  const iconName: AzureIconName =
    kind === "success"
      ? "status_success"
      : kind === "error"
        ? "status_error"
        : kind === "warning"
          ? "status_warning"
          : "status_inactive";
  return (
    <span className={`az-status az-status-${kind}`}>
      <AzureIcon name={iconName} size={15} />
      {status}
    </span>
  );
}
