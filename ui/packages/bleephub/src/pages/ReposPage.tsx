import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { Spinner, InlineError } from "@sockerless/ui-core/components";
import { fetchRepos } from "../api.js";
import type { BleephubRepo } from "../types.js";
import { PageTitle, Blankslate } from "../components/ui.js";
import { RepoIcon, BranchIcon } from "../components/octicons.js";

export function ReposPage() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["repos"],
    queryFn: fetchRepos,
    refetchInterval: 10000,
  });
  const [filter, setFilter] = useState("");

  const filtered = useMemo(() => {
    if (!data) return [];
    const q = filter.trim().toLowerCase();
    if (!q) return data;
    return data.filter(
      (r) => r.full_name.toLowerCase().includes(q) || (r.description ?? "").toLowerCase().includes(q),
    );
  }, [data, filter]);

  if (isError) return <InlineError title="Failed to load repositories" detail={String(error)} />;
  if (isLoading || !data) return <Spinner label="loading repos" />;

  return (
    <div>
      <PageTitle
        title="Repositories"
        meta={`${data.length} repositor${data.length === 1 ? "y" : "ies"} indexed`}
        actions={
          <input
            type="search"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Find a repository…"
            aria-label="Find a repository"
            style={{ fontSize: "0.82rem", minWidth: "16rem" }}
          />
        }
      />

      {data.length === 0 ? (
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
