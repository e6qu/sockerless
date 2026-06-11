import { useQuery } from "@tanstack/react-query";
import { fetchRepoIssuesPage, fetchRepoPRsPage } from "../api.js";

/**
 * Open-issue / open-PR counts for the repo tab badges, shared by the
 * Issues, Pulls and RepoDetail pages.
 *
 * The list endpoints are paginated, so a single page only bounds the
 * count from below. When the server advertises a further page (Link
 * rel="next"), the badge shows "N+" rather than a wrong exact number.
 */
export function useOpenCounts(
  owner: string,
  repo: string,
): { issueCount?: number | string; prCount?: number | string } {
  const { data: issuePage } = useQuery({
    queryKey: ["issues", owner, repo, "open", "count"],
    queryFn: () => fetchRepoIssuesPage(owner, repo, "open"),
    enabled: !!owner && !!repo,
  });
  const { data: prPage } = useQuery({
    queryKey: ["prs", owner, repo, "open", "count"],
    queryFn: () => fetchRepoPRsPage(owner, repo, "open"),
    enabled: !!owner && !!repo,
  });
  const badge = (page?: { items: unknown[]; nextUrl: string | null }) => {
    if (!page) return undefined;
    return page.nextUrl ? `${page.items.length}+` : page.items.length;
  };
  return { issueCount: badge(issuePage), prCount: badge(prPage) };
}
