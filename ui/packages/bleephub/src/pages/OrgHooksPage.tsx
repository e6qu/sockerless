import { useState } from "react";
import { Link, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import { fetchOrgHooksPage } from "../api.js";
import type { GithubOrgWebhook } from "../types.js";
import { OrgHeader } from "../components/Shell.js";
import { PageTitle, Box, Blankslate, Button, ErrorBanner } from "../components/ui.js";

/** Organization webhooks list with per-hook links to the deliveries drill-down. */
export function OrgHooksPage() {
  const { org = "" } = useParams<{ org: string }>();
  const [extra, setExtra] = useState<GithubOrgWebhook[]>([]);
  const [nextUrl, setNextUrl] = useState<string | null>(null);
  const [pageError, setPageError] = useState<string | null>(null);

  const firstPage = useQuery({
    queryKey: ["org-hooks", org],
    queryFn: () => fetchOrgHooksPage(org),
    enabled: !!org,
  });

  if (firstPage.isLoading) return <Spinner label={`loading ${org} webhooks`} />;
  if (firstPage.isError)
    return (
      <div>
        <OrgHeader org={org} active="hooks" />
        <InlineError title="Failed to load organization webhooks" detail={String(firstPage.error)} />
      </div>
    );

  const hooks = [...(firstPage.data?.items ?? []), ...extra];
  const followUrl = nextUrl ?? firstPage.data?.nextUrl ?? null;

  const loadMore = async () => {
    if (!followUrl) return;
    try {
      const page = await fetchOrgHooksPage(org, followUrl);
      setExtra((prev) => [...prev, ...page.items]);
      setNextUrl(page.nextUrl);
      setPageError(null);
    } catch (err) {
      setPageError(String(err));
    }
  };

  return (
    <div>
      <OrgHeader org={org} active="hooks" />
      <PageTitle title="Webhooks" />
      {pageError && <ErrorBanner>{pageError}</ErrorBanner>}
      {hooks.length === 0 ? (
        <Blankslate title="No organization webhooks">
          Webhooks created via POST /orgs/{org}/hooks appear here.
        </Blankslate>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}>
          <Box>
            {hooks.map((h, i) => (
              <div
                key={h.id}
                className="flex items-center gap-3"
                style={{
                  padding: "0.7rem 1rem",
                  borderBottom: i < hooks.length - 1 ? "1px solid var(--color-border)" : "none",
                }}
              >
                <span
                  aria-hidden
                  style={{
                    width: 8,
                    height: 8,
                    borderRadius: "999px",
                    background: h.active ? "var(--gh-open)" : "var(--color-fg-subtle)",
                    flexShrink: 0,
                  }}
                />
                <div className="min-w-0 flex-1">
                  <div style={{ fontSize: "0.88rem", fontWeight: 500, color: "var(--color-fg)" }}>
                    {h.name}{" "}
                    <span style={{ color: "var(--color-fg-subtle)", fontWeight: 400 }}>#{h.id}</span>
                  </div>
                  <div
                    className="font-mono"
                    style={{ fontSize: "0.74rem", color: "var(--color-fg-muted)" }}
                  >
                    {h.config.url || "no url"} · events: {h.events.join(", ") || "none"}
                  </div>
                </div>
                <Link to={`/ui/orgs/${org}/hooks/${h.id}/deliveries`}>
                  <Button variant="secondary" size="sm">
                    Deliveries
                  </Button>
                </Link>
              </div>
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
      )}
    </div>
  );
}
