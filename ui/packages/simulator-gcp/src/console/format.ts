/** The console shows a short resource name from a fully-qualified path like
 *  `projects/p/locations/l/jobs/name`. */
export function shortName(name: string): string {
  const parts = name.split("/");
  return parts[parts.length - 1] || name;
}

/** RFC 3339 create timestamps render as a readable local date-time. */
export function formatTimestamp(value: string): string {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
