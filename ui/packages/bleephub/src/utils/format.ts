/**
 * Format the elapsed time between two ISO timestamps as GitHub renders
 * job/step durations ("1h 2m 3s", "42s"). Returns "—" while the end
 * isn't known yet (in-flight) or when either timestamp is unparsable.
 */
export function formatDuration(startISO: string | null, endISO: string | null): string {
  if (!startISO || !endISO) return "—";
  const ms = new Date(endISO).getTime() - new Date(startISO).getTime();
  if (!Number.isFinite(ms) || ms < 0) return "—";
  const total = Math.round(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h}h ${m}m ${s}s`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

/**
 * Render an ISO timestamp as a coarse relative age the way GitHub labels a
 * last-commit time ("3 days ago", "just now", "last year"). Returns "" for a
 * missing or unparsable timestamp so callers can omit the label rather than
 * print a bogus 1970 date.
 */
export function relativeTimeFromNow(iso: string | null | undefined, now: number = Date.now()): string {
  if (!iso) return "";
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return "";
  const secs = Math.round((now - then) / 1000);
  if (secs < 0) return "just now";
  const units: [limit: number, div: number, name: string][] = [
    [60, 1, "second"],
    [3600, 60, "minute"],
    [86400, 3600, "hour"],
    [2592000, 86400, "day"],
    [31536000, 2592000, "month"],
    [Infinity, 31536000, "year"],
  ];
  if (secs < 45) return "just now";
  for (const [limit, div, name] of units) {
    if (secs < limit) {
      const value = Math.round(secs / div);
      return `${value} ${name}${value === 1 ? "" : "s"} ago`;
    }
  }
  return "";
}
