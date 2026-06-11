import { useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import type { Page } from "../api.js";

/**
 * Shared data hook for open/closed list pages (Issues, PRs).
 * Handles the state toggle, Link-header pagination, and loading/error
 * state so that IssuesPage and PullsPage don't duplicate this boilerplate.
 * Pages accumulate as the user loads more; `hasMore` reflects the server's
 * Link rel="next" header, never a guess.
 */
export function useRepoItemList<T>(
  queryKey: string,
  owner: string,
  repo: string,
  fetcher: (owner: string, repo: string, state: string, pageUrl?: string) => Promise<Page<T>>,
): {
  state: "open" | "closed";
  setState: (s: "open" | "closed") => void;
  items: T[];
  isLoading: boolean;
  isError: boolean;
  error: unknown;
  hasMore: boolean;
  loadMore: () => void;
  isLoadingMore: boolean;
} {
  const [state, setState] = useState<"open" | "closed">("open");
  const query = useInfiniteQuery({
    queryKey: [queryKey, owner, repo, state, "paged"],
    queryFn: ({ pageParam }) => fetcher(owner, repo, state, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.nextUrl ?? undefined,
    enabled: !!owner && !!repo,
  });
  return {
    state,
    setState,
    items: query.data?.pages.flatMap((p) => p.items) ?? [],
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
    hasMore: query.hasNextPage,
    loadMore: () => void query.fetchNextPage(),
    isLoadingMore: query.isFetchingNextPage,
  };
}
