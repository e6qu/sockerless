/** Azure resource IDs are long; the portal shows the resource group and name
 *  from them rather than the whole path. */
export function resourceGroupOf(id: string): string {
  const match = /\/resourceGroups\/([^/]+)/i.exec(id);
  return match ? match[1] : "—";
}

export function locationLabel(location: string): string {
  return location || "—";
}

/** The portal renders a resource's tags map as the real portal's Essentials
 *  line does — `key : value` pairs, or a dash when the resource has none. */
export function tagsSummary(tags: Record<string, string>): string {
  const entries = Object.entries(tags);
  if (entries.length === 0) return "—";
  return entries.map(([key, value]) => (value ? `${key} : ${value}` : key)).join(", ");
}
