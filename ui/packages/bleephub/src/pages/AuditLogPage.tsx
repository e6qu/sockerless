import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { buildAuditLogPhrase, fetchAuditLog, fetchAuditLogOrgs } from "../api.js";
import type { BleephubAuditEvent } from "../types.js";
import { Button, FormLabel, PageTitle } from "../components/ui.js";

const col = createColumnHelper<BleephubAuditEvent>();

export function AuditLogPage() {
  const [org, setOrg] = useState("");
  const [actor, setActor] = useState("");
  const [action, setAction] = useState("");
  const [phrase, setPhrase] = useState("");
  const [order, setOrder] = useState<"desc" | "asc">("desc");
  const [appliedFilters, setAppliedFilters] = useState({
    org: "",
    actor: "",
    action: "",
    phrase: "",
    order: "desc" as "desc" | "asc",
  });

  const { data: orgs, isLoading: orgsLoading, isError: orgsError } = useQuery({
    queryKey: ["audit-log-orgs"],
    queryFn: fetchAuditLogOrgs,
  });

  const effectiveOrg = appliedFilters.org || orgs?.[0]?.login || "";
  const effectivePhrase = buildAuditLogPhrase({
    actor: appliedFilters.actor || undefined,
    action: appliedFilters.action || undefined,
    text: appliedFilters.phrase || undefined,
  });

  const { data, isLoading, isError } = useQuery({
    queryKey: ["audit-log", appliedFilters],
    queryFn: () =>
      fetchAuditLog({
        org: effectiveOrg,
        phrase: effectivePhrase || undefined,
        order: appliedFilters.order,
      }),
    enabled: Boolean(effectiveOrg),
  });

  const apply = () =>
    setAppliedFilters({
      org: org.trim() || effectiveOrg,
      actor: actor.trim(),
      action: action.trim(),
      phrase: phrase.trim(),
      order,
    });

  if (orgsError || isError) return <InlineError title="Failed to load audit log" />;

  const columns = [
    col.accessor("id", {
      header: "ID",
      cell: (info) => (
        <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
          {info.getValue()}
        </span>
      ),
    }),
    col.accessor("created_at", {
      header: "Time",
      cell: (info) => new Date(info.getValue()).toLocaleString(),
    }),
    col.accessor("actor_login", {
      header: "Actor",
      cell: (info) => <span style={{ fontWeight: 500, color: "var(--color-fg)" }}>{info.getValue()}</span>,
    }),
    col.accessor("action", {
      header: "Action",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>,
    }),
    col.accessor("entity_type", {
      header: "Entity type",
      cell: (info) => <span style={{ color: "var(--color-fg-muted)" }}>{info.getValue()}</span>,
    }),
    col.accessor("entity_id", {
      header: "Entity ID",
      cell: (info) => (
        <span className="tabular-nums" style={{ color: "var(--color-fg-muted)" }}>
          {String(info.getValue())}
        </span>
      ),
    }),
    col.accessor("details", {
      header: "Details",
      cell: (info) => {
        const details = info.getValue();
        return (
          <pre
            style={{
              margin: 0,
              fontSize: "0.75rem",
              color: "var(--color-fg-muted)",
              maxWidth: "24rem",
              overflow: "auto",
            }}
          >
            {JSON.stringify(details, null, 2)}
          </pre>
        );
      },
    }),
  ];

  return (
    <div>
      <PageTitle title="Audit log" meta="GitHub Enterprise Server organization audit events." />

      <div
        className="mb-5 grid gap-3"
        style={{ gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))" }}
      >
        <div>
          <FormLabel id="filter-org">Organization</FormLabel>
          <select id="filter-org" value={org || effectiveOrg} onChange={(e) => setOrg(e.target.value)}>
            {(orgs ?? []).map((o) => (
              <option key={o.login} value={o.login}>
                {o.login}
              </option>
            ))}
          </select>
        </div>
        <div>
          <FormLabel id="filter-actor">Actor</FormLabel>
          <input
            id="filter-actor"
            type="text"
            value={actor}
            onChange={(e) => setActor(e.target.value)}
            placeholder="username"
          />
        </div>
        <div>
          <FormLabel id="filter-action">Action</FormLabel>
          <input
            id="filter-action"
            type="text"
            value={action}
            onChange={(e) => setAction(e.target.value)}
            placeholder="create_user"
          />
        </div>
        <div>
          <FormLabel id="filter-phrase">Search phrase</FormLabel>
          <input
            id="filter-phrase"
            type="text"
            value={phrase}
            onChange={(e) => setPhrase(e.target.value)}
            placeholder="repository settings"
          />
        </div>
        <div>
          <FormLabel id="filter-order">Order</FormLabel>
          <select id="filter-order" value={order} onChange={(e) => setOrder(e.target.value as "desc" | "asc")}>
            <option value="desc">Newest first</option>
            <option value="asc">Oldest first</option>
          </select>
        </div>
        <div className="flex items-end">
          <Button onClick={apply} variant="secondary" size="sm">
            Apply filters
          </Button>
        </div>
      </div>

      {!effectiveOrg && !orgsLoading ? (
        <InlineError title="No organization audit log is available" detail="The authenticated user does not belong to an organization." />
      ) : orgsLoading || isLoading || !data ? (
        <Spinner label="loading audit log" />
      ) : (
        <DataTable
          data={data}
          columns={columns}
          filterPlaceholder="Filter events…"
          emptyMessage="No audit events match the filters."
        />
      )}
    </div>
  );
}
