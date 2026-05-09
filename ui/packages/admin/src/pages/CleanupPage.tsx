import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import {
  Button,
  PageHeading,
  Spinner,
  useToast,
  useReportError,
} from "@sockerless/ui-core/components";
import {
  AdminApiClient,
  type CleanupItem,
  type CleanupScanResult,
} from "../api.js";

const api = new AdminApiClient();

const categoryLabels: Record<string, string> = {
  process: "Orphaned processes",
  tmp: "Stale temp files",
  container: "Stopped containers",
  resource: "Stale resources",
};

export function CleanupPage() {
  const [scanResult, setScanResult] = useState<CleanupScanResult | null>(null);
  const { push } = useToast();
  const reportError = useReportError();

  const scan = useMutation({
    mutationFn: () => api.cleanupScan(),
    onSuccess: (data) => setScanResult(data),
    onError: (err) => reportError(err, "Scan failed"),
  });

  const cleanProcesses = useMutation({
    mutationFn: () => api.cleanupProcesses(),
    onSuccess: () => {
      push({ tone: "success", title: "Cleaned orphaned processes" });
      scan.mutate();
    },
    onError: (err) => reportError(err, "Clean processes failed"),
  });

  const cleanTmp = useMutation({
    mutationFn: () => api.cleanupTmp(),
    onSuccess: () => {
      push({ tone: "success", title: "Cleaned stale temp files" });
      scan.mutate();
    },
    onError: (err) => reportError(err, "Clean tmp failed"),
  });

  const cleanContainers = useMutation({
    mutationFn: () => api.cleanupContainers(),
    onSuccess: () => {
      push({ tone: "success", title: "Cleaned stopped containers" });
      scan.mutate();
    },
    onError: (err) => reportError(err, "Clean containers failed"),
  });

  const categories = scanResult ? groupByCategory(scanResult.items) : {};

  return (
    <div>
      <PageHeading
        kicker="admin · cleanup"
        title={<>Stale-resource sweep</>}
        meta={
          scanResult
            ? `scanned ${new Date(scanResult.scanned_at).toLocaleString()} · ${scanResult.items.length} item${scanResult.items.length === 1 ? "" : "s"}`
            : "Click Scan to enumerate stale resources across processes, tmp files, and containers."
        }
        actions={
          <Button
            variant="primary"
            size="sm"
            onClick={() => scan.mutate()}
            disabled={scan.isPending}
          >
            {scan.isPending ? "Scanning…" : "Scan"}
          </Button>
        }
      />

      {scan.isPending && <Spinner label="scanning" />}
      {/* Errors and successes for scan / cleanup mutations are surfaced
          via toasts (top-right notification stack) rather than inline. */}

      {scanResult && (
        <div className="space-y-6 mt-4">
          {scanResult.items.length === 0 && (
            <div
              className="px-4 py-3 font-mono"
              style={{
                background: "var(--color-status-ok-soft)",
                color: "var(--color-status-ok)",
                border: "1px solid var(--color-status-ok)",
                borderLeft: "3px solid var(--color-status-ok)",
                borderRadius: "var(--radius-sm)",
                fontSize: "0.78rem",
              }}
            >
              <div
                className="text-[10px] uppercase tracking-[0.22em] mb-0.5"
                style={{ opacity: 0.85 }}
              >
                clean
              </div>
              no stale resources found.
            </div>
          )}

          {Object.entries(categories).map(([category, items]) => (
            <CategorySection
              key={category}
              category={category}
              items={items}
              onClean={() => {
                if (category === "process") cleanProcesses.mutate();
                else if (category === "tmp") cleanTmp.mutate();
                else if (category === "container") cleanContainers.mutate();
              }}
              cleanPending={
                (category === "process" && cleanProcesses.isPending) ||
                (category === "tmp" && cleanTmp.isPending) ||
                (category === "container" && cleanContainers.isPending)
              }
            />
          ))}
        </div>
      )}
    </div>
  );
}

function CategorySection({
  category,
  items,
  onClean,
  cleanPending,
}: {
  category: string;
  items: CleanupItem[];
  onClean: () => void;
  cleanPending: boolean;
}) {
  const cleanable = category === "process" || category === "tmp" || category === "container";
  const cleanLabel =
    category === "process"
      ? "Clean orphaned processes? This cannot be undone."
      : category === "tmp"
        ? "Delete stale temp files? This cannot be undone."
        : "Prune stopped containers? This cannot be undone.";

  return (
    <div
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-sm)",
      }}
    >
      <div
        className="flex items-center justify-between px-4 py-3"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <h3
          className="font-mono uppercase tracking-[0.15em]"
          style={{ fontSize: "0.72rem", color: "var(--color-fg)" }}
        >
          {categoryLabels[category] ?? category}
          <span
            className="ml-2"
            style={{ color: "var(--color-fg-subtle)" }}
          >
            ({items.length})
          </span>
        </h3>
        {cleanable && (
          <Button
            variant="danger"
            size="sm"
            onClick={() => {
              if (window.confirm(cleanLabel)) onClean();
            }}
            disabled={cleanPending}
          >
            {cleanPending ? "Cleaning…" : category === "container" ? "Prune" : "Clean"}
          </Button>
        )}
      </div>
      <div className="overflow-x-auto">
        <table className="min-w-full font-mono" style={{ fontSize: "0.78rem", borderCollapse: "collapse" }}>
          <thead style={{ background: "var(--color-bg-subtle)" }}>
            <tr>
              <Th>Name</Th>
              <Th>Description</Th>
              <Th>Age</Th>
              {category === "tmp" && <Th>Size</Th>}
            </tr>
          </thead>
          <tbody>
            {items.map((item, i) => (
              <tr
                key={i}
                style={{
                  background: i % 2 === 0 ? "var(--color-surface)" : "var(--color-bg-subtle)",
                  borderBottom:
                    "1px solid color-mix(in oklch, var(--color-border) 60%, transparent)",
                }}
              >
                <td className="px-3 py-1.5" style={{ color: "var(--color-fg)" }}>
                  {item.name}
                </td>
                <td className="px-3 py-1.5" style={{ color: "var(--color-fg-muted)" }}>
                  {item.description}
                </td>
                <td className="px-3 py-1.5" style={{ color: "var(--color-fg-muted)" }}>
                  {item.age}
                </td>
                {category === "tmp" && (
                  <td className="px-3 py-1.5" style={{ color: "var(--color-fg-muted)" }}>
                    {item.size ? formatBytes(item.size) : "—"}
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function Th({ children }: { children: React.ReactNode }) {
  return (
    <th
      className="px-3 py-2 text-left uppercase tracking-[0.15em]"
      style={{
        fontSize: "0.62rem",
        fontWeight: 500,
        color: "var(--color-fg-subtle)",
        borderBottom: "1px solid var(--color-border)",
        whiteSpace: "nowrap",
      }}
    >
      {children}
    </th>
  );
}

function groupByCategory(items: CleanupItem[]): Record<string, CleanupItem[]> {
  const groups: Record<string, CleanupItem[]> = {};
  for (const item of items) {
    if (!groups[item.category]) {
      groups[item.category] = [];
    }
    groups[item.category].push(item);
  }
  return groups;
}

function formatBytes(bytes: number): string {
  if (bytes >= 1 << 30) return `${(bytes / (1 << 30)).toFixed(1)} GB`;
  if (bytes >= 1 << 20) return `${(bytes / (1 << 20)).toFixed(1)} MB`;
  if (bytes >= 1 << 10) return `${(bytes / (1 << 10)).toFixed(1)} KB`;
  return `${bytes} B`;
}
