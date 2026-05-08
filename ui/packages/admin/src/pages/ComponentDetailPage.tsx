import { useParams } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  MetricsCard,
  PageHeading,
  Spinner,
  StatusBadge,
} from "@sockerless/ui-core/components";
import {
  AdminApiClient,
  type AdminComponent,
  type ComponentStatus,
  type ComponentMetrics,
} from "../api.js";
import { ErrorPanel } from "../components/ErrorPanel.js";

const api = new AdminApiClient();

export function ComponentDetailPage() {
  const { name } = useParams<{ name: string }>();
  const queryClient = useQueryClient();

  const {
    data: components,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["components"],
    queryFn: () => api.components(),
  });

  const comp = components?.find((c: AdminComponent) => c.name === name);

  const { data: status } = useQuery({
    queryKey: ["component-status", name],
    queryFn: () => api.componentStatus(name!),
    enabled: !!name,
    refetchInterval: 5000,
  });

  const { data: metrics } = useQuery({
    queryKey: ["component-metrics", name],
    queryFn: () => api.componentMetrics(name!),
    enabled: !!name,
    refetchInterval: 5000,
  });

  const { data: provider } = useQuery({
    queryKey: ["component-provider", name],
    queryFn: () => api.componentProvider(name!),
    enabled: !!name && comp?.type === "backend",
  });

  const reload = useMutation({
    mutationFn: () => api.componentReload(name!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["components"] });
      queryClient.invalidateQueries({ queryKey: ["component-status", name] });
      queryClient.invalidateQueries({ queryKey: ["component-metrics", name] });
      queryClient.invalidateQueries({ queryKey: ["component-provider", name] });
    },
  });

  if (isLoading) return <Spinner label="loading component" />;
  if (isError) return <ErrorPanel message={error?.message} />;
  if (!components) return <Spinner label="loading component" />;
  if (!comp) {
    return (
      <ErrorPanel
        kicker="not found"
        message={`component "${name ?? ""}" is not registered`}
      />
    );
  }

  const statusObj: ComponentStatus | undefined = status;
  const metricsObj: ComponentMetrics | undefined = metrics;

  const reloadable = comp.type === "backend" || comp.type === "frontend";

  return (
    <div>
      <PageHeading
        kicker={`admin · ${comp.type}`}
        title={comp.name}
        meta={
          <span className="inline-flex items-center gap-3">
            <StatusBadge
              status={
                comp.health === "up"
                  ? "ok"
                  : comp.health === "unknown"
                    ? "warning"
                    : "error"
              }
            />
            <span>{comp.addr}</span>
          </span>
        }
        actions={
          reloadable && (
            <Button
              variant="primary"
              size="sm"
              onClick={() => reload.mutate()}
              disabled={reload.isPending}
            >
              {reload.isPending ? "Reloading…" : "Reload"}
            </Button>
          )
        }
      />

      {reload.error && (
        <div className="mb-4">
          <ErrorPanel
            kicker="reload failed"
            message={(reload.error as Error)?.message}
          />
        </div>
      )}

      <div className="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <MetricsCard title="Type" value={comp.type} />
        <MetricsCard title="Address" value={comp.addr} />
        <MetricsCard
          title="Uptime"
          value={
            comp.uptime > 0
              ? (() => {
                  const h = Math.floor(comp.uptime / 3600);
                  const m = Math.floor((comp.uptime % 3600) / 60);
                  return h > 0 ? `${h}h ${m}m` : `${m}m`;
                })()
              : "—"
          }
        />
        <MetricsCard
          title="Containers"
          value={
            statusObj?.containers != null ? String(statusObj.containers) : "—"
          }
          emphasized={(statusObj?.containers ?? 0) > 0}
        />
      </div>

      {provider && comp.type === "backend" && (
        <section
          className="mb-6 px-4 py-4"
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
            Cloud connection
          </div>
          <div className="grid gap-4 sm:grid-cols-3 font-mono text-[13px]">
            <KV k="provider" v={provider.provider} />
            <div>
              <div
                className="text-[10px] uppercase tracking-[0.18em] mb-1"
                style={{ color: "var(--color-fg-subtle)" }}
              >
                mode
              </div>
              <StatusBadge
                status={
                  provider.mode === "cloud"
                    ? "ok"
                    : provider.mode === "custom-endpoint"
                      ? "warning"
                      : "stopped"
                }
              />
            </div>
            {provider.region && <KV k="region" v={provider.region} />}
          </div>
          {provider.endpoint && (
            <div className="mt-4">
              <KV k="endpoint" v={provider.endpoint} mono />
            </div>
          )}
          {provider.resources && Object.keys(provider.resources).length > 0 && (
            <div className="mt-4">
              <div
                className="mb-1 text-[10px] uppercase tracking-[0.18em]"
                style={{ color: "var(--color-fg-subtle)" }}
              >
                resources
              </div>
              <div className="space-y-1 font-mono text-[13px]">
                {Object.entries(provider.resources).map(([k, v]) => (
                  <div key={k} className="flex gap-2">
                    <span style={{ color: "var(--color-fg-subtle)" }}>{k}:</span>
                    <span style={{ color: "var(--color-fg)" }}>{v}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </section>
      )}

      {statusObj && (
        <section className="mb-6">
          <h3
            className="mb-2 text-[10px] uppercase tracking-[0.22em]"
            style={{ color: "var(--color-fg-subtle)" }}
          >
            Status
          </h3>
          <pre
            className="font-mono"
            style={{
              background: "var(--color-bg-subtle)",
              border: "1px solid var(--color-border)",
              borderRadius: "var(--radius-sm)",
              padding: "1rem",
              fontSize: "0.72rem",
              color: "var(--color-fg)",
              overflow: "auto",
              margin: 0,
            }}
          >
            {JSON.stringify(statusObj, null, 2)}
          </pre>
        </section>
      )}

      {metricsObj && (
        <section>
          <h3
            className="mb-2 text-[10px] uppercase tracking-[0.22em]"
            style={{ color: "var(--color-fg-subtle)" }}
          >
            Metrics
          </h3>
          <pre
            className="font-mono"
            style={{
              background: "var(--color-bg-subtle)",
              border: "1px solid var(--color-border)",
              borderRadius: "var(--radius-sm)",
              padding: "1rem",
              fontSize: "0.72rem",
              color: "var(--color-fg)",
              overflow: "auto",
              margin: 0,
            }}
          >
            {JSON.stringify(metricsObj, null, 2)}
          </pre>
        </section>
      )}
    </div>
  );
}

function KV({ k, v, mono }: { k: string; v: string; mono?: boolean }) {
  return (
    <div>
      <div
        className="text-[10px] uppercase tracking-[0.18em] mb-1"
        style={{ color: "var(--color-fg-subtle)" }}
      >
        {k}
      </div>
      <div
        className={mono ? "font-mono" : ""}
        style={{ color: "var(--color-fg)", fontWeight: mono ? 400 : 500 }}
      >
        {v}
      </div>
    </div>
  );
}
