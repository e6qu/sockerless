import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Spinner } from "@sockerless/ui-core/components";
import {
  AdminApiClient,
  type CleanupItem,
  type CleanupScanResult,
} from "../api.js";

const api = new AdminApiClient();

const categoryLabels: Record<string, string> = {
  process: "Orphaned Processes",
  tmp: "Stale Temp Files",
  container: "Stopped Containers",
  resource: "Stale Resources",
};

export function CleanupPage() {
  const [scanResult, setScanResult] = useState<CleanupScanResult | null>(null);

  const scan = useMutation({
    mutationFn: () => api.cleanupScan(),
    onSuccess: (data) => setScanResult(data),
  });

  const cleanProcesses = useMutation({
    mutationFn: () => api.cleanupProcesses(),
    onSuccess: () => scan.mutate(),
  });

  const cleanTmp = useMutation({
    mutationFn: () => api.cleanupTmp(),
    onSuccess: () => scan.mutate(),
  });

  const cleanContainers = useMutation({
    mutationFn: () => api.cleanupContainers(),
    onSuccess: () => scan.mutate(),
  });

  const categories = scanResult ? groupByCategory(scanResult.items) : {};

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Cleanup</h2>
        <button
          onClick={() => scan.mutate()}
          disabled={scan.isPending}
          className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {scan.isPending ? "Scanning..." : "Scan"}
        </button>
      </div>

      {scan.isPending && <Spinner />}

      {scan.isError && (
        <div className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400">
          Scan failed: {scan.error?.message ?? "Unknown error"}
        </div>
      )}

      {cleanProcesses.isError && (
        <div className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400">
          Clean processes failed:{" "}
          {cleanProcesses.error?.message ?? "Unknown error"}
        </div>
      )}

      {cleanTmp.isError && (
        <div className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400">
          Clean tmp failed: {cleanTmp.error?.message ?? "Unknown error"}
        </div>
      )}

      {cleanContainers.isError && (
        <div className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400">
          Clean containers failed:{" "}
          {cleanContainers.error?.message ?? "Unknown error"}
        </div>
      )}

      {scanResult && (
        <div className="space-y-6">
          <p className="text-sm text-gray-500 dark:text-gray-400">
            Scanned at {new Date(scanResult.scanned_at).toLocaleString()} —{" "}
            {scanResult.items.length} items found
          </p>

          {scanResult.items.length === 0 && (
            <p className="text-sm text-green-600 dark:text-green-400">
              No stale resources found.
            </p>
          )}

          {Object.entries(categories).map(([category, items]) => (
            <div key={category} className="space-y-3">
              <div className="flex items-center justify-between">
                <h3 className="text-lg font-medium">
                  {categoryLabels[category] ?? category} ({items.length})
                </h3>
                {category === "process" && (
                  <button
                    onClick={() => {
                      if (
                        window.confirm(
                          "Clean orphaned processes? This cannot be undone.",
                        )
                      )
                        cleanProcesses.mutate();
                    }}
                    disabled={cleanProcesses.isPending}
                    className="rounded-md bg-red-600 px-3 py-1 text-xs font-medium text-white hover:bg-red-700 disabled:opacity-50"
                  >
                    Clean
                  </button>
                )}
                {category === "tmp" && (
                  <button
                    onClick={() => {
                      if (
                        window.confirm(
                          "Delete stale temp files? This cannot be undone.",
                        )
                      )
                        cleanTmp.mutate();
                    }}
                    disabled={cleanTmp.isPending}
                    className="rounded-md bg-red-600 px-3 py-1 text-xs font-medium text-white hover:bg-red-700 disabled:opacity-50"
                  >
                    Clean
                  </button>
                )}
                {category === "container" && (
                  <button
                    onClick={() => {
                      if (
                        window.confirm(
                          "Prune stopped containers? This cannot be undone.",
                        )
                      )
                        cleanContainers.mutate();
                    }}
                    disabled={cleanContainers.isPending}
                    className="rounded-md bg-red-600 px-3 py-1 text-xs font-medium text-white hover:bg-red-700 disabled:opacity-50"
                  >
                    Prune
                  </button>
                )}
              </div>

              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                  <thead>
                    <tr>
                      <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                        Name
                      </th>
                      <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                        Description
                      </th>
                      <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                        Age
                      </th>
                      {category === "tmp" && (
                        <th className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                          Size
                        </th>
                      )}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                    {items.map((item, i) => (
                      <tr key={i}>
                        <td className="whitespace-nowrap px-4 py-2 text-sm font-medium">
                          {item.name}
                        </td>
                        <td className="px-4 py-2 text-sm text-gray-500 dark:text-gray-400">
                          {item.description}
                        </td>
                        <td className="whitespace-nowrap px-4 py-2 text-sm text-gray-500 dark:text-gray-400">
                          {item.age}
                        </td>
                        {category === "tmp" && (
                          <td className="whitespace-nowrap px-4 py-2 text-sm text-gray-500 dark:text-gray-400">
                            {item.size ? formatBytes(item.size) : "-"}
                          </td>
                        )}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
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
