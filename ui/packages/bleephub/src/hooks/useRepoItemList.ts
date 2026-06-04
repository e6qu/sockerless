import { useState } from "react";
import { useQuery } from "@tanstack/react-query";

/**
 * Shared data hook for open/closed list pages (Issues, PRs).
 * Handles the state toggle, query, and loading/error state so that
 * IssuesPage and PullsPage don't duplicate this boilerplate.
 */
export function useRepoItemList<T>(
  queryKey: string,
  owner: string,
  repo: string,
  fetcher: (owner: string, repo: string, state: string) => Promise<T[]>,
): {
  state: "open" | "closed";
  setState: (s: "open" | "closed") => void;
  items: T[];
  isLoading: boolean;
  isError: boolean;
  error: unknown;
} {
  const [state, setState] = useState<"open" | "closed">("open");
  const { data: items = [], isLoading, isError, error } = useQuery({
    queryKey: [queryKey, owner, repo, state],
    queryFn: () => fetcher(owner, repo, state),
    enabled: !!owner && !!repo,
  });
  return { state, setState, items, isLoading, isError, error };
}
