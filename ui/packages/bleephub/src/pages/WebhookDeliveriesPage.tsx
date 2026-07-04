import { useState } from "react";
import { useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import {
  fetchHookDeliveriesPage,
  fetchHookDelivery,
  redeliverHookDelivery,
} from "../api.js";
import type { HookScope } from "../api.js";
import type { GithubHookDelivery } from "../types.js";
import { OrgHeader, RepoHeader } from "../components/Shell.js";
import { PageTitle, Box, Blankslate, Button, CodeBlock, ErrorBanner } from "../components/ui.js";
import { ChevronDownIcon, ChevronRightIcon } from "../components/octicons.js";

/**
 * Webhook delivery drill-down for a repository or organization hook —
 * the route decides the scope: /ui/repos/{o}/{r}/hooks/{id}/deliveries
 * vs /ui/orgs/{org}/hooks/{id}/deliveries.
 */
export function WebhookDeliveriesPage() {
  const { owner, repo, org, hookId = "" } = useParams<{
    owner?: string;
    repo?: string;
    org?: string;
    hookId: string;
  }>();
  const id = parseInt(hookId, 10);
  const scope: HookScope | null =
    owner && repo
      ? { kind: "repo", owner, repo }
      : org
        ? { kind: "org", org }
        : null;

  if (scope === null || Number.isNaN(id)) {
    return <InlineError title="Invalid webhook route" detail="missing owner/repo or org, or non-numeric hook id" />;
  }

  return (
    <div>
      {scope.kind === "repo" ? (
        <RepoHeader owner={scope.owner} repo={scope.repo} active="code" />
      ) : (
        <OrgHeader org={scope.org} active="hooks" />
      )}
      <PageTitle title={`Webhook #${id} deliveries`} />
      <DeliveriesList scope={scope} hookId={id} />
    </div>
  );
}

function scopeKey(scope: HookScope): string {
  return scope.kind === "repo" ? `${scope.owner}/${scope.repo}` : `org:${scope.org}`;
}

function DeliveriesList({ scope, hookId }: { scope: HookScope; hookId: number }) {
  const [extra, setExtra] = useState<GithubHookDelivery[]>([]);
  const [nextUrl, setNextUrl] = useState<string | null>(null);
  const [pageError, setPageError] = useState<string | null>(null);

  const firstPage = useQuery({
    queryKey: ["hook-deliveries", scopeKey(scope), hookId],
    queryFn: () => fetchHookDeliveriesPage(scope, hookId),
  });

  if (firstPage.isLoading) return <Spinner label="loading deliveries" />;
  if (firstPage.isError)
    return <InlineError title="Failed to load deliveries" detail={String(firstPage.error)} />;

  const deliveries = [...(firstPage.data?.items ?? []), ...extra];
  const followUrl = nextUrl ?? firstPage.data?.nextUrl ?? null;

  if (deliveries.length === 0)
    return (
      <Blankslate title="No deliveries yet">
        Events delivered to this webhook appear here with their request and response payloads.
      </Blankslate>
    );

  const loadMore = async () => {
    if (!followUrl) return;
    try {
      const page = await fetchHookDeliveriesPage(scope, hookId, followUrl);
      setExtra((prev) => [...prev, ...page.items]);
      setNextUrl(page.nextUrl);
      setPageError(null);
    } catch (err) {
      setPageError(String(err));
    }
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}>
      {pageError && <ErrorBanner>{pageError}</ErrorBanner>}
      <Box>
        {deliveries.map((d, i) => (
          <DeliveryRow
            key={d.id}
            scope={scope}
            hookId={hookId}
            delivery={d}
            last={i === deliveries.length - 1}
          />
        ))}
      </Box>
      {followUrl && (
        <div className="flex justify-center">
          <Button variant="secondary" size="sm" onClick={() => void loadMore()}>
            Load more
          </Button>
        </div>
      )}
    </div>
  );
}

function StatusBadge({ statusCode, status }: { statusCode: number; status: string }) {
  const ok = statusCode >= 200 && statusCode < 300;
  return (
    <span
      className="font-mono"
      style={{
        fontSize: "0.72rem",
        fontWeight: 600,
        color: ok ? "var(--gh-open)" : "var(--color-danger-fg)",
        background: ok ? "var(--color-accent-soft)" : "transparent",
        border: `1px solid ${ok ? "var(--gh-open)" : "var(--color-danger-fg)"}`,
        borderRadius: "999px",
        padding: "0.05rem 0.5rem",
        flexShrink: 0,
      }}
    >
      {status}
    </span>
  );
}

function DeliveryRow({
  scope,
  hookId,
  delivery,
  last,
}: {
  scope: HookScope;
  hookId: number;
  delivery: GithubHookDelivery;
  last: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div style={{ borderBottom: last ? "none" : "1px solid var(--color-border)" }}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-3 text-left"
        style={{ padding: "0.7rem 1rem", background: "transparent", border: "none" }}
      >
        {open ? <ChevronDownIcon size={14} /> : <ChevronRightIcon size={14} />}
        <StatusBadge statusCode={delivery.status_code} status={delivery.status} />
        <div className="min-w-0 flex-1">
          <div className="font-mono" style={{ fontSize: "0.82rem", color: "var(--color-fg)" }}>
            {delivery.guid}
            {delivery.redelivery && (
              <span
                className="ml-2"
                style={{
                  fontSize: "0.7rem",
                  color: "var(--color-fg-muted)",
                  border: "1px solid var(--color-border)",
                  borderRadius: "999px",
                  padding: "0.05rem 0.45rem",
                }}
              >
                redelivery
              </span>
            )}
          </div>
          <div style={{ fontSize: "0.74rem", color: "var(--color-fg-muted)" }}>
            {delivery.event}
            {delivery.action ? `.${delivery.action}` : ""} ·{" "}
            {new Date(delivery.delivered_at).toLocaleString()} · {delivery.duration}s
          </div>
        </div>
      </button>
      {open && <DeliveryDetail scope={scope} hookId={hookId} deliveryId={delivery.id} />}
    </div>
  );
}

/** Pretty-print a payload: JSON gets indented; strings that parse as JSON too. */
function prettyPayload(payload: unknown): string {
  if (payload == null) return "(empty)";
  if (typeof payload === "string") {
    try {
      return JSON.stringify(JSON.parse(payload), null, 2);
    } catch {
      return payload;
    }
  }
  return JSON.stringify(payload, null, 2);
}

function headersBlock(headers: Record<string, string> | null): string {
  if (!headers || Object.keys(headers).length === 0) return "(no headers captured)";
  return Object.entries(headers)
    .map(([k, v]) => `${k}: ${v}`)
    .join("\n");
}

function DeliveryDetail({
  scope,
  hookId,
  deliveryId,
}: {
  scope: HookScope;
  hookId: number;
  deliveryId: number;
}) {
  const qc = useQueryClient();
  const detailQ = useQuery({
    queryKey: ["hook-delivery", scopeKey(scope), hookId, deliveryId],
    queryFn: () => fetchHookDelivery(scope, hookId, deliveryId),
  });

  const redeliver = useMutation({
    mutationFn: () => redeliverHookDelivery(scope, hookId, deliveryId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["hook-deliveries", scopeKey(scope), hookId] });
    },
  });

  if (detailQ.isLoading) return <Spinner label="loading delivery" />;
  if (detailQ.isError)
    return (
      <div style={{ padding: "0 1rem 0.75rem 2.4rem" }}>
        <InlineError title="Failed to load delivery" detail={String(detailQ.error)} />
      </div>
    );

  const d = detailQ.data;
  if (!d) return null;

  return (
    <div
      style={{
        padding: "0 1rem 0.9rem 2.4rem",
        display: "flex",
        flexDirection: "column",
        gap: "0.6rem",
      }}
    >
      <div className="font-mono" style={{ fontSize: "0.76rem", color: "var(--color-fg-muted)" }}>
        POST {d.url || "(target url unknown)"}
      </div>
      <div>
        <div style={{ fontSize: "0.8rem", fontWeight: 600, marginBottom: "0.25rem" }}>
          Request headers
        </div>
        <CodeBlock>{headersBlock(d.request.headers)}</CodeBlock>
      </div>
      <div>
        <div style={{ fontSize: "0.8rem", fontWeight: 600, marginBottom: "0.25rem" }}>
          Request payload
        </div>
        <CodeBlock>{prettyPayload(d.request.payload)}</CodeBlock>
      </div>
      <div>
        <div style={{ fontSize: "0.8rem", fontWeight: 600, marginBottom: "0.25rem" }}>
          Response ({d.status})
        </div>
        <CodeBlock>{prettyPayload(d.response.payload)}</CodeBlock>
      </div>
      {redeliver.isError && <ErrorBanner>{String(redeliver.error)}</ErrorBanner>}
      {redeliver.isSuccess && (
        <div style={{ fontSize: "0.8rem", color: "var(--gh-open)" }}>
          Redelivery queued — a new attempt will appear in the list shortly.
        </div>
      )}
      <div>
        <Button
          variant="secondary"
          size="sm"
          disabled={redeliver.isPending}
          onClick={() => redeliver.mutate()}
        >
          Redeliver
        </Button>
      </div>
    </div>
  );
}
