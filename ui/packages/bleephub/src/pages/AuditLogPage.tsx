import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { DataTable, InlineError, Spinner } from "@sockerless/ui-core/components";
import { createColumnHelper } from "@tanstack/react-table";
import { fetchAuditLog } from "../api.js";
import type { BleephubAuditEvent } from "../types.js";
import { Button, FormLabel, PageTitle } from "../components/ui.js";

const col = createColumnHelper<BleephubAuditEvent>();

export function AuditLogPage() {
  const [actor, setActor] = useState("");
  const [action, setAction] = useState("");
  const [entityType, setEntityType] = useState("");
  const [since, setSince] = useState("");
  const [until, setUntil] = useState("");
  const [appliedFilters, setAppliedFilters] = useState({
    actor: "",
    action: "",
    entity_type: "",
    since: "",
    until: "",
  });

  const filters = {
    actor: appliedFilters.actor || undefined,
    action: appliedFilters.action || undefined,
    entity_type: appliedFilters.entity_type || undefined,
    since: appliedFilters.since || undefined,
    until: appliedFilters.until || undefined,
  };

  const { data, isLoading, isError } = useQuery({
    queryKey: ["audit-log", appliedFilters],
    queryFn: () => fetchAuditLog(filters),
  });

  const apply = () =>
    setAppliedFilters({
      actor: actor.trim(),
      action: action.trim(),
      entity_type: entityType.trim(),
      since,
      until,
    });

  if (isError) return <InlineError title="Failed to load audit log" />;

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
      <PageTitle title="Audit log" meta="Administrative events." />

      <div
        className="mb-5 grid gap-3"
        style={{ gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))" }}
      >
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
          <FormLabel id="filter-entity">Entity type</FormLabel>
          <input
            id="filter-entity"
            type="text"
            value={entityType}
            onChange={(e) => setEntityType(e.target.value)}
            placeholder="user"
          />
        </div>
        <div>
          <FormLabel id="filter-since">Since</FormLabel>
          <input
            id="filter-since"
            type="datetime-local"
            value={since}
            onChange={(e) => setSince(e.target.value)}
          />
        </div>
        <div>
          <FormLabel id="filter-until">Until</FormLabel>
          <input
            id="filter-until"
            type="datetime-local"
            value={until}
            onChange={(e) => setUntil(e.target.value)}
          />
        </div>
        <div className="flex items-end">
          <Button onClick={apply} variant="secondary" size="sm">
            Apply filters
          </Button>
        </div>
      </div>

      {isLoading || !data ? (
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
