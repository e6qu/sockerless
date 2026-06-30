import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import type { BleephubRepo, RepoListFilters } from "../types.js";
import { PageTitle, Blankslate, Button } from "../components/ui.js";
import { RepoIcon, BranchIcon } from "../components/octicons.js";
import { RepoCreateDialog } from "../components/RepoCreateDialog.js";
import type { Page } from "../api.js";

interface RepoListPageProps {
  title: string;
  fetchPage: (filters: RepoListFilters, pageUrl?: string) => Promise<Page<BleephubRepo>>;
  queryKey: string[];
  allowCreate?: boolean;
  createTarget?: "user" | { org: string };
}

export function RepoListPage({
  title,
  fetchPage,
  queryKey,
  allowCreate = false,
  createTarget = "user",
}: RepoListPageProps) {
  const queryClient = useQueryClient();
  const [filter, setFilter] = useState("");
  const [filters, setFilters] = useState<RepoListFilters>({});
  const [pageUrl, setPageUrl] = useState<string | undefined>(undefined);
  const [pageStack, setPageStack] = useState<string[]>([]);
  const [createOpen, setCreateOpen] = useState(false);

  const { data, isLoading, isError, error } = useQuery({
    queryKey: [...queryKey, filters, pageUrl ?? "first"],
    queryFn: () => fetchPage(filters, pageUrl),
    refetchInterval: 10000,
  });

  const filtered = useMemo(() => {
    if (!data) return [];
    const q = filter.trim().toLowerCase();
    if (!q) return data.items;
    return data.items.filter(
      (r) =>
        r.full_name.toLowerCase().includes(q) ||
        (r.description ?? "").toLowerCase().includes(q),
    );
  }, [data, filter]);

  const hasNext = !!data?.nextUrl;
  const currentPage = pageStack.length + 1;
  const lastPage = data?.lastPage ?? currentPage;

  const goNext = () => {
    if (!data?.nextUrl) return;
    setPageStack((s) => [...s, pageUrl ?? ""]);
    setPageUrl(data.nextUrl);
  };

  const goPrev = () => {
    setPageStack((s) => {
      const prev = s[s.length - 1];
      setPageUrl(prev || undefined);
      return s.slice(0, -1);
    });
  };

  const resetFilters = () => {
    setFilters({});
    setFilter("");
    setPageUrl(undefined);
    setPageStack([]);
  };

  if (isError) return <InlineError title={`Failed to load ${title.toLowerCase()}`} detail={String(error)} />;
  if (isLoading || !data) return <Spinner label={`loading ${title.toLowerCase()}`} />;

  return (
    <div>
      <PageTitle
        title={title}
        meta={`page ${currentPage} of ${lastPage}`}
        actions={
          <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
            <select
              aria-label="Visibility filter"
              value={filters.visibility ?? ""}
              onChange={(e) => {
                setFilters((f: RepoListFilters) => ({ ...f, visibility: (e.target.value as RepoListFilters["visibility"]) || undefined }));
                setPageUrl(undefined);
                setPageStack([]);
              }}
            >
              <option value="">All visibilities</option>
              <option value="public">Public</option>
              <option value="private">Private</option>
              <option value="internal">Internal</option>
            </select>
            <select
              aria-label="Sort by"
              value={filters.sort ?? ""}
              onChange={(e) => {
                setFilters((f: RepoListFilters) => ({ ...f, sort: (e.target.value as RepoListFilters["sort"]) || undefined }));
                setPageUrl(undefined);
                setPageStack([]);
              }}
            >
              <option value="">Sort by created</option>
              <option value="updated">Updated</option>
              <option value="pushed">Pushed</option>
              <option value="full_name">Name</option>
            </select>
            <select
              aria-label="Sort direction"
              value={filters.direction ?? ""}
              onChange={(e) => {
                setFilters((f: RepoListFilters) => ({ ...f, direction: (e.target.value as RepoListFilters["direction"]) || undefined }));
                setPageUrl(undefined);
                setPageStack([]);
              }}
            >
              <option value="">Descending</option>
              <option value="asc">Ascending</option>
            </select>
            <input
              type="search"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Find a repository…"
              aria-label="Find a repository"
              style={{ fontSize: "0.82rem", minWidth: "16rem" }}
            />
            {allowCreate && <Button onClick={() => setCreateOpen(true)}>New repository</Button>}
          </div>
        }
      />

      {allowCreate && (
        <RepoCreateDialog
          open={createOpen}
          onClose={() => setCreateOpen(false)}
          onCreated={() => {
            setCreateOpen(false);
            queryClient.invalidateQueries({ queryKey });
          }}
          createTarget={createTarget}
        />
      )}

      {data.items.length === 0 ? (
        <Blankslate icon={<RepoIcon size={28} />} title="No repositories yet">
          Create one with <code>POST /api/v3/user/repos</code> or push to git.
        </Blankslate>
      ) : filtered.length === 0 ? (
        <Blankslate icon={<RepoIcon size={28} />} title="No matches">
          No repository matches “{filter}”.
        </Blankslate>
      ) : (
        <ul style={{ borderTop: "1px solid var(--color-border)" }}>
          {filtered.map((repo) => (
            <RepoRow key={repo.id} repo={repo} />
          ))}
        </ul>
      )}

      {(pageStack.length > 0 || hasNext) && (
        <div className="mt-4 flex items-center gap-2">
          <Button onClick={goPrev} disabled={pageStack.length === 0}>
            Previous
          </Button>
          <span style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)" }}>
            Page {currentPage} of {lastPage}
          </span>
          <Button onClick={goNext} disabled={!hasNext}>
            Next
          </Button>
          {(filters.visibility || filters.sort || filters.direction || filter) && (
            <Button onClick={resetFilters} variant="secondary">
              Clear filters
            </Button>
          )}
        </div>
      )}
    </div>
  );
}

function RepoRow({ repo }: { repo: BleephubRepo }) {
  const [owner, name] = repo.full_name.split("/");
  return (
    <li
      style={{
        padding: "1rem 0",
        borderBottom: "1px solid var(--color-border)",
      }}
    >
      <div className="flex items-baseline gap-2">
        <Link
          to={`/ui/repos/${owner}/${name}`}
          style={{
            color: "var(--color-accent)",
            fontWeight: 600,
            fontSize: "1.05rem",
            textDecoration: "none",
          }}
        >
          {repo.full_name}
        </Link>
        <span
          style={{
            fontSize: "0.7rem",
            fontWeight: 500,
            color: "var(--color-fg-muted)",
            border: "1px solid var(--color-border)",
            borderRadius: "2rem",
            padding: "0.05rem 0.55rem",
            textTransform: "capitalize",
          }}
        >
          {repo.private ? "Private" : repo.visibility || "Public"}
        </span>
      </div>
      {repo.description && (
        <p className="mt-1" style={{ fontSize: "0.85rem", color: "var(--color-fg-muted)", maxWidth: "48rem" }}>
          {repo.description}
        </p>
      )}
      <div
        className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1"
        style={{ fontSize: "0.78rem", color: "var(--color-fg-muted)" }}
      >
        <span className="inline-flex items-center gap-1">
          <BranchIcon size={14} /> {repo.default_branch}
        </span>
        <span>Updated {new Date(repo.updated_at).toLocaleDateString()}</span>
      </div>
    </li>
  );
}
