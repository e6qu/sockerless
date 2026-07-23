/** Azure resource IDs are long; the portal shows the resource group and name
 *  from them rather than the whole path. */
export function resourceGroupOf(id: string): string {
  const match = /\/resourceGroups\/([^/]+)/i.exec(id);
  return match ? match[1] : "—";
}

export function locationLabel(location: string): string {
  return location || "—";
}
