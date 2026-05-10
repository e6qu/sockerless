import { useState } from "react";
import { Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import {
  Button,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import {
  AdminApiClient,
  type RollupResource,
  type RollupSource,
} from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

const api = new AdminApiClient();

type GroupKey = "instance" | "cloud" | "service" | "flat";

const GROUP_OPTIONS: { value: GroupKey; label: string }[] = [
  { value: "instance", label: "instance" },
  { value: "cloud", label: "cloud" },
  { value: "service", label: "service product" },
  { value: "flat", label: "flat" },
];

/**
 * TopologyResourcesPage — rollup of cloud resources tracked by every
 * running backend instance in the current sockerless.yaml topology.
 *
 * Pivots: by sockerless instance (default), by cloud, by service
 * product (resource_type), or flat.
 *
 * Source-status banner surfaces backends the rollup couldn't reach,
 * so "0 resources" stays distinguishable from "couldn't query".
 */
export function TopologyResourcesPage() {
  const [activeOnly, setActiveOnly] = useState(true);
  const [grouping, setGrouping] = useState<GroupKey>("instance");

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["topology-resources", activeOnly],
    queryFn: () => api.topologyResources(activeOnly),
    refetchInterval: 5000,
  });

  if (isLoading) return <Spinner label="loading resources" />;
  if (isError) return <ErrorPanel message={error?.message} />;
  if (!data) return <Spinner label="loading resources" />;

  const okSources = data.sources.filter((s) => s.ok).length;
  const failedSources = data.sources.filter((s) => !s.ok);
  const groups = groupResources(data.resources, grouping);

  return (
    <div className="flex flex-col gap-6">
      <PageHeading
        kicker="admin · topology"
        title={<>Cloud resources</>}
        meta={
          <span className="inline-flex items-center gap-3">
            <Link
              to="/ui/topology"
              style={{ color: "var(--color-accent)", textDecoration: "none" }}
            >
              ← topology
            </Link>
            <span>
              {data.resources.length} resource
              {data.resources.length === 1 ? "" : "s"} from {okSources}/
              {data.sources.length} backend
              {data.sources.length === 1 ? "" : "s"}
            </span>
          </span>
        }
        actions={
          <div className="inline-flex gap-2">
            <Button
              variant={activeOnly ? "primary" : "ghost"}
              size="sm"
              onClick={() => setActiveOnly((v) => !v)}
            >
              {activeOnly ? "active only" : "include cleaned"}
            </Button>
          </div>
        }
      />

      {failedSources.length > 0 && (
        <FailedSourcesBanner sources={failedSources} />
      )}

      <div
        className="inline-flex"
        style={{
          background: "var(--color-bg-subtle)",
          border: "1px solid var(--color-border)",
          borderRadius: "var(--radius-sm)",
          padding: "2px",
          width: "fit-content",
        }}
      >
        {GROUP_OPTIONS.map((opt) => {
          const active = grouping === opt.value;
          return (
            <button
              key={opt.value}
              type="button"
              onClick={() => setGrouping(opt.value)}
              className="px-3 py-1 font-mono uppercase tracking-[0.12em]"
              style={{
                fontSize: "0.7rem",
                background: active ? "var(--color-accent)" : "transparent",
                color: active ? "var(--color-accent-fg)" : "var(--color-fg-muted)",
                border: 0,
                borderRadius: "2px",
                transition: "all 0.12s var(--ease-out-quint)",
                cursor: "pointer",
              }}
            >
              by {opt.label}
            </button>
          );
        })}
      </div>

      {groups.length === 0 ? (
        <p
          className="font-mono uppercase tracking-[0.18em] py-6 text-center"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.7rem" }}
        >
          — no resources —
        </p>
      ) : (
        <div className="flex flex-col gap-4">
          {groups.map((g) => (
            <GroupCard key={g.key} title={g.label} entries={g.entries} />
          ))}
        </div>
      )}
    </div>
  );
}

function FailedSourcesBanner({ sources }: { sources: RollupSource[] }) {
  return (
    <section
      style={{
        background: "var(--color-status-warn-soft)",
        border: "1px solid var(--color-status-warn)",
        borderRadius: "var(--radius-sm)",
        padding: "0.85rem 1rem",
      }}
    >
      <div
        className="font-mono uppercase tracking-[0.18em] mb-1"
        style={{ color: "var(--color-status-warn)", fontSize: "0.62rem" }}
      >
        {sources.length} backend{sources.length === 1 ? "" : "s"} unreachable
      </div>
      <ul
        className="font-mono"
        style={{ color: "var(--color-fg-muted)", fontSize: "0.78rem" }}
      >
        {sources.map((s) => (
          <li key={`${s.project}/${s.instance}`}>
            <span style={{ color: "var(--color-fg)" }}>
              {s.project}/{s.instance}
            </span>{" "}
            <span style={{ color: "var(--color-fg-subtle)" }}>
              (:{s.port})
            </span>{" "}
            — {s.error}
          </li>
        ))}
      </ul>
    </section>
  );
}

function GroupCard({
  title,
  entries,
}: {
  title: string;
  entries: RollupResource[];
}) {
  return (
    <section
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-sm)",
      }}
    >
      <header
        className="flex items-center justify-between px-4 py-3"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <div
          className="font-display"
          style={{
            fontStyle: "italic",
            fontWeight: 600,
            fontSize: "1rem",
            letterSpacing: "-0.02em",
          }}
        >
          {title}
        </div>
        <div
          className="font-mono uppercase tracking-[0.18em]"
          style={{ color: "var(--color-fg-subtle)", fontSize: "0.62rem" }}
        >
          {entries.length} resource{entries.length === 1 ? "" : "s"}
        </div>
      </header>
      <ul className="divide-y" style={{ borderColor: "var(--color-border)" }}>
        {entries.map((e, i) => (
          <li
            key={`${e.project}/${e.instance}/${e.resource_id}/${i}`}
            className="flex flex-wrap items-center justify-between gap-3 px-4 py-2"
          >
            <div className="flex flex-wrap items-center gap-2.5">
              <span
                className="font-mono uppercase tracking-[0.1em]"
                style={{
                  color: "var(--color-fg-subtle)",
                  fontSize: "0.62rem",
                }}
              >
                {e.resource_type}
              </span>
              <span
                className="font-mono"
                style={{ color: "var(--color-fg)", fontSize: "0.82rem" }}
              >
                {e.resource_id}
              </span>
              <span
                className="font-mono"
                style={{
                  color: "var(--color-fg-subtle)",
                  fontSize: "0.7rem",
                }}
              >
                {e.project}/{e.instance}
                {e.cloud ? ` · ${e.cloud}` : ""}
                {e.backend ? ` · ${e.backend}` : ""}
              </span>
            </div>
            <div className="flex items-center gap-2">
              {e.status && <StatusBadge status={e.status} />}
              {e.cleaned_up && (
                <span
                  className="font-mono uppercase tracking-[0.1em]"
                  style={{
                    color: "var(--color-fg-subtle)",
                    fontSize: "0.6rem",
                  }}
                >
                  cleaned
                </span>
              )}
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}

interface ResourceGroup {
  key: string;
  label: string;
  entries: RollupResource[];
}

function groupResources(
  resources: RollupResource[],
  by: GroupKey,
): ResourceGroup[] {
  if (by === "flat") {
    if (resources.length === 0) return [];
    return [{ key: "all", label: "All resources", entries: resources }];
  }
  const buckets = new Map<string, ResourceGroup>();
  for (const r of resources) {
    const key = groupKey(r, by);
    let g = buckets.get(key);
    if (!g) {
      g = { key, label: groupLabel(r, by, key), entries: [] };
      buckets.set(key, g);
    }
    g.entries.push(r);
  }
  return [...buckets.values()].sort((a, b) => a.key.localeCompare(b.key));
}

function groupKey(r: RollupResource, by: GroupKey): string {
  switch (by) {
    case "instance":
      return `${r.project}/${r.instance}`;
    case "cloud":
      return r.cloud ?? "(unknown)";
    case "service":
      return r.resource_type || "(unknown)";
    case "flat":
      return "all";
  }
}

function groupLabel(r: RollupResource, by: GroupKey, _key: string): string {
  switch (by) {
    case "instance":
      return `${r.project} / ${r.instance}` + (r.backend ? ` (${r.backend})` : "");
    case "cloud":
      return r.cloud ?? "(unknown)";
    case "service":
      return r.resource_type || "(unknown)";
    case "flat":
      return "All resources";
  }
}

// Exported for unit testing the grouping logic without rendering.
export const __test = { groupResources };
