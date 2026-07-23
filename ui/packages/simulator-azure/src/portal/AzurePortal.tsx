import { useEffect, useState, type ReactNode } from "react";
import { NavLink } from "react-router";
import { AzureIcon, type AzureIconName } from "./icons.js";

export interface ServiceMenuItem {
  label: string;
  to: string;
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
}

function AzureThemeToggle() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
  }, [dark]);
  const label = dark ? "Switch to light theme" : "Switch to dark theme";
  return (
    <button type="button" className="az-icon-button" onClick={() => setDark((on) => !on)} aria-label={label} title={label}>
      <AzureIcon name={dark ? "sun" : "moon"} size={18} />
    </button>
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
 *  the directory it belongs to — not with the service's name alone. */
export function AzureResourceTitle({ name, kind, directory }: { name: string; kind: string; directory: string }) {
  return (
    <div className="az-resource-title">
      <h1>{name}</h1>
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
        >
          <span className="az-command-glyph"><AzureIcon name={command.icon} size={16} /></span>
          {command.label}
        </button>
      ))}
    </div>
  );
}

/** The service menu carries its own search and expandable groups, so a long
 *  menu stays navigable without leaving the resource. */
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
            <NavLink
              to={item.to}
              end={item.to === "/ui/"}
              className={({ isActive }) => (isActive ? "az-service-link az-service-link-active" : "az-service-link")}
            >
              {item.label}
            </NavLink>
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
                    <NavLink
                      to={item.to}
                      className={({ isActive }) => (isActive ? "az-service-link az-service-link-active" : "az-service-link")}
                    >
                      {item.label}
                    </NavLink>
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
