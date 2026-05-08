import { useQuery } from "@tanstack/react-query";
import {
  AppShell,
  MetricsCard,
  NavLinkButton,
  PageHeading,
  Spinner,
  StatusBadge,
  type NavItem,
} from "@sockerless/ui-core/components";
import { fetchHealth, fetchStatus, fetchMetrics } from "./api.js";

const navItems: NavItem[] = [{ label: "Overview", to: "/ui/" }];

function renderLink(item: NavItem) {
  return <NavLinkButton active>{item.label}</NavLinkButton>;
}

export function App() {
  const { data: health } = useQuery({
    queryKey: ["frontend-health"],
    queryFn: fetchHealth,
    refetchInterval: 10_000,
  });
  const { data: status, isLoading } = useQuery({
    queryKey: ["frontend-status"],
    queryFn: fetchStatus,
    refetchInterval: 5_000,
  });
  const { data: metrics } = useQuery({
    queryKey: ["frontend-metrics"],
    queryFn: fetchMetrics,
    refetchInterval: 5_000,
  });

  return (
    <AppShell
      kicker="docker · frontend"
      title="docker.frontend"
      navItems={navItems}
      renderLink={renderLink}
    >
      {isLoading ? (
        <Spinner label="loading frontend" />
      ) : (
        <div>
          <PageHeading
            kicker="docker · proxy"
            title={<>Docker frontend</>}
            meta={
              status
                ? `${status.docker_addr} → ${status.backend_addr} · uptime ${formatUptime(status.uptime_seconds)}`
                : undefined
            }
          />

          <div className="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-4">
            <MetricsCard
              title="Docker requests"
              value={metrics?.docker_requests ?? 0}
              emphasized={(metrics?.docker_requests ?? 0) > 0}
            />
            <MetricsCard title="Goroutines" value={metrics?.goroutines ?? 0} />
            <MetricsCard
              title="Heap"
              value={`${(metrics?.heap_alloc_mb ?? 0).toFixed(1)} MB`}
            />
            <MetricsCard
              title="Uptime"
              value={formatUptime(status?.uptime_seconds ?? 0)}
            />
          </div>

          {status && (
            <div
              className="px-4 py-4 mb-4"
              style={{
                background: "var(--color-surface)",
                border: "1px solid var(--color-border)",
                borderLeft: "3px solid var(--color-accent)",
                borderRadius: "var(--radius-sm)",
              }}
            >
              <div
                className="mb-3 text-[10px] uppercase tracking-[0.22em]"
                style={{ color: "var(--color-fg-subtle)" }}
              >
                Configuration
              </div>
              <dl className="grid grid-cols-[8rem_1fr] gap-x-4 gap-y-2 text-[13px] font-mono">
                <dt style={{ color: "var(--color-fg-subtle)" }}>docker_addr</dt>
                <dd style={{ color: "var(--color-fg)" }}>{status.docker_addr}</dd>
                <dt style={{ color: "var(--color-fg-subtle)" }}>backend_addr</dt>
                <dd style={{ color: "var(--color-fg)" }}>{status.backend_addr}</dd>
              </dl>
            </div>
          )}

          {health && (
            <p
              className="font-mono text-[11px] inline-flex items-center gap-2"
              style={{ color: "var(--color-fg-subtle)" }}
            >
              health: <StatusBadge status={health.status} /> · component: {health.component}
            </p>
          )}
        </div>
      )}
    </AppShell>
  );
}

function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return "—";
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (m < 60) return `${m}m ${s}s`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}
